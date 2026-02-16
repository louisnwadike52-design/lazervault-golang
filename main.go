package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/subosito/gotenv"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	_ "google.golang.org/grpc/encoding/gzip" // Register gzip compressor for 60-80% payload reduction
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	shareddegradation "github.com/lazervault/shared/degradation"
	sharederrors "github.com/lazervault/shared/errors"
	"lazervaultGo/grpcApi/middleware"
	tlsutil "lazervaultGo/pkg/tls"

	// Import microservice proto packages
	accountspb "accounts-service/proto"
	whatsapppb "whatsapp-service/proto"
	notificationspb "notifications-service/proto"

	// Import gateway proto packages (includes auth service definitions)
	pb "lazervaultGo/pb"

	// Import internal packages
	"lazervaultGo/internal/interceptors"
	gatewaykafka "lazervaultGo/internal/kafka"
	"lazervaultGo/internal/proxy"
)

// loadEnvFiles loads environment variables from .env files based on environment
func loadEnvFiles() {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "development"
	}

	// Load environment-specific .env file first (higher priority)
	var envFile string
	switch env {
	case "production":
		envFile = ".env.production"
	default:
		envFile = ".env.local"
	}

	// Try to load environment-specific file
	if err := gotenv.Load(envFile); err != nil {
		// Not an error if file doesn't exist in production (uses real env vars)
		if env != "production" {
			log.Warn().Str("file", envFile).Msg("⚠️  Could not load env file, using system environment")
		}
	} else {
		log.Info().Str("file", envFile).Msg("✅ Loaded environment configuration")
	}
}

