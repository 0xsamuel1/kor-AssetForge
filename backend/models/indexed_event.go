package models

import (
	"time"

	"gorm.io/gorm"
)

type EventStatus string

const (
	EventStatusPending   EventStatus = "pending"
	EventStatusProcessed EventStatus = "processed"
	EventStatusFailed    EventStatus = "failed"
)

// IndexedEvent represents a blockchain event captured by the indexer
type IndexedEvent struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	ContractID     string         `gorm:"type:varchar(56);index" json:"contract_id"`
	Ledger         uint32         `gorm:"index" json:"ledger"`
	LedgerClosedAt time.Time      `json:"ledger_closed_at"`
	TxHash         string         `gorm:"type:varchar(64)" json:"tx_hash"`
	EventID        string         `gorm:"type:varchar(255);uniqueIndex" json:"event_id"`
	Topic          string         `gorm:"type:text" json:"topic"` // JSON array of topics
	Value          string         `gorm:"type:text" json:"value"` // JSON value
	Status         EventStatus    `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	RetryCount     int            `gorm:"default:0" json:"retry_count"`
	ErrorDetails   string         `gorm:"type:text" json:"error_details,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// EventCheckpoint tracks the sync state of the indexer
type EventCheckpoint struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Checkpoint string    `gorm:"type:varchar(100);uniqueIndex" json:"checkpoint"` // Name/Key of the checkpoint, e.g. "global_stellar_indexer"
	LastLedger uint32    `json:"last_ledger"`
	Cursor     string    `gorm:"type:varchar(255)" json:"cursor"` // Paging token or cursor if applicable
	UpdatedAt  time.Time `json:"updated_at"`
}
