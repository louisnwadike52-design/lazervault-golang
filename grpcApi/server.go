package grpcApi

import (
	"context"
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"lazervaultGo/worker"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

func NewServer(db *gorm.DB, config *configs.Config, tokenMaker token.Maker, redisWorker *worker.RedisWorker) *Server {
	server := &Server{
		config:      config,
		db:          db,
		tokenMaker:  tokenMaker,
		redisWorker: redisWorker,
	}

	// Create gRPC server with interceptors
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(middleware.AuthInterceptor(tokenMaker)),
	)

	// Initialize services and controllers
	authService := services.NewAuthService(db, config, tokenMaker)

	// Register gRPC services
	pb.RegisterAuthServiceServer(grpcServer, NewAuthController(authService))
	pb.RegisterUserServiceServer(grpcServer, NewUserController(server))

	server.grpcServer = grpcServer
	return server
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

	// Create a new ServeMux for gRPC-Gateway
	gwmux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(customHeaderMatcher),
		runtime.WithOutgoingHeaderMatcher(customHeaderMatcher),
	)

	// Dial the gRPC server
	grpcAddr := fmt.Sprintf("localhost:%s", s.config.GRPCServerPort)
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	// Register service handlers
	if err := pb.RegisterAuthServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register auth gateway: %w", err)
	}

	if err := pb.RegisterUserServiceHandlerFromEndpoint(ctx, gwmux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register user gateway: %w", err)
	}

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

	// Start HTTP server
	httpAddr := fmt.Sprintf(":%s", s.config.HTTPServerPort)
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
