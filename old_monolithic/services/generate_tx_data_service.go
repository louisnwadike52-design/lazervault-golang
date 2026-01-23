package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt" // Need ioutil again for ReadFile if < Go 1.16, or use os.ReadFile
	"io"
	"log"
	"net/http"
	"os" // Use os.ReadFile if Go >= 1.16
	"strconv"
	"strings"
	"time"

	"lazervaultGo/configs"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/token"

	"cloud.google.com/go/storage"
	// No longer need credentials import if not using the creds object directly
	// credentials "golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause" // Import for OnConflict
)

type GenerateTxDataService struct {
	pb.UnimplementedGenerateTxDataServiceServer
	db         *gorm.DB
	config     configs.Config
	httpClient *http.Client
}

// NewGenerateTxDataService creates a new GenerateTxDataService
func NewGenerateTxDataService(db *gorm.DB, config configs.Config) *GenerateTxDataService {
	return &GenerateTxDataService{
		db:     db,
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TransactionRecord defines a unified structure for CSV output
// Fields are pointers where they don't apply to all transaction types.
type TransactionRecord struct {
	// Common Fields
	UserID                uint64     `json:"user_id"`
	Timestamp             time.Time  `json:"timestamp"` // Usually CreatedAt
	Type                  string     `json:"type"`      // DEPOSIT, WITHDRAWAL, TRANSFER_OUT, TRANSFER_IN, EXCHANGE_OUT, EXCHANGE_IN
	Amount                float64    `json:"amount"`    // Primary amount (deposit, withdrawal, transfer, relevant exchange amount)
	Currency              string     `json:"currency"`  // Primary currency
	Status                string     `json:"status"`
	Description           string     `json:"description"`                       // Generated description
	Reference             string     `json:"reference"`                         // Unique ID (Deposit.ID, Withdrawal.ID, Transfer.Reference, Exchange.ID)
	FailureReason         *string    `json:"failure_reason,omitempty"`          // From Transfer, Withdrawal, Deposit
	CompletedAt           *time.Time `json:"completed_at,omitempty"`            // From Transfer, Withdrawal, Deposit
	ProcessingAt          *time.Time `json:"processing_at,omitempty"`           // From Withdrawal, Deposit
	FailedAt              *time.Time `json:"failed_at,omitempty"`               // From Transfer, Withdrawal, Deposit
	ExternalTransactionID *string    `json:"external_transaction_id,omitempty"` // From Withdrawal, Deposit

	// Account IDs
	FromAccountID *uint `json:"from_account_id,omitempty"`
	ToAccountID   *uint `json:"to_account_id,omitempty"`

	// Deposit Specific
	DepositSourceBankName *string `json:"deposit_source_bank_name,omitempty"`

	// Withdrawal Specific
	WithdrawalTargetBankName      *string `json:"withdrawal_target_bank_name,omitempty"`
	WithdrawalTargetAccountNumber *string `json:"withdrawal_target_account_number,omitempty"`
	WithdrawalTargetSortCode      *string `json:"withdrawal_target_sort_code,omitempty"`

	// Transfer Specific
	TransferFee         *float64 `json:"transfer_fee,omitempty"`
	TransferTotalAmount *float64 `json:"transfer_total_amount,omitempty"`
	TransferCategory    *string  `json:"transfer_category,omitempty"`
	TransferScheduledAt *string  `json:"transfer_scheduled_at,omitempty"` // Changed to string pointer
	SenderInfo          string   `json:"sender_info,omitempty"`           // Generated for TRANSFER_IN
	RecipientInfo       string   `json:"recipient_info,omitempty"`        // Generated for TRANSFER_OUT
	RecipientID         *uint    `json:"recipient_id,omitempty"`          // From Transfer model

	// Exchange Specific
	ExchangeFromCurrency    *string  `json:"exchange_from_currency,omitempty"`
	ExchangeToCurrency      *string  `json:"exchange_to_currency,omitempty"`
	ExchangeAmountFrom      *float64 `json:"exchange_amount_from,omitempty"`
	ExchangeAmountTo        *float64 `json:"exchange_amount_to,omitempty"`
	ExchangeRate            *float64 `json:"exchange_rate,omitempty"`
	ExchangeFees            *float64 `json:"exchange_fees,omitempty"`
	ExchangeReceiverDetails *string  `json:"exchange_receiver_details,omitempty"` // JSON string

	// Generic Related Party (Fallback/Specific Use)
	RelatedParty *string `json:"related_party,omitempty"` // For specific cases like exchange pair where other fields don't fit
}

// ServiceAccountCreds defines the structure for parsing the service account JSON key file.
// Only include fields necessary for signing the URL.
type ServiceAccountCreds struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	ClientID     string `json:"client_id"`
}

// GenerateUserTxDataFile implements the gRPC method
func (s *GenerateTxDataService) GenerateUserTxDataFile(ctx context.Context, req *pb.GenerateUserTxDataFileRequest) (*pb.GenerateUserTxDataFileResponse, error) {
	// 1. Retrieve Payload from Context (injected by middleware)
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload) // Use middleware key
	if !ok || authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}

	userIDStr := req.GetUserId()
	if userIDStr == "" {
		return nil, status.Errorf(codes.InvalidArgument, "User ID is required")
	}

	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID format: %v", err)
	}

	// --- Core Logic: Fetch, Format, Upload --- //
	var userAccounts []models.Account
	if err := s.db.WithContext(ctx).Where("owner_user_id = ?", userID).Find(&userAccounts).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query user accounts: %v", err)
	}
	if len(userAccounts) == 0 {
		return nil, status.Errorf(codes.NotFound, "user %d has no accounts", userID)
	}
	var accountIDs []uint
	for _, acc := range userAccounts {
		accountIDs = append(accountIDs, acc.ID)
	}

	allRecords, err := s.FetchAllUserTransactions(ctx, uint(userID))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch transactions: %v", err)
	}

	csvData, err := s.FormatTransactionsToCSV(allRecords)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to format data to CSV: %v", err)
	}

	// Upload
	objectFullPath := fmt.Sprintf("user-tx-data/%s/transactions.csv", userIDStr) // Path within bucket
	if err := s.UploadOrOverwriteTxFile(ctx, userIDStr, csvData); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to upload file to GCS: %v", err)
	}

	// Construct the Public HTTPS URL (ASSUMES OBJECT IS PUBLICLY READABLE IN GCP)
	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.config.GCSBucketName, objectFullPath)

	// Store/Update the Public URL in the database
	fileRecord := models.UserTransactionFile{
		UserID:   uint(userID),
		FilePath: publicURL, // Store the public URL
	}
	if errDb := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"file_path", "updated_at"}),
	}).Create(&fileRecord).Error; errDb != nil {
		fmt.Printf("ERROR: Failed to save/update user transaction file public URL in DB for user %d: %v\n", userID, errDb)
		// Don't fail the whole operation just for this DB save error
	}

	// Return response with the public HTTPS URL
	// Assuming proto uses FileGcsUrl field name for now
	return &pb.GenerateUserTxDataFileResponse{
		FileGcsUrl: publicURL,
	}, nil
}

