package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

type TaxService struct {
	db *gorm.DB
}

func NewTaxService(db *gorm.DB) *TaxService {
	return &TaxService{db: db}
}

func (ts *TaxService) RecordTaxEvent(record *models.TaxRecord) error {
	if record.UserID == 0 || record.AssetID == 0 {
		return errors.New("user_id and asset_id are required")
	}
	return ts.db.Create(record).Error
}

func (ts *TaxService) CalculateCapitalGain(costBasis, salePrice, quantity int64) int64 {
	gain := (salePrice - costBasis) * quantity
	return gain
}

func (ts *TaxService) GenerateTaxReport(userID uint, taxYear int) (*models.TaxReport, error) {
	startDate := time.Date(taxYear, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(taxYear, 12, 31, 23, 59, 59, 999999999, time.UTC)

	var totalGain, totalLoss, totalIncome int64
	var recordCount int64

	ts.db.Model(&models.TaxRecord{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ? AND capital_gain_loss_stroops > ?",
			userID, startDate, endDate, 0).
		Select("SUM(capital_gain_loss_stroops)").
		Row().Scan(&totalGain)

	ts.db.Model(&models.TaxRecord{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ? AND capital_gain_loss_stroops < ?",
			userID, startDate, endDate, 0).
		Select("SUM(ABS(capital_gain_loss_stroops))").
		Row().Scan(&totalLoss)

	ts.db.Model(&models.TaxRecord{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?",
			userID, startDate, endDate).
		Count(&recordCount)

	ts.db.Model(&models.TaxWithholding{}).
		Where("user_id = ? AND withholding_date >= ? AND withholding_date <= ?",
			userID, startDate, endDate).
		Select("SUM(withholding_amount_stroops)").
		Row().Scan(&totalIncome)

	netGainLoss := totalGain - totalLoss

	report := &models.TaxReport{
		UserID:             userID,
		TaxYear:            taxYear,
		PeriodStart:        startDate,
		PeriodEnd:          endDate,
		TotalGainStroops:   totalGain,
		TotalLossStroops:   totalLoss,
		NetGainLossStroops: netGainLoss,
		TotalIncomeStroops: totalIncome,
		TransactionCount:   recordCount,
		ReportStatus:       "draft",
		GeneratedAt:        time.Now(),
	}

	if err := ts.db.Create(report).Error; err != nil {
		return nil, err
	}

	return report, nil
}

func (ts *TaxService) GetTaxReport(userID uint, taxYear int) (*models.TaxReport, error) {
	var report models.TaxReport
	if err := ts.db.Where("user_id = ? AND tax_year = ?", userID, taxYear).
		Preload("Records").
		First(&report).Error; err != nil {
		return nil, err
	}
	return &report, nil
}

func (ts *TaxService) ListTaxReports(userID uint) ([]models.TaxReport, error) {
	var reports []models.TaxReport
	if err := ts.db.Where("user_id = ?", userID).
		Order("tax_year DESC").
		Find(&reports).Error; err != nil {
		return nil, err
	}
	return reports, nil
}

func (ts *TaxService) Generate1099Form(reportID uint, userID uint) (*models.Tax1099Form, error) {
	var report models.TaxReport
	if err := ts.db.First(&report, reportID).Error; err != nil {
		return nil, err
	}

	var user models.User
	if err := ts.db.First(&user, userID).Error; err != nil {
		return nil, err
	}

	formData := map[string]interface{}{
		"box_1a": report.TotalIncomeStroops,
		"box_3":  report.NetGainLossStroops,
		"box_5":  report.TotalWithheldTaxStroops,
	}

	formDataJSON, _ := json.Marshal(formData)

	form := &models.Tax1099Form{
		UserID:           userID,
		TaxReportID:      reportID,
		FormType:         "1099-NEC",
		FilerName:        "kor-AssetForge Marketplace",
		FilerTIN:         "00-0000000",
		RecipientAddress: "See Tax Report",
		TotalIncome:      report.TotalIncomeStroops,
		FormData:         string(formDataJSON),
		FormStatus:       "draft",
		CreatedAt:        time.Now(),
	}

	if err := ts.db.Create(form).Error; err != nil {
		return nil, err
	}

	return form, nil
}

func (ts *TaxService) Get1099Forms(userID uint) ([]models.Tax1099Form, error) {
	var forms []models.Tax1099Form
	if err := ts.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&forms).Error; err != nil {
		return nil, err
	}
	return forms, nil
}

func (ts *TaxService) RecordWithholding(withholding *models.TaxWithholding) error {
	if withholding.UserID == 0 || withholding.WithholdingAmountStroops <= 0 {
		return errors.New("invalid withholding parameters")
	}
	if withholding.WithholdingDate.IsZero() {
		withholding.WithholdingDate = time.Now()
	}
	return ts.db.Create(withholding).Error
}

func (ts *TaxService) GetWithholdings(userID uint) ([]models.TaxWithholding, error) {
	var withholdings []models.TaxWithholding
	if err := ts.db.Where("user_id = ?", userID).
		Order("withholding_date DESC").
		Find(&withholdings).Error; err != nil {
		return nil, err
	}
	return withholdings, nil
}

func (ts *TaxService) ExportTaxReport(reportID uint, userID uint, format string) (*models.TaxExport, error) {
	var report models.TaxReport
	if err := ts.db.First(&report, reportID).Error; err != nil {
		return nil, fmt.Errorf("tax report not found")
	}

	fileName := fmt.Sprintf("tax_report_%d_%d.%s", userID, report.TaxYear, format)
	filePath := fmt.Sprintf("/exports/%s", fileName)

	export := &models.TaxExport{
		UserID:      userID,
		TaxReportID: reportID,
		ExportType:  "tax_report",
		FileName:    fileName,
		FilePath:    filePath,
		FileFormat:  format,
		ExportedAt:  time.Now(),
	}

	if err := ts.db.Create(export).Error; err != nil {
		return nil, err
	}

	if err := ts.db.Model(&report).Update("exported_at", time.Now()).Error; err != nil {
		return nil, err
	}

	return export, nil
}

func (ts *TaxService) GetTaxExports(userID uint) ([]models.TaxExport, error) {
	var exports []models.TaxExport
	if err := ts.db.Where("user_id = ?", userID).
		Order("exported_at DESC").
		Find(&exports).Error; err != nil {
		return nil, err
	}
	return exports, nil
}

func (ts *TaxService) UpdateTaxReportStatus(reportID uint, status string) error {
	return ts.db.Model(&models.TaxReport{}).Where("id = ?", reportID).Update("report_status", status).Error
}

func (ts *TaxService) GetTaxSummary(userID uint, startDate, endDate time.Time) (map[string]interface{}, error) {
	var totalGain, totalLoss, totalWithheld int64

	ts.db.Model(&models.TaxRecord{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ? AND capital_gain_loss_stroops > ?",
			userID, startDate, endDate, 0).
		Select("SUM(capital_gain_loss_stroops)").
		Row().Scan(&totalGain)

	ts.db.Model(&models.TaxRecord{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ? AND capital_gain_loss_stroops < ?",
			userID, startDate, endDate, 0).
		Select("SUM(ABS(capital_gain_loss_stroops))").
		Row().Scan(&totalLoss)

	ts.db.Model(&models.TaxWithholding{}).
		Where("user_id = ? AND withholding_date >= ? AND withholding_date <= ?",
			userID, startDate, endDate).
		Select("SUM(withholding_amount_stroops)").
		Row().Scan(&totalWithheld)

	netGainLoss := totalGain - totalLoss

	return map[string]interface{}{
		"total_gain_stroops":     totalGain,
		"total_loss_stroops":     totalLoss,
		"net_gain_loss_stroops":  netGainLoss,
		"total_withheld_stroops": totalWithheld,
	}, nil
}
