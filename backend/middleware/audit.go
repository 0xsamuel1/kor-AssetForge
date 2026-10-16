package middleware

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

const (
	auditRetention = 7 * 365 * 24 * time.Hour
	compressionAge = 30 * 24 * time.Hour
)

type AuditService struct{ db *gorm.DB }

func NewAuditService(db *gorm.DB) *AuditService { return &AuditService{db: db} }

func (as *AuditService) LogAssetOperation(ctx context.Context, userID *uint, action, resourceID, method, path string, status int, ip, before, after string) error {
	return as.db.WithContext(ctx).Create(&models.AuditLog{
		UserID: userID, Action: action, Resource: "asset", ResourceID: resourceID, Method: method, Path: path,
		Status: status, IPAddress: ip, BeforeState: before, AfterState: after, StateEncoding: "json",
		ExpiresAt: time.Now().UTC().Add(auditRetention),
	}).Error
}

type AuditQuery struct {
	UserID  *uint      `form:"user_id"`
	AssetID string     `form:"asset_id"`
	Action  string     `form:"action"`
	Start   *time.Time `form:"start" time_format:"2006-01-02T15:04:05Z07:00"`
	End     *time.Time `form:"end" time_format:"2006-01-02T15:04:05Z07:00"`
	Page    int        `form:"page"`
	Limit   int        `form:"limit"`
}

func (as *AuditService) Search(ctx context.Context, filter AuditQuery) ([]models.AuditLog, int64, error) {
	q := as.db.WithContext(ctx).Model(&models.AuditLog{}).Where("resource = ?", "asset")
	if filter.UserID != nil {
		q = q.Where("user_id = ?", *filter.UserID)
	}
	if filter.AssetID != "" {
		q = q.Where("resource_id = ?", filter.AssetID)
	}
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if filter.Start != nil {
		q = q.Where("created_at >= ?", *filter.Start)
	}
	if filter.End != nil {
		q = q.Where("created_at <= ?", *filter.End)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 10000 {
		limit = 50
	}
	var logs []models.AuditLog
	err := q.Order("created_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&logs).Error
	return logs, total, err
}

func (as *AuditService) PurgeExpired(ctx context.Context) (int64, error) {
	result := as.db.WithContext(ctx).Where("expires_at <= ?", time.Now().UTC()).Delete(&models.AuditLog{})
	return result.RowsAffected, result.Error
}

// CompactOldStates reduces long-lived audit payloads while preserving them for
// compliance export. The rows remain queryable and DecodeAuditState reverses it.
func (as *AuditService) CompactOldStates(ctx context.Context) (int, error) {
	var logs []models.AuditLog
	if err := as.db.WithContext(ctx).Where("created_at < ? AND state_encoding = ?", time.Now().UTC().Add(-compressionAge), "json").Limit(500).Find(&logs).Error; err != nil {
		return 0, err
	}
	for _, log := range logs {
		before, err := compressAuditState(log.BeforeState)
		if err != nil {
			return 0, err
		}
		after, err := compressAuditState(log.AfterState)
		if err != nil {
			return 0, err
		}
		if err := as.db.WithContext(ctx).Model(&models.AuditLog{}).Where("id = ?", log.ID).Updates(map[string]interface{}{"before_state": before, "after_state": after, "state_encoding": "gzip+base64"}).Error; err != nil {
			return 0, err
		}
	}
	return len(logs), nil
}

func DecodeAuditState(value, encoding string) string {
	if value == "" || encoding != "gzip+base64" {
		return value
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return value
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return value
	}
	defer reader.Close()
	var out bytes.Buffer
	if _, err = out.ReadFrom(reader); err != nil {
		return value
	}
	return out.String()
}

func compressAuditState(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	if _, err := writer.Write([]byte(value)); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(out.Bytes()), nil
}

// SetAssetAuditState lets an asset handler attach authoritative before/after
// snapshots after it has performed the operation.
func SetAssetAuditState(c *gin.Context, before, after interface{}) {
	if before != nil {
		if payload, err := json.Marshal(before); err == nil {
			c.Set("audit_asset_before", string(payload))
		}
	}
	if after != nil {
		if payload, err := json.Marshal(after); err == nil {
			c.Set("audit_asset_after", string(payload))
		}
	}
}

func AuditMiddleware(auditService *AuditService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		path := c.Request.URL.Path
		if !(strings.Contains(path, "/api/v1/assets") || strings.Contains(path, "/api/v2/assets") || strings.Contains(path, "/api/v1/marketplace/")) {
			return
		}
		if c.Writer.Status() >= 500 {
			return
		} // do not record a speculative state after failed work
		resourceID := c.Param("id")
		if resourceID == "" {
			if value, ok := c.Get("audit_asset_id"); ok {
				resourceID = fmt.Sprint(value)
			}
		}
		if resourceID == "" {
			resourceID = "collection"
		}
		var uid *uint
		if value, ok := c.Get("user_id"); ok {
			if id, ok := value.(uint); ok {
				uid = &id
			}
		}
		before, _ := c.Get("audit_asset_before")
		after, _ := c.Get("audit_asset_after")
		beforeText, _ := before.(string)
		afterText, _ := after.(string)
		if err := auditService.LogAssetOperation(c.Request.Context(), uid, auditAction(c.Request.Method), resourceID, c.Request.Method, c.Request.URL.Path, c.Writer.Status(), c.ClientIP(), beforeText, afterText); err != nil {
			_ = c.Error(fmt.Errorf("audit log failed: %w", err))
		}
	}
}

func auditAction(method string) string {
	switch method {
	case "POST":
		return "CREATE"
	case "PUT", "PATCH":
		return "UPDATE"
	case "DELETE":
		return "DELETE"
	default:
		return "READ"
	}
}

// StartAuditMaintenance performs retention and compression on start and daily.
func StartAuditMaintenance(ctx context.Context, service *AuditService) {
	run := func() { _, _ = service.PurgeExpired(ctx); _, _ = service.CompactOldStates(ctx) }
	run()
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
