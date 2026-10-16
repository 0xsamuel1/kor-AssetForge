package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// FeeService handles dynamic fee configuration retrieval and calculation.
type FeeService struct {
	db *gorm.DB
}

// NewFeeService creates a new FeeService.
func NewFeeService(db *gorm.DB) *FeeService {
	return &FeeService{db: db}
}

// CreateFeeConfig persists a new fee configuration and records the audit log.
func (s *FeeService) CreateFeeConfig(cfg *models.FeeConfig, adminID uint, reason string) error {
	if err := s.validateConfig(cfg); err != nil {
		return err
	}

	cfg.CreatedByAdminID = adminID
	cfg.UpdatedByAdminID = adminID

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(cfg).Error; err != nil {
			return fmt.Errorf("failed to create fee config: %w", err)
		}

		newJSON, _ := json.Marshal(cfg)
		log := &models.FeeAuditLog{
			FeeConfigID: cfg.ID,
			AdminID:     adminID,
			Action:      "created",
			NewJSON:     string(newJSON),
			Reason:      reason,
			CreatedAt:   time.Now(),
		}
		return tx.Create(log).Error
	})
}

// UpdateFeeConfig updates an existing fee configuration and records a diff in
// the audit log.
func (s *FeeService) UpdateFeeConfig(id uint, updates *models.FeeConfig, adminID uint, reason string) (*models.FeeConfig, error) {
	if err := s.validateConfig(updates); err != nil {
		return nil, err
	}

	var existing models.FeeConfig
	if err := s.db.First(&existing, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("fee config not found")
		}
		return nil, fmt.Errorf("failed to load fee config: %w", err)
	}

	prevJSON, _ := json.Marshal(existing)

	var txErr error
	txErr = s.db.Transaction(func(tx *gorm.DB) error {
		updates.ID = existing.ID
		updates.CreatedByAdminID = existing.CreatedByAdminID
		updates.UpdatedByAdminID = adminID
		updates.CreatedAt = existing.CreatedAt
		updates.UpdatedAt = time.Now()

		if err := tx.Save(updates).Error; err != nil {
			return fmt.Errorf("failed to update fee config: %w", err)
		}

		newJSON, _ := json.Marshal(updates)
		log := &models.FeeAuditLog{
			FeeConfigID:  id,
			AdminID:      adminID,
			Action:       "updated",
			PreviousJSON: string(prevJSON),
			NewJSON:      string(newJSON),
			Reason:       reason,
			CreatedAt:    time.Now(),
		}
		return tx.Create(log).Error
	})
	if txErr != nil {
		return nil, txErr
	}
	return updates, nil
}

// DeactivateFeeConfig marks a fee config as inactive without deleting it.
func (s *FeeService) DeactivateFeeConfig(id uint, adminID uint, reason string) error {
	var cfg models.FeeConfig
	if err := s.db.First(&cfg, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("fee config not found")
		}
		return fmt.Errorf("failed to load fee config: %w", err)
	}

	prevJSON, _ := json.Marshal(cfg)

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&cfg).Updates(map[string]interface{}{
			"active":              false,
			"updated_by_admin_id": adminID,
			"updated_at":          time.Now(),
		}).Error; err != nil {
			return fmt.Errorf("failed to deactivate fee config: %w", err)
		}

		cfg.Active = false
		newJSON, _ := json.Marshal(cfg)
		log := &models.FeeAuditLog{
			FeeConfigID:  id,
			AdminID:      adminID,
			Action:       "deactivated",
			PreviousJSON: string(prevJSON),
			NewJSON:      string(newJSON),
			Reason:       reason,
			CreatedAt:    time.Now(),
		}
		return tx.Create(log).Error
	})
}

// GetActiveFeeConfig retrieves the currently active fee config for a given type.
// When multiple active configs exist for the same type, the one with the most
// recent EffectiveFrom that is not in the future is returned.
func (s *FeeService) GetActiveFeeConfig(feeType models.FeeType) (*models.FeeConfig, error) {
	var cfg models.FeeConfig
	err := s.db.
		Where("fee_type = ? AND active = true AND effective_from <= ?", feeType, time.Now()).
		Where("effective_until IS NULL OR effective_until > ?", time.Now()).
		Order("effective_from DESC").
		First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("no active fee config for type: %s", feeType)
		}
		return nil, fmt.Errorf("failed to query fee config: %w", err)
	}
	return &cfg, nil
}

// ListFeeConfigs returns paginated fee configs optionally filtered by type.
func (s *FeeService) ListFeeConfigs(feeType string, page, limit int) ([]models.FeeConfig, int64, error) {
	query := s.db.Model(&models.FeeConfig{})
	if feeType != "" {
		query = query.Where("fee_type = ?", feeType)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count failed: %w", err)
	}

	offset := (page - 1) * limit
	var configs []models.FeeConfig
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&configs).Error; err != nil {
		return nil, 0, fmt.Errorf("list failed: %w", err)
	}

	return configs, total, nil
}

