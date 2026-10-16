package models

import (
	"time"
)

type MarketMakerBot struct {
	ID                     uint                   `gorm:"primaryKey" json:"id"`
	Name                   string                 `gorm:"not null" json:"name"`
	Status                 string                 `gorm:"default:'inactive'" json:"status"`
	ManagedAssetID         uint                   `gorm:"not null;index" json:"managed_asset_id"`
	ManagedAsset           Asset                  `gorm:"foreignKey:ManagedAssetID" json:"managed_asset,omitempty"`
	OperatorAddress        string                 `gorm:"not null" json:"operator_address"`
	MinSpreadBps           int16                  `gorm:"default:50" json:"min_spread_bps"`
	MaxPositionStroops     int64                  `gorm:"not null" json:"max_position_stroops"`
	InventoryTargetStroops int64                  `gorm:"not null" json:"inventory_target_stroops"`
	TotalVolumeStroops     int64                  `gorm:"default:0" json:"total_volume_stroops"`
	ProfitLossStroops      int64                  `gorm:"default:0" json:"profit_loss_stroops"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
	Orders                 []MarketMakerOrder     `gorm:"foreignKey:BotID" json:"orders,omitempty"`
	Inventory              []MarketMakerInventory `gorm:"foreignKey:BotID" json:"inventory,omitempty"`
}

type MarketMakerOrder struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	BotID        uint           `gorm:"not null;index" json:"bot_id"`
	Bot          MarketMakerBot `gorm:"foreignKey:BotID" json:"bot,omitempty"`
	OrderType    string         `gorm:"not null" json:"order_type"`
	AssetID      uint           `gorm:"not null" json:"asset_id"`
	Asset        Asset          `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	PriceStroops int64          `gorm:"not null" json:"price_stroops"`
	AmountUnits  int64          `gorm:"not null" json:"amount_units"`
	FilledUnits  int64          `gorm:"default:0" json:"filled_units"`
	Status       string         `gorm:"default:'active'" json:"status"`
	OrderHash    string         `gorm:"index" json:"order_hash"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type MarketMakerTrade struct {
	ID                  uint             `gorm:"primaryKey" json:"id"`
	BotID               uint             `gorm:"not null;index" json:"bot_id"`
	Bot                 MarketMakerBot   `gorm:"foreignKey:BotID" json:"bot,omitempty"`
	OrderID             uint             `gorm:"not null" json:"order_id"`
	Order               MarketMakerOrder `gorm:"foreignKey:OrderID" json:"order,omitempty"`
	CounterpartyAddress string           `gorm:"not null" json:"counterparty_address"`
	Side                string           `gorm:"not null" json:"side"`
	PriceStroops        int64            `gorm:"not null" json:"price_stroops"`
	AmountUnits         int64            `gorm:"not null" json:"amount_units"`
	FeeStroops          int64            `gorm:"not null" json:"fee_stroops"`
	ProfitStroops       int64            `gorm:"type:numeric" json:"profit_stroops"`
	TxHash              string           `gorm:"index" json:"tx_hash"`
	CreatedAt           time.Time        `json:"created_at"`
}

type MarketMakerInventory struct {
	ID                   uint           `gorm:"primaryKey" json:"id"`
	BotID                uint           `gorm:"not null;index" json:"bot_id"`
	Bot                  MarketMakerBot `gorm:"foreignKey:BotID" json:"bot,omitempty"`
	AssetID              uint           `gorm:"not null" json:"asset_id"`
	Asset                Asset          `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	HeldUnits            int64          `gorm:"default:0" json:"held_units"`
	CostBasisStroops     int64          `gorm:"default:0" json:"cost_basis_stroops"`
	UnrealizedPnlStroops int64          `gorm:"default:0" json:"unrealized_pnl_stroops"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

type MarketMakerHealthCheck struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	BotID             uint           `gorm:"not null;index" json:"bot_id"`
	Bot               MarketMakerBot `gorm:"foreignKey:BotID" json:"bot,omitempty"`
	IsHealthy         bool           `gorm:"not null" json:"is_healthy"`
	UptimePercentage  float64        `gorm:"type:numeric(5,2)" json:"uptime_percentage"`
	LastTradeTime     time.Time      `json:"last_trade_time"`
	ActiveOrdersCount int64          `gorm:"default:0" json:"active_orders_count"`
	Message           string         `gorm:"type:text" json:"message"`
	CheckedAt         time.Time      `json:"checked_at"`
}
