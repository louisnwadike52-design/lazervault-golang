package grpcApi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"lazervaultGo/configs"
	"lazervaultGo/grpcApi/middleware"
	securityMiddleware "lazervaultGo/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"lazervaultGo/worker"
	"net"
	"net/http"
	"strings"
	"time"

	"os"

	authpb "auth-service/proto"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type Server struct {
	config                 *configs.Config
	db                     *gorm.DB
	tokenMaker             token.Maker
	grpcServer             *grpc.Server
	httpServer             *http.Server
	redisWorker            *worker.RedisWorker
	voiceSessionController *VoiceSessionController
	userService            services.IUserService
	qrCodeService          services.IQRCodeService
	transferService        services.ITransferService
}

func NewServer(
	db *gorm.DB,
	config *configs.Config,
	tokenMaker token.Maker,
	redisWorker *worker.RedisWorker,
) (*Server, error) {
	if redisWorker == nil {
		return nil, errors.New("redisWorker dependency cannot be nil in NewServer")
	}

	server := &Server{
		config:      config,
		db:          db,
		tokenMaker:  tokenMaker,
		redisWorker: redisWorker,
	}

	// Initialize auth service client for token validation
	authServiceAddr := getEnv("AUTH_SERVICE_GRPC_ADDR", "localhost:50051")
	if err := middleware.InitAuthServiceClient(authServiceAddr); err != nil {
		return nil, fmt.Errorf("failed to init auth service client: %w", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(middleware.AuthInterceptor()),
	)

	// Initialize services
	distributor := redisWorker.GetDistributor()
	authService := services.NewAuthService(db, config, tokenMaker, distributor)
	accountService := services.NewAccountService(db, distributor)
	recipientService := services.NewRecipientService(db)
	transferService := services.NewTransferService(db, config, distributor, recipientService, accountService)
	// accountCardService := services.NewAccountCardService(db, config)
	cardService := services.NewCardService(db)
	chatService := services.NewChatService(db)
	userService := services.NewUserService(db, config, tokenMaker)
	exchangeService := services.NewExchangeService(db, distributor)
	notificationService := services.NewNotificationService(db)
	invoiceService := services.NewInvoiceService(db, distributor, notificationService)
	depositService := services.NewDepositService(db, distributor, accountService)
	withdrawalService := services.NewWithdrawalService(db, distributor)
	generateTxDataService := services.NewGenerateTxDataService(db, *config)
	generateTxDataController := NewGenerateTxDataController(*generateTxDataService, tokenMaker, db)
	txFileService := services.NewTxFileService(db)
	txFileController := NewTxFileController(txFileService, tokenMaker, db)

	// Initialize AI Chat Service
	aiChatService := services.NewAIChatService(db, config, distributor)

	// Initialize AI Chat Controller
	aiChatController := NewAIChatController(aiChatService, userService)

	// Initialize Group Account Service
	groupAccountService := services.NewGroupAccountService(db)

	// Initialize Group Account Controller
	groupAccountController := NewGroupAccountController(groupAccountService, userService)

	// Initialize Crowdfund Service
	crowdfundService := services.NewCrowdfundService(db)

	// Initialize Crowdfund Controller
	crowdfundController := NewCrowdfundController(crowdfundService, userService)

	// Initialize Crypto Service
	cryptoService := services.NewCryptoService()

	// Initialize Crypto Controller
	cryptoController := NewCryptoController(cryptoService)

	// Initialize Gift Card Service
	giftCardService := services.NewGiftCardService()

	// Initialize Gift Card Controller
	giftCardController := NewGiftCardController(giftCardService, userService)

	// Initialize Stock Service
	stockService := services.NewStockService()

	// Initialize Stock Controller
	stockController := NewStockController(stockService, userService)

	// Initialize Statistics Service
	statisticsService := services.NewStatisticsService(db)

	// Initialize AI Statistics Service
	aiStatisticsService := services.NewAIStatisticsService(db, config, statisticsService, aiChatService)

	// Initialize Wrapped Service (Financial Wrapped experience)
	wrappedService := services.NewWrappedService(db, config, statisticsService)

	// Initialize Portfolio Service
	portfolioService := services.NewPortfolioService(db)

	// Initialize Portfolio Controller
	portfolioController := NewPortfolioController(portfolioService)

	// Initialize Tag Pay Service
	tagPayService := services.NewTagPayService(db)

	// Initialize Barcode Payment Service
	barcodePaymentService := services.NewBarcodePaymentService(db)

	// Initialize Support Controller
	supportController := NewSupportController(db, userService)

	// Initialize Statistics Controller
	statisticsController := NewStatisticsController(db, userService, aiStatisticsService, wrappedService)

	// Initialize AI Scan Service
	aiScanService := services.NewAiScanService(db, config)

	// Initialize AI Scan Controller
	aiScanController := NewAiScanController(aiScanService)

	// Initialize Voice Session Service
	voiceSessionService := services.NewVoiceSessionService(db, config, tokenMaker, aiChatService)

	// Initialize Voice Session Controller
	voiceSessionController := NewVoiceSessionController(voiceSessionService, userService)

	// Initialize Facial Recognition Service
	facialRecognitionService := services.NewFacialRecognitionService(config.AiServiceURL)

	// Initialize Referral Service
	referralService := services.NewReferralService(db)

	// Initialize Referral Controller
	referralController := NewReferralController(referralService, userService, db)

	// Initialize Facial Recognition Controller
	facialRecognitionController := NewFacialRecognitionController(facialRecognitionService)

	// Initialize Invoice Payment Service
	invoicePaymentService := services.NewInvoicePaymentService(db, notificationService)

	// Initialize Invoice Payment Controller
	invoicePaymentController := NewInvoicePaymentController(invoicePaymentService, userService, db)

	// Initialize Tagged Invoice Service
	taggedInvoiceService := services.NewTaggedInvoiceService(db)

	// Initialize Tagged Invoice Controller
	taggedInvoiceController := NewTaggedInvoiceController(taggedInvoiceService, userService, db)

	// Initialize Invoice Notification Service
	invoiceNotificationService := services.NewInvoiceNotificationService(db)

	// Initialize Invoice Notification Controller (not implemented yet)
	_ = invoiceNotificationService

	// Initialize Insurance Service
	insuranceService := services.NewInsuranceService(db, distributor)

	// Initialize Insurance Controller
	insuranceController := NewInsuranceController(insuranceService)

	// Initialize Lock Funds Service
	lockFundsService := services.NewLockFundsService(db)

	// Initialize Lock Funds Controller
	lockFundsController := NewLockFundsController(lockFundsService, userService)

	// Initialize Contact Sync Service
	contactSyncService := services.NewContactSyncService(db)

	// Initialize Contact Sync Controller
	contactSyncController := NewContactSyncController(contactSyncService, userService)

	// Initialize Auto-Save Service
	autoSaveService := services.NewAutoSaveService(db, redisWorker.GetDistributor(), accountService, transferService)

	// Initialize Auto-Save Controller
	autoSaveController := NewAutoSaveController(autoSaveService, userService)

	// Initialize Invoice Controller
	invoiceController := NewInvoiceController(invoiceService, userService)

	// Initialize QR Code Service
	qrCodeService := services.NewQRCodeService(db, config)

	// Initialize Bill Payment Providers
	flutterwaveConfig := services.FlutterwaveConfig{
		SecretKey: config.FlutterwaveSecretKey,
		PublicKey: config.FlutterwavePublicKey,
		BaseURL:   config.FlutterwaveBaseURL,
		Enabled:   config.FlutterwaveEnabled,
	}
	flutterwaveClient := services.NewFlutterwaveBillClient(flutterwaveConfig)

	paystackConfig := services.PaystackConfig{
		SecretKey: config.PaystackSecretKey,
		PublicKey: config.PaystackPublicKey,
		BaseURL:   config.PaystackBaseURL,
		Enabled:   config.PaystackEnabled,
	}
	paystackClient := services.NewPaystackBillClient(paystackConfig)

	billProviderFactory := services.NewBillPaymentProviderFactory(flutterwaveClient, paystackClient)

	// Initialize Electricity Bill Service
	electricityBillService := services.NewElectricityBillService(db, billProviderFactory, distributor)

	// Store controllers and services in server for HTTP handlers
	server.voiceSessionController = voiceSessionController
	server.userService = userService
	server.qrCodeService = qrCodeService
	server.transferService = transferService

	// Store facial recognition controller for custom HTTP handlers if needed
	_ = facialRecognitionController

	// Register gRPC services
	pb.RegisterAuthServiceServer(grpcServer, NewAuthController(authService))
	pb.RegisterUserServiceServer(grpcServer, NewUserController(server))
	pb.RegisterTransferServiceServer(grpcServer, NewTransferController(transferService, db))
	pb.RegisterAccountServiceServer(grpcServer, NewAccountController(accountService, userService))
	pb.RegisterAccountCardServiceServer(grpcServer, NewAccountCardController(cardService, userService))
	pb.RegisterRecipientServiceServer(grpcServer, NewRecipientController(recipientService, userService))
	pb.RegisterChatServiceServer(grpcServer, NewChatController(chatService))
	pb.RegisterExchangeServiceServer(grpcServer, NewExchangeController(exchangeService, userService))
	pb.RegisterInvoiceServiceServer(grpcServer, invoiceController)
	pb.RegisterDepositServiceServer(grpcServer, NewDepositController(depositService, userService, db))
	pb.RegisterWithdrawServiceServer(grpcServer, NewWithdrawalController(withdrawalService, userService))
	pb.RegisterGenerateTxDataServiceServer(grpcServer, generateTxDataController)
	pb.RegisterTxFileServiceServer(grpcServer, txFileController)
	pb.RegisterAIChatServiceServer(grpcServer, aiChatController)
	pb.RegisterVoiceSessionServiceServer(grpcServer, voiceSessionController)
	pb.RegisterFacialRecognitionServiceServer(grpcServer, facialRecognitionService)
	pb.RegisterInvoicePaymentServiceServer(grpcServer, invoicePaymentController)
	pb.RegisterTaggedInvoiceServiceServer(grpcServer, taggedInvoiceController)
	pb.RegisterInsuranceServiceServer(grpcServer, insuranceController)
	pb.RegisterLockFundsServiceServer(grpcServer, lockFundsController)
	pb.RegisterContactSyncServiceServer(grpcServer, contactSyncController)
	pb.RegisterGroupAccountServiceServer(grpcServer, groupAccountController)
	pb.RegisterCrowdfundServiceServer(grpcServer, crowdfundController)
	pb.RegisterCryptoServiceServer(grpcServer, cryptoController)
	pb.RegisterGiftCardServiceServer(grpcServer, giftCardController)
	pb.RegisterStockServiceServer(grpcServer, stockController)
	pb.RegisterStatisticsServiceServer(grpcServer, statisticsController)
	pb.RegisterPortfolioServiceServer(grpcServer, portfolioController)
	pb.RegisterAiScanServiceServer(grpcServer, aiScanController)
	pb.RegisterTagPayServiceServer(grpcServer, tagPayService)
	pb.RegisterBarcodePaymentServiceServer(grpcServer, barcodePaymentService)
	pb.RegisterElectricityBillServiceServer(grpcServer, electricityBillService)
	pb.RegisterSupportServiceServer(grpcServer, supportController)
	pb.RegisterAutoSaveServiceServer(grpcServer, autoSaveController)
	pb.RegisterReferralServiceServer(grpcServer, referralController)
	server.grpcServer = grpcServer

	// Register reflection service on gRPC server.
	reflection.Register(grpcServer)

	return server, nil
}

func (s *Server) Start() error {
	// Start gRPC server
	grpcAddr := fmt.Sprintf(":%s", s.config.GRPCServerPort)
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC port %s: %w", s.config.GRPCServerPort, err)
	}
	fmt.Printf("Starting gRPC server on %s\n", grpcAddr)
	go func() {
		if err := s.grpcServer.Serve(listener); err != nil {
			panic(fmt.Sprintf("failed to serve gRPC: %v", err))
		}
	}()

	// Start HTTP gateway server
	return s.startHTTPServer()
}

