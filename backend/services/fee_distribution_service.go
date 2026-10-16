package services

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/yourusername/kor-assetforge/models"
)

const feeDistributionBpsDenominator = 10000

// FeeDistributionService computes and records the automated split of
// collected marketplace fees between the platform, liquidity providers, and
// token holders, according to the currently active FeeDistributionRule.
type FeeDistributionService struct {
	db *gorm.DB
}

// NewFeeDistributionService creates a new FeeDistributionService.
func NewFeeDistributionService(db *gorm.DB) *FeeDistributionService {
	return &FeeDistributionService{db: db}
}

// CreateRule creates a new fee distribution rule. Shares must sum to 10000 bps (100%).
func (s *FeeDistributionService) CreateRule(rule *models.FeeDistributionRule) error {
	if rule.PlatformShareBps+rule.LiquidityProvidersShareBps+rule.TokenHoldersShareBps != feeDistributionBpsDenominator {
		return errors.New("platform, liquidity provider, and token holder shares must sum to 10000 basis points")
	}
	return s.db.Create(rule).Error
}

// ActivateRule marks the given rule as the active rule and deactivates all others.
func (s *FeeDistributionService) ActivateRule(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.FeeDistributionRule{}).Where("active = ?", true).Update("active", false).Error; err != nil {
			return err
		}
		result := tx.Model(&models.FeeDistributionRule{}).Where("id = ?", id).Update("active", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("fee distribution rule not found")
		}
		return nil
	})
}

