package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
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

	"lazervaultGo/grpcApi/middleware"
	tlsutil "lazervaultGo/pkg/tls"

	"github.com/lazervault/shared/auth-interceptor/appcheck"
	shareddegradation "github.com/lazervault/shared/degradation"
	sharederrors "github.com/lazervault/shared/errors"

	// Import microservice proto packages
	accountspb "accounts-service/proto"
	groupaccountspb "group-accounts-service/proto"
	notificationspb "notifications-service/proto"
	whatsapppb "whatsapp-service/proto"

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
		envFile = ".env"
	default:
		envFile = ".env"
	}

	// Try to load environment-specific file
	if err := gotenv.OverLoad(envFile); err != nil {
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
	groupAccountsServiceAddr := getEnv("GROUP_ACCOUNTS_SERVICE_GRPC_ADDR", "127.0.0.1:50066")
	whatsappServiceAddr := getEnv("WHATSAPP_SERVICE_GRPC_ADDR", "127.0.0.1:50062")
	notificationsServiceAddr := getEnv("NOTIFICATIONS_SERVICE_GRPC_ADDR", "127.0.0.1:50059")
	referralServiceAddr := getEnv("REFERRAL_SERVICE_GRPC_ADDR", "127.0.0.1:50084")

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
		Str("referral_service", referralServiceAddr).
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
	jwtIssuer := getEnv("JWT_ISSUER", "https://auth.lazervault.app")
	jwtAudience := getEnv("JWT_AUDIENCE", "lazervault-api")

	// Initialize banking-grade JWT verification (NO RPC to auth service)
	log.Info().Msg("🔐 Initializing banking-grade JWT verification...")
	if err := middleware.InitJWTVerifier(jwksURL, jwtIssuer, jwtAudience, zapLogger); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize JWT verifier")
	}
	log.Info().Msg("✅ JWT verifier initialized (JWKS-based, zero network calls)")

	// Initialize Firebase App Check verification (device attestation: App Attest
	// on iOS, Play Integrity on Android). Verifies the X-Firebase-AppCheck token
	// minted by the genuine app. Mode (off/report/enforce) is admin-tunable via
	// system_settings key auth_appcheck_mode (60s cache), defaulting to the
	// AUTH_APPCHECK_MODE env (report) — so an operator can flip to enforce
	// without a redeploy. The verifier is always initialised so a dynamic flip
	// to enforce works immediately.
	appCheckFallbackMode := interceptors.ParseAppCheckMode(getEnv("AUTH_APPCHECK_MODE", "report"))
	appCheckModeProvider := interceptors.NewAppCheckModeProvider(appCheckFallbackMode, zapLogger)
	var appCheckVerifier *appcheck.Verifier
	{
		log.Info().Str("fallback_mode", string(appCheckFallbackMode)).Msg("🛡️  Initializing Firebase App Check verification...")
		acv, err := appcheck.NewVerifier(appcheck.Config{
			ProjectNumber: getEnv("APPCHECK_PROJECT_NUMBER", "815870072849"),
			ProjectID:     getEnv("APPCHECK_PROJECT_ID", "lazervault-28875"),
			Logger:        zapLogger,
		})
		if err != nil {
			// Non-fatal: App Check must not block gateway boot. Fall back to off.
			log.Error().Err(err).Msg("App Check verifier init failed; disabling App Check (fail-open)")
		} else {
			appCheckVerifier = acv
			log.Info().Msg("✅ App Check verifier initialized")
		}
	}

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

	// Register multi-country account service handler (from accounts-microservice)
	// This will proxy /api/v1/accounts/by-locale, /api/v1/accounts/locale, etc.
	log.Info().Msg("🔌 Connecting to multi-country-account-service gRPC...")
	if err := registerMultiCountryServiceHandler(ctx, mux, accountsServiceAddr, opts); err != nil {
		log.Fatal().Err(err).Msg("Failed to register multi-country account service handler")
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

	// Register Voice Session service handler (proxies to local gRPC voice session proxy)
	log.Info().Msg("Registering Voice Session service gRPC-gateway...")
	if err := registerVoiceSessionServiceHandler(ctx, mux, localGrpcAddr, opts); err != nil {
		log.Warn().Err(err).Msg("Failed to register voice session service handler - voice sessions will be unavailable via HTTP")
	}

	// Register Referral service handler (from referral-microservice)
	// This will proxy /api/v1/referral/* to referral-service gRPC
	log.Info().Msg("🔌 Connecting to referral-service gRPC...")
	if err := registerReferralServiceHandler(ctx, mux, referralServiceAddr, opts); err != nil {
		log.Warn().Err(err).Msg("Failed to register referral service handler - referral will be unavailable")
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
	// Backend health, checked against the real dependencies (auth, accounts,
	// redis) rather than just "the process is up".
	healthHandler := middleware.HealthCheckMiddleware(map[string]func() error{
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
	})

	router.GET("/health", healthHandler)
	// The same handler is ALSO served at /api/v1/health — see
	// interceptHealthCheck, which is where it has to live: a plain
	// router.GET("/api/v1/health") would overlap apiGroup's Any("/*path")
	// wildcard and panic Gin at startup.

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

	// Custom HTTP handler for VerifyTransactionPin (gRPC-only service exposed via HTTP for chatbot)
	var txPinClient pb.TransactionPinServiceClient
	txPinConn, err := grpc.Dial(authServiceAddr, opts...)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create transaction PIN gRPC connection - PIN verification via HTTP will be unavailable")
	} else {
		txPinClient = pb.NewTransactionPinServiceClient(txPinConn)
		log.Info().Msg("✅ Transaction PIN + Channel Management HTTP endpoints registered")
	}

	// Storage proxy — gives the Flutter app a JWT-protected route to
	// obtain a scoped upload URL from storage-service. The Flutter app
	// then PUTs the bytes directly to the returned upload_url and saves
	// the returned public_url via UpdateProfile. We hide the
	// X-Service-Name handshake inside this proxy so the client never
	// needs to be on storage-service's allow-list.
	storageBaseURL := getEnv("STORAGE_SERVICE_URL", "http://localhost:8094")
	storageProxy := proxy.NewStorageProxy(storageBaseURL, "core-gateway")
	log.Info().Str("storage_base_url", storageBaseURL).Msg("✅ Storage proxy registered (POST /api/v1/profile-picture/upload-url, POST /api/v1/bank-scan/upload-url, POST /api/v1/chat-media/upload-url, POST /api/v1/invoice/upload-url)")

	// Support proxy — user-facing "Contact support" surface. Gives the Flutter
	// app a JWT-protected route to support-service's chat + tickets API so it
	// never touches the raw :8030 microservice port directly (which isn't
	// publicly exposed). support-service re-validates the forwarded JWT.
	supportBaseURL := getEnv("SUPPORT_SERVICE_HTTP_URL", "http://127.0.0.1:8030")
	supportProxy := proxy.NewSupportProxy(supportBaseURL)
	log.Info().Str("support_base_url", supportBaseURL).Msg("✅ Support proxy registered (/api/v1/support/*)")

	// Client-logs ingest — Flutter devices ship structured logs to our internal
	// Loki through this proxy (POST /api/v1/client-logs). Registered BEFORE the
	// JWT middleware below so the pre-login biometric lock screen can log too;
	// the payload's user_id is a label-only field, never an auth decision.
	clientLogsProxy := proxy.NewClientLogsProxy()
	log.Info().Str("loki_push_url", getEnv("LOKI_PUSH_URL", "http://loki:3100/loki/api/v1/push")).Msg("✅ Client-logs proxy registered (POST /api/v1/client-logs → Loki)")

	// Unified user search (local saved recipients incl. alias → global users).
	// Dialed here (its own clients) so the route can be registered BEFORE the
	// Any("/*path") wildcard below — the manual recipient/auth proxies are
	// constructed later in the file, after this route group is wired.
	var unifiedSearchProxy *proxy.UnifiedSearchProxy
	if uniAcctConn, e1 := grpc.Dial(accountsServiceAddr, opts...); e1 == nil {
		if uniAuthConn, e2 := grpc.Dial(authServiceAddr, opts...); e2 == nil {
			// Org/group users are an OPTIONAL third source — a nil client (dial
			// failed) degrades gracefully to local + global directory search.
			var grpAcctClient groupaccountspb.GroupAccountServiceClient
			if grpConn, e3 := grpc.Dial(groupAccountsServiceAddr, opts...); e3 == nil {
				grpAcctClient = groupaccountspb.NewGroupAccountServiceClient(grpConn)
			} else {
				log.Warn().Err(e3).Msg("Unified search: group-accounts dial failed - org users unavailable")
			}
			unifiedSearchProxy = proxy.NewUnifiedSearchProxy(
				accountspb.NewRecipientServiceClient(uniAcctConn),
				pb.NewAuthServiceClient(uniAuthConn),
				grpAcctClient,
			)
			log.Info().Msg("✅ Unified user search registered (GET /api/v1/users/search-unified) — saved + directory + org")
		} else {
			log.Warn().Err(e2).Msg("Unified search: auth dial failed - route unavailable")
		}
	} else {
		log.Warn().Err(e1).Msg("Unified search: accounts dial failed - route unavailable")
	}

	// API group with JWT authentication (applies to all /api/* routes except auth public endpoints)
	apiGroup := router.Group("/api")
	// Health FIRST — before JWT — the probe runs on the pre-login screen and on
	// a dead/expired session, so requiring a token would report the backend as
	// DOWN to exactly the users who most need to reach it.
	apiGroup.Use(interceptHealthCheck(healthHandler))
	// Client-logs next — also before JWT — so pre-login (biometric lock) logs are
	// accepted anonymously; authenticated logs simply carry a user_id in-body.
	apiGroup.Use(interceptClientLogs(clientLogsProxy))
	apiGroup.Use(middleware.JWTAuthMiddleware())
	apiGroup.Use(interceptVerifyTransactionPin(txPinClient))
	// Storage proxy is intercepted in the same middleware-style as the
	// transaction-pin handlers — registering it as a POST route would
	// conflict with the Any("/*path") wildcard below (gin panics on
	// overlap).
	apiGroup.Use(interceptProfilePictureUploadURL(storageProxy))
	// Unified user search — composed (saved-recipients + global) BFF route.
	// Intercepted in the same middleware style as the storage/PIN handlers
	// because the Any("/*path") wildcard below would otherwise claim it.
	apiGroup.Use(interceptUnifiedSearch(unifiedSearchProxy))
	// User-facing support (chat + tickets) → support-service. Intercepted in the
	// same middleware style because the Any("/*path") wildcard below would
	// otherwise claim /api/v1/support/*.
	apiGroup.Use(interceptSupport(supportProxy))
	// Recipient routes are generated WITHOUT the /api prefix — rewrite
	// /api/v1/recipients/* → /v1/recipients/* onto the same mux.
	apiGroup.Use(interceptRecipients(mux))
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
	multiCountryProxy := proxy.NewMultiCountryServiceProxy(accountspb.NewMultiCountryAccountServiceClient(accountsConn))
	var notificationsClient notificationspb.NotificationsServiceClient
	if notificationsConn != nil {
		notificationsClient = notificationspb.NewNotificationsServiceClient(notificationsConn)
	}
	userProxy := proxy.NewUserServiceProxy(pb.NewAuthServiceClient(authConn), notificationsClient)

	referralConn, err := grpc.Dial(referralServiceAddr, opts...)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to referral service for gRPC - referral will be unavailable")
	}
	if referralConn != nil {
		defer referralConn.Close()
	}
	var referralProxy *proxy.ReferralServiceProxy
	if referralConn != nil {
		referralProxy = proxy.NewReferralServiceProxy(pb.NewReferralServiceClient(referralConn))
	}

	// Create gRPC server with interceptor chain
	// Configured for low-network regions (Nigeria) with compression and lenient keepalive
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptors.PanicRecoveryInterceptor(zapLogger),
			interceptors.RequestLoggingInterceptor(zapLogger),
			interceptors.AppCheckInterceptor(appCheckVerifier, appCheckModeProvider.Mode, zapLogger),
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
	accountspb.RegisterMultiCountryAccountServiceServer(grpcServer, multiCountryProxy)
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

	// Register Referral service proxy (if connection available)
	if referralProxy != nil {
		pb.RegisterReferralServiceServer(grpcServer, referralProxy)
		log.Info().Msg("✅ Referral service registered on gRPC server")
	}

	// Register AI Chat proxy (proxies gRPC to Python chat-agent-gateway via HTTP)
	chatGatewayURL := getEnv("CHAT_AGENT_GATEWAY_URL", "http://localhost:3011")
	aiChatProxy := proxy.NewAIChatServiceProxy(chatGatewayURL)
	pb.RegisterAIChatServiceServer(grpcServer, aiChatProxy)
	log.Info().Str("chat_gateway_url", chatGatewayURL).Msg("AI Chat service registered on gRPC server")

	// Register Voice Session proxy (proxies gRPC to Python voice-agent-gateway via HTTP)
	voiceGatewayURL := getEnv("VOICE_AGENT_GATEWAY_URL", "http://localhost:3010")
	voiceSessionProxy := proxy.NewVoiceSessionServiceProxy(voiceGatewayURL)
	pb.RegisterVoiceSessionServiceServer(grpcServer, voiceSessionProxy)
	log.Info().Str("voice_gateway_url", voiceGatewayURL).Msg("Voice Session service registered on gRPC server")

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

