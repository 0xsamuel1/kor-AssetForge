package services

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

type DataExportService struct {
	db           *gorm.DB
	emailService EmailService
	storageDir   string
	baseURL      string
}

// NewDataExportService creates a new GDPR data export service
func NewDataExportService(db *gorm.DB, emailService EmailService) *DataExportService {
	storageDir := os.Getenv("GDPR_EXPORT_STORAGE_DIR")
	if storageDir == "" {
		storageDir = filepath.Join(".", "storage", "exports")
	}

	baseURL := os.Getenv("APP_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	return &DataExportService{
		db:           db,
		emailService: emailService,
		storageDir:   storageDir,
		baseURL:      baseURL,
	}
}

// StartExportJob initiates an asynchronous data export job
func (s *DataExportService) StartExportJob(userID uint, format string) (*models.ExportJob, error) {
	if format != "json" && format != "csv" {
		return nil, errors.New("unsupported export format: must be 'json' or 'csv'")
	}

	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Generate unique download token
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)

	job := &models.ExportJob{
		UserID:        userID,
		Status:        models.ExportJobStatusPending,
		Format:        format,
		DownloadToken: token,
		ExpiresAt:     time.Now().Add(7 * 24 * time.Hour), // 7 days expiry
	}

	if err := s.db.Create(job).Error; err != nil {
		return nil, err
	}

	// Process asynchronously
	go s.processExport(job.ID)

	return job, nil
}

// GetDecryptedExport retrieves and decrypts the export file if valid and not expired
func (s *DataExportService) GetDecryptedExport(token string) ([]byte, string, error) {
	var job models.ExportJob
	if err := s.db.Preload("User").Where("download_token = ?", token).First(&job).Error; err != nil {
		return nil, "", errors.New("invalid or expired download link")
	}

	if time.Now().After(job.ExpiresAt) {
		return nil, "", errors.New("download link has expired (7 days limit)")
	}

	if job.Status != models.ExportJobStatusCompleted {
		return nil, "", fmt.Errorf("export job is in status: %s", job.Status)
	}

	// Read ciphertext from disk
	ciphertext, err := os.ReadFile(job.FilePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read export file: %w", err)
	}

	// Decrypt
	key := s.getEncryptionKey()
	plaintext, err := s.decrypt(ciphertext, key)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decrypt export data: %w", err)
	}

	filename := fmt.Sprintf("assetforge-data-export-%d.%s", job.UserID, job.Format)
	if job.Format == "csv" {
		filename += ".zip" // CSV format is packaged as a ZIP
	}

	return plaintext, filename, nil
}

func (s *DataExportService) processExport(jobID uint) {
	var job models.ExportJob
	if err := s.db.Preload("User").First(&job, jobID).Error; err != nil {
		log.Printf("GDPR Export Error: Failed to find job %d: %v", jobID, err)
		return
	}

	// Update status to processing
	s.db.Model(&job).Update("status", models.ExportJobStatusProcessing)

	// Gather user data
	data, err := s.gatherUserData(job.UserID)
	if err != nil {
		s.failJob(&job, fmt.Sprintf("failed to gather user data: %v", err))
		return
	}

	// Serialize
	var serialized []byte
	if job.Format == "json" {
		serialized, err = s.serializeJSON(data)
	} else {
		serialized, err = s.serializeCSVZip(data)
	}

	if err != nil {
		s.failJob(&job, fmt.Sprintf("failed to serialize data: %v", err))
		return
	}

	// Encrypt
	key := s.getEncryptionKey()
	ciphertext, err := s.encrypt(serialized, key)
	if err != nil {
		s.failJob(&job, fmt.Sprintf("failed to encrypt data: %v", err))
		return
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(s.storageDir, 0755); err != nil {
		s.failJob(&job, fmt.Sprintf("failed to create storage directory: %v", err))
		return
	}

	// Save ciphertext to disk
	filePath := filepath.Join(s.storageDir, fmt.Sprintf("export-%d-%s.enc", job.UserID, job.DownloadToken[:8]))
	if err := os.WriteFile(filePath, ciphertext, 0600); err != nil {
		s.failJob(&job, fmt.Sprintf("failed to write export file to disk: %v", err))
		return
	}

	// Update job state
	err = s.db.Model(&job).Updates(map[string]interface{}{
		"status":     models.ExportJobStatusCompleted,
		"file_path":  filePath,
		"updated_at": time.Now(),
	}).Error

	if err != nil {
		s.failJob(&job, fmt.Sprintf("failed to update job status: %v", err))
		return
	}

	// Send email with download link
	downloadLink := fmt.Sprintf("%s/api/v1/legal/gdpr/export/download/%s", s.baseURL, job.DownloadToken)
	toName := job.User.Username
	if toName == "" {
		toName = "User"
	}

	err = s.emailService.SendGDPRDataExportLink(job.User.Email, toName, downloadLink)
	if err != nil {
		log.Printf("GDPR Export Warning: Failed to send export ready email: %v", err)
	}
}

type exportPayload struct {
	User         models.User          `json:"user"`
	KYCRecord    *models.KYCRecord    `json:"kyc_record,omitempty"`
	Balances     []models.UserBalance `json:"balances"`
	Listings     []models.Listing     `json:"listings"`
	Transactions []models.Transaction `json:"transactions"`
}

func (s *DataExportService) gatherUserData(userID uint) (*exportPayload, error) {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, err
	}

	var kyc models.KYCRecord
	var kycPtr *models.KYCRecord
	if err := s.db.Where("user_id = ?", userID).First(&kyc).Error; err == nil {
		kycPtr = &kyc
	}

	var balances []models.UserBalance
	s.db.Where("user_id = ?", userID).Find(&balances)

	var listings []models.Listing
	s.db.Where("seller_address = ?", user.StellarAddress).Find(&listings)

	var transactions []models.Transaction
	s.db.Where("from_address = ? OR to_address = ?", user.StellarAddress, user.StellarAddress).Find(&transactions)

	return &exportPayload{
		User:         user,
		KYCRecord:    kycPtr,
		Balances:     balances,
		Listings:     listings,
		Transactions: transactions,
	}, nil
}

