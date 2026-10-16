package models

import (
	"time"
)

type ExportJobStatus string

const (
	ExportJobStatusPending    ExportJobStatus = "pending"
	ExportJobStatusProcessing ExportJobStatus = "processing"
	ExportJobStatusCompleted  ExportJobStatus = "completed"
	ExportJobStatusFailed     ExportJobStatus = "failed"
)

// ExportJob tracks the progress of a user's GDPR data export request
type ExportJob struct {
	ID            uint            `gorm:"primaryKey" json:"id"`
	UserID        uint            `gorm:"not null;index" json:"user_id"`
	User          User            `gorm:"foreignKey:UserID" json:"-"`
	Status        ExportJobStatus `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	Format        string          `gorm:"type:varchar(10);not null" json:"format"` // "json" or "csv"
	FilePath      string          `json:"-"` // Server path to the encrypted file
	DownloadToken string          `gorm:"type:varchar(255);uniqueIndex" json:"download_token,omitempty"`
	ExpiresAt     time.Time       `json:"expires_at"`
	ErrorDetails  string          `gorm:"type:text" json:"error_details,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}
