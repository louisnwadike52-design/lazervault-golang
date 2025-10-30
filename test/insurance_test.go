package test

import (
	"context"
	"encoding"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lazervaultGo/configs"
	"lazervaultGo/database"
	"lazervaultGo/models"
	"lazervaultGo/services"
	"lazervaultGo/tasks"
)

// MockTaskDistributor implements tasks.TaskDistributor for testing
type MockTaskDistributor struct{}

func (m *MockTaskDistributor) DistributeTask(ctx context.Context, taskType string, payload []byte, opts ...asynq.Option) error {
	return nil
}

func (m *MockTaskDistributor) DistributeTaskSendVerifyEmail(ctx context.Context, payload *tasks.PayloadSendVerifyEmail, opts ...asynq.Option) error {
	return nil
}

func (m *MockTaskDistributor) DistributeTaskSendPasswordResetOTP(ctx context.Context, payload *tasks.PayloadSendPasswordResetOTP, opts ...asynq.Option) error {
	return nil
}

func (m *MockTaskDistributor) DistributeTaskDepositProcess(ctx context.Context, payload *tasks.DepositProcessPayload, opts ...asynq.Option) error {
	return nil
}

func (m *MockTaskDistributor) DistributeTaskWithdrawalProcess(ctx context.Context, payload *tasks.WithdrawalProcessPayload, opts ...asynq.Option) error {
	return nil
}

func (m *MockTaskDistributor) DistributeTaskSendWithdrawalConfirmation(ctx context.Context, payload *tasks.EmailSendWithdrawalConfirmationPayload, opts ...asynq.Option) error {
	return nil
}

func (m *MockTaskDistributor) DistributeTaskProcessTransfer(ctx context.Context, payload encoding.BinaryMarshaler, opts ...asynq.Option) error {
	return nil
}

func (m *MockTaskDistributor) DistributeTaskProcessExternalTransfer(ctx context.Context, payload encoding.BinaryMarshaler, opts ...asynq.Option) error {
	return nil
}