// Core Gateway - Routes to auth-service and accounts-service via gRPC
func main() {
	// Setup logger
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Load environment variables from .env files
	loadEnvFiles()

	log.Info().Msg("🚀 Starting Core Gateway (gRPC-based)")

	// Get microservice addresses from environment
	authServiceAddr := getEnv("AUTH_SERVICE_GRPC_ADDR", "127.0.0.1:50051")
	accountsServiceAddr := getEnv("ACCOUNTS_SERVICE_GRPC_ADDR", "127.0.0.1:50052")
	whatsappServiceAddr := getEnv("WHATSAPP_SERVICE_GRPC_ADDR", "127.0.0.1:50062")
	notificationsServiceAddr := getEnv("NOTIFICATIONS_SERVICE_GRPC_ADDR", "127.0.0.1:50061")

	// Get Redis configuration
	redisURL := getEnv("REDIS_URL", "redis://localhost:6379")
	enableCache := getEnv("ENABLE_CACHE", "true") == "true"
	cacheTTL := getEnvDuration("CACHE_TTL", 5*time.Minute)

	// Check if mTLS is enabled
	enableMTLS := getEnv("ENABLE_MTLS", "false") == "true"
	certDir := getEnv("CERT_DIR", "./certs")

	// Check if CORS should be enabled
	enableCORS := getEnv("ENABLE_CORS", "true") == "true"
	corsMode := getEnv("CORS_MODE", "permissive") // permissive or restricted

	log.Info().
		Str("auth_service", authServiceAddr).
		Str("accounts_service", accountsServiceAddr).
		Str("whatsapp_service", whatsappServiceAddr).
		Str("notifications_service", notificationsServiceAddr).
		Str("redis_url", redisURL).
		Bool("cache_enabled", enableCache).
		Dur("cache_ttl", cacheTTL).
		Bool("mtls_enabled", enableMTLS).
		Bool("cors_enabled", enableCORS).
		Str("cors_mode", corsMode).
		Msg("📡 Gateway configuration")

	// Initialize Redis client for caching
	var redisClient *redis.Client
	if enableCache {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to parse Redis URL, caching disabled")
			redisClient = nil
		} else {
			redisClient = redis.NewClient(opt)
			// Test connection
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := redisClient.Ping(ctx).Err(); err != nil {
				log.Warn().Err(err).Msg("Failed to connect to Redis, caching disabled")
				redisClient = nil
			} else {
				log.Info().Msg("✅ Redis connected - caching enabled")
			}
		}
	} else {
		log.Info().Msg("⚠️  Caching disabled by configuration")
	}

	// Setup zap logger for JWT verifier and error recovery
	zapLogger, _ := zap.NewProduction()
	defer zapLogger.Sync()

	// Initialize Kafka-based balance cache invalidator (if Redis is available)
	kafkaBrokers := getEnv("KAFKA_BROKERS", "127.0.0.1:9092")
	balanceChangedTopic := getEnv("KAFKA_BALANCE_CHANGED_TOPIC", "accounts.balance-changed")
	kafkaGroupID := getEnv("KAFKA_CONSUMER_GROUP_ID", "core-gateway-cache-invalidator")

	var balanceCacheInvalidator *gatewaykafka.BalanceCacheInvalidator
	if redisClient != nil {
		var err error
		balanceCacheInvalidator, err = gatewaykafka.NewBalanceCacheInvalidator(
			[]string{kafkaBrokers},
			balanceChangedTopic,
			kafkaGroupID,
			redisClient,
			zapLogger,
		)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to create balance cache invalidator, balance cache will not be auto-invalidated")
		} else {
			if err := balanceCacheInvalidator.Start(context.Background()); err != nil {
				log.Warn().Err(err).Msg("Failed to start balance cache invalidator")
			} else {
				log.Info().Msg("✅ Balance cache invalidator started - listening for balance changes")
				defer balanceCacheInvalidator.Stop()
			}
		}
	}

	// Initialize error handler (available for future use)
	_ = sharederrors.NewErrorHandler(zapLogger)

	// Initialize degradation manager
	degradationManager := shareddegradation.NewManager(shareddegradation.DegradationConfig{
		CheckInterval: 30 * time.Second,
		MaxFailures:   3,
		AutoRecover:   true,
		Logger:        zapLogger,
	})

	// Register features for degradation monitoring
	degradationManager.RegisterFeature(
		"auth-service",
		"Authentication service",
		func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conn, err := grpc.DialContext(ctx, authServiceAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
			)
			if err != nil {
				return err
			}
			conn.Close()
			return nil
		},
		shareddegradation.WithMaxFailures(3),
		shareddegradation.WithAutoRecover(),
	)

	degradationManager.RegisterFeature(
		"accounts-service",
		"Accounts and cards service",
		func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conn, err := grpc.DialContext(ctx, accountsServiceAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
			)
			if err != nil {
				return err
			}
			conn.Close()
			return nil
		},
		shareddegradation.WithMaxFailures(3),
		shareddegradation.WithAutoRecover(),
	)

	// Start degradation monitoring
	degradationManager.Start()
	defer degradationManager.Stop()

	log.Info().Msg("✅ Error recovery components initialized")

	// Get JWT configuration from environment
	jwksURL := getEnv("JWKS_URL", "http://127.0.0.1:8081/.well-known/jwks.json") // Use 127.0.0.1 to avoid IPv6 resolution issues
	jwtIssuer := getEnv("JWT_ISSUER", "https://auth.lazervault.com")
	jwtAudience := getEnv("JWT_AUDIENCE", "lazervault-api")

	// Initialize banking-grade JWT verification (NO RPC to auth service)
	log.Info().Msg("🔐 Initializing banking-grade JWT verification...")
	if err := middleware.InitJWTVerifier(jwksURL, jwtIssuer, jwtAudience, zapLogger); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize JWT verifier")
	}
	log.Info().Msg("✅ JWT verifier initialized (JWKS-based, zero network calls)")

	// Create gRPC-Gateway mux
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(customHeaderMatcher),
		runtime.WithOutgoingHeaderMatcher(customHeaderMatcher),
	)

	// Setup gRPC dial options with optional mTLS
	var opts []grpc.DialOption
	if enableMTLS {
		tlsCreds, err := tlsutil.LoadClientTLSCredentials(enableMTLS, certDir)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load mTLS credentials")
		}
		opts = []grpc.DialOption{grpc.WithTransportCredentials(tlsCreds)}
		log.Info().Msg("🔒 mTLS enabled for microservice connections")
	} else {
		opts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
		log.Warn().Msg("⚠️  mTLS disabled - using insecure connections")
	}

	// Register auth service handler (from auth-microservice)
	// This will proxy /api/v1/auth/* to auth-service gRPC
	log.Info().Msg("🔌 Connecting to auth-service gRPC...")
	if err := registerAuthServiceHandler(ctx, mux, authServiceAddr, opts); err != nil {
		log.Fatal().Err(err).Msg("Failed to register auth service handler")
	}

	// Register accounts service handler (from accounts-microservice)
	// This will proxy /api/v1/accounts/*, /api/v1/cards/* to accounts-service gRPC
	log.Info().Msg("🔌 Connecting to accounts-service gRPC...")
	if err := registerAccountsServiceHandler(ctx, mux, accountsServiceAddr, opts); err != nil {
		log.Fatal().Err(err).Msg("Failed to register accounts service handler")
	}

	// Register family accounts service handler (from accounts-microservice)
	// This will proxy /api/v1/family-accounts/* to accounts-service gRPC
	log.Info().Msg("🔌 Connecting to family-accounts-service gRPC...")
	if err := registerFamilyAccountsServiceHandler(ctx, mux, accountsServiceAddr, opts); err != nil {
		log.Fatal().Err(err).Msg("Failed to register family accounts service handler")
	}

	// Register recipient service handler (from accounts-microservice)
	// This will proxy /api/v1/recipients/* to accounts-service gRPC
	log.Info().Msg("🔌 Connecting to recipient-service gRPC...")
	if err := registerRecipientServiceHandler(ctx, mux, accountsServiceAddr, opts); err != nil {
		log.Fatal().Err(err).Msg("Failed to register recipient service handler")
	}

	// Register user service handler (proxies to auth-service)
	// This will proxy /v1/users/* to auth-service via UserServiceProxy
	log.Info().Msg("🔌 Registering user-service gRPC-gateway...")
	if err := registerUserServiceHandler(ctx, mux, authServiceAddr, opts); err != nil {
		log.Fatal().Err(err).Msg("Failed to register user service handler")
	}

	// Register WhatsApp service handler (from whatsapp-microservice)
	// This will proxy /api/v1/whatsapp/* to whatsapp-service gRPC
	log.Info().Msg("🔌 Connecting to whatsapp-service gRPC...")
	if err := registerWhatsAppServiceHandler(ctx, mux, whatsappServiceAddr, opts); err != nil {
		log.Warn().Err(err).Msg("Failed to register whatsapp service handler - WhatsApp banking will be unavailable")
	}

	// Register notifications service handler (from notifications-microservice)
	log.Info().Msg("Connecting to notifications-service gRPC...")
	if err := registerNotificationsServiceHandler(ctx, mux, notificationsServiceAddr, opts); err != nil {
		log.Warn().Err(err).Msg("Failed to register notifications service handler - notifications will be unavailable")
	}

	// Register AI Chat service handler (proxies to local gRPC AI chat proxy)
	grpcPort := getEnv("GRPC_PORT", "50070")
	localGrpcAddr := "127.0.0.1:" + grpcPort
	log.Info().Msg("Registering AI Chat service gRPC-gateway...")
	if err := registerAIChatServiceHandler(ctx, mux, localGrpcAddr, opts); err != nil {
		log.Warn().Err(err).Msg("Failed to register AI chat service handler - AI chat will be unavailable via HTTP")
	}

	// Create Gin router for additional middleware and routing
	router := gin.New()

	// Add panic recovery middleware FIRST (highest priority)
	router.Use(middleware.PanicRecoveryMiddleware(zapLogger))

	// Add request logging middleware
	router.Use(middleware.RequestLoggingMiddleware(zapLogger))

	// Add request ID middleware
	router.Use(middleware.RequestID())

	// Add timeout middleware
	router.Use(middleware.TimeoutMiddleware(30 * time.Second))

	// Add error handling middleware
	router.Use(middleware.ErrorHandlerMiddleware(zapLogger))

	// Add cache middleware for GET requests
	if redisClient != nil {
		router.Use(middleware.CacheMiddleware(redisClient, cacheTTL))
		router.Use(middleware.InvalidateCacheMiddleware(redisClient))
	}

	// Add rate limiting
	router.Use(middleware.RateLimitMiddleware(100, 200))

	// Configure CORS middleware (if enabled)
	if enableCORS {
		corsConfig := cors.Config{
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Request-ID"},
			ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
		}

		// Configure based on CORS mode
		if corsMode == "restricted" {
			// Production mode - restrict to specific origins
			corsConfig.AllowOrigins = []string{
				"http://localhost:3000",
				"http://localhost:7878",
				"http://10.0.2.2:7878",
				"capacitor://localhost",
				"ionic://localhost",
				"https://lazervault.com",
				"https://app.lazervault.com",
			}
			log.Info().Msg("🔒 CORS enabled in RESTRICTED mode - only approved origins allowed")
		} else {
			// Development mode - allow all origins
			corsConfig.AllowOriginFunc = func(origin string) bool {
				return true
			}
			log.Info().Msg("⚠️  CORS enabled in PERMISSIVE mode - all origins allowed")
		}

		router.Use(cors.New(corsConfig))
	} else {
		log.Warn().Msg("⚠️  CORS disabled - no origin restrictions")
	}

	// Add security headers
	router.Use(middleware.SecurityHeaders())

	// Health check endpoint with detailed status
	router.GET("/health", middleware.HealthCheckMiddleware(map[string]func() error{
		"auth_service": func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conn, err := grpc.DialContext(ctx, authServiceAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
			)
			if err != nil {
				return err
			}
			conn.Close()
			return nil
		},
		"accounts_service": func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conn, err := grpc.DialContext(ctx, accountsServiceAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
			)
			if err != nil {
				return err
			}
			conn.Close()
			return nil
		},
		"redis": func() error {
			if redisClient == nil {
				return nil // Redis is optional
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return redisClient.Ping(ctx).Err()
		},
	}))

	// Readiness check endpoint
	router.GET("/ready", middleware.ReadinessCheckMiddleware(func() bool {
		return degradationManager.IsEnabled("auth-service") &&
			degradationManager.IsEnabled("accounts-service")
	}))

	// Admin endpoints for degradation management
	admin := router.Group("/admin")
	admin.Use(middleware.RateLimitMiddleware(10, 20)) // Stricter rate limiting for admin
	{
		// Get degradation status
		admin.GET("/degradation", func(c *gin.Context) {
			status := degradationManager.GetStatus()
			c.JSON(http.StatusOK, gin.H{
				"status":    status,
				"timestamp": time.Now().UTC(),
			})
		})

		// Enable/disable features
		admin.POST("/degradation/:feature/enable", func(c *gin.Context) {
			feature := c.Param("feature")
			if err := degradationManager.EnableFeature(feature); err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "Feature enabled", "feature": feature})
		})

		admin.POST("/degradation/:feature/disable", func(c *gin.Context) {
			feature := c.Param("feature")
			var req struct {
				Reason string `json:"reason"`
			}
			if err := c.BindJSON(&req); err != nil {
				req.Reason = "Manual disable"
			}
			if err := degradationManager.DisableFeature(feature, req.Reason); err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "Feature disabled", "feature": feature})
		})

		admin.POST("/degradation/:feature/readonly", func(c *gin.Context) {
			feature := c.Param("feature")
			var req struct {
				Reason string `json:"reason"`
			}
			if err := c.BindJSON(&req); err != nil {
				req.Reason = "Switched to read-only mode"
			}
			if err := degradationManager.SetReadOnlyMode(feature, req.Reason); err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "Feature set to read-only", "feature": feature})
		})
	}

	// Metrics endpoint for Prometheus
	router.GET("/metrics", func(c *gin.Context) {
		// Return Prometheus metrics
		c.Header("Content-Type", "text/plain")
		c.String(http.StatusOK, "# Metrics endpoint - integrate with Prometheus\n")
	})

	// API group with JWT authentication (applies to all /api/* routes except auth public endpoints)
	apiGroup := router.Group("/api")
	apiGroup.Use(middleware.JWTAuthMiddleware())
	apiGroup.Any("/*path", wrapGrpcGateway(mux))

	// Note: Auth service routes (/api/v1/auth/*) are registered via grpc-gateway mux
	// They go through apiGroup but JWT middleware skips public paths (login, signup, etc.)
	// This keeps gRPC and HTTP routes in sync - no workarounds needed

	// Start HTTP server
	httpPort := getEnv("HTTP_PORT", "7878")
	if httpPort[0] != ':' {
		httpPort = ":" + httpPort
	}

	srv := &http.Server{
		Addr:         httpPort,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server in goroutine
	go func() {
		log.Info().
			Str("port", httpPort).
			Msg("✅ Core Gateway HTTP server started")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	// ========== gRPC SERVER SETUP ==========

	// Create upstream service connections for gRPC proxying
	log.Info().Msg("🔌 Creating upstream service connections for gRPC server...")

	whatsappConn, err := grpc.Dial(whatsappServiceAddr, opts...)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to whatsapp service for gRPC - WhatsApp banking will be unavailable")
	}
	if whatsappConn != nil {
		defer whatsappConn.Close()
	}

	notificationsConn, err := grpc.Dial(notificationsServiceAddr, opts...)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to notifications service for gRPC - notifications will be unavailable")
	}
	if notificationsConn != nil {
		defer notificationsConn.Close()
	}

	authConn, err := grpc.Dial(authServiceAddr, opts...)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to auth service for gRPC")
	}
	defer authConn.Close()

	accountsConn, err := grpc.Dial(accountsServiceAddr, opts...)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to accounts service for gRPC")
	}
	defer accountsConn.Close()

	// Create proxy services
	authProxy := proxy.NewAuthServiceProxy(pb.NewAuthServiceClient(authConn))
	transactionPinProxy := proxy.NewTransactionPinServiceProxy(pb.NewTransactionPinServiceClient(authConn))
	accountsProxy := proxy.NewAccountsServiceProxy(accountspb.NewAccountsServiceClient(accountsConn))
	familyAccountsProxy := proxy.NewFamilyAccountsServiceProxy(accountspb.NewFamilyAccountsServiceClient(accountsConn))
	recipientProxy := proxy.NewRecipientServiceProxy(accountspb.NewRecipientServiceClient(accountsConn))
	userProxy := proxy.NewUserServiceProxy(pb.NewAuthServiceClient(authConn))

	// Create gRPC server with interceptor chain
	// Configured for low-network regions (Nigeria) with compression and lenient keepalive
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptors.PanicRecoveryInterceptor(zapLogger),
			interceptors.RequestLoggingInterceptor(zapLogger),
			interceptors.JWTAuthInterceptor(middleware.GetJWTVerifier()),
			interceptors.RateLimitInterceptor(redisClient),
		),
		grpc.ChainStreamInterceptor(
			interceptors.StreamPanicRecoveryInterceptor(zapLogger),
		),
		// Keepalive settings optimized for mobile clients in low-network regions
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     5 * time.Minute,  // Allow idle connections longer for mobile
			MaxConnectionAge:      30 * time.Minute, // Max connection lifetime
			MaxConnectionAgeGrace: 10 * time.Second, // Grace period for existing RPCs
			Time:                  30 * time.Second, // Ping interval
			Timeout:               10 * time.Second, // Ping timeout
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second, // Minimum ping interval from client
			PermitWithoutStream: true,             // Allow pings when no active streams
		}),
		// Message size limits for large payloads
		grpc.MaxRecvMsgSize(10*1024*1024), // 10MB max receive
		grpc.MaxSendMsgSize(10*1024*1024), // 10MB max send
	)

	// Register services
	pb.RegisterAuthServiceServer(grpcServer, authProxy)
	pb.RegisterTransactionPinServiceServer(grpcServer, transactionPinProxy)
	accountspb.RegisterAccountsServiceServer(grpcServer, accountsProxy)
	accountspb.RegisterFamilyAccountsServiceServer(grpcServer, familyAccountsProxy)
	accountspb.RegisterRecipientServiceServer(grpcServer, recipientProxy)
	pb.RegisterUserServiceServer(grpcServer, userProxy)

	// Register WhatsApp service proxy (if connection available)
	if whatsappConn != nil {
		whatsappProxy := proxy.NewWhatsAppServiceProxy(whatsapppb.NewWhatsAppServiceClient(whatsappConn))
		whatsapppb.RegisterWhatsAppServiceServer(grpcServer, whatsappProxy)
		log.Info().Msg("✅ WhatsApp service registered on gRPC server")
	}

	// Register notifications service proxy (if connection available)
	if notificationsConn != nil {
		notificationsProxy := proxy.NewNotificationsServiceProxy(notificationspb.NewNotificationsServiceClient(notificationsConn))
		notificationspb.RegisterNotificationsServiceServer(grpcServer, notificationsProxy)
		log.Info().Msg("Notifications service registered on gRPC server")
	}

	// Register AI Chat proxy (proxies gRPC to Python chat-agent-gateway via HTTP)
	chatGatewayURL := getEnv("CHAT_AGENT_GATEWAY_URL", "http://localhost:3011")
	aiChatProxy := proxy.NewAIChatServiceProxy(chatGatewayURL)
	pb.RegisterAIChatServiceServer(grpcServer, aiChatProxy)
	log.Info().Str("chat_gateway_url", chatGatewayURL).Msg("AI Chat service registered on gRPC server")

	// Register health check
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	// Register reflection for debugging
	reflection.Register(grpcServer)

	// Start gRPC server in goroutine
	go func() {
		grpcPort := getEnv("GRPC_PORT", "50070")
		listener, err := net.Listen("tcp", ":"+grpcPort)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to create gRPC listener")
		}

		log.Info().
			Str("port", grpcPort).
			Msg("✅ Core Gateway gRPC server started")

		if err := grpcServer.Serve(listener); err != nil {
			log.Fatal().Err(err).Msg("gRPC server failed")
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("🛑 Shutting down Core Gateway...")

	// Update health status
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	time.Sleep(2 * time.Second) // Allow health checks to propagate

	// Shutdown HTTP server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("HTTP server shutdown error")
		} else {
			log.Info().Msg("✅ HTTP server stopped gracefully")
		}
	}()

	// Shutdown gRPC server (GracefulStop waits for active RPCs)
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		log.Info().Msg("✅ gRPC server stopped gracefully")
	case <-shutdownCtx.Done():
		log.Warn().Msg("Forcing gRPC server stop")
		grpcServer.Stop() // Force stop
	}

	// Close Redis connection
	if redisClient != nil {
		if err := redisClient.Close(); err != nil {
			log.Warn().Err(err).Msg("Failed to close Redis connection gracefully")
		} else {
			log.Info().Msg("✅ Redis connection closed")
		}
	}

	log.Info().Msg("✅ Core Gateway stopped gracefully")
}

