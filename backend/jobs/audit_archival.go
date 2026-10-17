package jobs

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// ArchiveAuditLogs writes immutable JSONL gzip archives before deleting the
// archived rows. The archive directory is the configured cold-storage mount.
func ArchiveAuditLogs(ctx context.Context, db *gorm.DB, coldStorageDir string, olderThan time.Time) (int, error) {
	var logs []models.AuditLog
	if err := db.WithContext(ctx).Where("created_at < ?", olderThan).Find(&logs).Error; err != nil { return 0, err }
	if len(logs) == 0 { return 0, nil }
	if err := os.MkdirAll(coldStorageDir, 0700); err != nil { return 0, err }
	path := filepath.Join(coldStorageDir, fmt.Sprintf("audit-%s.jsonl.gz", olderThan.Format("2006-01-02")))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil { return 0, err }
	writer := gzip.NewWriter(file)
	for _, log := range logs {
		if err := json.NewEncoder(writer).Encode(log); err != nil { writer.Close(); file.Close(); return 0, err }
	}
	if err := writer.Close(); err != nil { file.Close(); return 0, err }
	if err := file.Close(); err != nil { return 0, err }
	result := db.WithContext(ctx).Where("created_at < ?", olderThan).Delete(&models.AuditLog{})
	return int(result.RowsAffected), result.Error
}