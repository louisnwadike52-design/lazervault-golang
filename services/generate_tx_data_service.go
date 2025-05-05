package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt" // Need ioutil again for ReadFile if < Go 1.16, or use os.ReadFile
	"os"  // Use os.ReadFile if Go >= 1.16
	"strconv"
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
	db     *gorm.DB
	config configs.Config
}

// NewGenerateTxDataService creates a new GenerateTxDataService
func NewGenerateTxDataService(db *gorm.DB, config configs.Config) *GenerateTxDataService {
	return &GenerateTxDataService{db: db, config: config}
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
	TransferFee         *float64   `json:"transfer_fee,omitempty"`
	TransferTotalAmount *float64   `json:"transfer_total_amount,omitempty"`
	TransferCategory    *string    `json:"transfer_category,omitempty"`
	TransferScheduledAt *time.Time `json:"transfer_scheduled_at,omitempty"`
	SenderInfo          string     `json:"sender_info,omitempty"`    // Generated for TRANSFER_IN
	RecipientInfo       string     `json:"recipient_info,omitempty"` // Generated for TRANSFER_OUT
	RecipientID         *uint      `json:"recipient_id,omitempty"`   // From Transfer model

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
			TransferScheduledAt: timePtr(t.ScheduledAt),
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
			TransferScheduledAt: timePtr(t.ScheduledAt),
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
		return ptr.Format(time.RFC3339)
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
			formatTimePointer(rec.TransferScheduledAt),
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

// UploadOrOverwriteTxFile uploads the data to GCS, overwriting if it exists.
// Renamed from uploadOrOverwriteTxFile to be public for the processor.
func (s *GenerateTxDataService) UploadOrOverwriteTxFile(ctx context.Context, userID string, data *bytes.Buffer) error {
	var client *storage.Client
	var err error

	if s.config.GCSBucketName == "" {
		return fmt.Errorf("GCS_BUCKET_NAME is not configured")
	}

	// --- Client Initialization Logic --- // Simplified
	gcpCredsEnv := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if gcpCredsEnv != "" {
		client, err = storage.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("storage.NewClient (ADC): %w", err)
		}
	} else if s.config.GCSCredentialsFile != "" {
		fmt.Printf("BG Task: GOOGLE_APPLICATION_CREDENTIALS not set. Using GCS credentials file from config: %s\n", s.config.GCSCredentialsFile)
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

	// --- File Upload Logic ---
	objectName := fmt.Sprintf("user-tx-data/%s/transactions.csv", userID)
	bucket := client.Bucket(s.config.GCSBucketName)
	obj := bucket.Object(objectName)

	wc := obj.NewWriter(ctx)
	wc.ContentType = "text/csv"

	if _, err := wc.Write(data.Bytes()); err != nil {
		wc.Close() // Close writer even on error
		return fmt.Errorf("wc.Write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("wc.Close: %w", err)
	}

	fmt.Printf("BG Task: File uploaded/overwritten at gs://%s/%s\n", s.config.GCSBucketName, objectName)
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