// GetActiveRule returns the currently active distribution rule.
func (s *FeeDistributionService) GetActiveRule() (*models.FeeDistributionRule, error) {
	var rule models.FeeDistributionRule
	if err := s.db.Where("active = ?", true).First(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// ListRules returns all configured distribution rules.
func (s *FeeDistributionService) ListRules() ([]models.FeeDistributionRule, error) {
	var rules []models.FeeDistributionRule
	if err := s.db.Order("created_at DESC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// ListRuns returns the most recent distribution runs, newest first.
func (s *FeeDistributionService) ListRuns(limit int) ([]models.FeeDistributionRun, error) {
	var runs []models.FeeDistributionRun
	if err := s.db.Preload("Allocations").Order("created_at DESC").Limit(limit).Find(&runs).Error; err != nil {
		return nil, err
	}
	return runs, nil
}

// GetRun returns a single distribution run with its allocations.
func (s *FeeDistributionService) GetRun(id uint) (*models.FeeDistributionRun, error) {
	var run models.FeeDistributionRun
	if err := s.db.Preload("Allocations").First(&run, id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

// RunDistribution computes and persists the fee split for [periodStart, periodEnd)
// using the currently active rule, allocating pro-rata shares to liquidity
// providers (by LP token holdings) and token holders (by asset balance).
func (s *FeeDistributionService) RunDistribution(periodStart, periodEnd time.Time, triggeredBy string) (*models.FeeDistributionRun, error) {
	rule, err := s.GetActiveRule()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("no active fee distribution rule is configured")
		}
		return nil, err
	}

	var totalFees int64
	if err := s.db.Model(&models.FeeTransaction{}).
		Where("created_at >= ? AND created_at < ? AND status = ?", periodStart, periodEnd, "completed").
		Select("COALESCE(SUM(total_fee_stroops), 0)").
		Row().Scan(&totalFees); err != nil {
		return nil, fmt.Errorf("failed to total fees for period: %w", err)
	}

	run := &models.FeeDistributionRun{
		RuleID:            rule.ID,
		PeriodStart:       periodStart,
		PeriodEnd:         periodEnd,
		TotalFeesStroops:  totalFees,
		Status:            models.DistributionStatusPending,
		TriggeredBy:       triggeredBy,
	}

	if totalFees <= 0 {
		run.Status = models.DistributionStatusCompleted
		now := time.Now()
		run.CompletedAt = &now
		if err := s.db.Create(run).Error; err != nil {
			return nil, err
		}
		return run, nil
	}

	platformAmount := totalFees * int64(rule.PlatformShareBps) / feeDistributionBpsDenominator
	lpAmount := totalFees * int64(rule.LiquidityProvidersShareBps) / feeDistributionBpsDenominator
	holderAmount := totalFees - platformAmount - lpAmount // remainder absorbed by token holders to avoid rounding loss

	run.PlatformAmountStroops = platformAmount
	run.LiquidityProvidersAmountStroops = lpAmount
	run.TokenHoldersAmountStroops = holderAmount

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(run).Error; err != nil {
			return err
		}

		allocations := make([]models.FeeDistributionAllocation, 0)

		if platformAmount > 0 {
			allocations = append(allocations, models.FeeDistributionAllocation{
				RunID:            run.ID,
				RecipientType:    models.RecipientPlatform,
				RecipientAddress: platformTreasuryAddress(),
				AmountStroops:    platformAmount,
				Status:           "pending",
			})
		}

		if lpAmount > 0 {
			lpAllocations, err := allocateProRataToLiquidityProviders(tx, run.ID, lpAmount)
			if err != nil {
				return err
			}
			allocations = append(allocations, lpAllocations...)
		}

		if holderAmount > 0 {
			holderAllocations, err := allocateProRataToTokenHolders(tx, run.ID, holderAmount)
			if err != nil {
				return err
			}
			allocations = append(allocations, holderAllocations...)
		}

		if len(allocations) > 0 {
			if err := tx.Create(&allocations).Error; err != nil {
				return err
			}
		}

		now := time.Now()
		return tx.Model(run).Updates(map[string]interface{}{
			"recipient_count": len(allocations),
			"status":          models.DistributionStatusCompleted,
			"completed_at":    now,
		}).Error
	})

	if err != nil {
		s.db.Model(run).Update("status", models.DistributionStatusFailed)
		return nil, fmt.Errorf("fee distribution run failed: %w", err)
	}

	return s.GetRun(run.ID)
}

// allocateProRataToLiquidityProviders splits totalAmount across all active LP
// positions in proportion to each provider's share of total LP tokens.
func allocateProRataToLiquidityProviders(tx *gorm.DB, runID uint, totalAmount int64) ([]models.FeeDistributionAllocation, error) {
	var positions []models.LiquidityPosition
	if err := tx.Where("lp_tokens > 0").Find(&positions).Error; err != nil {
		return nil, err
	}

	totalsByProvider := map[string]int64{}
	var totalLPTokens int64
	for _, p := range positions {
		totalsByProvider[p.ProviderAddress] += p.LPTokens
		totalLPTokens += p.LPTokens
	}

	return buildProRataAllocations(runID, models.RecipientLiquidityProvider, totalAmount, totalsByProvider, totalLPTokens), nil
}

// allocateProRataToTokenHolders splits totalAmount across all user balances in
// proportion to each holder's share of total tokens held across assets.
func allocateProRataToTokenHolders(tx *gorm.DB, runID uint, totalAmount int64) ([]models.FeeDistributionAllocation, error) {
	var balances []models.UserBalance
	if err := tx.Where("balance > 0").Preload("User").Find(&balances).Error; err != nil {
		return nil, err
	}

	totalsByHolder := map[string]int64{}
	var totalBalance int64
	for _, b := range balances {
		address := b.User.StellarAddress
		if address == "" {
			continue
		}
		totalsByHolder[address] += b.Balance
		totalBalance += b.Balance
	}

	return buildProRataAllocations(runID, models.RecipientTokenHolder, totalAmount, totalsByHolder, totalBalance), nil
}

func buildProRataAllocations(runID uint, recipientType models.DistributionRecipientType, totalAmount int64, sharesByAddress map[string]int64, totalShares int64) []models.FeeDistributionAllocation {
	if totalShares <= 0 {
		return nil
	}

	allocations := make([]models.FeeDistributionAllocation, 0, len(sharesByAddress))
	for address, share := range sharesByAddress {
		amount := totalAmount * share / totalShares
		if amount <= 0 {
			continue
		}
		allocations = append(allocations, models.FeeDistributionAllocation{
			RunID:            runID,
			RecipientType:    recipientType,
			RecipientAddress: address,
			AmountStroops:    amount,
			Status:           "pending",
		})
	}
	return allocations
}

// platformTreasuryAddress resolves the address fee revenue is paid to. In the
// absence of a dedicated treasury configuration table, this is a fixed
// well-known identifier the settlement worker maps to the platform's
// configured Stellar account.
func platformTreasuryAddress() string {
	return "PLATFORM_TREASURY"
}