// FetchAllUserTransactions fetches and compiles all transaction types for a user.
// It now requires the userID to populate the records.
func (s *GenerateTxDataService) FetchAllUserTransactions(ctx context.Context, userID uint) ([]TransactionRecord, error) {
	var records []TransactionRecord
	userID64 := uint64(userID) // Convert once for convenience

	// --- Helper funcs for formatting pointers ---
	stringPtr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	float64Ptr := func(f float64) *float64 {
		// Decide if 0.0 should be nil or represented
		// if f == 0.0 { return nil } // Uncomment if 0 should be omitted
		return &f
	}
	timePtr := func(t *time.Time) *time.Time {
		if t == nil {
			return nil
		}
		if t.IsZero() {
			return nil
		}
		return t
	}
	// --- End Helper funcs ---

	// Fetch Deposits
	var deposits []models.Deposit
	if err := s.db.WithContext(ctx).Joins("join accounts on accounts.id = deposits.target_account_id").Where("accounts.owner_user_id = ?", userID).Order("deposits.created_at desc").Find(&deposits).Error; err != nil {
		return nil, fmt.Errorf("failed to query deposits: %w", err)
	}
	for _, d := range deposits {
		amountFloat := float64(d.Amount) / 100.0
		toAccountID := d.TargetAccountID
		records = append(records, TransactionRecord{
			UserID:                userID64,
			Timestamp:             d.CreatedAt,
			Type:                  "DEPOSIT",
			Amount:                amountFloat,
			Currency:              d.Currency,
			Status:                string(d.Status),
			Description:           "Deposit from " + d.SourceBankName,
			Reference:             d.ID,
			ToAccountID:           &toAccountID,
			FailureReason:         d.FailureReason,
			CompletedAt:           timePtr(d.CompletedAt),
			ProcessingAt:          timePtr(d.ProcessingAt),
			FailedAt:              timePtr(d.FailedAt),
			ExternalTransactionID: d.ExternalTransactionID,
			DepositSourceBankName: stringPtr(d.SourceBankName),
		})
	}

	// Fetch Withdrawals
	var withdrawals []models.Withdrawal
	if err := s.db.WithContext(ctx).Joins("join accounts on accounts.id = withdrawals.source_account_id").Where("accounts.owner_user_id = ?", userID).Order("withdrawals.created_at desc").Find(&withdrawals).Error; err != nil {
		return nil, fmt.Errorf("failed to query withdrawals: %w", err)
	}
	for _, w := range withdrawals {
		amountFloat := float64(w.Amount) / 100.0
		fromAccountID := w.SourceAccountID
		ref := w.TransactionReference
		if ref == "" {
			ref = w.ID
		}
		records = append(records, TransactionRecord{
			UserID:                        userID64,
			Timestamp:                     w.CreatedAt,
			Type:                          "WITHDRAWAL",
			Amount:                        amountFloat,
			Currency:                      w.Currency,
			Status:                        string(w.Status),
			Description:                   fmt.Sprintf("Withdrawal to %s (%s)", w.TargetBankName, w.TargetAccountNumber),
			Reference:                     ref,
			FromAccountID:                 &fromAccountID,
			FailureReason:                 stringPtr(w.FailureReason),
			CompletedAt:                   timePtr(w.CompletedAt),
			ProcessingAt:                  timePtr(w.ProcessingAt),
			FailedAt:                      timePtr(w.FailedAt),
			ExternalTransactionID:         w.ExternalTransactionID,
			WithdrawalTargetBankName:      stringPtr(w.TargetBankName),
			WithdrawalTargetAccountNumber: stringPtr(w.TargetAccountNumber),
			WithdrawalTargetSortCode:      stringPtr(w.TargetSortCode),
		})
	}

	// Fetch Transfers (Outgoing)
	var outgoingTransfers []models.Transfer
	if err := s.db.WithContext(ctx).Where("from_user_id = ?", userID).Preload("ToUser").Preload("Recipient").Order("created_at desc").Find(&outgoingTransfers).Error; err != nil {
		return nil, fmt.Errorf("failed to query outgoing transfers: %w", err)
	}
	for _, t := range outgoingTransfers {
		recipientInfoStr := "Unknown Recipient"
		if t.ToUser != nil {
			recipientInfoStr = t.ToUser.FirstName + " " + t.ToUser.LastName
			if recipientInfoStr == " " {
				recipientInfoStr = t.ToUser.Email
			}
		} else if t.Recipient != nil {
			recipientInfoStr = fmt.Sprintf("%s (%s)", t.Recipient.Name, t.Recipient.AccountNumber)
		} else if t.ToAccountID != nil {
			recipientInfoStr = fmt.Sprintf("Internal Account ID: %d", *t.ToAccountID)
		}

		amountFloat := float64(t.Amount) / 100.0
		transferFee := float64(t.Fee) / 100.0
		transferTotalAmount := float64(t.TotalAmount) / 100.0
		var accountCurrency string
		var fromAccount models.Account
		if err := s.db.WithContext(ctx).Select("currency").First(&fromAccount, t.FromAccountID).Error; err == nil {
			accountCurrency = fromAccount.Currency
		} else {
			fmt.Printf("Warning: Outgoing transfer record (ID: %s) missing currency: %v\n", t.Reference, err)
			accountCurrency = "UNKNOWN" // Fallback currency
		}

		fromAccID := t.FromAccountID
		records = append(records, TransactionRecord{
			UserID:              userID64,
			Timestamp:           t.CreatedAt,
			Type:                "TRANSFER_OUT",
			Amount:              amountFloat,
			Currency:            accountCurrency,
			Status:              string(t.Status),
			Description:         "Transfer to " + recipientInfoStr,
			Reference:           t.Reference,
			FromAccountID:       &fromAccID,
			ToAccountID:         t.ToAccountID,
			RecipientInfo:       recipientInfoStr,
			RecipientID:         t.RecipientID,
			FailureReason:       stringPtr(t.FailureReason),
			CompletedAt:         timePtr(t.CompletedAt),
			FailedAt:            timePtr(t.FailedAt),
			TransferFee:         float64Ptr(transferFee),
			TransferTotalAmount: float64Ptr(transferTotalAmount),
			TransferCategory:    stringPtr(t.Category),
			TransferScheduledAt: t.ScheduledAt, // Use the string pointer directly
		})
	}

	// Fetch Transfers (Incoming)
	var incomingTransfers []models.Transfer
	if err := s.db.WithContext(ctx).Where("to_user_id = ?", userID).Preload("FromUser").Order("created_at desc").Find(&incomingTransfers).Error; err != nil {
		return nil, fmt.Errorf("failed to query incoming transfers: %w", err)
	}
	for _, t := range incomingTransfers {
		senderInfoStr := "Unknown Sender"
		if t.FromUser.ID != 0 {
			senderInfoStr = t.FromUser.FirstName + " " + t.FromUser.LastName
			if senderInfoStr == " " {
				senderInfoStr = t.FromUser.Email
			}
		} else {
			senderInfoStr = fmt.Sprintf("Account ID: %d", t.FromAccountID)
		}

		amountFloat := float64(t.Amount) / 100.0
		transferFee := float64(t.Fee) / 100.0
		transferTotalAmount := float64(t.TotalAmount) / 100.0
		var accountCurrency string
		var toAccount models.Account
		if t.ToAccountID != nil {
			if err := s.db.WithContext(ctx).Select("currency").First(&toAccount, *t.ToAccountID).Error; err == nil {
				accountCurrency = toAccount.Currency
			} else {
				fmt.Printf("Warning: Incoming transfer record (ID: %s) missing currency: %v\n", t.Reference, err)
				accountCurrency = "UNKNOWN" // Fallback
			}
		} else {
			fmt.Printf("Warning: Skipping incoming transfer record (ID: %s) because ToAccountID is nil\n", t.Reference)
			continue
		}

		fromAccID := t.FromAccountID
		records = append(records, TransactionRecord{
			UserID:              userID64,
			Timestamp:           t.CreatedAt,
			Type:                "TRANSFER_IN",
			Amount:              amountFloat,
			Currency:            accountCurrency,
			Status:              string(t.Status),
			Description:         "Transfer from " + senderInfoStr,
			Reference:           t.Reference,
			FromAccountID:       &fromAccID,
			ToAccountID:         t.ToAccountID,
			SenderInfo:          senderInfoStr,
			FailureReason:       stringPtr(t.FailureReason),
			CompletedAt:         timePtr(t.CompletedAt),
			FailedAt:            timePtr(t.FailedAt),
			TransferFee:         float64Ptr(transferFee),
			TransferTotalAmount: float64Ptr(transferTotalAmount),
			TransferCategory:    stringPtr(t.Category),
			TransferScheduledAt: t.ScheduledAt, // Use the string pointer directly
		})
	}

	// Fetch Exchanges
	var exchanges []models.ExchangeTransaction
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at desc").Find(&exchanges).Error; err != nil {
		return nil, fmt.Errorf("failed to query exchanges: %w", err)
	}
	for _, e := range exchanges {
		var txType string
		var amount float64
		var description string
		var accountIDForRecord *uint
		var exchangeAccount models.Account
		var baseCurrency string

		if e.AmountFrom > 0 { // Treat as outgoing from the FromCurrency account
			txType = "EXCHANGE_OUT"
			amount = float64(e.AmountFrom) // Use raw amount
			baseCurrency = e.FromCurrency
			description = fmt.Sprintf("Exchange %s to %s", e.FromCurrency, e.ToCurrency)
			// Find account matching FromCurrency
			if err := s.db.WithContext(ctx).Where("owner_user_id = ? AND currency = ?", userID, e.FromCurrency).First(&exchangeAccount).Error; err == nil {
				accID := exchangeAccount.ID
				accountIDForRecord = &accID
			} else {
				fmt.Printf("Warning: Exchange record (ID: %s) missing FromAccount: %v\n", e.ID, err)
			}
		} else if e.AmountTo > 0 { // Treat as incoming to the ToCurrency account
			txType = "EXCHANGE_IN"
			amount = float64(e.AmountTo) // Use raw amount
			baseCurrency = e.ToCurrency
			description = fmt.Sprintf("Exchange from %s to %s", e.FromCurrency, e.ToCurrency)
			// Find account matching ToCurrency
			if err := s.db.WithContext(ctx).Where("owner_user_id = ? AND currency = ?", userID, e.ToCurrency).First(&exchangeAccount).Error; err == nil {
				accID := exchangeAccount.ID
				accountIDForRecord = &accID
			} else {
				fmt.Printf("Warning: Exchange record (ID: %s) missing ToAccount: %v\n", e.ID, err)
			}
		} else {
			continue // Skip if no amount is set
		}

		// Marshal receiver details if they exist
		var receiverDetailsStr *string
		if e.ReceiverDetails != nil {
			receiverJSON, marshalErr := e.ReceiverDetails.MarshalJSON()
			if marshalErr == nil {
				receiverStr := string(receiverJSON)
				receiverDetailsStr = &receiverStr // Assign address of string
			} else {
				fmt.Printf("Warning: Failed to marshal receiver details for exchange %s: %v\n", e.ID, marshalErr)
			}
		}

		rec := TransactionRecord{
			UserID:                  userID64,
			Timestamp:               e.CreatedAt,
			Type:                    txType,
			Amount:                  amount, // Use the appropriate amount (From or To)
			Currency:                baseCurrency,
			Status:                  string(e.Status),
			Description:             description,
			Reference:               e.ID,
			CompletedAt:             &e.UpdatedAt, // Use UpdatedAt as CompletedAt approximation for Exchange
			ExchangeFromCurrency:    stringPtr(e.FromCurrency),
			ExchangeToCurrency:      stringPtr(e.ToCurrency),
			ExchangeAmountFrom:      float64Ptr(e.AmountFrom),
			ExchangeAmountTo:        float64Ptr(e.AmountTo),
			ExchangeRate:            float64Ptr(e.ExchangeRate),
			ExchangeFees:            float64Ptr(e.Fees),
			ExchangeReceiverDetails: receiverDetailsStr,                                            // Assign the *string directly
			RelatedParty:            stringPtr(fmt.Sprintf("%s/%s", e.FromCurrency, e.ToCurrency)), // Use RelatedParty for pair
		}

		// Assign account ID based on type
		if txType == "EXCHANGE_OUT" {
			rec.FromAccountID = accountIDForRecord
		} else { // EXCHANGE_IN
			rec.ToAccountID = accountIDForRecord
		}
		records = append(records, rec)
	}

	return records, nil
}

