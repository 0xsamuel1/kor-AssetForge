package services

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// Referral domain errors.
var (
	ErrSelfReferral      = errors.New("users cannot refer themselves")
	ErrAlreadyReferred   = errors.New("user has already been referred")
	ErrReferralCodeUsed  = errors.New("referral code has reached its usage limit")
	ErrReferralInactive  = errors.New("referral code is not active")
	ErrReferralNotFound  = errors.New("referral code not found")
)

// rewardTier defines the commission a referrer earns based on how many
// successful referrals they have accumulated. Tiers reward power-referrers (#170).
type rewardTier struct {
	Tier         int
	MinReferrals int
	// CommissionUSD is the referrer reward in cents per qualified referral.
	CommissionUSD int64
}

// referralTiers is ordered from highest threshold to lowest so the first match
// wins.
var referralTiers = []rewardTier{
	{Tier: 4, MinReferrals: 50, CommissionUSD: 5000},
	{Tier: 3, MinReferrals: 20, CommissionUSD: 3000},
	{Tier: 2, MinReferrals: 5, CommissionUSD: 2000},
	{Tier: 1, MinReferrals: 0, CommissionUSD: 1000},
}

// refereeBonusUSD is the signup bonus credited to a referee on qualification.
const refereeBonusUSD int64 = 500

// ReferralService implements the referral program: code generation, referral
// linking with fraud detection, tiered reward distribution and chain stats (#170).
type ReferralService struct {
	db *gorm.DB
}

// NewReferralService creates a ReferralService.
func NewReferralService(db *gorm.DB) *ReferralService {
	return &ReferralService{db: db}
}

// GetOrCreateCode returns the user's existing referral code, creating one if
// needed.
func (s *ReferralService) GetOrCreateCode(userID uint) (*models.ReferralCode, error) {
	var code models.ReferralCode
	err := s.db.Where("user_id = ?", userID).First(&code).Error
	if err == nil {
		return &code, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Generate a unique code, retrying on the rare collision.
	for attempt := 0; attempt < 5; attempt++ {
		candidate, genErr := generateReferralCode()
		if genErr != nil {
			return nil, genErr
		}
		code = models.ReferralCode{UserID: userID, Code: candidate, Active: true}
		if createErr := s.db.Create(&code).Error; createErr == nil {
			return &code, nil
		}
	}
	return nil, errors.New("failed to generate a unique referral code")
}

// ApplyReferral links a new referee to the owner of the given code. It runs fraud
// checks: self-referral, double-referral, code limits, and IP-collision flags.
func (s *ReferralService) ApplyReferral(refereeID uint, code, signupIP string) (*models.Referral, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, ErrReferralNotFound
	}

	var rc models.ReferralCode
	if err := s.db.Where("code = ?", code).First(&rc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReferralNotFound
		}
		return nil, err
	}
	if !rc.Active {
		return nil, ErrReferralInactive
	}
	if rc.MaxUses > 0 && rc.Uses >= rc.MaxUses {
		return nil, ErrReferralCodeUsed
	}
	if rc.UserID == refereeID {
		return nil, ErrSelfReferral
	}

	// A user can only be referred once.
	var existing int64
	s.db.Model(&models.Referral{}).Where("referee_id = ?", refereeID).Count(&existing)
	if existing > 0 {
		return nil, ErrAlreadyReferred
	}

	referral := &models.Referral{
		ReferrerID: rc.UserID,
		RefereeID:  refereeID,
		Code:       code,
		Status:     models.ReferralPending,
		Tier:       1,
		SignupIP:   signupIP,
	}

	// Fraud heuristic: if the referee signs up from the same IP as the referrer's
	// other referrals, flag for review rather than auto-approving.
	if signupIP != "" && s.ipCollision(rc.UserID, signupIP) {
		referral.Status = models.ReferralRejected
		referral.RejectReason = "suspected self-referral: signup IP matches prior referral from same referrer"
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(referral).Error; err != nil {
			return err
		}
		return tx.Model(&models.ReferralCode{}).Where("id = ?", rc.ID).
			UpdateColumn("uses", gorm.Expr("uses + 1")).Error
	})
	if err != nil {
		return nil, err
	}
	return referral, nil
}

// QualifyReferral marks a referral as qualified and distributes tiered rewards to
// the referrer (and a signup bonus to the referee). It is idempotent: already
// rewarded referrals are returned unchanged.
func (s *ReferralService) QualifyReferral(refereeID uint) (*models.Referral, error) {
	var referral models.Referral
	if err := s.db.Where("referee_id = ?", refereeID).First(&referral).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReferralNotFound
		}
		return nil, err
	}
	if referral.Status == models.ReferralRewarded {
		return &referral, nil
	}
	if referral.Status == models.ReferralRejected {
		return &referral, errors.New("referral was rejected and cannot be rewarded")
	}

	tier := s.tierFor(referral.ReferrerID)
	now := time.Now().UTC()

	err := s.db.Transaction(func(tx *gorm.DB) error {
		referral.Status = models.ReferralRewarded
		referral.Tier = tier.Tier
		referral.RewardAmountUSD = tier.CommissionUSD
		referral.QualifiedAt = &now
		referral.RewardedAt = &now
		if err := tx.Save(&referral).Error; err != nil {
			return err
		}

		rewards := []models.ReferralReward{
			{ReferralID: referral.ID, UserID: referral.ReferrerID, AmountUSD: tier.CommissionUSD, Type: "referrer_commission", Status: "credited"},
			{ReferralID: referral.ID, UserID: referral.RefereeID, AmountUSD: refereeBonusUSD, Type: "referee_bonus", Status: "credited"},
		}
		return tx.Create(&rewards).Error
	})
	if err != nil {
		return nil, err
	}
	return &referral, nil
}