// handleVerifyTransactionPin creates a Gin handler that proxies to the gRPC VerifyTransactionPin RPC.
// This exposes the gRPC-only TransactionPinService via HTTP for chat microservices.
func handleVerifyTransactionPin(client pb.TransactionPinServiceClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Pin             string  `json:"pin"`
			TransactionID   string  `json:"transaction_id"`
			TransactionType string  `json:"transaction_type"`
			Amount          float64 `json:"amount"`
			Currency        string  `json:"currency"`
			DeviceID        string  `json:"device_id"`
			ChannelType     string  `json:"channel_type"` // "app", "whatsapp", "telephony"
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "message": err.Error()})
			return
		}

		userID, _ := c.Get("user_id")
		userIDStr, _ := userID.(string)

		grpcReq := &pb.VerifyTransactionPinRequest{
			UserId:          userIDStr,
			Pin:             req.Pin,
			TransactionId:   req.TransactionID,
			TransactionType: req.TransactionType,
			Amount:          req.Amount,
			Currency:        req.Currency,
			DeviceId:        req.DeviceID,
			ChannelType:     channelTypeToProto(req.ChannelType),
		}

		resp, err := client.VerifyTransactionPin(c.Request.Context(), grpcReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   true,
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success":            resp.Success,
			"message":            resp.Message,
			"verification_token": resp.VerificationToken,
			"remaining_attempts": resp.RemainingAttempts,
			"is_locked":          resp.IsLocked,
		})
	}
}