// FormatTransactionsToCSV formats the records into a CSV byte buffer.
func (s *GenerateTxDataService) FormatTransactionsToCSV(records []TransactionRecord) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Write header
	header := []string{
		"UserID", "Timestamp", "Type", "Amount", "Currency", "Status", "Description", "Reference",
		"FailureReason", "CompletedAt", "ProcessingAt", "FailedAt", "ExternalTransactionID",
		"FromAccountID", "ToAccountID",
		"DepositSourceBankName",
		"WithdrawalTargetBankName", "WithdrawalTargetAccountNumber", "WithdrawalTargetSortCode",
		"TransferFee", "TransferTotalAmount", "TransferCategory", "TransferScheduledAt",
		"SenderInfo", "RecipientInfo", "RecipientID",
		"ExchangeFromCurrency", "ExchangeToCurrency", "ExchangeAmountFrom", "ExchangeAmountTo",
		"ExchangeRate", "ExchangeFees", "ExchangeReceiverDetails",
		"RelatedParty",
	}
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Helper funcs for formatting pointers
	formatUintPointer := func(ptr *uint) string {
		if ptr == nil {
			return ""
		}
		return strconv.FormatUint(uint64(*ptr), 10)
	}
	formatFloat64Pointer := func(ptr *float64) string {
		if ptr == nil {
			return ""
		}
		return strconv.FormatFloat(*ptr, 'f', -1, 64)
	}
	formatStringPointer := func(ptr *string) string {
		if ptr == nil {
			return ""
		}
		return *ptr
	}
	formatTimePointer := func(ptr *time.Time) string {
		if ptr == nil {
			return ""
		}
		if ptr.IsZero() {
			return ""
		}
		return ptr.UTC().Format(time.RFC3339)
	}

	// Write records
	for _, rec := range records {
		row := []string{
			strconv.FormatUint(rec.UserID, 10),
			formatTimePointer(&rec.Timestamp),
			rec.Type,
			strconv.FormatFloat(rec.Amount, 'f', 2, 64),
			rec.Currency,
			rec.Status,
			rec.Description,
			rec.Reference,
			formatStringPointer(rec.FailureReason),
			formatTimePointer(rec.CompletedAt),
			formatTimePointer(rec.ProcessingAt),
			formatTimePointer(rec.FailedAt),
			formatStringPointer(rec.ExternalTransactionID),
			formatUintPointer(rec.FromAccountID),
			formatUintPointer(rec.ToAccountID),
			formatStringPointer(rec.DepositSourceBankName),
			formatStringPointer(rec.WithdrawalTargetBankName),
			formatStringPointer(rec.WithdrawalTargetAccountNumber),
			formatStringPointer(rec.WithdrawalTargetSortCode),
			formatFloat64Pointer(rec.TransferFee),
			formatFloat64Pointer(rec.TransferTotalAmount),
			formatStringPointer(rec.TransferCategory),
			formatStringPointer(rec.TransferScheduledAt), // Use string pointer formatter
			rec.SenderInfo,
			rec.RecipientInfo,
			formatUintPointer(rec.RecipientID),
			formatStringPointer(rec.ExchangeFromCurrency),
			formatStringPointer(rec.ExchangeToCurrency),
			formatFloat64Pointer(rec.ExchangeAmountFrom),
			formatFloat64Pointer(rec.ExchangeAmountTo),
			formatFloat64Pointer(rec.ExchangeRate),
			formatFloat64Pointer(rec.ExchangeFees),
			formatStringPointer(rec.ExchangeReceiverDetails),
			formatStringPointer(rec.RelatedParty),
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("failed to write CSV row for reference %s: %w", rec.Reference, err)
		}
	}

	w.Flush() // Ensure all data is written to the buffer

	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("CSV writer error: %w", err)
	}

	return &buf, nil
}

