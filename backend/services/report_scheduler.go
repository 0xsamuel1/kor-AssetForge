package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

const (
	defaultReportHour          = 9
	defaultReportRetentionDays = 90
	maxReportRetentionDays     = 365
)

var supportedReportTypes = map[string]bool{
	"platform_summary": true,
	"user_activity":    true,
	"assets":           true,
	"transactions":     true,
	"tax_summary":      true,
}

type ReportSchedulerService struct {
	db         *gorm.DB
	email      EmailService
	cron       *cron.Cron
	httpClient *http.Client
	mu         sync.Mutex
	entries    map[uint]cron.EntryID
}

func NewReportSchedulerService(db *gorm.DB, email EmailService) *ReportSchedulerService {
	return &ReportSchedulerService{
		db:    db,
		email: email,
		cron: cron.New(cron.WithParser(cron.NewParser(
			cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		))),
		httpClient: &http.Client{Timeout: 15 * time.Second},
		entries:    make(map[uint]cron.EntryID),
	}
}

func (s *ReportSchedulerService) Start(ctx context.Context) error {
	var schedules []models.ScheduledReport
	if err := s.db.Where("status = ?", models.ScheduledReportStatusActive).Find(&schedules).Error; err != nil {
		return err
	}

	for i := range schedules {
		if err := s.scheduleLocked(&schedules[i]); err != nil {
			log.Printf("report scheduler failed to register schedule %d: %v", schedules[i].ID, err)
		}
	}

	s.cron.Start()
	go func() {
		<-ctx.Done()
		stopCtx := s.cron.Stop()
		<-stopCtx.Done()
	}()

	return nil
}

func (s *ReportSchedulerService) CreateSchedule(schedule *models.ScheduledReport) error {
	if err := normalizeSchedule(schedule); err != nil {
		return err
	}
	if err := s.db.Create(schedule).Error; err != nil {
		return err
	}
	if schedule.Status == models.ScheduledReportStatusActive {
		return s.RegisterSchedule(schedule)
	}
	return nil
}

func (s *ReportSchedulerService) UpdateSchedule(schedule *models.ScheduledReport) error {
	if err := normalizeSchedule(schedule); err != nil {
		return err
	}
	if err := s.db.Save(schedule).Error; err != nil {
		return err
	}
	s.UnregisterSchedule(schedule.ID)
	if schedule.Status == models.ScheduledReportStatusActive {
		return s.RegisterSchedule(schedule)
	}
	return nil
}

func (s *ReportSchedulerService) DeleteSchedule(userID, scheduleID uint) error {
	result := s.db.Where("user_id = ?", userID).Delete(&models.ScheduledReport{}, scheduleID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	s.UnregisterSchedule(scheduleID)
	return nil
}

func (s *ReportSchedulerService) RegisterSchedule(schedule *models.ScheduledReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scheduleLocked(schedule)
}

func (s *ReportSchedulerService) UnregisterSchedule(scheduleID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[scheduleID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, scheduleID)
	}
}

func (s *ReportSchedulerService) RunScheduleNow(scheduleID uint) (*models.ReportDeliveryHistory, error) {
	var schedule models.ScheduledReport
	if err := s.db.First(&schedule, scheduleID).Error; err != nil {
		return nil, err
	}
	return s.runSchedule(&schedule)
}

func (s *ReportSchedulerService) ListSchedules(userID uint) ([]models.ScheduledReport, error) {
	var schedules []models.ScheduledReport
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&schedules).Error
	return schedules, err
}

func (s *ReportSchedulerService) GetSchedule(userID, scheduleID uint) (*models.ScheduledReport, error) {
	var schedule models.ScheduledReport
	if err := s.db.Where("user_id = ?", userID).First(&schedule, scheduleID).Error; err != nil {
		return nil, err
	}
	return &schedule, nil
}