func (s *Server) startHTTPServer() error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	gwmux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(customHeaderMatcher),
		runtime.WithOutgoingHeaderMatcher(customHeaderMatcher),
	)

	// The gRPC server is now on its own port again.
	grpcDialAddr := fmt.Sprintf("localhost:%s", s.config.GRPCServerPort)
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	// Register service handlers
	if err := pb.RegisterAuthServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register auth gateway: %w", err)
	}
	if err := pb.RegisterUserServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register user gateway: %w", err)
	}
	if err := pb.RegisterTransferServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register transfer gateway: %w", err)
	}
	if err := pb.RegisterAccountServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register account gateway: %w", err)
	}
	if err := pb.RegisterAccountCardServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register account card gateway: %w", err)
	}
	if err := pb.RegisterRecipientServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register recipient gateway: %w", err)
	}
	if err := pb.RegisterChatServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register chat gateway: %w", err)
	}
	if err := pb.RegisterExchangeServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register exchange gateway: %w", err)
	}
	if err := pb.RegisterInvoiceServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register invoice gateway: %w", err)
	}
	if err := pb.RegisterDepositServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register deposit gateway: %w", err)
	}
	if err := pb.RegisterWithdrawServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register withdraw gateway: %w", err)
	}
	if err := pb.RegisterAIChatServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register ai chat gateway: %w", err)
	}

	// Register remaining service handlers
	if err := pb.RegisterGenerateTxDataServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register generate tx data gateway: %w", err)
	}
	if err := pb.RegisterTxFileServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register tx file service gateway: %w", err)
	}
	if err := pb.RegisterVoiceSessionServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register voice session service gateway: %w", err)
	}
	// Note: Invoice Payment and Tagged Invoice services don't have HTTP gateway support yet
	// To enable HTTP endpoints, add google.api.http annotations to the proto files
	if err := pb.RegisterFacialRecognitionServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register facial recognition service gateway: %w", err)
	}
	if err := pb.RegisterInsuranceServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register insurance gateway: %w", err)
	}
	if err := pb.RegisterContactSyncServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register contact sync gateway: %w", err)
	}
	if err := pb.RegisterCryptoServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register crypto gateway: %w", err)
	}
	if err := pb.RegisterGiftCardServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register gift card gateway: %w", err)
	}
	if err := pb.RegisterStockServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register stock gateway: %w", err)
	}
	if err := pb.RegisterStatisticsServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register statistics gateway: %w", err)
	}
	if err := pb.RegisterPortfolioServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register portfolio gateway: %w", err)
	}
	if err := pb.RegisterGroupAccountServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register group account gateway: %w", err)
	}
	if err := pb.RegisterCrowdfundServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register crowdfund gateway: %w", err)
	}
	if err := pb.RegisterAutoSaveServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register auto-save gateway: %w", err)
	}
	if err := pb.RegisterElectricityBillServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts); err != nil {
		return fmt.Errorf("failed to register electricity bill gateway: %w", err)
	}
	// Note: Support service doesn't have HTTP gateway as it uses gRPC only

	// Create main HTTP mux for non-gRPC traffic (swagger, gateway)
	httpMux := http.NewServeMux()

	// Add Swagger handler
	httpMux.Handle("/swagger/", http.StripPrefix("/swagger/", http.FileServer(http.Dir("./swagger"))))

	// Add custom voice note handler for multipart form uploads
	httpMux.HandleFunc("/v1/voice/note/upload", func(w http.ResponseWriter, r *http.Request) {
		s.handleVoiceNoteUpload(w, r, s.voiceSessionController, s.userService)
	})

	// Add QR code generation endpoint
	httpMux.HandleFunc("/v1/user/qr-code", func(w http.ResponseWriter, r *http.Request) {
		s.handleQRCodeGeneration(w, r)
	})

	// Add split bill batch transfer endpoint
	httpMux.HandleFunc("/v1/transfers/split-bill", func(w http.ResponseWriter, r *http.Request) {
		s.handleSplitBillBatch(w, r)
	})

	// Add gateway handler (this should come last to catch all other routes)
	httpMux.Handle("/", gwmux)

	// Setup CORS for HTTP traffic (gateway, swagger)
	corsHandler := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Content-Type",
			"Content-Length",
			"Accept-Encoding",
			"Authorization",
			"X-CSRF-Token",
			"X-Refresh-Token",
		},
		ExposedHeaders: []string{
			"Authorization",
			"X-Refresh-Token",
		},
		AllowCredentials: true,
	})

	// Wrap the HTTP mux with CORS
	corsEnabledHttpMux := corsHandler.Handler(httpMux)

	// Apply security middleware chain (wraps CORS handler)
	// Order matters: RequestID -> SecurityHeaders -> RateLimit -> XSS -> SQLInjection -> CORS + Mux
	securityHandler := securityMiddleware.ChainMiddleware(
		corsEnabledHttpMux,
		securityMiddleware.RequestIDHTTP(),
		securityMiddleware.SecurityHeadersHTTP(),
		securityMiddleware.RateLimitMiddlewareHTTP(100, 200), // 100 req/sec per IP, burst 200
		securityMiddleware.XSSProtectionHTTP(),
		securityMiddleware.SQLInjectionProtectionHTTP(),
	)

	// Determine port for HTTP server
	httpPort := s.config.HTTPServerPort
	if httpPort == "" {
		httpPort = s.config.HTTPServerPort // Fallback to config for local development
	}

	// Start HTTP server
	httpAddr := fmt.Sprintf(":%s", httpPort)
	fmt.Printf("Starting HTTP gateway server on %s\n", httpAddr)

	// Store the server instance so it can be shut down gracefully if needed.
	s.httpServer = &http.Server{
		Addr:    httpAddr,
		Handler: securityHandler, // Now using security-wrapped handler
	}
	return s.httpServer.ListenAndServe()
}