func TestInsuranceFlow(t *testing.T) {
	// Load test configuration
	config, err := configs.LoadConfig(".")
	require.NoError(t, err)

	// Connect to test database
	db, err := database.ConnectDB(config)
	require.NoError(t, err)

	// Run migrations
	migrator := database.NewMigrator(db)
	err = migrator.RunMigrations()
	require.NoError(t, err)

	// Create a mock task distributor
	taskDistributor := &MockTaskDistributor{}

	// Create insurance service
	insuranceService := services.NewInsuranceService(db, taskDistributor)

	// Create a test user
	user := &models.User{
		FirstName:   "John",
		LastName:    "Doe",
		Email:       "john.doe@test.com",
		PhoneNumber: "+1234567890",
		Role:        "user",
	}
	err = db.Create(user).Error
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("Create Insurance", func(t *testing.T) {
		// Create insurance policy
		insurance := &models.Insurance{
			ID:                uuid.New().String(),
			PolicyHolderName:  "John Doe",
			PolicyHolderEmail: "john.doe@test.com",
			PolicyHolderPhone: "+1234567890",
			Type:              "health",
			Provider:          "Test Insurance Co",
			ProviderLogo:      "https://example.com/logo.png",
			PremiumAmount:     299.99,
			CoverageAmount:    50000.00,
			Currency:          "USD",
			StartDate:         time.Now(),
			EndDate:           time.Now().AddDate(1, 0, 0),
			NextPaymentDate:   time.Now().AddDate(0, 1, 0),
			Status:            "active",
			UserID:            user.ID,
		}

		err := insuranceService.CreateInsurance(ctx, insurance)
		assert.NoError(t, err)
		assert.NotEmpty(t, insurance.ID)
		assert.NotEmpty(t, insurance.PolicyNumber)
		assert.True(t, len(insurance.PolicyNumber) > 0)
	})

	t.Run("Get User Insurances", func(t *testing.T) {
		insurances, pagination, err := insuranceService.GetUserInsurances(ctx, user.ID, 1, 10)
		assert.NoError(t, err)
		assert.Len(t, insurances, 1)
		assert.Equal(t, int32(1), pagination.TotalItems)
		assert.Equal(t, "health", insurances[0].Type)
		assert.Equal(t, "active", insurances[0].Status)
	})

	t.Run("Create Payment", func(t *testing.T) {
		// Get the insurance first
		insurances, _, err := insuranceService.GetUserInsurances(ctx, user.ID, 1, 10)
		require.NoError(t, err)
		require.Len(t, insurances, 1)

		insurance := insurances[0]

		// Create payment
		payment := &models.InsurancePayment{
			ID:            uuid.New().String(),
			InsuranceID:   insurance.ID,
			PolicyNumber:  insurance.PolicyNumber,
			Amount:        insurance.PremiumAmount,
			Currency:      insurance.Currency,
			PaymentMethod: "card",
			Status:        "pending",
			PaymentDate:   time.Now(),
			DueDate:       time.Now().AddDate(0, 1, 0),
			UserID:        user.ID,
		}

		err = insuranceService.CreatePayment(ctx, payment)
		assert.NoError(t, err)
		assert.NotEmpty(t, payment.ID)
	})

	t.Run("Process Payment", func(t *testing.T) {
		// Get payments
		payments, _, err := insuranceService.GetUserPayments(ctx, user.ID, 1, 10)
		require.NoError(t, err)
		require.Len(t, payments, 1)

		payment := payments[0]

		// Process payment
		processedPayment, err := insuranceService.ProcessPayment(ctx, payment.ID, "card", map[string]string{
			"card_number": "4111111111111111",
			"expiry":      "12/25",
		}, user.ID)

		assert.NoError(t, err)
		assert.Equal(t, "completed", processedPayment.Status)
		assert.NotEmpty(t, processedPayment.TransactionID)
		assert.NotEmpty(t, processedPayment.ReferenceNumber)
		assert.NotEmpty(t, processedPayment.ReceiptURL)
	})

	t.Run("Create Claim", func(t *testing.T) {
		// Get the insurance first
		insurances, _, err := insuranceService.GetUserInsurances(ctx, user.ID, 1, 10)
		require.NoError(t, err)
		require.Len(t, insurances, 1)

		insurance := insurances[0]

		// Create claim
		claim := &models.InsuranceClaim{
			ID:               uuid.New().String(),
			InsuranceID:      insurance.ID,
			PolicyNumber:     insurance.PolicyNumber,
			Type:             "medical",
			Status:           "submitted",
			Title:            "Medical Emergency",
			Description:      "Emergency room visit for chest pain",
			ClaimAmount:      5000.00,
			Currency:         "USD",
			IncidentDate:     time.Now().AddDate(0, 0, -1),
			IncidentLocation: "City General Hospital",
			UserID:           user.ID,
		}

		err = insuranceService.CreateClaim(ctx, claim)
		assert.NoError(t, err)
		assert.NotEmpty(t, claim.ID)
		assert.NotEmpty(t, claim.ClaimNumber)
	})

	t.Run("Get User Claims", func(t *testing.T) {
		claims, pagination, err := insuranceService.GetUserClaims(ctx, user.ID, 1, 10)
		assert.NoError(t, err)
		assert.Len(t, claims, 1)
		assert.Equal(t, int32(1), pagination.TotalItems)
		assert.Equal(t, "medical", claims[0].Type)
		assert.Equal(t, "submitted", claims[0].Status)
	})

	t.Run("Get Insurance Statistics", func(t *testing.T) {
		stats, err := insuranceService.GetInsuranceStatistics(ctx, user.ID)
		assert.NoError(t, err)
		assert.Equal(t, 1, stats.TotalPolicies)
		assert.Equal(t, 1, stats.ActivePolicies)
		assert.Equal(t, 0, stats.ExpiredPolicies)
		assert.Equal(t, 50000.00, stats.TotalCoverageAmount)
		assert.Equal(t, 299.99, stats.TotalPremiumAmount)
		assert.Equal(t, 1, stats.PoliciesByType["health"])
	})

	t.Run("Get Payment Statistics", func(t *testing.T) {
		stats, err := insuranceService.GetPaymentStatistics(ctx, user.ID, nil, nil)
		assert.NoError(t, err)
		assert.Equal(t, 1, stats.TotalPayments)
		assert.Equal(t, 1, stats.CompletedPayments)
		assert.Equal(t, 0, stats.PendingPayments)
		assert.Equal(t, 0, stats.FailedPayments)
		assert.Equal(t, 299.99, stats.TotalAmount)
		assert.Equal(t, 299.99, stats.CompletedAmount)
		assert.Equal(t, 1, stats.PaymentsByMethod["card"])
	})

	t.Run("Search Insurances", func(t *testing.T) {
		insurances, pagination, err := insuranceService.SearchInsurances(ctx, user.ID, "health", 1, 10)
		assert.NoError(t, err)
		assert.Len(t, insurances, 1)
		assert.Equal(t, int32(1), pagination.TotalItems)
		assert.Equal(t, "health", insurances[0].Type)
	})

	t.Run("Get Overdue Payments", func(t *testing.T) {
		// Create an overdue payment
		insurances, _, err := insuranceService.GetUserInsurances(ctx, user.ID, 1, 10)
		require.NoError(t, err)
		require.Len(t, insurances, 1)

		insurance := insurances[0]

		overduePayment := &models.InsurancePayment{
			ID:            uuid.New().String(),
			InsuranceID:   insurance.ID,
			PolicyNumber:  insurance.PolicyNumber,
			Amount:        insurance.PremiumAmount,
			Currency:      insurance.Currency,
			PaymentMethod: "card",
			Status:        "pending",
			PaymentDate:   time.Now(),
			DueDate:       time.Now().AddDate(0, 0, -1), // Overdue
			UserID:        user.ID,
		}

		err = insuranceService.CreatePayment(ctx, overduePayment)
		require.NoError(t, err)

		// Get overdue payments
		overduePayments, err := insuranceService.GetOverduePayments(ctx, user.ID)
		assert.NoError(t, err)
		assert.Len(t, overduePayments, 1)
		assert.Equal(t, "pending", overduePayments[0].Status)
	})
}
