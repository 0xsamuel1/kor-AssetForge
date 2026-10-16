package models

import "time"

// BatchStatus constants
const (
	BatchStatusQueued              = "queued"
	BatchStatusProcessing          = "processing"
	BatchStatusCompleted           = "completed"
	BatchStatusCompletedWithErrors = "completed_with_errors"
	BatchStatusFailed              = "failed"
	BatchStatusRolledBack          = "rolled_back"
)

const MaxBatchSize = 50

// BatchQueueEntry represents a batch in the processing queue
type BatchQueueEntry struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	BatchID      uint       `gorm:"not null;uniqueIndex" json:"batch_id"`
	Priority     int        `gorm:"not null;default:0;index" json:"priority"`
	AttemptCount int        `gorm:"not null;default:0" json:"attempt_count"`
	MaxAttempts  int        `gorm:"not null;default:3" json:"max_attempts"`
	ScheduledAt  time.Time  `gorm:"not null;index" json:"scheduled_at"`
	ProcessedAt  *time.Time `json:"processed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// BatchOperationResult tracks the result of each individual operation in a batch
type BatchOperationResult struct {
	Index          int    `json:"index"`
	Type           string `json:"type"`
	Success        bool   `json:"success"`
	Error          string `json:"error,omitempty"`
	TxHash         string `json:"tx_hash,omitempty"`
	ResourceID     string `json:"resource_id,omitempty"`
	PreviousActive *bool  `json:"previous_active,omitempty"`
}