func (s *DataExportService) serializeJSON(payload *exportPayload) ([]byte, error) {
	return json.MarshalIndent(payload, "", "  ")
}

func (s *DataExportService) serializeCSVZip(payload *exportPayload) ([]byte, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// 1. Profile CSV
	fProfile, err := zipWriter.Create("profile.csv")
	if err != nil {
		return nil, err
	}
	wProfile := csv.NewWriter(fProfile)
	wProfile.Write([]string{"ID", "StellarAddress", "Email", "Username", "Role", "KYCVerified", "AccreditedInvestor", "CreatedAt"})
	wProfile.Write([]string{
		fmt.Sprintf("%d", payload.User.ID),
		payload.User.StellarAddress,
		payload.User.Email,
		payload.User.Username,
		string(payload.User.Role),
		fmt.Sprintf("%t", payload.User.KYCVerified),
		fmt.Sprintf("%t", payload.User.AccreditedInvestor),
		payload.User.CreatedAt.Format(time.RFC3339),
	})
	wProfile.Flush()

	// 2. Balances CSV
	fBalances, err := zipWriter.Create("balances.csv")
	if err != nil {
		return nil, err
	}
	wBalances := csv.NewWriter(fBalances)
	wBalances.Write([]string{"AssetID", "Balance", "LockedBalance", "UpdatedAt"})
	for _, b := range payload.Balances {
		wBalances.Write([]string{
			fmt.Sprintf("%d", b.AssetID),
			fmt.Sprintf("%d", b.Balance),
			fmt.Sprintf("%d", b.LockedBalance),
			b.UpdatedAt.Format(time.RFC3339),
		})
	}
	wBalances.Flush()

	// 3. Listings CSV
	fListings, err := zipWriter.Create("listings.csv")
	if err != nil {
		return nil, err
	}
	wListings := csv.NewWriter(fListings)
	wListings.Write([]string{"ListingID", "AssetID", "Amount", "PricePerUnit", "Active", "CreatedAt"})
	for _, l := range payload.Listings {
		wListings.Write([]string{
			l.ListingID,
			fmt.Sprintf("%d", l.AssetID),
			fmt.Sprintf("%d", l.Amount),
			fmt.Sprintf("%d", l.PricePerUnit),
			fmt.Sprintf("%t", l.Active),
			l.CreatedAt.Format(time.RFC3339),
		})
	}
	wListings.Flush()

	// 4. Transactions CSV
	fTx, err := zipWriter.Create("transactions.csv")
	if err != nil {
		return nil, err
	}
	wTx := csv.NewWriter(fTx)
	wTx.Write([]string{"TxHash", "AssetID", "Amount", "FromAddress", "ToAddress", "Status", "CreatedAt"})
	for _, tx := range payload.Transactions {
		wTx.Write([]string{
			tx.TxHash,
			fmt.Sprintf("%d", tx.AssetID),
			fmt.Sprintf("%d", tx.Amount),
			tx.FromAddress,
			tx.ToAddress,
			tx.Status,
			tx.CreatedAt.Format(time.RFC3339),
		})
	}
	wTx.Flush()

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *DataExportService) failJob(job *models.ExportJob, errMsg string) {
	log.Printf("GDPR Export Job %d failed: %s", job.ID, errMsg)
	s.db.Model(job).Updates(map[string]interface{}{
		"status":        models.ExportJobStatusFailed,
		"error_details": errMsg,
		"updated_at":    time.Now(),
	})
}

func (s *DataExportService) getEncryptionKey() []byte {
	secret := os.Getenv("GDPR_EXPORT_SECRET")
	if secret == "" {
		secret = "a-very-secure-32-byte-secret-key-fallback!"
	}
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}

func (s *DataExportService) encrypt(plaintext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func (s *DataExportService) decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, actualCiphertext, nil)
}
