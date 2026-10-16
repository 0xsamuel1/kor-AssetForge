package models

import (
	"time"

	"gorm.io/gorm"
)

type ReportFrequency string

const (
	ReportFrequencyDaily   ReportFrequency = "daily"
	ReportFrequencyWeekly  ReportFrequency = "weekly"
	ReportFrequencyMonthly ReportFrequency = "monthly"
)

type ReportFormat string

const (
	ReportFormatPDF   ReportFormat = "pdf"
	ReportFormatCSV   ReportFormat = "csv"
	ReportFormatExcel ReportFormat = "excel"
)

type ReportDeliveryMethod string

const (
	ReportDeliveryEmail   ReportDeliveryMethod = "email"
	ReportDeliveryWebhook ReportDeliveryMethod = "webhook"
)

type ScheduledReportStatus string

const (
	ScheduledReportStatusActive   ScheduledReportStatus = "active"
	ScheduledReportStatusPaused   ScheduledReportStatus = "paused"
	ScheduledReportStatusDisabled ScheduledReportStatus = "disabled"
)

type ReportDeliveryStatus string

const (
	ReportDeliveryStatusPending ReportDeliveryStatus = "pending"
	ReportDeliveryStatusSuccess ReportDeliveryStatus = "success"
	ReportDeliveryStatusFailed  ReportDeliveryStatus = "failed"
)

// ScheduledReport stores a user's recurring report configuration.
type ScheduledReport struct {
	ID              uint                    `gorm:"primaryKey" json:"id"`
	UserID          uint                    `gorm:"not null;index" json:"user_id"`
	User            User                    `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Name            string                  `gorm:"not null" json:"name"`
	ReportType      string                  `gorm:"not null;index" json:"report_type"`
	Frequency       ReportFrequency         `gorm:"not null" json:"frequency"`
	CronExpression  string                  `gorm:"not null" json:"cron_expression"`
	Timezone        string                  `gorm:"not null;default:'UTC'" json:"timezone"`
	Format          ReportFormat            `gorm:"not null;default:'pdf'" json:"format"`
	DeliveryMethod  ReportDeliveryMethod    `gorm:"not null" json:"delivery_method"`
	EmailRecipients string                  `gorm:"type:text" json:"email_recipients,omitempty"`
	WebhookURL      string                  `gorm:"type:text" json:"webhook_url,omitempty"`
	Filters         string                  `gorm:"type:jsonb;default:'{}'" json:"filters,omitempty"`
	RetentionDays   int                     `gorm:"not null;default:90" json:"retention_days"`
	Status          ScheduledReportStatus   `gorm:"not null;default:'active';index" json:"status"`
	LastRunAt       *time.Time              `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time              `json:"next_run_at,omitempty"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
	DeletedAt       gorm.DeletedAt          `gorm:"index" json:"-"`
	Histories       []ReportDeliveryHistory `gorm:"foreignKey:ScheduledReportID" json:"histories,omitempty"`
}

// ReportDeliveryHistory records each generated report and delivery outcome.
type ReportDeliveryHistory struct {
	ID                uint                 `gorm:"primaryKey" json:"id"`
	ScheduledReportID uint                 `gorm:"not null;index" json:"scheduled_report_id"`
	ScheduledReport   ScheduledReport      `gorm:"foreignKey:ScheduledReportID" json:"-"`
	UserID            uint                 `gorm:"not null;index" json:"user_id"`
	ReportType        string               `gorm:"not null" json:"report_type"`
	Format            ReportFormat         `gorm:"not null" json:"format"`
	DeliveryMethod    ReportDeliveryMethod `gorm:"not null" json:"delivery_method"`
	Status            ReportDeliveryStatus `gorm:"not null;default:'pending';index" json:"status"`
	FileName          string               `gorm:"not null" json:"file_name"`
	FilePath          string               `gorm:"type:text" json:"file_path"`
	Payload           string               `gorm:"type:text" json:"payload,omitempty"`
	ErrorMessage      string               `gorm:"type:text" json:"error_message,omitempty"`
	GeneratedAt       time.Time            `json:"generated_at"`
	DeliveredAt       *time.Time           `json:"delivered_at,omitempty"`
	CreatedAt         time.Time            `json:"created_at"`
}