// wrapGrpcGateway wraps grpc-gateway mux as a Gin handler
func wrapGrpcGateway(mux *runtime.ServeMux) gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Printf("[wrapGrpcGateway] Path: %s, Method: %s\n", c.Request.URL.Path, c.Request.Method)
		mux.ServeHTTP(c.Writer, c.Request)
	}
}

// customHeaderMatcher matches custom headers
func customHeaderMatcher(key string) (string, bool) {
	switch key {
	case "Authorization", "X-User-Id", "X-Username":
		return key, true
	default:
		return runtime.DefaultHeaderMatcher(key)
	}
}

// registerAuthServiceHandler registers auth service gRPC-gateway handler
// This connects to auth-microservice on port 50051
func registerAuthServiceHandler(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
	return pb.RegisterAuthServiceHandlerFromEndpoint(ctx, mux, addr, opts)
}

// registerAccountsServiceHandler registers accounts service gRPC-gateway handler
// This connects to accounts-microservice on port 50052
func registerAccountsServiceHandler(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
	return accountspb.RegisterAccountsServiceHandlerFromEndpoint(ctx, mux, addr, opts)
}

// registerFamilyAccountsServiceHandler registers family accounts service gRPC-gateway handler
// This connects to accounts-microservice (family accounts are part of accounts service)
func registerFamilyAccountsServiceHandler(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
	return accountspb.RegisterFamilyAccountsServiceHandlerFromEndpoint(ctx, mux, addr, opts)
}

