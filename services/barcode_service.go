package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"lazervaultGo/models"
	pb "lazervaultGo/pb"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type BarcodePaymentService struct {
	pb.UnimplementedBarcodePaymentServiceServer
	db *gorm.DB
}

func NewBarcodePaymentService(db *gorm.DB) *BarcodePaymentService {
	return &BarcodePaymentService{db: db}
}

// GenerateBarcode creates a new payment barcode
func (s *BarcodePaymentService) GenerateBarcode(ctx context.Context, req *pb.GenerateBarcodeRequest) (*pb.GenerateBarcodeResponse, error) {
	// Get user from context
	userEmail, ok := ctx.Value("email").(string)
	if !ok || userEmail == "" {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	// Get user details
	var user models.User
	if err := s.db.Where("email = ?", userEmail).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	// Validate amount
	if req.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be greater than zero")
	}

	// Set default validity if not provided
	validityMinutes := req.ValidityMinutes
	if validityMinutes <= 0 {
		validityMinutes = 30 // Default 30 minutes
	}

	// Generate unique barcode code
	barcodeCode, err := generateBarcodeCode()
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate barcode code")
	}

	// Calculate expiry time
	expiresAt := time.Now().Add(time.Duration(validityMinutes) * time.Minute)

	// Get username
	username := ""
	if user.Username != nil {
		username = *user.Username
	}

	// Create barcode payment
	barcode := &models.BarcodePayment{
		UserID:      user.UUID,
		Username:    username,
		FullName:    fmt.Sprintf("%s %s", user.FirstName, user.LastName),
		BarcodeCode: barcodeCode,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Description: req.Description,
		Status:      models.BarcodePaymentStatusPending,
		ExpiresAt:   expiresAt,
	}

	if err := s.db.Create(barcode).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to create barcode")
	}

	// Create QR data JSON
	qrData := map[string]interface{}{
		"type":         "barcode_payment",
		"barcode_code": barcodeCode,
		"amount":       req.Amount,
		"currency":     req.Currency,
		"recipient":    username,
		"expires_at":   expiresAt.Unix(),
	}

	qrDataJSON, _ := json.Marshal(qrData)

	return &pb.GenerateBarcodeResponse{
		BarcodeId:   barcode.ID,
		BarcodeCode: barcodeCode,
		QrData:      string(qrDataJSON),
		ExpiresAt:   timestamppb.New(expiresAt),
		Message:     "Barcode generated successfully",
	}, nil
}

// GetBarcodeDetails retrieves barcode details by code
func (s *BarcodePaymentService) GetBarcodeDetails(ctx context.Context, req *pb.GetBarcodeDetailsRequest) (*pb.GetBarcodeDetailsResponse, error) {
	var barcode models.BarcodePayment

	if err := s.db.Where("barcode_code = ?", req.BarcodeCode).First(&barcode).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Error(codes.NotFound, "barcode not found")
		}
		return nil, status.Error(codes.Internal, "failed to retrieve barcode")
	}

	// Check if barcode is expired
	if time.Now().After(barcode.ExpiresAt) && barcode.Status == models.BarcodePaymentStatusPending {
		barcode.Status = models.BarcodePaymentStatusExpired
		s.db.Save(&barcode)
	}

	return &pb.GetBarcodeDetailsResponse{
		Barcode: toBarcodePaymentProto(&barcode),
	}, nil
}