// ReferralStats summarizes a user's referral activity.
type ReferralStats struct {
	Code              string  `json:"code"`
	TotalReferrals    int64   `json:"total_referrals"`
	PendingReferrals  int64   `json:"pending_referrals"`
	RewardedReferrals int64   `json:"rewarded_referrals"`
	CurrentTier       int     `json:"current_tier"`
	NextTierAt        int     `json:"next_tier_at"`
	TotalEarnedUSD    int64   `json:"total_earned_usd"`
	ChainDepth        int     `json:"chain_depth"`
}

// Stats returns aggregate referral statistics for a user, including the depth of
// the referral chain that originates from them.
func (s *ReferralService) Stats(userID uint) (*ReferralStats, error) {
	stats := &ReferralStats{}

	var rc models.ReferralCode
	if err := s.db.Where("user_id = ?", userID).First(&rc).Error; err == nil {
		stats.Code = rc.Code
	}

	s.db.Model(&models.Referral{}).Where("referrer_id = ?", userID).Count(&stats.TotalReferrals)
	s.db.Model(&models.Referral{}).Where("referrer_id = ? AND status = ?", userID, models.ReferralPending).Count(&stats.PendingReferrals)
	s.db.Model(&models.Referral{}).Where("referrer_id = ? AND status = ?", userID, models.ReferralRewarded).Count(&stats.RewardedReferrals)

	var totalEarned int64
	s.db.Model(&models.ReferralReward{}).
		Where("user_id = ? AND type = ?", userID, "referrer_commission").
		Select("COALESCE(SUM(amount_usd), 0)").Scan(&totalEarned)
	stats.TotalEarnedUSD = totalEarned

	tier := s.tierFor(userID)
	stats.CurrentTier = tier.Tier
	stats.NextTierAt = nextTierThreshold(tier.Tier)
	stats.ChainDepth = s.chainDepth(userID, 0, make(map[uint]bool))

	return stats, nil
}

// ListReferrals returns the referrals a user has made.
func (s *ReferralService) ListReferrals(userID uint) ([]models.Referral, error) {
	var referrals []models.Referral
	err := s.db.Where("referrer_id = ?", userID).Order("created_at DESC").Find(&referrals).Error
	return referrals, err
}

// tierFor resolves the reward tier for a referrer based on their count of
// successful (rewarded) referrals.
func (s *ReferralService) tierFor(userID uint) rewardTier {
	var rewarded int64
	s.db.Model(&models.Referral{}).Where("referrer_id = ? AND status = ?", userID, models.ReferralRewarded).Count(&rewarded)
	for _, t := range referralTiers {
		if int(rewarded) >= t.MinReferrals {
			return t
		}
	}
	return referralTiers[len(referralTiers)-1]
}

// ipCollision reports whether the referrer already has a referral that signed up
// from the same IP — a common self-referral abuse pattern.
func (s *ReferralService) ipCollision(referrerID uint, signupIP string) bool {
	var count int64
	s.db.Model(&models.Referral{}).
		Where("referrer_id = ? AND signup_ip = ?", referrerID, signupIP).Count(&count)
	return count > 0
}

// chainDepth computes the maximum depth of the referral tree rooted at userID,
// guarding against cycles. Depth 0 means the user has referred no one.
func (s *ReferralService) chainDepth(userID uint, depth int, visited map[uint]bool) int {
	if visited[userID] || depth > 20 {
		return depth
	}
	visited[userID] = true

	var refereeIDs []uint
	s.db.Model(&models.Referral{}).Where("referrer_id = ?", userID).Pluck("referee_id", &refereeIDs)
	maxChild := depth
	for _, id := range refereeIDs {
		if d := s.chainDepth(id, depth+1, visited); d > maxChild {
			maxChild = d
		}
	}
	return maxChild
}

// nextTierThreshold returns the referral count required to reach the next tier,
// or -1 if already at the top tier.
func nextTierThreshold(currentTier int) int {
	best := -1
	for _, t := range referralTiers {
		if t.Tier == currentTier+1 {
			return t.MinReferrals
		}
	}
	return best
}

// generateReferralCode produces a random, human-friendly uppercase code without
// easily-confused characters.
func generateReferralCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	const length = 8
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate referral code: %w", err)
	}
	out := make([]byte, length)
	for i, v := range b {
		out[i] = alphabet[int(v)%len(alphabet)]
	}
	return string(out), nil
}