// registerRecipientServiceHandler registers recipient service gRPC-gateway handler
// This connects to accounts-microservice (recipients are part of accounts service)
func registerRecipientServiceHandler(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
	return accountspb.RegisterRecipientServiceHandlerFromEndpoint(ctx, mux, addr, opts)
}

// registerUserServiceHandler registers user service gRPC-gateway handler
// This proxies user profile operations to auth-service via UserServiceProxy
func registerUserServiceHandler(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
	return pb.RegisterUserServiceHandlerFromEndpoint(ctx, mux, addr, opts)
}

// registerWhatsAppServiceHandler registers WhatsApp service gRPC-gateway handler
// This connects to whatsapp-microservice on port 50062
func registerWhatsAppServiceHandler(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
	return whatsapppb.RegisterWhatsAppServiceHandlerFromEndpoint(ctx, mux, addr, opts)
}

// registerNotificationsServiceHandler registers notifications service gRPC-gateway handler
// This connects to notifications-microservice on port 50061
func registerNotificationsServiceHandler(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
	return notificationspb.RegisterNotificationsServiceHandlerFromEndpoint(ctx, mux, addr, opts)
}

// registerAIChatServiceHandler registers AI Chat service gRPC-gateway handler
// This proxies /v1/ai/* to the local AI chat proxy (which forwards to Python chat-agent-gateway)
func registerAIChatServiceHandler(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
	return pb.RegisterAIChatServiceHandlerFromEndpoint(ctx, mux, addr, opts)
}

// getEnv gets environment variable or returns default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvDuration gets environment variable as duration or returns default
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