// ProcessBarcodePayment processes payment for a scanned barcode
func (s *BarcodePaymentService) ProcessBarcodePayment(ctx context.Context, req *pb.ProcessBarcodePaymentRequest) (*pb.ProcessBarcodePaymentResponse, error) {
	// Get payer user from context
	payerEmail, ok := ctx.Value("email").(string)
	if !ok || payerEmail == "" {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	// Get payer details
	var payer models.User
	if err := s.db.Where("email = ?", payerEmail).First(&payer).Error; err != nil {
		return nil, status.Error(codes.NotFound, "payer not found")
	}

	// Get barcode
	var barcode models.BarcodePayment
	if err := s.db.Where("barcode_code = ?", req.BarcodeCode).First(&barcode).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Error(codes.NotFound, "barcode not found")
		}
		return nil, status.Error(codes.Internal, "failed to retrieve barcode")
	}

	// Validate barcode
	if barcode.Status != models.BarcodePaymentStatusPending {
		return nil, status.Errorf(codes.FailedPrecondition, "barcode is %s", barcode.Status)
	}

	if time.Now().After(barcode.ExpiresAt) {
		barcode.Status = models.BarcodePaymentStatusExpired
		s.db.Save(&barcode)
		return nil, status.Error(codes.FailedPrecondition, "barcode has expired")
	}

	// Check if user is trying to pay their own barcode
	if barcode.UserID == payer.UUID {
		return nil, status.Error(codes.InvalidArgument, "cannot pay your own barcode")
	}

	// Begin transaction
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Generate reference number
	refNumber := fmt.Sprintf("BC-%s", generateShortID())

	// Get payer username
	payerUsername := ""
	if payer.Username != nil {
		payerUsername = *payer.Username
	}

	// Create transaction record
	now := time.Now()
	transaction := &models.BarcodeTransaction{
		BarcodeID:         barcode.ID,
		PayerID:           payer.UUID,
		PayerUsername:     payerUsername,
		PayerName:         fmt.Sprintf("%s %s", payer.FirstName, payer.LastName),
		RecipientID:       barcode.UserID,
		RecipientUsername: barcode.Username,
		RecipientName:     barcode.FullName,
		Amount:            barcode.Amount,
		Currency:          barcode.Currency,
		Description:       barcode.Description,
		ReferenceNumber:   refNumber,
		Status:            models.BarcodeTransactionStatusCompleted,
		CreatedAt:         now,
	}

	if err := tx.Create(transaction).Error; err != nil {
		tx.Rollback()
		return nil, status.Error(codes.Internal, "failed to create transaction")
	}

	// Update barcode status
	barcode.Status = models.BarcodePaymentStatusPaid
	barcode.PaidAt = &now

	if err := tx.Save(&barcode).Error; err != nil {
		tx.Rollback()
		return nil, status.Error(codes.Internal, "failed to update barcode")
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to commit transaction")
	}

	// Load the complete transaction with barcode relation
	s.db.Preload("Barcode").First(transaction, transaction.ID)

	return &pb.ProcessBarcodePaymentResponse{
		Transaction: toBarcodeTransactionProto(transaction),
		Message:     "Payment processed successfully",
	}, nil
}

// GetMyGeneratedBarcodes retrieves user's generated barcodes
func (s *BarcodePaymentService) GetMyGeneratedBarcodes(ctx context.Context, req *pb.GetMyGeneratedBarcodesRequest) (*pb.GetMyGeneratedBarcodesResponse, error) {
	userEmail, ok := ctx.Value("email").(string)
	if !ok || userEmail == "" {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", userEmail).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	var barcodes []models.BarcodePayment
	var total int64

	s.db.Model(&models.BarcodePayment{}).Where("user_id = ?", user.UUID).Count(&total)

	if err := s.db.Where("user_id = ?", user.UUID).
		Order("created_at DESC").
		Limit(int(limit)).
		Offset(int(offset)).
		Find(&barcodes).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to retrieve barcodes")
	}

	// Update expired barcodes
	for i := range barcodes {
		if time.Now().After(barcodes[i].ExpiresAt) && barcodes[i].Status == models.BarcodePaymentStatusPending {
			barcodes[i].Status = models.BarcodePaymentStatusExpired
			s.db.Save(&barcodes[i])
		}
	}

	pbBarcodes := make([]*pb.BarcodePayment, len(barcodes))
	for i, barcode := range barcodes {
		pbBarcodes[i] = toBarcodePaymentProto(&barcode)
	}

	return &pb.GetMyGeneratedBarcodesResponse{
		Barcodes: pbBarcodes,
		Total:    int32(total),
	}, nil
}