// UploadOrOverwriteTxFile uploads or overwrites a transaction file in GCS.
func (s *GenerateTxDataService) UploadOrOverwriteTxFile(ctx context.Context, userID string, data *bytes.Buffer) error {
	if s.config.GCSBucketName == "" {
		return fmt.Errorf("GCS_BUCKET_NAME is not configured")
	}

	log.Printf("INFO: Using GCS bucket: %s", s.config.GCSBucketName)

	// Initialize GCS client
	var client *storage.Client
	var err error

	gcpCredsEnv := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if gcpCredsEnv != "" {
		client, err = storage.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("storage.NewClient (ADC): %w", err)
		}
	} else if s.config.GCSCredentialsFile != "" {
		if _, statErr := os.Stat(s.config.GCSCredentialsFile); os.IsNotExist(statErr) {
			return fmt.Errorf("GCS credentials file from config not found at: %s", s.config.GCSCredentialsFile)
		}
		client, err = storage.NewClient(ctx, option.WithCredentialsFile(s.config.GCSCredentialsFile))
		if err != nil {
			return fmt.Errorf("storage.NewClient with credentials file from config: %w", err)
		}
	} else {
		return fmt.Errorf("GCS authentication failed: Neither GOOGLE_APPLICATION_CREDENTIALS env var nor GCS_CREDENTIALS_FILE config is set")
	}
	defer client.Close()

	// Get the object
	objectName := fmt.Sprintf("user-tx-data/%s/transactions.csv", userID)
	bucket := client.Bucket(s.config.GCSBucketName)
	obj := bucket.Object(objectName)

	// Create a new writer
	wc := obj.NewWriter(ctx)
	wc.ContentType = "text/csv"

	// Write the data
	if _, err := wc.Write(data.Bytes()); err != nil {
		wc.Close()
		return fmt.Errorf("failed to write data to GCS: %w", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	fmt.Printf("Successfully uploaded/overwritten transaction file for user %s\n", userID)
	return nil
}

/* // Commenting out as GetTxFileSignedUrl is no longer used by the primary flow
// GetTxFileSignedUrl generates a signed URL for reading the user's transaction file.
func (s *GenerateTxDataService) GetTxFileSignedUrl(ctx context.Context, userID string) (string, error) {
	objectPath := fmt.Sprintf("user-tx-data/%s/transactions.csv", userID)
	gcsBucketName := s.config.GCSBucketName

	// --- Determine Credential Source & Get Service Account Email if possible --- //
	var credOption option.ClientOption
	var serviceAccountEmail string
	var err error

	credsFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if credsFile == "" {
		credsFile = s.config.GCSCredentialsFile
	}

	if credsFile != "" {
		// Using a specific credentials file
		if _, statErr := os.Stat(credsFile); statErr != nil {
			return "", fmt.Errorf("GCS credentials file specified but not found or unreadable (%s): %w", credsFile, statErr)
		}
		credOption = option.WithCredentialsFile(credsFile)

		// Load credentials bytes from file ONLY to extract email for signing info
		credsBytes, readErr := os.ReadFile(credsFile) // Use os.ReadFile
		if readErr != nil {
			return "", fmt.Errorf("failed to read service account key file %s for signing info: %w", credsFile, readErr)
		}
		// Extract email from JSON bytes
		var saInfo ServiceAccountCreds // Re-use struct defined earlier
		if err := json.Unmarshal(credsBytes, &saInfo); err == nil {
			serviceAccountEmail = saInfo.ClientEmail
		} else {
			fmt.Printf("Warning: Could not unmarshal service account key file %s to extract email: %v\n", credsFile, err)
		}

	} else {
		// Relying on Application Default Credentials (ADC)
		credOption = option.WithScopes(storage.ScopeReadOnly)
		fmt.Println("Warning: Generating Signed URL using ADC without explicit key file. Email for GoogleAccessID might not be available.")
	}

	// --- Check if Object Exists --- //
	gcsClient, err := storage.NewClient(ctx, credOption)
	if err != nil {
		return "", fmt.Errorf("failed to create GCS client: %w", err)
	}
	defer gcsClient.Close()

	objHandle := gcsClient.Bucket(gcsBucketName).Object(objectPath)
	_, err = objHandle.Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return "", storage.ErrObjectNotExist
		} else {
			return "", fmt.Errorf("failed to get object attributes for gs://%s/%s: %w", gcsBucketName, objectPath, err)
		}
	}

	// --- Prepare Signed URL Options --- //
	opts := &storage.SignedURLOptions{
		Scheme:         storage.SigningSchemeV4,
		Method:         "GET",
		Expires:        time.Now().Add(15 * time.Minute),
		GoogleAccessID: serviceAccountEmail, // <<< Set GoogleAccessID
	}

	if opts.GoogleAccessID == "" {
		fmt.Println("Warning: GoogleAccessID is empty; SignedURL may fail if ADC cannot determine service account email.")
	}

	// --- Generate the Signed URL --- //
	signedURL, err := storage.SignedURL(gcsBucketName, objectPath, opts)
	if err != nil {
		errMsg := fmt.Sprintf("failed to sign URL for gs://%s/%s: %v", gcsBucketName, objectPath, err)
		if credsFile != "" {
			errMsg += fmt.Sprintf(" (using creds file: %s)", credsFile)
		} else {
			errMsg += " (using ADC)"
		}
		return "", fmt.Errorf(errMsg)
	}

	return signedURL, nil
}
*/

// AppendTransactionToFile appends a single transaction record to the user's transaction file in GCS.
func (s *GenerateTxDataService) AppendTransactionToFile(ctx context.Context, userID uint, record TransactionRecord) error {
	var client *storage.Client
	var err error

	if s.config.GCSBucketName == "" {
		return fmt.Errorf("GCS_BUCKET_NAME is not configured")
	}

	// Step 1: Construct the file path and public URL
	userIDStr := strconv.FormatUint(uint64(userID), 10)
	objectName := fmt.Sprintf("user-tx-data/%s/transactions.csv", userIDStr)
	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.config.GCSBucketName, objectName)

	// Step 2: Update or create the file path record in the database
	fileRecord := models.UserTransactionFile{
		UserID:   userID,
		FilePath: publicURL,
	}
	if errDb := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"file_path", "updated_at"}),
	}).Create(&fileRecord).Error; errDb != nil {
		log.Printf("ERROR: Failed to save/update user transaction file public URL in DB for user %d: %v\n", userID, errDb)
		// Continue with the upload even if DB update fails
	}

	// Initialize GCS client
	gcpCredsEnv := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if gcpCredsEnv != "" {
		client, err = storage.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("storage.NewClient (ADC): %w", err)
		}
	} else if s.config.GCSCredentialsFile != "" {
		if _, statErr := os.Stat(s.config.GCSCredentialsFile); os.IsNotExist(statErr) {
			return fmt.Errorf("GCS credentials file from config not found at: %s", s.config.GCSCredentialsFile)
		}
		client, err = storage.NewClient(ctx, option.WithCredentialsFile(s.config.GCSCredentialsFile))
		if err != nil {
			return fmt.Errorf("storage.NewClient with credentials file from config: %w", err)
		}
	} else {
		return fmt.Errorf("GCS authentication failed: Neither GOOGLE_APPLICATION_CREDENTIALS env var nor GCS_CREDENTIALS_FILE config is set")
	}
	defer client.Close()

	// Format the record as CSV
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)

	// Helper functions for formatting pointers
	formatUintPointer := func(ptr *uint) string {
		if ptr == nil {
			return ""
		}
		return strconv.FormatUint(uint64(*ptr), 10)
	}
	formatFloat64Pointer := func(ptr *float64) string {
		if ptr == nil {
			return ""
		}
		return strconv.FormatFloat(*ptr, 'f', -1, 64)
	}
	formatStringPointer := func(ptr *string) string {
		if ptr == nil {
			return ""
		}
		return *ptr
	}
	formatTimePointer := func(ptr *time.Time) string {
		if ptr == nil {
			return ""
		}
		if ptr.IsZero() {
			return ""
		}
		return ptr.UTC().Format(time.RFC3339)
	}

	// Format the record
	row := []string{
		strconv.FormatUint(record.UserID, 10),
		formatTimePointer(&record.Timestamp),
		record.Type,
		strconv.FormatFloat(record.Amount, 'f', 2, 64),
		record.Currency,
		record.Status,
		record.Description,
		record.Reference,
		formatStringPointer(record.FailureReason),
		formatTimePointer(record.CompletedAt),
		formatTimePointer(record.ProcessingAt),
		formatTimePointer(record.FailedAt),
		formatStringPointer(record.ExternalTransactionID),
		formatUintPointer(record.FromAccountID),
		formatUintPointer(record.ToAccountID),
		formatStringPointer(record.DepositSourceBankName),
		formatStringPointer(record.WithdrawalTargetBankName),
		formatStringPointer(record.WithdrawalTargetAccountNumber),
		formatStringPointer(record.WithdrawalTargetSortCode),
		formatFloat64Pointer(record.TransferFee),
		formatFloat64Pointer(record.TransferTotalAmount),
		formatStringPointer(record.TransferCategory),
		formatStringPointer(record.TransferScheduledAt),
		record.SenderInfo,
		record.RecipientInfo,
		formatUintPointer(record.RecipientID),
		formatStringPointer(record.ExchangeFromCurrency),
		formatStringPointer(record.ExchangeToCurrency),
		formatFloat64Pointer(record.ExchangeAmountFrom),
		formatFloat64Pointer(record.ExchangeAmountTo),
		formatFloat64Pointer(record.ExchangeRate),
		formatFloat64Pointer(record.ExchangeFees),
		formatStringPointer(record.ExchangeReceiverDetails),
		formatStringPointer(record.RelatedParty),
	}

	if err := w.Write(row); err != nil {
		return fmt.Errorf("failed to write CSV row for reference %s: %w", record.Reference, err)
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("CSV writer error: %w", err)
	}

	// Get the object
	bucket := client.Bucket(s.config.GCSBucketName)
	obj := bucket.Object(objectName)

	// Check if file exists
	_, err = obj.Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			// File doesn't exist, create it with header
			wc := obj.NewWriter(ctx)
			wc.ContentType = "text/csv"

			// Write header
			header := []string{
				"UserID", "Timestamp", "Type", "Amount", "Currency", "Status", "Description", "Reference",
				"FailureReason", "CompletedAt", "ProcessingAt", "FailedAt", "ExternalTransactionID",
				"FromAccountID", "ToAccountID",
				"DepositSourceBankName",
				"WithdrawalTargetBankName", "WithdrawalTargetAccountNumber", "WithdrawalTargetSortCode",
				"TransferFee", "TransferTotalAmount", "TransferCategory", "TransferScheduledAt",
				"SenderInfo", "RecipientInfo", "RecipientID",
				"ExchangeFromCurrency", "ExchangeToCurrency", "ExchangeAmountFrom", "ExchangeAmountTo",
				"ExchangeRate", "ExchangeFees", "ExchangeReceiverDetails",
				"RelatedParty",
			}
			if err := w.Write(header); err != nil {
				wc.Close()
				return fmt.Errorf("failed to write CSV header: %w", err)
			}
			w.Flush()

			// Write the record
			if _, err := wc.Write(buf.Bytes()); err != nil {
				wc.Close()
				return fmt.Errorf("failed to write initial record: %w", err)
			}

			if err := wc.Close(); err != nil {
				return fmt.Errorf("failed to close writer: %w", err)
			}

			fmt.Printf("Created new transaction file for user %s and wrote first record\n", userIDStr)
		} else {
			return fmt.Errorf("failed to check if file exists: %w", err)
		}
	} else {
		// File exists, we need to read it, append the new record, and write it back
		reader, err := obj.NewReader(ctx)
		if err != nil {
			return fmt.Errorf("failed to create reader for existing file: %w", err)
		}
		defer reader.Close()

		// Read existing content
		existingContent, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("failed to read existing file content: %w", err)
		}

		// Create a new writer
		wc := obj.NewWriter(ctx)
		wc.ContentType = "text/csv"

		// Write existing content
		if _, err := wc.Write(existingContent); err != nil {
			wc.Close()
			return fmt.Errorf("failed to write existing content: %w", err)
		}

		// Write the new record
		if _, err := wc.Write(buf.Bytes()); err != nil {
			wc.Close()
			return fmt.Errorf("failed to append new record: %w", err)
		}

		if err := wc.Close(); err != nil {
			return fmt.Errorf("failed to close writer: %w", err)
		}

		fmt.Printf("Appended transaction record to existing file for user %s\n", userIDStr)
	}

	// After successfully appending the transaction, trigger AI indexing
	if err := s.TriggerAddTransactionIndexing(ctx, userID, record); err != nil {
		log.Printf("WARN: Failed to trigger add-transaction indexing for User ID %d: %v", userID, err)
		// Don't return error as this is not critical for the transaction processing
	}

	return nil
}