func (s *ReportSchedulerService) ListHistory(userID, scheduleID uint, limit int) ([]models.ReportDeliveryHistory, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var history []models.ReportDeliveryHistory
	q := s.db.Where("user_id = ?", userID)
	if scheduleID != 0 {
		q = q.Where("scheduled_report_id = ?", scheduleID)
	}
	err := q.Order("created_at DESC").Limit(limit).Find(&history).Error
	return history, err
}

func (s *ReportSchedulerService) scheduleLocked(schedule *models.ScheduledReport) error {
	if entryID, ok := s.entries[schedule.ID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, schedule.ID)
	}
	entryID, err := s.cron.AddFunc(schedule.CronExpression, func() {
		var fresh models.ScheduledReport
		if err := s.db.First(&fresh, schedule.ID).Error; err != nil {
			log.Printf("report scheduler failed to load schedule %d: %v", schedule.ID, err)
			return
		}
		if fresh.Status != models.ScheduledReportStatusActive {
			return
		}
		if _, err := s.runSchedule(&fresh); err != nil {
			log.Printf("report scheduler failed to run schedule %d: %v", schedule.ID, err)
		}
	})
	if err != nil {
		return err
	}
	s.entries[schedule.ID] = entryID
	next := s.cron.Entry(entryID).Next
	if !next.IsZero() {
		s.db.Model(schedule).Update("next_run_at", next)
		schedule.NextRunAt = &next
	}
	return nil
}

func (s *ReportSchedulerService) runSchedule(schedule *models.ScheduledReport) (*models.ReportDeliveryHistory, error) {
	payload, err := s.generateReportPayload(schedule)
	if err != nil {
		return nil, err
	}

	rendered, err := renderReport(schedule.Format, payload)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	fileName := fmt.Sprintf("scheduled_report_%d_%s.%s", schedule.ID, now.Format("20060102T150405"), schedule.Format)
	history := &models.ReportDeliveryHistory{
		ScheduledReportID: schedule.ID,
		UserID:            schedule.UserID,
		ReportType:        schedule.ReportType,
		Format:            schedule.Format,
		DeliveryMethod:    schedule.DeliveryMethod,
		Status:            models.ReportDeliveryStatusPending,
		FileName:          fileName,
		FilePath:          fmt.Sprintf("/reports/%d/%s", schedule.ID, fileName),
		Payload:           string(rendered),
		GeneratedAt:       now,
	}
	if err := s.db.Create(history).Error; err != nil {
		return nil, err
	}

	deliveredAt, deliveryErr := s.deliver(schedule, history, rendered)
	updates := map[string]interface{}{
		"last_run_at": now,
	}
	if deliveryErr != nil {
		history.Status = models.ReportDeliveryStatusFailed
		history.ErrorMessage = deliveryErr.Error()
	} else {
		history.Status = models.ReportDeliveryStatusSuccess
		history.DeliveredAt = &deliveredAt
	}

	if err := s.db.Save(history).Error; err != nil {
		return nil, err
	}
	if entryID, ok := s.entries[schedule.ID]; ok {
		next := s.cron.Entry(entryID).Next
		if !next.IsZero() {
			updates["next_run_at"] = next
		}
	}
	s.db.Model(schedule).Updates(updates)
	s.cleanupHistory(schedule)

	if deliveryErr != nil {
		return history, deliveryErr
	}
	return history, nil
}

