package jobs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestArchiveAuditLogsWritesBeforeDelete(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AuditLog{}))
	created := time.Now().UTC().Add(-8 * 365 * 24 * time.Hour)
	require.NoError(t, db.Create(&models.AuditLog{Action: "READ", Resource: "asset", ResourceID: "1", Method: "GET", Path: "/assets/1", Status: 200, IPAddress: "127.0.0.1", ExpiresAt: created.Add(7 * 365 * 24 * time.Hour), CreatedAt: created}).Error)
	dir := t.TempDir()
	count, err := ArchiveAuditLogs(context.Background(), db, dir, time.Now().UTC().Add(-7*365*24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, 1, count)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	var remaining int64
	require.NoError(t, db.Model(&models.AuditLog{}).Count(&remaining).Error)
	require.Equal(t, int64(0), remaining)
}