// TriggerAddTransactionIndexing sends individual transaction data to the AI service for add-transaction indexing
func (s *GenerateTxDataService) TriggerAddTransactionIndexing(ctx context.Context, userID uint, record TransactionRecord) error {
	userIDStr := strconv.FormatUint(uint64(userID), 10)
	log.Printf("INFO: TriggerAddTransactionIndexing: Starting for User ID: %s", userIDStr)

	if s.config.AiServiceURL == "" {
		log.Printf("ERROR: TriggerAddTransactionIndexing: AI service URL is not configured")
		return fmt.Errorf("AI service URL is not configured")
	}

	// Helper function to format time pointer
	formatTimePointer := func(t *time.Time) *string {
		if t == nil {
			return nil
		}
		if t.IsZero() {
			return nil
		}
		s := t.UTC().Format(time.RFC3339)
		return &s
	}

	// Prepare simplified transaction object
	transaction := map[string]interface{}{
		"timestamp":                        record.Timestamp.Format(time.RFC3339),
		"type":                             record.Type,
		"amount":                           record.Amount,
		"currency":                         record.Currency,
		"status":                           record.Status,
		"description":                      record.Description,
		"reference":                        record.Reference,
		"failure_reason":                   record.FailureReason,
		"completed_at":                     formatTimePointer(record.CompletedAt),
		"processing_at":                    formatTimePointer(record.ProcessingAt),
		"failed_at":                        formatTimePointer(record.FailedAt),
		"external_transaction_id":          record.ExternalTransactionID,
		"from_account_id":                  record.FromAccountID,
		"to_account_id":                    record.ToAccountID,
		"deposit_source_bank_name":         record.DepositSourceBankName,
		"withdrawal_target_bank_name":      record.WithdrawalTargetBankName,
		"withdrawal_target_account_number": record.WithdrawalTargetAccountNumber,
		"withdrawal_target_sort_code":      record.WithdrawalTargetSortCode,
		"transfer_fee":                     record.TransferFee,
		"transfer_total_amount":            record.TransferTotalAmount,
		"transfer_category":                record.TransferCategory,
		"transfer_scheduled_at":            record.TransferScheduledAt,
		"sender_info":                      record.SenderInfo,
		"recipient_info":                   record.RecipientInfo,
		"recipient_id":                     record.RecipientID,
		"exchange_from_currency":           record.ExchangeFromCurrency,
		"exchange_to_currency":             record.ExchangeToCurrency,
		"exchange_amount_from":             record.ExchangeAmountFrom,
		"exchange_amount_to":               record.ExchangeAmountTo,
		"exchange_rate":                    record.ExchangeRate,
		"exchange_fees":                    record.ExchangeFees,
		"exchange_receiver_details":        record.ExchangeReceiverDetails,
		"related_party":                    record.RelatedParty,
	}

	// Prepare simplified payload with just user_id and transaction
	payload := map[string]interface{}{
		"user_id":     userIDStr,
		"transaction": transaction,
	}

	// Make HTTP POST Request to /api/add-transaction
	addTransactionEndpointURL := s.config.AiServiceURL + "/api/add-transaction"
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal add-transaction payload: %w", err)
	}

	log.Printf("INFO: TriggerAddTransactionIndexing: Sending transaction to %s for User ID %s", addTransactionEndpointURL, userIDStr)
	log.Printf("INFO: TriggerAddTransactionIndexing: Payload: %s", string(payloadBytes))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", addTransactionEndpointURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create add-transaction request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := s.httpClient.Do(httpReq)
	if err != nil {
		if os.IsTimeout(err) {
			log.Printf("ERROR: TriggerAddTransactionIndexing: Request timed out for User ID %s: %v", userIDStr, err)
			return fmt.Errorf("add-transaction request timed out: %w", err)
		}
		if strings.Contains(err.Error(), "connection refused") {
			log.Printf("ERROR: TriggerAddTransactionIndexing: Connection refused for User ID %s: %v", userIDStr, err)
			return fmt.Errorf("AI add-transaction service not available: %w", err)
		}
		log.Printf("ERROR: TriggerAddTransactionIndexing: Failed to send request for User ID %s: %v", userIDStr, err)
		return fmt.Errorf("failed to send add-transaction request: %w", err)
	}
	defer httpResp.Body.Close()

	// Always read the response body
	bodyBytes, readErr := io.ReadAll(httpResp.Body)
	if readErr != nil {
		log.Printf("ERROR: TriggerAddTransactionIndexing: Failed to read response body for User ID %s: %v", userIDStr, readErr)
		return fmt.Errorf("failed to read response body: %w", readErr)
	}

	// Log response details
	log.Printf("INFO: TriggerAddTransactionIndexing: Received response for User ID %s - Status: %d, Body: %s",
		userIDStr, httpResp.StatusCode, string(bodyBytes))

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
		// Try to parse error response
		var errorResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(bodyBytes, &errorResp); err == nil && (errorResp.Error != "" || errorResp.Message != "") {
			errMsg := errorResp.Error
			if errMsg == "" {
				errMsg = errorResp.Message
			}
			log.Printf("ERROR: TriggerAddTransactionIndexing: AI service error for User ID %s: %s", userIDStr, errMsg)
			return fmt.Errorf("AI add-transaction service error: %s", errMsg)
		}

		return fmt.Errorf("add-transaction request failed with status %d: %s", httpResp.StatusCode, string(bodyBytes))
	}

	log.Printf("INFO: TriggerAddTransactionIndexing: Successfully triggered add-transaction indexing for User ID: %s", userIDStr)
	return nil
}