func (s *ReportSchedulerService) deliver(schedule *models.ScheduledReport, history *models.ReportDeliveryHistory, rendered []byte) (time.Time, error) {
	switch schedule.DeliveryMethod {
	case models.ReportDeliveryEmail:
		if s.email == nil {
			return time.Time{}, errors.New("email service is not configured")
		}
		recipients := splitRecipients(schedule.EmailRecipients)
		if len(recipients) == 0 {
			return time.Time{}, errors.New("email delivery requires at least one recipient")
		}
		if err := s.email.SendScheduledReportEmail(recipients, schedule.Name, string(schedule.Format), history.FileName, string(rendered)); err != nil {
			return time.Time{}, err
		}
	case models.ReportDeliveryWebhook:
		if strings.TrimSpace(schedule.WebhookURL) == "" {
			return time.Time{}, errors.New("webhook delivery requires webhook_url")
		}
		body, _ := json.Marshal(map[string]interface{}{
			"schedule_id":  schedule.ID,
			"report_type":  schedule.ReportType,
			"format":       schedule.Format,
			"file_name":    history.FileName,
			"payload":      string(rendered),
			"generated_at": history.GeneratedAt,
		})
		req, err := http.NewRequest(http.MethodPost, schedule.WebhookURL, bytes.NewReader(body))
		if err != nil {
			return time.Time{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-AssetForge-Report-Schedule", fmt.Sprintf("%d", schedule.ID))
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return time.Time{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return time.Time{}, fmt.Errorf("webhook responded with %d: %s", resp.StatusCode, string(respBody))
		}
	default:
		return time.Time{}, fmt.Errorf("unsupported delivery method %q", schedule.DeliveryMethod)
	}
	return time.Now().UTC(), nil
}

func (s *ReportSchedulerService) generateReportPayload(schedule *models.ScheduledReport) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"schedule_id":  schedule.ID,
		"name":         schedule.Name,
		"report_type":  schedule.ReportType,
		"generated_at": time.Now().UTC(),
	}

	switch schedule.ReportType {
	case "platform_summary":
		var totalUsers, totalAssets, activeListings, transactions int64
		s.db.Table("users").Where("deleted_at IS NULL").Count(&totalUsers)
		s.db.Table("assets").Where("deleted_at IS NULL").Count(&totalAssets)
		s.db.Table("listings").Where("status = ?", "active").Count(&activeListings)
		s.db.Table("transactions").Count(&transactions)
		payload["metrics"] = map[string]int64{
			"total_users":     totalUsers,
			"total_assets":    totalAssets,
			"active_listings": activeListings,
			"transactions":    transactions,
		}
	case "assets":
		var count int64
		s.db.Table("assets").Where("deleted_at IS NULL").Count(&count)
		payload["asset_count"] = count
	case "transactions":
		var count int64
		s.db.Table("transactions").Count(&count)
		payload["transaction_count"] = count
	case "user_activity":
		var count int64
		s.db.Table("user_activities").Where("user_id = ?", schedule.UserID).Count(&count)
		payload["activity_count"] = count
	case "tax_summary":
		year := time.Now().UTC().Year()
		taxService := NewTaxService(s.db)
		summary, err := taxService.GetTaxSummary(schedule.UserID, time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC), time.Now().UTC())
		if err != nil {
			return nil, err
		}
		payload["tax_year"] = year
		payload["summary"] = summary
	default:
		return nil, fmt.Errorf("unsupported report type %q", schedule.ReportType)
	}

	return payload, nil
}

func (s *ReportSchedulerService) cleanupHistory(schedule *models.ScheduledReport) {
	retention := schedule.RetentionDays
	if retention <= 0 {
		retention = defaultReportRetentionDays
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retention)
	s.db.Where("scheduled_report_id = ? AND created_at < ?", schedule.ID, cutoff).Delete(&models.ReportDeliveryHistory{})
}

