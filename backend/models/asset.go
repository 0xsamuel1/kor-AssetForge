package models

import (
	"time"

	"gorm.io/gorm"
)

// Asset represents a tokenized real-world asset
type Asset struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Name         string         `gorm:"not null" json:"name"`
	Symbol       string         `gorm:"not null;uniqueIndex" json:"symbol"`
	Description  string         `json:"description"`
	AssetType    string         `gorm:"not null" json:"asset_type"` // real_estate, art, commodity, etc.
	TotalSupply  int64          `gorm:"not null" json:"total_supply"`
	Fractions    uint64         `gorm:"default:0" json:"fractions"`
	ContractID   string         `gorm:"uniqueIndex" json:"contract_id"`
	OwnerAddress string         `gorm:"not null" json:"owner_address"`
	Metadata     string         `gorm:"type:text" json:"metadata"` // JSON string of map[string]string
	ImageURL     string         `json:"image_url"`
	DocumentURL  string         `json:"document_url"`
	Verified     bool           `gorm:"default:false" json:"verified"`
	MetadataURI  string         `json:"metadata_uri"`
	MetadataHash string         `json:"metadata_hash"`
	IsImmutable  bool           `gorm:"default:false" json:"is_immutable"`
	IPFSCID      string         `json:"ipfs_cid"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// NFTMetadata represents the NFT metadata standard format
type NFTMetadata struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Image       string                 `json:"image"`
	ExternalURL string                 `json:"external_url"`
	Attributes  []NFTAttribute         `json:"attributes"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
}

// NFTAttribute represents a metadata attribute
type NFTAttribute struct {
	TraitType string      `json:"trait_type"`
	Value     interface{} `json:"value"`
	Display   string      `json:"display_type,omitempty"`
}

// Listing represents a marketplace listing
type Listing struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	AssetID      uint           `gorm:"not null" json:"asset_id"`
	Asset        Asset          `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	SellerAddr   string         `gorm:"not null" json:"seller_address"`
	Amount       int64          `gorm:"not null" json:"amount"`
	PricePerUnit int64          `gorm:"not null" json:"price_per_unit"` // in stroops
	Active       bool           `gorm:"default:true" json:"active"`
	ListingID    string         `gorm:"uniqueIndex" json:"listing_id"` // On-chain listing ID
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// Transaction represents an asset transfer
type Transaction struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	AssetID     uint      `gorm:"not null" json:"asset_id"`
	Asset       Asset     `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	FromAddress string    `gorm:"not null" json:"from_address"`
	ToAddress   string    `gorm:"not null" json:"to_address"`
	Amount      int64     `gorm:"not null" json:"amount"`
	TxHash      string    `gorm:"uniqueIndex" json:"tx_hash"`
	Status      string    `gorm:"default:'pending'" json:"status"` // pending, confirmed, failed
	Memo        string    `gorm:"type:varchar(500)" json:"memo,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// BatchTransaction represents a batch of multiple operations
type BatchTransaction struct {
	ID                        uint       `gorm:"primaryKey" json:"id"`
	UserID                    uint       `gorm:"not null;index" json:"user_id"`
	User                      User       `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Operations                string     `gorm:"type:jsonb;not null" json:"operations"` // JSON array of operations
	Status                    string     `gorm:"default:'pending'" json:"status"`       // pending, processing, completed, failed, rolled_back
	TxHash                    string     `gorm:"uniqueIndex" json:"tx_hash,omitempty"`
	ErrorDetails              string     `gorm:"type:text" json:"error_details,omitempty"`
	GasEstimate               int64      `json:"gas_estimate,omitempty"`
	CompletedCount            int        `gorm:"default:0" json:"completed_count"`
	FailedCount               int        `gorm:"default:0" json:"failed_count"`
	TotalOperations           int        `gorm:"not null" json:"total_operations"`
	ProgressPercent           float64    `gorm:"default:0" json:"progress_percent"`
	EstimatedSecondsRemaining int64      `gorm:"default:0" json:"estimated_seconds_remaining"`
	OperationResults          string     `gorm:"type:jsonb" json:"operation_results,omitempty"`
	StartedAt                 *time.Time `json:"started_at,omitempty"`
	FinishedAt                *time.Time `json:"finished_at,omitempty"`
	RollbackAt                *time.Time `json:"rollback_at,omitempty"`
	RollbackDetails           string     `gorm:"type:text" json:"rollback_details,omitempty"`
	// Retry metadata used by BatchRetryService for automatic retry scheduling.
	RetryCount  int            `gorm:"default:0" json:"retry_count"`
	MaxRetries  int            `gorm:"default:3" json:"max_retries"`
	NextRetryAt *time.Time     `json:"next_retry_at,omitempty"`
	LastRetryAt *time.Time     `json:"last_retry_at,omitempty"`
	IsRetryable bool           `gorm:"default:true" json:"is_retryable"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// BatchOperation represents a single operation within a batch
type BatchOperation struct {
	Type        string                 `json:"type"` // transfer, mint, burn, list, cancel_listing
	AssetID     uint                   `json:"asset_id"`
	FromAddress string                 `json:"from_address,omitempty"`
	ToAddress   string                 `json:"to_address,omitempty"`
	Amount      int64                  `json:"amount"`
	ExtraParams map[string]interface{} `json:"extra_params,omitempty"`
}