// TriggerTransactionFileIndexing sends the stored transaction public URL to the AI service.
func (s *GenerateTxDataService) TriggerTransactionFileIndexing(ctx context.Context, userID uint) error {
	userIDStr := strconv.FormatUint(uint64(userID), 10)
	log.Printf("INFO: TriggerTransactionFileIndexing: Starting for User ID: %s", userIDStr)

	// 1. Fetch Tx File Path (which is now the Public URL)
	var txFile models.UserTransactionFile
	txErr := s.db.WithContext(ctx).Where("user_id = ?", userID).Select("file_path").First(&txFile).Error
	if txErr != nil {
		if errors.Is(txErr, gorm.ErrRecordNotFound) {
			log.Printf("WARN: TriggerTransactionFileIndexing: Transaction file record not found for User ID %s. Skipping indexing.", userIDStr)
			return nil
		}
		log.Printf("ERROR: TriggerTransactionFileIndexing: Failed to fetch transaction file path for User ID %s: %v", userIDStr, txErr)
		return fmt.Errorf("db error fetching tx file path: %w", txErr)
	}

	// 2. Prepare Indexing Payload with the stored Public URL
	indexPayload := map[string]string{
		"file_path": txFile.FilePath,
		"user_id":   userIDStr,
	}

	// 3. Make HTTP POST Request
	indexEndpointURL := s.config.AiServiceURL + "/api/index_transactions"
	payloadBytes, err := json.Marshal(indexPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal index payload: %w", err)
	}

	log.Printf("INFO: TriggerTransactionFileIndexing: Sending request to %s with payload: %s", indexEndpointURL, string(payloadBytes))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", indexEndpointURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create index request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := s.httpClient.Do(httpReq)
	if err != nil {
		if os.IsTimeout(err) {
			log.Printf("ERROR: TriggerTransactionFileIndexing: Request timed out for User ID %s: %v", userIDStr, err)
			return fmt.Errorf("indexing request timed out: %w", err)
		}
		if strings.Contains(err.Error(), "connection refused") {
			log.Printf("ERROR: TriggerTransactionFileIndexing: Connection refused for User ID %s: %v", userIDStr, err)
			return fmt.Errorf("AI service not available: %w", err)
		}
		log.Printf("ERROR: TriggerTransactionFileIndexing: Failed to send request for User ID %s: %v", userIDStr, err)
		return fmt.Errorf("failed to send index request: %w", err)
	}
	defer httpResp.Body.Close()

	// Always read the response body
	bodyBytes, readErr := io.ReadAll(httpResp.Body)
	if readErr != nil {
		log.Printf("ERROR: TriggerTransactionFileIndexing: Failed to read response body for User ID %s: %v", userIDStr, readErr)
		return fmt.Errorf("failed to read response body: %w", readErr)
	}

	// Log response details
	log.Printf("INFO: TriggerTransactionFileIndexing: Received response for User ID %s - Status: %d, Body: %s",
		userIDStr, httpResp.StatusCode, string(bodyBytes))

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
		// Try to parse error response
		var errorResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(bodyBytes, &errorResp); err == nil && (errorResp.Error != "" || errorResp.Message != "") {
			errMsg := errorResp.Error
			if errMsg == "" {
				errMsg = errorResp.Message
			}
			log.Printf("ERROR: TriggerTransactionFileIndexing: AI service error for User ID %s: %s", userIDStr, errMsg)
			return fmt.Errorf("AI service error: %s", errMsg)
		}

		return fmt.Errorf("indexing request failed with status %d: %s", httpResp.StatusCode, string(bodyBytes))
	}

	log.Printf("INFO: TriggerTransactionFileIndexing: Successfully triggered indexing for User ID: %s", userIDStr)
	return nil
}

