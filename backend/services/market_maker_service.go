package services

import (
	"errors"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

type MarketMakerService struct {
	db *gorm.DB
}

func NewMarketMakerService(db *gorm.DB) *MarketMakerService {
	return &MarketMakerService{db: db}
}

func (ms *MarketMakerService) CreateBot(bot *models.MarketMakerBot) error {
	if bot.Name == "" || bot.OperatorAddress == "" || bot.ManagedAssetID == 0 {
		return errors.New("missing required fields")
	}
	if bot.MaxPositionStroops <= 0 || bot.InventoryTargetStroops <= 0 {
		return errors.New("position and inventory limits must be positive")
	}
	if bot.MinSpreadBps == 0 {
		bot.MinSpreadBps = 50
	}
	return ms.db.Create(bot).Error
}

func (ms *MarketMakerService) GetBot(id uint) (*models.MarketMakerBot, error) {
	var bot models.MarketMakerBot
	if err := ms.db.Where("id = ?", id).
		Preload("ManagedAsset").
		Preload("Inventory").
		First(&bot).Error; err != nil {
		return nil, err
	}
	return &bot, nil
}

func (ms *MarketMakerService) GetBotsByStatus(status string) ([]models.MarketMakerBot, error) {
	var bots []models.MarketMakerBot
	if err := ms.db.Where("status = ?", status).
		Preload("ManagedAsset").
		Find(&bots).Error; err != nil {
		return nil, err
	}
	return bots, nil
}

func (ms *MarketMakerService) UpdateBotStatus(id uint, status string) error {
	return ms.db.Model(&models.MarketMakerBot{}).Where("id = ?", id).Update("status", status).Error
}

func (ms *MarketMakerService) CalculatePricingSpread(marketPrice int64, volatility float64, targetSpreadBps int16) (int64, int64) {
	minSpreadStroops := (marketPrice * int64(targetSpreadBps)) / 10000

	adjustedSpreadStroops := int64(float64(minSpreadStroops) * (1.0 + volatility))
	if adjustedSpreadStroops < minSpreadStroops {
		adjustedSpreadStroops = minSpreadStroops
	}

	bidPrice := marketPrice - adjustedSpreadStroops/2
	askPrice := marketPrice + adjustedSpreadStroops/2

	return bidPrice, askPrice
}

func (ms *MarketMakerService) CreateOrder(order *models.MarketMakerOrder) error {
	if order.BotID == 0 || order.AssetID == 0 || order.AmountUnits <= 0 {
		return errors.New("invalid order parameters")
	}
	if order.OrderType != "buy" && order.OrderType != "sell" {
		return errors.New("order type must be buy or sell")
	}
	return ms.db.Create(order).Error
}

func (ms *MarketMakerService) GetActiveOrders(botID uint) ([]models.MarketMakerOrder, error) {
	var orders []models.MarketMakerOrder
	if err := ms.db.Where("bot_id = ? AND status = ?", botID, "active").
		Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

func (ms *MarketMakerService) CancelOrder(orderID uint) error {
	return ms.db.Model(&models.MarketMakerOrder{}).Where("id = ?", orderID).Update("status", "cancelled").Error
}

func (ms *MarketMakerService) FillOrder(orderID uint, filledUnits int64) error {
	var order models.MarketMakerOrder
	if err := ms.db.First(&order, orderID).Error; err != nil {
		return err
	}

	if order.FilledUnits+filledUnits > order.AmountUnits {
		return errors.New("fill amount exceeds order amount")
	}

	newFilledUnits := order.FilledUnits + filledUnits
	status := "active"
	if newFilledUnits == order.AmountUnits {
		status = "filled"
	}

	return ms.db.Model(&models.MarketMakerOrder{}).
		Where("id = ?", orderID).
		Updates(map[string]interface{}{
			"filled_units": newFilledUnits,
			"status":       status,
		}).Error
}

func (ms *MarketMakerService) RecordTrade(trade *models.MarketMakerTrade) error {
	if trade.BotID == 0 || trade.OrderID == 0 || trade.AmountUnits <= 0 {
		return errors.New("invalid trade parameters")
	}
	return ms.db.Create(trade).Error
}

func (ms *MarketMakerService) UpdateInventory(botID, assetID uint, unitsChange int64, costBasisChange int64) error {
	var inv models.MarketMakerInventory
	err := ms.db.Where("bot_id = ? AND asset_id = ?", botID, assetID).First(&inv).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		inv = models.MarketMakerInventory{
			BotID:            botID,
			AssetID:          assetID,
			HeldUnits:        unitsChange,
			CostBasisStroops: costBasisChange,
			UpdatedAt:        time.Now(),
		}
		return ms.db.Create(&inv).Error
	}

	if err != nil {
		return err
	}

	newHeldUnits := inv.HeldUnits + unitsChange
	if newHeldUnits < 0 {
		newHeldUnits = 0
	}

	return ms.db.Model(&inv).Updates(map[string]interface{}{
		"held_units":         newHeldUnits,
		"cost_basis_stroops": inv.CostBasisStroops + costBasisChange,
		"updated_at":         time.Now(),
	}).Error
}

func (ms *MarketMakerService) GetInventory(botID uint) ([]models.MarketMakerInventory, error) {
	var inv []models.MarketMakerInventory
	if err := ms.db.Where("bot_id = ?", botID).
		Preload("Asset").
		Find(&inv).Error; err != nil {
		return nil, err
	}
	return inv, nil
}

func (ms *MarketMakerService) CheckPositionLimit(botID uint) (bool, error) {
	var bot models.MarketMakerBot
	if err := ms.db.First(&bot, botID).Error; err != nil {
		return false, err
	}

	var totalPosition int64
	if err := ms.db.Model(&models.MarketMakerInventory{}).
		Where("bot_id = ?", botID).
		Select("SUM(held_units)").
		Row().Scan(&totalPosition); err != nil {
		return false, err
	}

	if totalPosition*100 > bot.MaxPositionStroops {
		return false, nil
	}
	return true, nil
}

func (ms *MarketMakerService) RecordHealthCheck(botID uint, isHealthy bool, uptime float64, activeOrders int64) error {
	var lastTrade time.Time
	ms.db.Model(&models.MarketMakerTrade{}).
		Where("bot_id = ?", botID).
		Order("created_at DESC").
		Limit(1).
		Pluck("created_at", &lastTrade)

	check := &models.MarketMakerHealthCheck{
		BotID:             botID,
		IsHealthy:         isHealthy,
		UptimePercentage:  uptime,
		LastTradeTime:     lastTrade,
		ActiveOrdersCount: activeOrders,
		CheckedAt:         time.Now(),
	}

	return ms.db.Create(check).Error
}

func (ms *MarketMakerService) GetHealthChecks(botID uint, limit int) ([]models.MarketMakerHealthCheck, error) {
	var checks []models.MarketMakerHealthCheck
	if err := ms.db.Where("bot_id = ?", botID).
		Order("checked_at DESC").
		Limit(limit).
		Find(&checks).Error; err != nil {
		return nil, err
	}
	return checks, nil
}

func (ms *MarketMakerService) GetBotStats(botID uint) (map[string]interface{}, error) {
	var bot models.MarketMakerBot
	if err := ms.db.First(&bot, botID).Error; err != nil {
		return nil, err
	}

	var tradeCount int64
	ms.db.Model(&models.MarketMakerTrade{}).Where("bot_id = ?", botID).Count(&tradeCount)

	var totalProfit int64
	ms.db.Model(&models.MarketMakerTrade{}).
		Where("bot_id = ?", botID).
		Select("SUM(profit_stroops)").
		Row().Scan(&totalProfit)

	var avgPrice float64
	ms.db.Model(&models.MarketMakerTrade{}).
		Where("bot_id = ?", botID).
		Select("AVG(price_stroops)").
		Row().Scan(&avgPrice)

	return map[string]interface{}{
		"bot_id":       botID,
		"status":       bot.Status,
		"total_volume": bot.TotalVolumeStroops,
		"profit_loss":  bot.ProfitLossStroops,
		"trade_count":  tradeCount,
		"avg_price":    avgPrice,
		"uptime":       "N/A",
	}, nil
}
