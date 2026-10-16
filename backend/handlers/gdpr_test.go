package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockEmailService struct {
	sentEmails map[string]string
}

func (m *mockEmailService) SendVerificationEmail(toEmail, toName, token string) error {
	return nil
}

func (m *mockEmailService) SendKYCStatusUpdate(toEmail, toName, status, notes string) error {
	return nil
}

func (m *mockEmailService) SendTransactionConfirmation(toEmail, toName, hash string, amount int64, assetID uint, from, to string) error {
	return nil
}

func (m *mockEmailService) SendGDPRDataExportLink(toEmail, toName, link string) error {
	m.sentEmails[toEmail] = link
	return nil
}

func (m *mockEmailService) SendApprovalPendingEmail(toEmail, toName string, requestID uint, expiresAt time.Time) error {
	return nil
}

func (m *mockEmailService) SendScheduledReportEmail(recipients []string, reportName, format, fileName, body string) error {
	return nil
}

func (m *mockEmailService) SendCustomEmail(toEmail, toName, subject, html, plainText string) error {
	return nil
}

func TestGDPRHandler_DataExport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create temp directory for export files
	tempDir, err := os.MkdirTemp("", "exports_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	os.Setenv("GDPR_EXPORT_STORAGE_DIR", tempDir)

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.User{},
		&models.ExportJob{},
		&models.UserBalance{},
		&models.Listing{},
		&models.Transaction{},
	)
	require.NoError(t, err)

	// Seed user data
	user := models.User{
		StellarAddress:     "G-STELLAR-ADDR",
		Email:              "test@example.com",
		Username:           "testuser",
		Role:               models.RoleUser,
		KYCVerified:        true,
		AccreditedInvestor: false,
	}
	db.Create(&user)

	emailMock := &mockEmailService{sentEmails: make(map[string]string)}
	exportSvc := services.NewDataExportService(db, emailMock)
	handler := NewGDPRHandler(db, exportSvc)

	t.Run("JSON Export Request & Download", func(t *testing.T) {
		r := gin.New()
		// Inject auth context mock
		r.Use(func(c *gin.Context) {
			c.Set("user_id", user.ID)
			c.Next()
		})
		r.POST("/gdpr/export", handler.RequestDataExport)
		r.GET("/gdpr/export/:id", handler.GetDataExportStatus)
		r.GET("/gdpr/download/:token", handler.DownloadExport)

		// 1. Request export
		w := httptest.NewRecorder()
		reqBody := `{"format":"json"}`
		req, _ := http.NewRequest("POST", "/gdpr/export", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)

		var requestResult map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &requestResult)
		require.NoError(t, err)
		jobID := uint(requestResult["job_id"].(float64))

		// 2. Wait for completion
		require.Eventually(t, func() bool {
			var job models.ExportJob
			db.First(&job, jobID)
			return job.Status == models.ExportJobStatusCompleted
		}, 3*time.Second, 100*time.Millisecond)

		// 3. Get status
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", fmt.Sprintf("/gdpr/export/%d", jobID), nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 4. Retrieve download token from email mock
		downloadLink, exists := emailMock.sentEmails[user.Email]
		require.True(t, exists)

		// Link format: http://localhost:8080/api/v1/legal/gdpr/export/download/<token>
		// Extract token
		var token string
		_, err = fmt.Sscanf(downloadLink, "http://localhost:8080/api/v1/legal/gdpr/export/download/%s", &token)
		if err != nil {
			// Try without full URL
			token = downloadLink[len(downloadLink)-64:]
		}

		// 5. Download and verify content
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/gdpr/download/"+token, nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var exportedData map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &exportedData)
		require.NoError(t, err)

		exportedUser := exportedData["user"].(map[string]interface{})
		assert.Equal(t, user.Email, exportedUser["email"])
	})

	t.Run("CSV Export Request & Download", func(t *testing.T) {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user_id", user.ID)
			c.Next()
		})
		r.POST("/gdpr/export", handler.RequestDataExport)
		r.GET("/gdpr/download/:token", handler.DownloadExport)

		// 1. Request export
		w := httptest.NewRecorder()
		reqBody := `{"format":"csv"}`
		req, _ := http.NewRequest("POST", "/gdpr/export", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)

		var requestResult map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &requestResult)
		require.NoError(t, err)
		jobID := uint(requestResult["job_id"].(float64))

		// 2. Wait for completion
		require.Eventually(t, func() bool {
			var job models.ExportJob
			db.First(&job, jobID)
			return job.Status == models.ExportJobStatusCompleted
		}, 3*time.Second, 100*time.Millisecond)

		// 3. Retrieve download link and token
		downloadLink := emailMock.sentEmails[user.Email]
		token := downloadLink[len(downloadLink)-64:]

		// 4. Download ZIP file
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/gdpr/download/"+token, nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))

		// 5. Verify ZIP structure
		zipReader, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
		require.NoError(t, err)

		var hasProfileCSV bool
		for _, file := range zipReader.File {
			if file.Name == "profile.csv" {
				hasProfileCSV = true
			}
		}
		assert.True(t, hasProfileCSV)
	})

	t.Run("Expired Link Denied", func(t *testing.T) {
		r := gin.New()
		r.GET("/gdpr/download/:token", handler.DownloadExport)

		// Setup job expired in past
		job := models.ExportJob{
			UserID:        user.ID,
			Status:        models.ExportJobStatusCompleted,
			Format:        "json",
			DownloadToken: "expired-token-12345",
			ExpiresAt:     time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
			FilePath:      filepath.Join(tempDir, "dummy.enc"),
		}
		db.Create(&job)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/gdpr/download/expired-token-12345", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
