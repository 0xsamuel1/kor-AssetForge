package models

import "time"

type TaxRecord struct {
	ID                     uint        `gorm:"primaryKey" json:"id"`
	UserID                 uint        `gorm:"not null;index" json:"user_id"`
	User                   User        `gorm:"foreignKey:UserID" json:"user,omitempty"`
	TransactionID          uint        `gorm:"index" json:"transaction_id"`
	Transaction            Transaction `gorm:"foreignKey:TransactionID" json:"transaction,omitempty"`
	AssetID                uint        `gorm:"not null" json:"asset_id"`
	Asset                  Asset       `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	TransactionType        string      `gorm:"not null" json:"transaction_type"`
	Quantity               int64       `gorm:"not null" json:"quantity"`
	CostBasisStroops       int64       `gorm:"not null" json:"cost_basis_stroops"`
	SalePriceStroops       int64       `json:"sale_price_stroops"`
	CapitalGainLossStroops int64       `json:"capital_gain_loss_stroops"`
	TransactionDate        time.Time   `gorm:"not null" json:"transaction_date"`
	CreatedAt              time.Time   `json:"created_at"`
}

type TaxReport struct {
	ID                      uint        `gorm:"primaryKey" json:"id"`
	UserID                  uint        `gorm:"not null;index" json:"user_id"`
	User                    User        `gorm:"foreignKey:UserID" json:"user,omitempty"`
	TaxYear                 int         `gorm:"not null" json:"tax_year"`
	PeriodStart             time.Time   `gorm:"not null" json:"period_start"`
	PeriodEnd               time.Time   `gorm:"not null" json:"period_end"`
	TotalGainStroops        int64       `gorm:"default:0" json:"total_gain_stroops"`
	TotalLossStroops        int64       `gorm:"default:0" json:"total_loss_stroops"`
	NetGainLossStroops      int64       `gorm:"default:0" json:"net_gain_loss_stroops"`
	TotalIncomeStroops      int64       `gorm:"default:0" json:"total_income_stroops"`
	TotalWithheldTaxStroops int64       `gorm:"default:0" json:"total_withheld_tax_stroops"`
	TransactionCount        int64       `gorm:"default:0" json:"transaction_count"`
	ReportStatus            string      `gorm:"default:'draft'" json:"report_status"`
	GeneratedAt             time.Time   `json:"generated_at"`
	ExportedAt              time.Time   `json:"exported_at"`
	Records                 []TaxRecord `gorm:"foreignKey:UserID;references:UserID" json:"records,omitempty"`
	UniqueIndex             string      `gorm:"uniqueIndex:idx_user_tax_year"`
}

type Tax1099Form struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	UserID           uint      `gorm:"not null;index" json:"user_id"`
	User             User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	TaxReportID      uint      `gorm:"not null" json:"tax_report_id"`
	TaxReport        TaxReport `gorm:"foreignKey:TaxReportID" json:"tax_report,omitempty"`
	FormType         string    `gorm:"default:'1099-NEC'" json:"form_type"`
	FilerName        string    `gorm:"not null" json:"filer_name"`
	FilerTIN         string    `gorm:"not null" json:"filer_tin"`
	RecipientAddress string    `gorm:"type:text" json:"recipient_address"`
	TotalIncome      int64     `gorm:"not null" json:"total_income"`
	FormData         string    `gorm:"type:text" json:"form_data"`
	FormStatus       string    `gorm:"default:'draft'" json:"form_status"`
	CreatedAt        time.Time `json:"created_at"`
	SignedAt         time.Time `json:"signed_at"`
}

type TaxWithholding struct {
	ID                       uint        `gorm:"primaryKey" json:"id"`
	UserID                   uint        `gorm:"not null;index" json:"user_id"`
	User                     User        `gorm:"foreignKey:UserID" json:"user,omitempty"`
	TransactionID            uint        `gorm:"index" json:"transaction_id"`
	Transaction              Transaction `gorm:"foreignKey:TransactionID" json:"transaction,omitempty"`
	WithholdingAmountStroops int64       `gorm:"not null" json:"withholding_amount_stroops"`
	WithholdingRate          float64     `gorm:"not null" json:"withholding_rate"`
	WithholdingDate          time.Time   `json:"withholding_date"`
	Status                   string      `gorm:"default:'completed'" json:"status"`
}

type TaxExport struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"not null;index" json:"user_id"`
	User        User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	TaxReportID uint      `gorm:"index" json:"tax_report_id"`
	TaxReport   TaxReport `gorm:"foreignKey:TaxReportID" json:"tax_report,omitempty"`
	ExportType  string    `gorm:"not null" json:"export_type"`
	FileName    string    `gorm:"not null" json:"file_name"`
	FilePath    string    `gorm:"type:text" json:"file_path"`
	FileFormat  string    `gorm:"default:'PDF'" json:"file_format"`
	ExportedAt  time.Time `json:"exported_at"`
}