func customHeaderMatcher(key string) (string, bool) {
	switch key {
	case "Authorization", "X-Refresh-Token":
		return key, true
	default:
		return runtime.DefaultHeaderMatcher(key)
	}
}

func (s *Server) Stop() {
	// Gracefully stop gRPC server
	s.grpcServer.GracefulStop()

	// Gracefully stop HTTP server
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(context.Background()); err != nil {
			fmt.Printf("Error shutting down HTTP server: %v\n", err)
		}
	}
}

func (s *Server) handleVoiceNoteUpload(w http.ResponseWriter, r *http.Request, voiceSessionController *VoiceSessionController, userService services.IUserService) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow POST method
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (25MB max)
	err := r.ParseMultipartForm(25 << 20)
	if err != nil {
		http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
		return
	}

	// Extract JWT token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header is required", http.StatusUnauthorized)
		return
	}

	// Validate Bearer token format
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		http.Error(w, "Authorization header must be Bearer token", http.StatusUnauthorized)
		return
	}

	jwtToken := authHeader[len(bearerPrefix):]
	if jwtToken == "" {
		http.Error(w, "JWT token is required", http.StatusUnauthorized)
		return
	}

	// Verify token using auth-service
	email, err := validateTokenViaAuthService(jwtToken)
	if err != nil {
		http.Error(w, "Invalid JWT token", http.StatusUnauthorized)
		return
	}

	// Get user from token payload
	user, err := getUserByEmail(userService, s.db, email)
	if err != nil {
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	}

	userID := user.ID
	// Note: tx_history is automatically fetched by the service, not sent from frontend

	// Get audio file
	file, fileHeader, err := r.FormFile("audio_file")
	if err != nil {
		http.Error(w, "audio_file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read file content
	audioContent, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read audio file", http.StatusInternalServerError)
		return
	}

	// Create gRPC request (tx_history will be fetched automatically by service)
	req := &pb.ProcessVoiceNoteRequest{
		TxHistory: "", // Empty - service will fetch transaction history automatically
	}

	// Create context with authentication payload (like other gRPC endpoints)
	authPayload := &middleware.AuthPayload{
		UserID: fmt.Sprintf("%d", userID),
		Email:  email,
	}
	ctx := r.Context()
	ctx = context.WithValue(ctx, middleware.AuthorizationPayloadKey, authPayload)
	ctx = context.WithValue(ctx, middleware.AccessTokenKey, jwtToken)

	// Call the controller multipart method with audio data
	resp, err := voiceSessionController.ProcessVoiceNoteMultipart(ctx, userID, jwtToken, audioContent, fileHeader.Filename, fileHeader.Header.Get("Content-Type"), req)
	if err != nil {
		// Handle gRPC errors
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument:
				http.Error(w, st.Message(), http.StatusBadRequest)
			case codes.Unauthenticated:
				http.Error(w, st.Message(), http.StatusUnauthorized)
			case codes.Internal:
				http.Error(w, st.Message(), http.StatusInternalServerError)
			case codes.Unavailable:
				http.Error(w, st.Message(), http.StatusServiceUnavailable)
			default:
				http.Error(w, st.Message(), http.StatusInternalServerError)
			}
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Convert gRPC response to JSON
	jsonResp, err := json.Marshal(map[string]interface{}{
		"success":            resp.Success,
		"msg":                resp.Msg,
		"response":           resp.Response,
		"transcribed_text":   resp.TranscribedText,
		"processing_time_ms": resp.ProcessingTimeMs,
	})
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}

	// Send response
	w.WriteHeader(http.StatusOK)
	w.Write(jsonResp)
}

