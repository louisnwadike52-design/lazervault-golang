package grpcApi

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"lazervaultGo/worker"
	"net"
	"net/http"
	"os"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"gorm.io/gorm"
)

type Server struct {
	config      *configs.Config
	db          *gorm.DB
	tokenMaker  token.Maker
	grpcServer  *grpc.Server
	httpServer  *http.Server
	redisWorker *worker.RedisWorker
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

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(middleware.AuthInterceptor(tokenMaker)),
	)

	// Initialize services
	distributor := redisWorker.GetDistributor()
	authService := services.NewAuthService(db, config, tokenMaker, distributor)
	accountService := services.NewAccountService(db, distributor)
	recipientService := services.NewRecipientService(db)
	transferService := services.NewTransferService(db, config, distributor, recipientService, accountService)
	accountCardService := services.NewAccountCardService(db, config)
	chatService := services.NewChatService(db)
	userService := services.NewUserService(db, config, tokenMaker)
	exchangeService := services.NewExchangeService(db, distributor)
	invoiceService := services.NewInvoiceService(db)
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

	// Initialize Voice Session Service
	voiceSessionService := services.NewVoiceSessionService(db, config, tokenMaker)

	// Initialize Voice Session Controller
	voiceSessionController := NewVoiceSessionController(voiceSessionService, userService)

	// Register gRPC services
	pb.RegisterAuthServiceServer(grpcServer, NewAuthController(authService))
	pb.RegisterUserServiceServer(grpcServer, NewUserController(server))
	pb.RegisterTransferServiceServer(grpcServer, NewTransferController(transferService, db))
	pb.RegisterAccountServiceServer(grpcServer, NewAccountController(accountService, userService))
	pb.RegisterAccountCardServiceServer(grpcServer, NewAccountCardController(accountCardService, userService))
	pb.RegisterRecipientServiceServer(grpcServer, NewRecipientController(recipientService, userService))
	pb.RegisterChatServiceServer(grpcServer, NewChatController(chatService))
	pb.RegisterExchangeServiceServer(grpcServer, NewExchangeController(exchangeService, userService))
	pb.RegisterInvoiceServiceServer(grpcServer, NewInvoiceController(invoiceService, userService))
	pb.RegisterDepositServiceServer(grpcServer, NewDepositController(depositService, userService))
	pb.RegisterWithdrawServiceServer(grpcServer, NewWithdrawalController(withdrawalService, userService))
	pb.RegisterGenerateTxDataServiceServer(grpcServer, generateTxDataController)
	pb.RegisterTxFileServiceServer(grpcServer, txFileController)
	pb.RegisterAIChatServiceServer(grpcServer, aiChatController)
	pb.RegisterVoiceSessionServiceServer(grpcServer, voiceSessionController)
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
		return fmt.Errorf("failed to listen: %w", err)
	}

	go func() {
		fmt.Printf("Starting gRPC server on %s\n", grpcAddr)
		if err := s.grpcServer.Serve(listener); err != nil {
			panic(fmt.Sprintf("failed to serve gRPC: %v", err))
		}
	}()

	// Start HTTP gateway
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

	grpcAddr := fmt.Sprintf("localhost:%s", s.config.GRPCServerPort)
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	// Register service handlers
	if err := pb.RegisterAuthServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register auth gateway: %w", err)
	}
	if err := pb.RegisterUserServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register user gateway: %w", err)
	}
	if err := pb.RegisterTransferServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register transfer gateway: %w", err)
	}
	if err := pb.RegisterAccountServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register account gateway: %w", err)
	}
	if err := pb.RegisterAccountCardServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register account card gateway: %w", err)
	}
	if err := pb.RegisterRecipientServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register recipient gateway: %w", err)
	}
	if err := pb.RegisterChatServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register chat gateway: %w", err)
	}
	if err := pb.RegisterExchangeServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register exchange gateway: %w", err)
	}
	if err := pb.RegisterInvoiceServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register invoice gateway: %w", err)
	}
	if err := pb.RegisterDepositServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register deposit gateway: %w", err)
	}
	if err := pb.RegisterWithdrawServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register withdraw gateway: %w", err)
	}
	// Register VoiceSessionService gateway handler
	if err := pb.RegisterVoiceSessionServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register voice session gateway: %w", err)
	}
	// Comment out problematic gateway registrations until root cause is found
	/*
		if err := pb.RegisterGenerateTxDataServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
			return fmt.Errorf("failed to register generate tx data gateway: %w", err)
		}
		if err := pb.RegisterTxFileServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
			return fmt.Errorf("failed to register tx file service gateway: %w", err)
		}
	*/

	// Create main HTTP mux
	mux := http.NewServeMux()

	// Add Swagger handler
	mux.Handle("/swagger/", http.StripPrefix("/swagger/", http.FileServer(http.Dir("./swagger"))))

	// Add gateway handler
	mux.Handle("/", gwmux)

	// Setup CORS
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

	handler := corsHandler.Handler(mux)

	// Determine port for HTTP server
	httpPort := os.Getenv("PORT")
	if httpPort == "" {
		httpPort = s.config.HTTPServerPort // Fallback to config for local development
	}

	// Start HTTP server
	httpAddr := fmt.Sprintf(":%s", httpPort)
	fmt.Printf("Starting HTTP server on %s\n", httpAddr)

	return http.ListenAndServe(httpAddr, handler)
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
