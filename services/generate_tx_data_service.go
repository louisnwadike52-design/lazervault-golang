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
type TransactionRecord struct {
	Timestamp    time.Time
	Type         string // "DEPOSIT", "WITHDRAWAL", "TRANSFER_OUT", "TRANSFER_IN", "EXCHANGE"
	Amount       float64
	Currency     string
	Status       string
	Description  string
	RelatedParty string // Recipient Name/Account, Source, Exchange Pair etc.
	Reference    string
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

	allRecords, err := s.FetchAllUserTransactions(ctx, accountIDs)
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
// Renamed from fetchAllUserTransactions to be public for the processor.
func (s *GenerateTxDataService) FetchAllUserTransactions(ctx context.Context, accountIDs []uint) ([]TransactionRecord, error) {
	var records []TransactionRecord

	// Fetch Deposits
	var deposits []models.Deposit
	if err := s.db.WithContext(ctx).Where("target_account_id IN ?", accountIDs).Order("created_at desc").Find(&deposits).Error; err != nil {
		return nil, fmt.Errorf("failed to query deposits: %w", err)
	}
	for _, d := range deposits {
		amountFloat := float64(d.Amount) / 100.0
		records = append(records, TransactionRecord{
			Timestamp:    d.CreatedAt,
			Type:         "DEPOSIT",
			Amount:       amountFloat,
			Currency:     d.Currency,
			Status:       string(d.Status),
			Description:  "Deposit from " + d.SourceBankName,
			RelatedParty: d.SourceBankName,
			Reference:    d.ID,
		})
	}

	// Fetch Withdrawals
	var withdrawals []models.Withdrawal
	if err := s.db.WithContext(ctx).Where("source_account_id IN ?", accountIDs).Order("created_at desc").Find(&withdrawals).Error; err != nil {
		return nil, fmt.Errorf("failed to query withdrawals: %w", err)
	}
	for _, w := range withdrawals {
		amountFloat := float64(w.Amount) / 100.0
		records = append(records, TransactionRecord{
			Timestamp:    w.CreatedAt,
			Type:         "WITHDRAWAL",
			Amount:       amountFloat,
			Currency:     w.Currency,
			Status:       string(w.Status),
			Description:  fmt.Sprintf("Withdrawal to %s (%s)", w.TargetBankName, w.TargetAccountNumber),
			RelatedParty: w.TargetBankName,
			Reference:    w.TransactionReference,
		})
	}

	// Fetch Transfers (Outgoing)
	var outgoingTransfers []models.Transfer
	if err := s.db.WithContext(ctx).Where("from_account_id IN ?", accountIDs).Preload("ToUser").Preload("Recipient").Order("created_at desc").Find(&outgoingTransfers).Error; err != nil {
		return nil, fmt.Errorf("failed to query outgoing transfers: %w", err)
	}
	for _, t := range outgoingTransfers {
		recipientInfo := "Unknown Recipient"
		if t.ToUser != nil {
			recipientInfo = t.ToUser.FirstName + " " + t.ToUser.LastName
			if recipientInfo == " " {
				recipientInfo = t.ToUser.Email
			}
		} else if t.Recipient != nil {
			recipientInfo = fmt.Sprintf("%s (%s)", t.Recipient.Name, t.Recipient.AccountNumber)
		} else if t.ToAccountID != nil {
			recipientInfo = fmt.Sprintf("Account ID: %d", *t.ToAccountID)
		}

		amountFloat := float64(t.Amount) / 100.0
		var fromAccount models.Account
		if err := s.db.WithContext(ctx).Select("currency").First(&fromAccount, t.FromAccountID).Error; err == nil {
			accountCurrency := fromAccount.Currency
			records = append(records, TransactionRecord{
				Timestamp:    t.CreatedAt,
				Type:         "TRANSFER_OUT",
				Amount:       amountFloat,
				Currency:     accountCurrency,
				Status:       string(t.Status),
				Description:  "Transfer to " + recipientInfo,
				RelatedParty: recipientInfo,
				Reference:    t.Reference,
			})
		} else {
			// Log that this transfer record is skipped due to missing currency info
			fmt.Printf("Warning: Skipping outgoing transfer record (ID: %s) due to error fetching FromAccount currency: %v\n", t.Reference, err)
		}
	}

	// Fetch Transfers (Incoming)
	var incomingTransfers []models.Transfer
	if err := s.db.WithContext(ctx).Where("to_account_id IN ?", accountIDs).Preload("FromUser").Order("created_at desc").Find(&incomingTransfers).Error; err != nil {
		return nil, fmt.Errorf("failed to query incoming transfers: %w", err)
	}
	for _, t := range incomingTransfers {
		senderInfo := "Unknown Sender"
		if t.FromUser.ID != 0 {
			senderInfo = t.FromUser.FirstName + " " + t.FromUser.LastName
			if senderInfo == " " {
				senderInfo = t.FromUser.Email
			}
		} else {
			senderInfo = fmt.Sprintf("Account ID: %d", t.FromAccountID)
		}

		amountFloat := float64(t.Amount) / 100.0
		var toAccount models.Account
		if t.ToAccountID != nil {
			if err := s.db.WithContext(ctx).Select("currency").First(&toAccount, *t.ToAccountID).Error; err == nil {
				accountCurrency := toAccount.Currency
				records = append(records, TransactionRecord{
					Timestamp:    t.CreatedAt,
					Type:         "TRANSFER_IN",
					Amount:       amountFloat,
					Currency:     accountCurrency,
					Status:       string(t.Status),
					Description:  "Transfer from " + senderInfo,
					RelatedParty: senderInfo,
					Reference:    t.Reference,
				})
			} else {
				// Log that this transfer record is skipped due to missing currency info
				fmt.Printf("Warning: Skipping incoming transfer record (ID: %s) due to error fetching ToAccount currency: %v\n", t.Reference, err)
			}
		} else {
			// Log that this transfer record is skipped because ToAccountID was nil (should be rare)
			fmt.Printf("Warning: Skipping incoming transfer record (ID: %s) because ToAccountID is nil\n", t.Reference)
		}
	}

	// Fetch Exchange Transactions
	var exchanges []models.ExchangeTransaction
	var ownerUserID uint // Keep as uint based on Account model

	if len(accountIDs) > 0 {
		var account models.Account
		// Fetch the uint owner_user_id from the account
		if err := s.db.WithContext(ctx).Select("owner_user_id").First(&account, accountIDs[0]).Error; err == nil {
			ownerUserID = account.OwnerUserID
		} else if err != gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("failed to get owner user ID for account %d: %w", accountIDs[0], err)
		}
	}

	// Check if we found a valid ownerUserID
	if ownerUserID == 0 {
		// Depending on requirements, you might want to try other accountIDs or return an error.
		fmt.Println("Warning: Could not determine owner_user_id from provided account IDs, skipping exchange transaction fetch.")
	} else {
		// Query using the uint ownerUserID directly.
		// This relies on GORM/pgx/postgres implicitly handling the comparison
		// between uint and the uuid column, which caused the original error.
		// NOTE: This might still fail depending on driver/DB configuration.
		// The RECOMMENDED fix is to make User.ID and ExchangeTransaction.UserID types consistent.
		if err := s.db.WithContext(ctx).Where("user_id = ?", ownerUserID).Order("created_at desc").Find(&exchanges).Error; err != nil {
			// Handle potential errors during the query itself
			return nil, fmt.Errorf("failed to query exchanges for user %d: %w", ownerUserID, err)
		}
		for _, ex := range exchanges {
			records = append(records, TransactionRecord{
				Timestamp:    ex.CreatedAt,
				Type:         "EXCHANGE",
				Amount:       ex.AmountTo,
				Currency:     ex.ToCurrency,
				Status:       ex.Status,
				Description:  fmt.Sprintf("Exchange %.2f %s -> %.2f %s (Rate: %.6f)", ex.AmountFrom, ex.FromCurrency, ex.AmountTo, ex.ToCurrency, ex.ExchangeRate),
				RelatedParty: fmt.Sprintf("%s/%s", ex.FromCurrency, ex.ToCurrency),
				Reference:    ex.ID, // Use string ID directly as inferred from linter
			})
		}
	}

	// TODO: Add Failed Deposits, Invoices if they represent user-facing financial events

	// Sort all records chronologically (most recent first) - might already be mostly sorted
	// Consider a more robust sort if exact order across types is critical
	// sort.SliceStable(records, func(i, j int) bool {
	// 	return records[i].Timestamp.After(records[j].Timestamp)
	// })

	return records, nil
}

// FormatTransactionsToCSV formats records into a CSV buffer.
// Renamed from formatTransactionsToCSV to be public for the processor.
func (s *GenerateTxDataService) FormatTransactionsToCSV(records []TransactionRecord) (*bytes.Buffer, error) {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)

	// Write header
	header := []string{"Timestamp", "Type", "Amount", "Currency", "Status", "Description", "RelatedParty", "Reference"}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	// Write records
	for _, r := range records {
		row := []string{
			r.Timestamp.Format(time.RFC3339), // ISO 8601 format
			r.Type,
			strconv.FormatFloat(r.Amount, 'f', 2, 64), // Format amount to 2 decimal places
			r.Currency,
			r.Status,
			r.Description,
			r.RelatedParty,
			r.Reference,
		}
		if err := writer.Write(row); err != nil {
			// Log the error but potentially continue? Or fail? Let's fail for now.
			return nil, fmt.Errorf("failed to write record %+v: %w", r, err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buffer, nil
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
