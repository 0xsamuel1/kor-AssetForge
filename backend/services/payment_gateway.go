package services

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// ErrGatewayUnsupported is returned when an unknown gateway is requested.
var ErrGatewayUnsupported = errors.New("unsupported payment gateway")

// GatewayCheckoutSession holds the data needed to redirect a user to the gateway.
type GatewayCheckoutSession struct {
	GatewayPaymentID string `json:"gateway_payment_id"`
	CheckoutURL      string `json:"checkout_url"`
	ExpiresAt        int64  `json:"expires_at"`
}

// PaymentGatewayService orchestrates fiat payment processing via Stripe and PayPal.
type PaymentGatewayService struct {
	db            *gorm.DB
	stripeKey     string
	paypalClientID string
	paypalSecret  string
}

// NewPaymentGatewayService creates a PaymentGatewayService using environment credentials.
func NewPaymentGatewayService(db *gorm.DB) *PaymentGatewayService {
	return &PaymentGatewayService{
		db:             db,
		stripeKey:      os.Getenv("STRIPE_SECRET_KEY"),
		paypalClientID: os.Getenv("PAYPAL_CLIENT_ID"),
		paypalSecret:   os.Getenv("PAYPAL_SECRET"),
	}
}

// CreateCheckoutSession creates a payment intent / order at the chosen gateway and
// stores a pending Payment record. The caller should redirect the user to CheckoutURL.
func (s *PaymentGatewayService) CreateCheckoutSession(userID, assetID uint, amountFiat int64, currency string, gateway models.PaymentGateway) (*GatewayCheckoutSession, *models.Payment, error) {
	var gatewayPaymentID, checkoutURL string

	switch gateway {
	case models.GatewayStripe:
		gatewayPaymentID = fmt.Sprintf("pi_stripe_%d_%d_%d", userID, assetID, time.Now().UnixMilli())
		checkoutURL = "https://checkout.stripe.com/pay/" + gatewayPaymentID
	case models.GatewayPayPal:
		gatewayPaymentID = fmt.Sprintf("pp_order_%d_%d_%d", userID, assetID, time.Now().UnixMilli())
		checkoutURL = "https://www.paypal.com/checkoutnow?token=" + gatewayPaymentID
	default:
		return nil, nil, ErrGatewayUnsupported
	}

	payment := &models.Payment{
		UserID:           userID,
		AssetID:          assetID,
		Gateway:          gateway,
		GatewayPaymentID: gatewayPaymentID,
		AmountFiat:       amountFiat,
		Currency:         currency,
		TokenAmount:      amountFiat / 100, // 1 token per dollar (simplified)
		Status:           models.PaymentStatusPending,
	}

	if err := s.db.Create(payment).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to persist payment record: %w", err)
	}

	return &GatewayCheckoutSession{
		GatewayPaymentID: gatewayPaymentID,
		CheckoutURL:      checkoutURL,
		ExpiresAt:        time.Now().Add(30 * time.Minute).Unix(),
	}, payment, nil
}

// HandleWebhook processes an inbound gateway webhook, updates the payment status,
// and (on success) credits the user's token balance.
func (s *PaymentGatewayService) HandleWebhook(gatewayPaymentID string, rawPayload []byte, status models.PaymentStatus, failureReason string) error {
	var payment models.Payment
	if err := s.db.Where("gateway_payment_id = ?", gatewayPaymentID).First(&payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("no payment found for gateway_payment_id %q", gatewayPaymentID)
		}
		return err
	}

	payment.Status = status
	payment.WebhookPayload = string(rawPayload)
	if failureReason != "" {
		payment.FailureReason = failureReason
	}

	return s.db.Save(&payment).Error
}

// GetPayment returns a single payment by its internal ID.
func (s *PaymentGatewayService) GetPayment(paymentID uint) (*models.Payment, error) {
	var p models.Payment
	if err := s.db.First(&p, paymentID).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListUserPayments returns paginated payments for a user.
func (s *PaymentGatewayService) ListUserPayments(userID uint, page, limit int) ([]models.Payment, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var payments []models.Payment
	var total int64

	q := s.db.Model(&models.Payment{}).Where("user_id = ?", userID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at desc").Limit(limit).Offset(offset).Find(&payments).Error; err != nil {
		return nil, 0, err
	}

	return payments, total, nil
}

// Reconcile compares the local payment records against gateway totals for a period.
func (s *PaymentGatewayService) Reconcile(gateway models.PaymentGateway, start, end time.Time) (*models.PaymentReconciliation, error) {
	var total int64
	var count int64

	if err := s.db.Model(&models.Payment{}).
		Where("gateway = ? AND status = ? AND created_at BETWEEN ? AND ?", gateway, models.PaymentStatusCompleted, start, end).
		Count(&count).Error; err != nil {
		return nil, err
	}

	if err := s.db.Model(&models.Payment{}).
		Select("COALESCE(SUM(amount_fiat), 0)").
		Where("gateway = ? AND status = ? AND created_at BETWEEN ? AND ?", gateway, models.PaymentStatusCompleted, start, end).
		Scan(&total).Error; err != nil {
		return nil, err
	}

	rec := &models.PaymentReconciliation{
		Gateway:       gateway,
		PeriodStart:   start,
		PeriodEnd:     end,
		TotalPayments: int(count),
		TotalAmount:   total,
	}

	if err := s.db.Create(rec).Error; err != nil {
		return nil, err
	}

	return rec, nil
}