// CalculateFee computes the effective fee for a given transaction amount and
// the user's 30-day volume. Both amounts are expressed in stroops.
func (s *FeeService) CalculateFee(feeType models.FeeType, amountStroops int64, userVolume30dStroops int64) (*models.EffectiveFee, error) {
	cfg, err := s.GetActiveFeeConfig(feeType)
	if err != nil {
		return nil, err
	}

	tiers, err := parseVolumeTiers(cfg.VolumeTiers)
	if err != nil {
		return nil, fmt.Errorf("invalid volume tiers in config: %w", err)
	}

	appliedBps, tierApplied := applyTierDiscount(cfg.BaseBasisPoints, cfg.MinBasisPoints, tiers, userVolume30dStroops)

	feeAmount := amountStroops * int64(appliedBps) / 10_000
	netAmount := amountStroops - feeAmount

	discountBps := cfg.BaseBasisPoints - appliedBps

	return &models.EffectiveFee{
		FeeConfigID:     cfg.ID,
		FeeType:         cfg.FeeType,
		BaseBasisPoints: cfg.BaseBasisPoints,
		AppliedBps:      appliedBps,
		DiscountBps:     discountBps,
		FeeAmount:       feeAmount,
		NetAmount:       netAmount,
		TierApplied:     tierApplied,
	}, nil
}

// GetAuditLog returns the audit trail for a specific fee config.
func (s *FeeService) GetAuditLog(feeConfigID uint, page, limit int) ([]models.FeeAuditLog, int64, error) {
	query := s.db.Model(&models.FeeAuditLog{}).Where("fee_config_id = ?", feeConfigID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count failed: %w", err)
	}

	offset := (page - 1) * limit
	var logs []models.FeeAuditLog
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("list failed: %w", err)
	}

	return logs, total, nil
}

// parseVolumeTiers deserialises the JSON column into a slice of VolumeTier.
func parseVolumeTiers(raw string) ([]models.VolumeTier, error) {
	if raw == "" || raw == "[]" {
		return nil, nil
	}
	var tiers []models.VolumeTier
	if err := json.Unmarshal([]byte(raw), &tiers); err != nil {
		return nil, err
	}
	return tiers, nil
}

// applyTierDiscount selects the best matching tier for the user's volume and
// returns the discounted basis-points value and the tier that was applied.
func applyTierDiscount(baseBps, minBps int, tiers []models.VolumeTier, volumeStroops int64) (int, *models.VolumeTier) {
	if len(tiers) == 0 {
		return baseBps, nil
	}

	// Sort descending by threshold so we pick the highest matching tier first.
	sort.Slice(tiers, func(i, j int) bool {
		return tiers[i].MinVolumeStroops > tiers[j].MinVolumeStroops
	})

	for i := range tiers {
		t := &tiers[i]
		if volumeStroops >= t.MinVolumeStroops {
			applied := baseBps - t.DiscountBps
			if applied < minBps {
				applied = minBps
			}
			return applied, t
		}
	}

	return baseBps, nil
}

// validateConfig performs basic sanity checks on a FeeConfig before save.
func (s *FeeService) validateConfig(cfg *models.FeeConfig) error {
	validTypes := map[models.FeeType]bool{
		models.FeeTypeMarketplace: true,
		models.FeeTypeTransfer:    true,
		models.FeeTypeStaking:     true,
		models.FeeTypeLiquidity:   true,
	}
	if !validTypes[cfg.FeeType] {
		return fmt.Errorf("invalid fee_type: %s", cfg.FeeType)
	}
	if cfg.BaseBasisPoints < 0 {
		return errors.New("base_basis_points cannot be negative")
	}
	if cfg.MaxBasisPoints < cfg.BaseBasisPoints {
		return errors.New("max_basis_points must be >= base_basis_points")
	}
	if cfg.MinBasisPoints < 0 {
		return errors.New("min_basis_points cannot be negative")
	}
	if cfg.MinBasisPoints > cfg.BaseBasisPoints {
		return errors.New("min_basis_points cannot exceed base_basis_points")
	}
	if cfg.Name == "" {
		return errors.New("name is required")
	}
	if cfg.EffectiveFrom.IsZero() {
		cfg.EffectiveFrom = time.Now()
	}

	// Validate volume tiers JSON if provided.
	if cfg.VolumeTiers != "" && cfg.VolumeTiers != "[]" {
		if _, err := parseVolumeTiers(cfg.VolumeTiers); err != nil {
			return fmt.Errorf("volume_tiers is not valid JSON: %w", err)
		}
	}

	return nil
}