// Helper function to get user by ID
func getUserByID(userService services.IUserService, db *gorm.DB, userID uint) (*models.User, error) {
	// This is a simplified implementation - you may need to adjust based on your UserService interface
	user, err := models.User{}.FindById(db, userID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// Helper function to get user by email
func getUserByEmail(userService services.IUserService, db *gorm.DB, email string) (*models.User, error) {
	// Find user by email using the existing model method
	user, err := models.User{}.GetUserByEmail(db, email)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// handleQRCodeGeneration generates a QR code for the authenticated user
func (s *Server) handleQRCodeGeneration(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow GET method
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract JWT token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header is required", http.StatusUnauthorized)
		return
	}

	// Validate Bearer token format
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		http.Error(w, "Authorization header must be Bearer token", http.StatusUnauthorized)
		return
	}

	jwtToken := authHeader[len(bearerPrefix):]
	if jwtToken == "" {
		http.Error(w, "JWT token is required", http.StatusUnauthorized)
		return
	}

	// Verify token using auth-service
	email, err := validateTokenViaAuthService(jwtToken)
	if err != nil {
		http.Error(w, "Invalid JWT token", http.StatusUnauthorized)
		return
	}

	// Get user from token payload
	user, err := getUserByEmail(s.userService, s.db, email)
	if err != nil {
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	}

	// Generate QR code
	ctx := r.Context()
	qrCodeResp, err := s.qrCodeService.GenerateUserQRCode(ctx, user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate QR code: %v", err), http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Convert response to JSON
	jsonResp, err := json.Marshal(map[string]interface{}{
		"type":          qrCodeResp.Type,
		"recipient_id":  qrCodeResp.RecipientID,
		"username":      qrCodeResp.Username,
		"name":          qrCodeResp.Name,
		"version":       qrCodeResp.Version,
		"qr_code_image": qrCodeResp.QRCodeImage,
	})
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}

	// Send response
	w.WriteHeader(http.StatusOK)
	w.Write(jsonResp)
}

// handleSplitBillBatch handles batch transfer creation for split bills
func (s *Server) handleSplitBillBatch(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow POST method
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract JWT token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header is required", http.StatusUnauthorized)
		return
	}

	// Validate Bearer token format
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		http.Error(w, "Authorization header must be Bearer token", http.StatusUnauthorized)
		return
	}

	jwtToken := authHeader[len(bearerPrefix):]
	if jwtToken == "" {
		http.Error(w, "JWT token is required", http.StatusUnauthorized)
		return
	}

	// Verify token using auth-service
	email, err := validateTokenViaAuthService(jwtToken)
	if err != nil {
		http.Error(w, "Invalid JWT token", http.StatusUnauthorized)
		return
	}

	// Get user from token payload
	user, err := getUserByEmail(s.userService, s.db, email)
	if err != nil {
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var req services.SplitBillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Call service
	ctx := r.Context()
	resp, err := s.transferService.InitiateSplitBillBatch(ctx, user.ID, req)
	if err != nil {
		// Map service errors to HTTP status codes
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else if strings.Contains(err.Error(), "insufficient") {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "must") {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "already processed") {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, fmt.Sprintf("Failed to process split bill: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Extract transfer IDs from transfers array
	transferIDs := make([]uint, len(resp.Transfers))
	for i, t := range resp.Transfers {
		transferIDs[i] = t.TransferID
	}

	// Convert response to JSON
	jsonResp, err := json.Marshal(map[string]interface{}{
		"batch_id":     resp.BatchID,
		"split_count":  resp.SplitCount,
		"total_amount": resp.TotalAmount,
		"status":       resp.Status,
		"created_at":   resp.CreatedAt,
		"transfer_ids": transferIDs,
		"transfers":    resp.Transfers,
	})
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}

	// Send response
	w.WriteHeader(http.StatusCreated)
	w.Write(jsonResp)
}

// getEnv gets environment variable or returns default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// validateTokenViaAuthService validates a JWT token using auth-service gRPC
func validateTokenViaAuthService(token string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := middleware.GetAuthServiceClient().ValidateToken(ctx, &authpb.ValidateTokenRequest{
		Token: token,
	})
	if err != nil {
		return "", fmt.Errorf("failed to validate token: %w", err)
	}

	if !resp.Valid {
		return "", fmt.Errorf("invalid token")
	}

	return resp.Email, nil
}
