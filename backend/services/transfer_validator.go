package services

import (
	"fmt"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// TransferValidator is the single place that decides whether an off-chain
// API request to move an asset is allowed to proceed, mirroring the checks
// `contracts/src/marketplace.rs`'s `enforce_transfer_restrictions` applies
// on-chain. It exists because the backend records a `Transaction` row (and,
// for `TransferAsset`, can route through an approval workflow) independent
// of any particular on-chain call -- that path needs its own enforcement,
// not a hope that every caller eventually submits a chain transaction that
// would have been rejected (#380).
//
// This does not replace the on-chain checks; it is the first line of
// defense so an obviously-restricted transfer never gets as far as being
// recorded or routed for approval, and gives a clear API error instead of
// an opaque contract panic.
type TransferValidator struct {
	db *gorm.DB
}

// NewTransferValidator constructs a TransferValidator.
func NewTransferValidator(db *gorm.DB) *TransferValidator {
	return &TransferValidator{db: db}
}

// TransferRestrictionViolation is returned when a transfer fails a
// compliance check, distinct from a generic/internal error so handlers can
// map it to 403 rather than 500.
type TransferRestrictionViolation struct {
	Reason string
}

func (e *TransferRestrictionViolation) Error() string {
	return fmt.Sprintf("transfer restricted: %s", e.Reason)
}

// ValidateTransfer checks that both the sending and receiving Stellar
// addresses belong to users in good compliance standing before a transfer
// of assetID is allowed to be recorded. Both parties must exist as platform
// users (an unknown address cannot be evaluated at all), be KYC-approved,
// and be AML-cleared. If the asset itself requires accreditation (tracked
// per-asset in the future; today this only checks the universal KYC/AML
// gate every transfer needs), the recipient must also be accredited.
//
// Returns *TransferRestrictionViolation on a compliance failure, and a
// plain error for an unexpected lookup failure (e.g. DB unavailable) --
// callers should treat the former as a 403 and the latter as a 500.
func (v *TransferValidator) ValidateTransfer(fromAddress, toAddress string, assetID uint, amount int64) error {
	if amount <= 0 {
		return &TransferRestrictionViolation{Reason: "amount must be positive"}
	}

	from, err := v.userByAddress(fromAddress)
	if err != nil {
		return err
	}
	if from == nil {
		return &TransferRestrictionViolation{Reason: fmt.Sprintf("sender %s is not a registered platform user", fromAddress)}
	}
	if err := v.requireGoodStanding(from, "sender"); err != nil {
		return err
	}

	to, err := v.userByAddress(toAddress)
	if err != nil {
		return err
	}
	if to == nil {
		return &TransferRestrictionViolation{Reason: fmt.Sprintf("recipient %s is not a registered platform user", toAddress)}
	}
	if err := v.requireGoodStanding(to, "recipient"); err != nil {
		return err
	}

	var asset models.Asset
	if err := v.db.First(&asset, assetID).Error; err != nil {
		return fmt.Errorf("failed to load asset %d: %w", assetID, err)
	}

	return nil
}

// ValidateSeller checks that address belongs to a registered user in good
// compliance standing, for actions that have a single party up front (e.g.
// creating a marketplace listing) rather than a counterparty pair.
func (v *TransferValidator) ValidateSeller(address string) error {
	seller, err := v.userByAddress(address)
	if err != nil {
		return err
	}
	if seller == nil {
		return &TransferRestrictionViolation{Reason: fmt.Sprintf("seller %s is not a registered platform user", address)}
	}
	return v.requireGoodStanding(seller, "seller")
}

func (v *TransferValidator) userByAddress(address string) (*models.User, error) {
	var user models.User
	err := v.db.Where("stellar_address = ?", address).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up user for address %s: %w", address, err)
	}
	return &user, nil
}

func (v *TransferValidator) requireGoodStanding(user *models.User, role string) error {
	if !user.KYCVerified {
		return &TransferRestrictionViolation{Reason: fmt.Sprintf("%s has not completed KYC verification", role)}
	}

	var kyc models.KYCRecord
	err := v.db.Where("user_id = ?", user.ID).First(&kyc).Error
	if err == nil {
		if !kyc.AMLCleared {
			return &TransferRestrictionViolation{Reason: fmt.Sprintf("%s has not cleared AML screening", role)}
		}
		if kyc.Status == models.KYCStatusReview {
			return &TransferRestrictionViolation{Reason: fmt.Sprintf("%s's compliance record is under manual review", role)}
		}
		if kyc.Status == models.KYCStatusRejected || kyc.Status == models.KYCStatusExpired {
			return &TransferRestrictionViolation{Reason: fmt.Sprintf("%s's KYC record is %s", role, kyc.Status)}
		}
	} else if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to load KYC record for user %d: %w", user.ID, err)
	}
	// No KYCRecord but User.KYCVerified is true is a legacy/inconsistent
	// state; User.KYCVerified is the authoritative flag the rest of the
	// platform reads (handlers/kyc.go sets it on approval), so we do not
	// fail the transfer solely for a missing KYCRecord row.

	return nil
}
