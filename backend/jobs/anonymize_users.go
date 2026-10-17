package jobs

import (
	"context"

	"github.com/yourusername/kor-assetforge/services"
	"gorm.io/gorm"
)

// AnonymizeDeletedUsers processes users whose deletion request has been
// approved. The caller supplies the retention predicate so policy timing stays
// outside the job implementation.
func AnonymizeDeletedUsers(ctx context.Context, db *gorm.DB, userIDs []uint) error {
	service := services.NewAnonymizationService(db)
	for _, userID := range userIDs {
		if err := RunTraced(ctx, "anonymize-user", func(context.Context) error { return service.AnonymizeUser(userID) }); err != nil {
			return err
		}
	}
	return nil
}