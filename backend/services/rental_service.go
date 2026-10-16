package services

import (
	"fmt"
	"log"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

type RentalService struct {
	db *gorm.DB
}

func NewRentalService(db *gorm.DB) *RentalService {
	return &RentalService{db: db}
}

func (s *RentalService) CreateRental(lessorID, lesseeID, assetID uint, period models.RentalPeriod, rateAmount, securityDeposit int64, startDate, endDate time.Time, autoRenew bool, lateFeePercent float64, terms string) (*models.Rental, error) {
	var asset models.Asset
	if err := s.db.First(&asset, assetID).Error; err != nil {
		return nil, fmt.Errorf("asset not found: %w", err)
	}

	if startDate.After(endDate) {
		return nil, fmt.Errorf("start date must be before end date")
	}

	rental := models.Rental{
		AssetID:         assetID,
		LessorID:        lessorID,
		LesseeID:        lesseeID,
		Period:          period,
		RateAmount:      rateAmount,
		SecurityDeposit: securityDeposit,
		StartDate:       startDate,
		EndDate:         endDate,
		Status:          models.RentalActive,
		AutoRenew:       autoRenew,
		LateFeePercent:  lateFeePercent,
		Terms:           terms,
	}

	if err := s.db.Create(&rental).Error; err != nil {
		return nil, fmt.Errorf("failed to create rental: %w", err)
	}

	s.logHistory(rental.ID, "created", "Rental agreement created")

	s.generatePaymentSchedule(&rental)

	return &rental, nil
}

func (s *RentalService) generatePaymentSchedule(rental *models.Rental) {
	var payments []models.RentalPayment
	current := rental.StartDate

	for current.Before(rental.EndDate) || current.Equal(rental.EndDate) {
		var nextDue time.Time
		switch rental.Period {
		case models.RentalDaily:
			nextDue = current.AddDate(0, 0, 1)
		case models.RentalWeekly:
			nextDue = current.AddDate(0, 0, 7)
		case models.RentalMonthly:
			nextDue = current.AddDate(0, 1, 0)
		default:
			nextDue = current.AddDate(0, 1, 0)
		}

		if nextDue.After(rental.EndDate) {
			nextDue = rental.EndDate
		}

		payment := models.RentalPayment{
			RentalID: rental.ID,
			Amount:   rental.RateAmount,
			Currency: rental.RateCurrency,
			DueDate:  nextDue,
			Status:   "pending",
		}
		payments = append(payments, payment)

		current = nextDue
	}

	if len(payments) > 0 {
		s.db.Create(&payments)
	}
}

func (s *RentalService) ProcessPayment(rentalID uint, amount int64, txHash string) (*models.RentalPayment, error) {
	var rental models.Rental
	if err := s.db.First(&rental, rentalID).Error; err != nil {
		return nil, fmt.Errorf("rental not found: %w", err)
	}

	var pendingPayment models.RentalPayment
	if err := s.db.Where("rental_id = ? AND status = 'pending'", rentalID).
		Order("due_date ASC").First(&pendingPayment).Error; err != nil {
		return nil, fmt.Errorf("no pending payment found")
	}

	now := time.Now()
	lateFee := int64(0)
	if now.After(pendingPayment.DueDate) {
		daysLate := int(now.Sub(pendingPayment.DueDate).Hours() / 24)
		if daysLate > 0 {
			lateFee = int64(float64(pendingPayment.Amount) * rental.LateFeePercent / 100.0 * float64(daysLate))
		}
	}

	totalDue := pendingPayment.Amount + lateFee
	if amount < totalDue {
		return nil, fmt.Errorf("insufficient payment: got %d, need %d (includes %d late fee)", amount, totalDue, lateFee)
	}

	pendingPayment.Status = "paid"
	pendingPayment.PaidAt = &now
	pendingPayment.TransactionHash = txHash
	pendingPayment.LateFee = lateFee
	if err := s.db.Save(&pendingPayment).Error; err != nil {
		return nil, fmt.Errorf("failed to record payment: %w", err)
	}

	s.logHistory(rentalID, "payment", fmt.Sprintf("Payment of %d received (late fee: %d)", amount, lateFee))

	return &pendingPayment, nil
}

func (s *RentalService) CancelRental(rentalID, userID uint) error {
	var rental models.Rental
	if err := s.db.First(&rental, rentalID).Error; err != nil {
		return fmt.Errorf("rental not found: %w", err)
	}

	if rental.LessorID != userID && rental.LesseeID != userID {
		return fmt.Errorf("unauthorized: only parties to the rental can cancel")
	}

	if rental.Status != models.RentalActive {
		return fmt.Errorf("rental is not active")
	}

	rental.Status = models.RentalCancelled
	s.db.Save(&rental)
	s.logHistory(rentalID, "cancelled", "Rental agreement cancelled")
	return nil
}

func (s *RentalService) CompleteRental(rentalID uint) error {
	var rental models.Rental
	if err := s.db.First(&rental, rentalID).Error; err != nil {
		return fmt.Errorf("rental not found: %w", err)
	}

	var pendingCount int64
	s.db.Model(&models.RentalPayment{}).Where("rental_id = ? AND status = 'pending'", rentalID).Count(&pendingCount)
	if pendingCount > 0 {
		return fmt.Errorf("cannot complete rental with %d pending payments", pendingCount)
	}

	rental.Status = models.RentalCompleted
	s.db.Save(&rental)
	s.logHistory(rentalID, "completed", "Rental agreement completed")
	return nil
}

func (s *RentalService) SignAgreement(rentalID, userID uint) error {
	var rental models.Rental
	if err := s.db.First(&rental, rentalID).Error; err != nil {
		return fmt.Errorf("rental not found: %w", err)
	}

	switch userID {
	case rental.LessorID:
		rental.SignedByLessor = true
	case rental.LesseeID:
		rental.SignedByLessee = true
	default:
		return fmt.Errorf("unauthorized: not a party to this rental")
	}

	s.db.Save(&rental)
	s.logHistory(rentalID, "signed", fmt.Sprintf("User %d signed the agreement", userID))
	return nil
}

func (s *RentalService) DisputeRental(rentalID, userID uint, reason string) error {
	var rental models.Rental
	if err := s.db.First(&rental, rentalID).Error; err != nil {
		return fmt.Errorf("rental not found: %w", err)
	}

	rental.Status = models.RentalDisputed
	s.db.Save(&rental)
	s.logHistory(rentalID, "disputed", fmt.Sprintf("User %d disputed: %s", userID, reason))
	return nil
}

func (s *RentalService) CheckOverdueRentals() {
	var activeRentals []models.Rental
	s.db.Where("status = ?", models.RentalActive).Find(&activeRentals)

	now := time.Now()
	for _, rental := range activeRentals {
		var overduePayment models.RentalPayment
		if err := s.db.Where("rental_id = ? AND status = 'pending' AND due_date < ?", rental.ID, now).
			First(&overduePayment).Error; err != nil {
			continue
		}

		rental.Status = models.RentalOverdue
		s.db.Save(&rental)
		s.logHistory(rental.ID, "overdue", "Rental marked as overdue due to missed payments")
		log.Printf("[rental] Rental %d marked overdue", rental.ID)
	}
}

func (s *RentalService) AutoRenewRentals() {
	var expiringRentals []models.Rental
	s.db.Where("status = ? AND auto_renew = ? AND end_date <= ? AND end_date > ?",
		models.RentalActive, true, time.Now().Add(24*time.Hour), time.Now()).Find(&expiringRentals)

	for _, rental := range expiringRentals {
		var nextPeriod time.Time
		switch rental.Period {
		case models.RentalDaily:
			nextPeriod = rental.EndDate.AddDate(0, 0, 1)
		case models.RentalWeekly:
			nextPeriod = rental.EndDate.AddDate(0, 0, 7)
		case models.RentalMonthly:
			nextPeriod = rental.EndDate.AddDate(0, 1, 0)
		default:
			nextPeriod = rental.EndDate.AddDate(0, 1, 0)
		}

		oldEnd := rental.EndDate
		rental.StartDate = rental.EndDate
		rental.EndDate = nextPeriod
		s.db.Save(&rental)

		s.generatePaymentSchedule(&rental)
		s.logHistory(rental.ID, "auto_renewed", fmt.Sprintf("Rental auto-renewed from %s to %s", oldEnd.Format("2006-01-02"), nextPeriod.Format("2006-01-02")))
		log.Printf("[rental] Rental %d auto-renewed", rental.ID)
	}
}

func (s *RentalService) GetRentalHistory(rentalID uint) ([]models.RentalHistory, error) {
	var history []models.RentalHistory
	s.db.Where("rental_id = ?", rentalID).Order("created_at ASC").Find(&history)
	return history, nil
}

func (s *RentalService) GetPaymentSchedule(rentalID uint) ([]models.RentalPayment, error) {
	var payments []models.RentalPayment
	s.db.Where("rental_id = ?", rentalID).Order("due_date ASC").Find(&payments)
	return payments, nil
}

func (s *RentalService) logHistory(rentalID uint, event, detail string) {
	entry := models.RentalHistory{
		RentalID: rentalID,
		Event:    event,
		Detail:   detail,
	}
	s.db.Create(&entry)
}