// TriggerAppendTransactionFileIndexing appends a transaction to the GCS file and then calls the AI service for indexing
func (s *GenerateTxDataService) TriggerAppendTransactionFileIndexing(ctx context.Context, userID uint, record TransactionRecord) error {
	userIDStr := strconv.FormatUint(uint64(userID), 10)
	log.Printf("INFO: TriggerAppendTransactionFileIndexing: Starting for User ID: %s", userIDStr)

	// Step 1: Append transaction to GCS file
	if err := s.appendTransactionToGCSFile(ctx, userID, record); err != nil {
		log.Printf("ERROR: TriggerAppendTransactionFileIndexing: Failed to append transaction to GCS file for User ID %s: %v", userIDStr, err)
		return fmt.Errorf("failed to append transaction to file: %w", err)
	}

	// Step 2: Send transaction data to AI service for indexing
	if err := s.TriggerAddTransactionIndexing(ctx, userID, record); err != nil {
		log.Printf("WARN: TriggerAppendTransactionFileIndexing: Failed to trigger AI indexing for User ID %s: %v", userIDStr, err)
		// Don't return error as this is not critical for the transaction processing
		// The file has been successfully updated, AI indexing failure shouldn't fail the whole operation
	}

	log.Printf("INFO: TriggerAppendTransactionFileIndexing: Completed for User ID: %s", userIDStr)
	return nil
}