func normalizeSchedule(schedule *models.ScheduledReport) error {
	schedule.Name = strings.TrimSpace(schedule.Name)
	schedule.ReportType = strings.TrimSpace(schedule.ReportType)
	schedule.Timezone = strings.TrimSpace(schedule.Timezone)
	schedule.EmailRecipients = strings.TrimSpace(schedule.EmailRecipients)
	schedule.WebhookURL = strings.TrimSpace(schedule.WebhookURL)
	if schedule.Name == "" {
		return errors.New("name is required")
	}
	if !supportedReportTypes[schedule.ReportType] {
		return fmt.Errorf("unsupported report type %q", schedule.ReportType)
	}
	if schedule.Timezone == "" {
		schedule.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(schedule.Timezone); err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	if schedule.RetentionDays <= 0 {
		schedule.RetentionDays = defaultReportRetentionDays
	}
	if schedule.RetentionDays > maxReportRetentionDays {
		return fmt.Errorf("retention_days cannot exceed %d", maxReportRetentionDays)
	}
	if schedule.Status == "" {
		schedule.Status = models.ScheduledReportStatusActive
	}
	if err := validateScheduleEnums(schedule); err != nil {
		return err
	}
	if schedule.CronExpression == "" {
		schedule.CronExpression = cronExpressionForFrequency(schedule.Frequency, schedule.Timezone)
	}
	if _, err := cron.ParseStandard(schedule.CronExpression); err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}
	if schedule.DeliveryMethod == models.ReportDeliveryEmail && len(splitRecipients(schedule.EmailRecipients)) == 0 {
		return errors.New("email delivery requires email_recipients")
	}
	if schedule.DeliveryMethod == models.ReportDeliveryWebhook && schedule.WebhookURL == "" {
		return errors.New("webhook delivery requires webhook_url")
	}
	if strings.TrimSpace(schedule.Filters) == "" {
		schedule.Filters = "{}"
	}
	if !json.Valid([]byte(schedule.Filters)) {
		return errors.New("filters must be valid JSON")
	}
	return nil
}

func validateScheduleEnums(schedule *models.ScheduledReport) error {
	switch schedule.Frequency {
	case models.ReportFrequencyDaily, models.ReportFrequencyWeekly, models.ReportFrequencyMonthly:
	default:
		return fmt.Errorf("unsupported frequency %q", schedule.Frequency)
	}
	switch schedule.Format {
	case models.ReportFormatPDF, models.ReportFormatCSV, models.ReportFormatExcel:
	default:
		return fmt.Errorf("unsupported format %q", schedule.Format)
	}
	switch schedule.DeliveryMethod {
	case models.ReportDeliveryEmail, models.ReportDeliveryWebhook:
	default:
		return fmt.Errorf("unsupported delivery method %q", schedule.DeliveryMethod)
	}
	switch schedule.Status {
	case models.ScheduledReportStatusActive, models.ScheduledReportStatusPaused, models.ScheduledReportStatusDisabled:
	default:
		return fmt.Errorf("unsupported status %q", schedule.Status)
	}
	return nil
}

func cronExpressionForFrequency(freq models.ReportFrequency, timezone string) string {
	prefix := fmt.Sprintf("CRON_TZ=%s ", timezone)
	switch freq {
	case models.ReportFrequencyWeekly:
		return prefix + fmt.Sprintf("0 %d * * 1", defaultReportHour)
	case models.ReportFrequencyMonthly:
		return prefix + fmt.Sprintf("0 %d 1 * *", defaultReportHour)
	default:
		return prefix + fmt.Sprintf("0 %d * * *", defaultReportHour)
	}
}

func renderReport(format models.ReportFormat, payload map[string]interface{}) ([]byte, error) {
	switch format {
	case models.ReportFormatCSV:
		var buf bytes.Buffer
		writer := csv.NewWriter(&buf)
		_ = writer.Write([]string{"key", "value"})
		writeCSVRows(writer, "", payload)
		writer.Flush()
		return buf.Bytes(), writer.Error()
	case models.ReportFormatExcel:
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, err
		}
		return append([]byte("AssetForge Excel-compatible report\n"), raw...), nil
	case models.ReportFormatPDF:
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, err
		}
		return append([]byte("AssetForge PDF report\n"), raw...), nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func writeCSVRows(writer *csv.Writer, prefix string, value interface{}) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for k, v := range typed {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			writeCSVRows(writer, key, v)
		}
	case map[string]int64:
		for k, v := range typed {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			_ = writer.Write([]string{key, fmt.Sprintf("%d", v)})
		}
	default:
		_ = writer.Write([]string{prefix, fmt.Sprintf("%v", typed)})
	}
}

func splitRecipients(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	recipients := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			recipients = append(recipients, trimmed)
		}
	}
	return recipients
}