// handleGetChannelPins returns PIN status for all banking channels.
// GET /api/v1/auth/channel-pins
func handleGetChannelPins(client pb.TransactionPinServiceClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		userIDStr, _ := userID.(string)
		if userIDStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}

		resp, err := client.GetUserChannelPins(c.Request.Context(), &pb.GetUserChannelPinsRequest{
			UserId: userIDStr,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": true, "message": err.Error()})
			return
		}

		pins := make([]gin.H, 0, len(resp.ChannelPins))
		for _, p := range resp.ChannelPins {
			pin := gin.H{
				"channel_type": channelTypeFromProto(p.ChannelType),
				"has_pin":      p.HasPin,
				"is_active":    p.IsActive,
				"is_locked":    p.IsLocked,
			}
			if p.CreatedAt != nil {
				pin["created_at"] = p.CreatedAt.AsTime()
			}
			if p.LastUsedAt != nil {
				pin["last_used_at"] = p.LastUsedAt.AsTime()
			}
			pins = append(pins, pin)
		}

		c.JSON(http.StatusOK, gin.H{"channel_pins": pins})
	}
}

// handleRegisterChannel initiates channel registration (sends OTP).
// POST /api/v1/channels/register
func handleRegisterChannel(client pb.TransactionPinServiceClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			ChannelType string `json:"channel_type"`
			PhoneNumber string `json:"phone_number"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "message": err.Error()})
			return
		}

		userID, _ := c.Get("user_id")
		userIDStr, _ := userID.(string)
		if userIDStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}

		if req.ChannelType != "whatsapp" && req.ChannelType != "telephony" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel_type, must be 'whatsapp' or 'telephony'"})
			return
		}

		e164Re := regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
		if !e164Re.MatchString(req.PhoneNumber) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid phone_number, E.164 format required"})
			return
		}

		resp, err := client.CreateChannelRegistration(c.Request.Context(), &pb.CreateChannelRegistrationRequest{
			UserId:      userIDStr,
			ChannelType: req.ChannelType,
			PhoneNumber: req.PhoneNumber,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": true, "message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success":                resp.Success,
			"message":                resp.Message,
			"masked_phone":           resp.MaskedPhone,
			"otp_expires_in_seconds": resp.OtpExpiresInSeconds,
		})
	}
}

// handleVerifyChannelOTP verifies the OTP to activate a channel.
// POST /api/v1/channels/verify-otp
func handleVerifyChannelOTP(client pb.TransactionPinServiceClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			ChannelType string `json:"channel_type"`
			OtpCode     string `json:"otp_code"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "message": err.Error()})
			return
		}

		userID, _ := c.Get("user_id")
		userIDStr, _ := userID.(string)
		if userIDStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}

		otpRe := regexp.MustCompile(`^\d{4,6}$`)
		if !otpRe.MatchString(req.OtpCode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid otp_code, must be 4-6 digits"})
			return
		}

		resp, err := client.VerifyChannelOTP(c.Request.Context(), &pb.VerifyChannelOTPRequest{
			UserId:      userIDStr,
			ChannelType: req.ChannelType,
			OtpCode:     req.OtpCode,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": true, "message": err.Error()})
			return
		}

		result := gin.H{
			"success": resp.Success,
			"message": resp.Message,
		}
		if resp.Registration != nil {
			result["registration"] = gin.H{
				"id":           resp.Registration.Id,
				"channel_type": resp.Registration.ChannelType,
				"phone_number": resp.Registration.PhoneNumber,
				"status":       resp.Registration.Status,
				"has_pin":      resp.Registration.HasPin,
			}
		}
		c.JSON(http.StatusOK, result)
	}
}

