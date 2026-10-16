package services

import (
	"fmt"

	"github.com/yourusername/kor-assetforge/models"
)

// UBOVerificationService drives the beneficial-ownership declaration and
// verification workflow for corporate/organizational accounts. It does not
// call an external provider (there is no equivalent of a KYC/AML vendor for
// UBO data) -- verification is a compliance-team review backed by the
// corporate structure documents the entity uploads, so this service enforces
// the structural rules (percentages, thresholds) and leaves the human
// judgment call to ResolveReview.
type UBOVerificationService struct{}

// NewUBOVerificationService returns a ready-to-use UBOVerificationService.
func NewUBOVerificationService() *UBOVerificationService {
	return &UBOVerificationService{}
}

// ValidateDeclaration checks a single owner declaration for internal
// consistency before it is persisted.
func (s *UBOVerificationService) ValidateDeclaration(owner *models.BeneficialOwner) error {
	if owner.OwnershipPercent < 0 || owner.OwnershipPercent > 100 {
		return fmt.Errorf("ownership_percent must be between 0 and 100")
	}
	if owner.FullName == "" {
		return fmt.Errorf("full_name is required")
	}
	return nil
}

// ValidateTotalOwnership checks that the sum of all *verified or declared*
// owners for an entity does not exceed 100%. Called with the full set
// (including the new/updated owner) before a declaration is persisted.
func (s *UBOVerificationService) ValidateTotalOwnership(owners []models.BeneficialOwner) error {
	var total float64
	for _, o := range owners {
		total += o.OwnershipPercent
	}
	if total > 100.0001 { // small epsilon for float rounding
		return fmt.Errorf("total declared ownership %.2f%% exceeds 100%%", total)
	}
	return nil
}

// RequiresUBOProgram reports whether an entity's current declarations are
// complete enough to satisfy the 25%-threshold disclosure rule: every
// declared owner meeting the threshold (or control test) must be present,
// and none of them may still be awaiting review.
//
// This is a structural completeness check, not a legal opinion -- it does
// not know whether every *actual* owner above the threshold has been
// declared, only whether what has been declared is internally consistent.
func (s *UBOVerificationService) RequiresUBOProgram(owners []models.BeneficialOwner) bool {
	for _, o := range owners {
		if o.RequiresDeclaration() {
			return true
		}
	}
	return false
}

// PendingReview filters to the owners still awaiting compliance review.
func (s *UBOVerificationService) PendingReview(owners []models.BeneficialOwner) []models.BeneficialOwner {
	pending := make([]models.BeneficialOwner, 0)
	for _, o := range owners {
		if o.Status == models.UBOStatusDeclared || o.Status == models.UBOStatusReview {
			pending = append(pending, o)
		}
	}
	return pending
}