// appendTransactionToGCSFile appends a single transaction record to the user's transaction file in GCS
func (s *GenerateTxDataService) appendTransactionToGCSFile(ctx context.Context, userID uint, record TransactionRecord) error {
	var client *storage.Client
	var err error

	if s.config.GCSBucketName == "" {
		return fmt.Errorf("GCS_BUCKET_NAME is not configured")
	}

	// Step 1: Construct the file path and public URL
	userIDStr := strconv.FormatUint(uint64(userID), 10)
	objectName := fmt.Sprintf("user-tx-data/%s/transactions.csv", userIDStr)
	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.config.GCSBucketName, objectName)

	// Step 2: Update or create the file path record in the database
	fileRecord := models.UserTransactionFile{
		UserID:   userID,
		FilePath: publicURL,
	}
	if errDb := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"file_path", "updated_at"}),
	}).Create(&fileRecord).Error; errDb != nil {
		log.Printf("ERROR: appendTransactionToGCSFile: Failed to save/update user transaction file public URL in DB for user %d: %v\n", userID, errDb)
		// Continue with the upload even if DB update fails
	}

	// Initialize GCS client
	gcpCredsEnv := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if gcpCredsEnv != "" {
		client, err = storage.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("storage.NewClient (ADC): %w", err)
		}
	} else if s.config.GCSCredentialsFile != "" {
		if _, statErr := os.Stat(s.config.GCSCredentialsFile); os.IsNotExist(statErr) {
			return fmt.Errorf("GCS credentials file from config not found at: %s", s.config.GCSCredentialsFile)
		}
		client, err = storage.NewClient(ctx, option.WithCredentialsFile(s.config.GCSCredentialsFile))
		if err != nil {
			return fmt.Errorf("storage.NewClient with credentials file from config: %w", err)
		}
	} else {
		return fmt.Errorf("GCS authentication failed: Neither GOOGLE_APPLICATION_CREDENTIALS env var nor GCS_CREDENTIALS_FILE config is set")
	}
	defer client.Close()

	// Format the record as CSV
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)

	// Helper functions for formatting pointers
	formatUintPointer := func(ptr *uint) string {
		if ptr == nil {
			return ""
		}
		return strconv.FormatUint(uint64(*ptr), 10)
	}
	formatFloat64Pointer := func(ptr *float64) string {
		if ptr == nil {
			return ""
		}
		return strconv.FormatFloat(*ptr, 'f', -1, 64)
	}
	formatStringPointer := func(ptr *string) string {
		if ptr == nil {
			return ""
		}
		return *ptr
	}
	formatTimePointer := func(ptr *time.Time) string {
		if ptr == nil {
			return ""
		}
		if ptr.IsZero() {
			return ""
		}
		return ptr.UTC().Format(time.RFC3339)
	}

	// Format the record
	row := []string{
		strconv.FormatUint(record.UserID, 10),
		formatTimePointer(&record.Timestamp),
		record.Type,
		strconv.FormatFloat(record.Amount, 'f', 2, 64),
		record.Currency,
		record.Status,
		record.Description,
		record.Reference,
		formatStringPointer(record.FailureReason),
		formatTimePointer(record.CompletedAt),
		formatTimePointer(record.ProcessingAt),
		formatTimePointer(record.FailedAt),
		formatStringPointer(record.ExternalTransactionID),
		formatUintPointer(record.FromAccountID),
		formatUintPointer(record.ToAccountID),
		formatStringPointer(record.DepositSourceBankName),
		formatStringPointer(record.WithdrawalTargetBankName),
		formatStringPointer(record.WithdrawalTargetAccountNumber),
		formatStringPointer(record.WithdrawalTargetSortCode),
		formatFloat64Pointer(record.TransferFee),
		formatFloat64Pointer(record.TransferTotalAmount),
		formatStringPointer(record.TransferCategory),
		formatStringPointer(record.TransferScheduledAt),
		record.SenderInfo,
		record.RecipientInfo,
		formatUintPointer(record.RecipientID),
		formatStringPointer(record.ExchangeFromCurrency),
		formatStringPointer(record.ExchangeToCurrency),
		formatFloat64Pointer(record.ExchangeAmountFrom),
		formatFloat64Pointer(record.ExchangeAmountTo),
		formatFloat64Pointer(record.ExchangeRate),
		formatFloat64Pointer(record.ExchangeFees),
		formatStringPointer(record.ExchangeReceiverDetails),
		formatStringPointer(record.RelatedParty),
	}

	if err := w.Write(row); err != nil {
		return fmt.Errorf("failed to write CSV row for reference %s: %w", record.Reference, err)
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("CSV writer error: %w", err)
	}

	// Get the object
	bucket := client.Bucket(s.config.GCSBucketName)
	obj := bucket.Object(objectName)

	// Check if file exists
	_, err = obj.Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			// File doesn't exist, create it with header
			wc := obj.NewWriter(ctx)
			wc.ContentType = "text/csv"

			// Write header
			header := []string{
				"UserID", "Timestamp", "Type", "Amount", "Currency", "Status", "Description", "Reference",
				"FailureReason", "CompletedAt", "ProcessingAt", "FailedAt", "ExternalTransactionID",
				"FromAccountID", "ToAccountID",
				"DepositSourceBankName",
				"WithdrawalTargetBankName", "WithdrawalTargetAccountNumber", "WithdrawalTargetSortCode",
				"TransferFee", "TransferTotalAmount", "TransferCategory", "TransferScheduledAt",
				"SenderInfo", "RecipientInfo", "RecipientID",
				"ExchangeFromCurrency", "ExchangeToCurrency", "ExchangeAmountFrom", "ExchangeAmountTo",
				"ExchangeRate", "ExchangeFees", "ExchangeReceiverDetails",
				"RelatedParty",
			}
			if err := w.Write(header); err != nil {
				wc.Close()
				return fmt.Errorf("failed to write CSV header: %w", err)
			}
			w.Flush()

			// Write the record
			if _, err := wc.Write(buf.Bytes()); err != nil {
				wc.Close()
				return fmt.Errorf("failed to write initial record: %w", err)
			}

			if err := wc.Close(); err != nil {
				return fmt.Errorf("failed to close writer: %w", err)
			}

			log.Printf("INFO: appendTransactionToGCSFile: Created new transaction file for user %s and wrote first record", userIDStr)
		} else {
			return fmt.Errorf("failed to check if file exists: %w", err)
		}
	} else {
		// File exists, we need to read it, append the new record, and write it back
		reader, err := obj.NewReader(ctx)
		if err != nil {
			return fmt.Errorf("failed to create reader for existing file: %w", err)
		}
		defer reader.Close()

		// Read existing content
		existingContent, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("failed to read existing file content: %w", err)
		}

		// Create a new writer
		wc := obj.NewWriter(ctx)
		wc.ContentType = "text/csv"

		// Write existing content
		if _, err := wc.Write(existingContent); err != nil {
			wc.Close()
			return fmt.Errorf("failed to write existing content: %w", err)
		}

		// Write the new record
		if _, err := wc.Write(buf.Bytes()); err != nil {
			wc.Close()
			return fmt.Errorf("failed to append new record: %w", err)
		}

		if err := wc.Close(); err != nil {
			return fmt.Errorf("failed to close writer: %w", err)
		}

		log.Printf("INFO: appendTransactionToGCSFile: Appended transaction record to existing file for user %s", userIDStr)
	}

	return nil
}