// handleGetChannelRegistrations returns all channel registrations for the user.
// GET /api/v1/channels/status
func handleGetChannelRegistrations(client pb.TransactionPinServiceClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		userIDStr, _ := userID.(string)

		resp, err := client.GetChannelRegistrations(c.Request.Context(), &pb.GetChannelRegistrationsRequest{
			UserId: userIDStr,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": true, "message": err.Error()})
			return
		}

		registrations := make([]gin.H, 0, len(resp.Registrations))
		for _, r := range resp.Registrations {
			reg := gin.H{
				"id":           r.Id,
				"channel_type": r.ChannelType,
				"phone_number": r.PhoneNumber,
				"status":       r.Status,
				"has_pin":      r.HasPin,
			}
			if r.ActivatedAt != nil {
				reg["activated_at"] = r.ActivatedAt.AsTime()
			}
			if r.CreatedAt != nil {
				reg["created_at"] = r.CreatedAt.AsTime()
			}
			registrations = append(registrations, reg)
		}

		c.JSON(http.StatusOK, gin.H{"registrations": registrations})
	}
}

// handleDeactivateChannel deactivates a banking channel.
// DELETE /api/v1/channels/{type}
func handleDeactivateChannel(client pb.TransactionPinServiceClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelType := c.Param("type")
		if channelType == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "channel type is required"})
			return
		}

		if channelType != "whatsapp" && channelType != "telephony" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "can only deactivate 'whatsapp' or 'telephony' channels"})
			return
		}

		userID, _ := c.Get("user_id")
		userIDStr, _ := userID.(string)
		if userIDStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}

		resp, err := client.DeactivateChannel(c.Request.Context(), &pb.DeactivateChannelRequest{
			UserId:      userIDStr,
			ChannelType: channelType,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": true, "message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": resp.Success,
			"message": resp.Message,
		})
	}
}