// GetMyScannedBarcodes retrieves user's scanned/paid barcodes
func (s *BarcodePaymentService) GetMyScannedBarcodes(ctx context.Context, req *pb.GetMyScannedBarcodesRequest) (*pb.GetMyScannedBarcodesResponse, error) {
	userEmail, ok := ctx.Value("email").(string)
	if !ok || userEmail == "" {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", userEmail).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	var transactions []models.BarcodeTransaction
	var total int64

	s.db.Model(&models.BarcodeTransaction{}).Where("payer_id = ?", user.UUID).Count(&total)

	if err := s.db.Preload("Barcode").
		Where("payer_id = ?", user.UUID).
		Order("created_at DESC").
		Limit(int(limit)).
		Offset(int(offset)).
		Find(&transactions).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to retrieve transactions")
	}

	pbTransactions := make([]*pb.BarcodeTransaction, len(transactions))
	for i, transaction := range transactions {
		pbTransactions[i] = toBarcodeTransactionProto(&transaction)
	}

	return &pb.GetMyScannedBarcodesResponse{
		Transactions: pbTransactions,
		Total:        int32(total),
	}, nil
}

// CancelBarcode cancels/invalidates a generated barcode
func (s *BarcodePaymentService) CancelBarcode(ctx context.Context, req *pb.CancelBarcodeRequest) (*pb.CancelBarcodeResponse, error) {
	userEmail, ok := ctx.Value("email").(string)
	if !ok || userEmail == "" {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", userEmail).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	var barcode models.BarcodePayment
	if err := s.db.Where("id = ? AND user_id = ?", req.BarcodeId, user.UUID).First(&barcode).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Error(codes.NotFound, "barcode not found")
		}
		return nil, status.Error(codes.Internal, "failed to retrieve barcode")
	}

	if barcode.Status != models.BarcodePaymentStatusPending {
		return nil, status.Errorf(codes.FailedPrecondition, "cannot cancel barcode with status %s", barcode.Status)
	}

	barcode.Status = models.BarcodePaymentStatusCancelled
	if err := s.db.Save(&barcode).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to cancel barcode")
	}

	return &pb.CancelBarcodeResponse{
		Message: "Barcode cancelled successfully",
	}, nil
}

// Helper functions
func generateBarcodeCode() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:22], nil
}

func generateShortID() string {
	bytes := make([]byte, 6)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)[:8]
}

func toBarcodePaymentProto(barcode *models.BarcodePayment) *pb.BarcodePayment {
	pb := &pb.BarcodePayment{
		Id:          barcode.ID,
		UserId:      barcode.UserID,
		Username:    barcode.Username,
		FullName:    barcode.FullName,
		BarcodeCode: barcode.BarcodeCode,
		Amount:      barcode.Amount,
		Currency:    barcode.Currency,
		Description: barcode.Description,
		Status:      string(barcode.Status),
		CreatedAt:   timestamppb.New(barcode.CreatedAt),
		ExpiresAt:   timestamppb.New(barcode.ExpiresAt),
	}

	if barcode.PaidAt != nil {
		pb.PaidAt = timestamppb.New(*barcode.PaidAt)
	}

	return pb
}

func toBarcodeTransactionProto(transaction *models.BarcodeTransaction) *pb.BarcodeTransaction {
	return &pb.BarcodeTransaction{
		Id:                transaction.ID,
		BarcodeId:         transaction.BarcodeID,
		PayerId:           transaction.PayerID,
		PayerUsername:     transaction.PayerUsername,
		PayerName:         transaction.PayerName,
		RecipientId:       transaction.RecipientID,
		RecipientUsername: transaction.RecipientUsername,
		RecipientName:     transaction.RecipientName,
		Amount:            transaction.Amount,
		Currency:          transaction.Currency,
		Description:       transaction.Description,
		ReferenceNumber:   transaction.ReferenceNumber,
		Status:            string(transaction.Status),
		CreatedAt:         timestamppb.New(transaction.CreatedAt),
	}
}