// handleResolvePhoneToUser resolves a phone number to a user ID (service-to-service).
// POST /api/v1/channels/resolve-phone
func handleResolvePhoneToUser(client pb.TransactionPinServiceClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Service-to-service auth: restrict to internal callers only
		serviceSecret := c.GetHeader("X-Service-Secret")
		expected := os.Getenv("INTERNAL_SERVICE_SECRET")
		if expected == "" || serviceSecret != expected {
			c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized: service-to-service only"})
			return
		}

		var req struct {
			PhoneNumber string `json:"phone_number"`
			ChannelType string `json:"channel_type"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "message": err.Error()})
			return
		}

		resp, err := client.ResolvePhoneToUser(c.Request.Context(), &pb.ResolvePhoneToUserRequest{
			PhoneNumber: req.PhoneNumber,
			ChannelType: req.ChannelType,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": true, "message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"found":          resp.Found,
			"user_id":        resp.UserId,
			"channel_status": resp.ChannelStatus,
			"has_pin":        resp.HasPin,
		})
	}
}

// channelTypeToProto converts a string channel type to the proto enum.
func channelTypeToProto(channelType string) pb.PinChannelType {
	switch channelType {
	case "whatsapp":
		return pb.PinChannelType_PIN_CHANNEL_WHATSAPP
	case "telephony":
		return pb.PinChannelType_PIN_CHANNEL_TELEPHONY
	default:
		return pb.PinChannelType_PIN_CHANNEL_APP
	}
}

// channelTypeFromProto converts the proto enum to a string channel type.
func channelTypeFromProto(ct pb.PinChannelType) string {
	switch ct {
	case pb.PinChannelType_PIN_CHANNEL_WHATSAPP:
		return "whatsapp"
	case pb.PinChannelType_PIN_CHANNEL_TELEPHONY:
		return "telephony"
	default:
		return "app"
	}
}

// interceptVerifyTransactionPin is a Gin middleware that intercepts PIN and channel management
// HTTP endpoints and handles them via custom gRPC handlers instead of passing to grpc-gateway.
func interceptVerifyTransactionPin(client pb.TransactionPinServiceClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		if client == nil {
			c.Next()
			return
		}

		path := c.Param("path")
		method := c.Request.Method

		// POST /v1/auth/verify-transaction-pin
		if method == http.MethodPost && path == "/v1/auth/verify-transaction-pin" {
			handleVerifyTransactionPin(client)(c)
			c.Abort()
			return
		}

		// GET /v1/auth/channel-pins
		if method == http.MethodGet && path == "/v1/auth/channel-pins" {
			handleGetChannelPins(client)(c)
			c.Abort()
			return
		}

		// POST /v1/channels/register
		if method == http.MethodPost && path == "/v1/channels/register" {
			handleRegisterChannel(client)(c)
			c.Abort()
			return
		}

		// POST /v1/channels/verify-otp
		if method == http.MethodPost && path == "/v1/channels/verify-otp" {
			handleVerifyChannelOTP(client)(c)
			c.Abort()
			return
		}

		// GET /v1/channels/status
		if method == http.MethodGet && path == "/v1/channels/status" {
			handleGetChannelRegistrations(client)(c)
			c.Abort()
			return
		}

		// POST /v1/channels/resolve-phone (service-to-service)
		if method == http.MethodPost && path == "/v1/channels/resolve-phone" {
			handleResolvePhoneToUser(client)(c)
			c.Abort()
			return
		}

		// DELETE /v1/channels/{type} — match pattern /v1/channels/whatsapp or /v1/channels/telephony
		if method == http.MethodDelete && strings.HasPrefix(path, "/v1/channels/") {
			channelType := strings.TrimPrefix(path, "/v1/channels/")
			if channelType == "whatsapp" || channelType == "telephony" {
				// Inject the channel type as a param for the handler
				c.Params = append(c.Params, gin.Param{Key: "type", Value: channelType})
				handleDeactivateChannel(client)(c)
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// interceptProfilePictureUploadURL intercepts the storage-proxy endpoints
// (profile-picture + bank-scan upload-url) and dispatches to the
// StorageProxy handler. Same pattern as interceptVerifyTransactionPin —
// we need it because Any("/*path") below already claims every path
// under /api, so adding these as POST routes would panic on overlap.
func interceptProfilePictureUploadURL(p *proxy.StorageProxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p == nil {
			c.Next()
			return
		}
		path := c.Param("path")
		method := c.Request.Method
		if method == http.MethodPost {
			switch path {
			case "/v1/profile-picture/upload-url":
				p.HandleProfilePictureUploadURL(c)
				c.Abort()
				return
			case "/v1/bank-scan/upload-url":
				p.HandleBankScanUploadURL(c)
				c.Abort()
				return
			case "/v1/chat-media/upload-url":
				p.HandleChatMediaUploadURL(c)
				c.Abort()
				return
			case "/v1/invoice/upload-url":
				p.HandleInvoiceUploadURL(c)
				c.Abort()
				return
			case "/v1/escrow/upload-url":
				p.HandleEscrowUploadURL(c)
				c.Abort()
				return
			case "/v1/fcy-document/upload-url":
				p.HandleFCYDocumentUploadURL(c)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// interceptClientLogs dispatches POST /api/v1/client-logs to the ClientLogsProxy
// (Flutter → Loki). Registered as the FIRST apiGroup middleware, so it runs
// BEFORE JWTAuthMiddleware — pre-login logs are accepted without a token. Same
// intercept-before-wildcard pattern as the storage/support proxies (a POST route
// would collide with the Any("/*path") wildcard and panic Gin).
// interceptHealthCheck serves GET /api/v1/health with the SAME dependency-
// checking handler bound to the origin-root /health.
//
// It exists as an interceptor rather than a route because apiGroup registers
// Any("/*path"), and Gin panics at startup on an overlapping concrete route —
// the same reason the storage / unified-search / support handlers are wired
// this way.
//
// Why /api/v1 at all: the Cloudflare tunnel only routes `^/api/v1/...`, so the
// root /health is unreachable from outside the origin. That is what pushed the
// mobile app into probing `/api/v1/internal/voice-agents/settings` to decide
// whether the backend was up — tying "is the platform alive" to one unrelated
// feature's endpoint, where a routing change would have shown every user the
// maintenance screen while everything was fine.
//
// Deliberately registered BEFORE JWTAuthMiddleware: this is probed from the
// pre-login screen and from sessions whose token has expired, so gating it
// would report DOWN precisely when the app needs the truth.
func interceptHealthCheck(handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if handler != nil && c.Request.Method == http.MethodGet && c.Param("path") == "/v1/health" {
			handler(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

func interceptClientLogs(p *proxy.ClientLogsProxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p != nil && c.Request.Method == http.MethodPost && c.Param("path") == "/v1/client-logs" {
			p.Handle(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

// interceptUnifiedSearch dispatches GET /api/v1/users/search-unified to the
// composed UnifiedSearchProxy (saved recipients incl. alias → global users).
// Same intercept-before-wildcard pattern as interceptProfilePictureUploadURL.
func interceptUnifiedSearch(p *proxy.UnifiedSearchProxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p != nil && c.Request.Method == http.MethodGet && c.Param("path") == "/v1/users/search-unified" {
			p.HandleUnifiedSearch(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

// interceptRecipients bridges /api/v1/recipients/* onto the grpc-gateway mux,
// whose generated RecipientService routes are rooted at /v1/recipients/* (the
// proto's http rules carry no /api prefix, unlike most other services on this
// mux). Without this rewrite every recipient HTTP call 404s — the Flutter app
// talks gRPC so it never noticed, but HTTP consumers (chat agents' saved
// recipient search/list/auto-save) silently failed. Same
// intercept-before-wildcard pattern as interceptUnifiedSearch; JWT middleware
// has already run on the apiGroup.
func interceptRecipients(mux *runtime.ServeMux) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Param("path"), "/v1/recipients") {
			c.Request.URL.Path = strings.TrimPrefix(c.Request.URL.Path, "/api")
			c.Request.URL.RawPath = ""
			c.Status(http.StatusOK)
			mux.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}
		c.Next()
	}
}

// interceptSupport dispatches the user-facing support surface
// (/api/v1/support/...) to support-service via SupportProxy. Any method under
// the /v1/support/ prefix is forwarded. Same intercept-before-wildcard pattern
// as interceptUnifiedSearch — the Any("/*path") wildcard would otherwise claim
// these paths. JWTAuthMiddleware has already run on the apiGroup, so the caller
// is authenticated before we forward the token to support-service.
func interceptSupport(p *proxy.SupportProxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p != nil && strings.HasPrefix(c.Param("path"), "/v1/support/") {
			p.Handle(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

// wrapGrpcGateway wraps grpc-gateway mux as a Gin handler.
// Proto HTTP annotations use full "/api/v1/..." paths, so we pass the request
// as-is without stripping the /api prefix.
//
// Gin pre-sets status 404 on NoRoute handlers; grpc-gateway's success path
// doesn't call WriteHeader(200), so the first Write flushes the pre-set 404
// with a valid body. Pin the status to 200 — error paths in grpc-gateway
// explicitly call WriteHeader(4xx/5xx) and still override correctly.
func wrapGrpcGateway(mux *runtime.ServeMux) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Status(http.StatusOK)
		mux.ServeHTTP(c.Writer, c.Request)
	}
}

// customHeaderMatcher matches custom headers
// Includes all headers needed for proper request routing and context propagation
func customHeaderMatcher(key string) (string, bool) {
	switch key {
	case "Authorization", "X-User-Id", "X-Username", "X-Account-Id", "X-Currency", "X-User-Country", "X-Locale", "X-Service-Name",
		// Forward the Firebase App Check token (device attestation) from the HTTP
		// path to gRPC metadata so the App Check interceptor can verify it.
		"X-Firebase-Appcheck":
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

// registerMultiCountryServiceHandler registers multi-country account service gRPC-gateway handler
// This connects to accounts-microservice (multi-country accounts are part of accounts service)
func registerMultiCountryServiceHandler(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
	return accountspb.RegisterMultiCountryAccountServiceHandlerFromEndpoint(ctx, mux, addr, opts)
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

// registerReferralServiceHandler registers referral service gRPC-gateway handler
// This connects to referral-microservice on port 50084
func registerReferralServiceHandler(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
	return pb.RegisterReferralServiceHandlerFromEndpoint(ctx, mux, addr, opts)
}

// registerVoiceSessionServiceHandler registers voice session service gRPC-gateway handler
// This proxies /v1/voice/session/start to the local voice session proxy (which forwards to Python voice-agent-gateway)
func registerVoiceSessionServiceHandler(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
	return pb.RegisterVoiceSessionServiceHandlerFromEndpoint(ctx, mux, addr, opts)
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
