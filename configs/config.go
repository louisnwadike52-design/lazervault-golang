package configs

import (
	"log"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	DBDriver                 string        `mapstructure:"DB_DRIVER"`
	DBSource                 string        `mapstructure:"DB_SOURCE"`
	ServerAddress            string        `mapstructure:"SERVER_ADDRESS"`
	GrpcServerAddress        string        `mapstructure:"GRPC_SERVER_ADDRESS"`
	DBHost                   string        `mapstructure:"DB_HOST"`
	DBPort                   string        `mapstructure:"DB_PORT"`
	DBUser                   string        `mapstructure:"DB_USER"`
	DBPassword               string        `mapstructure:"DB_PASSWORD"`
	DBName                   string        `mapstructure:"DB_NAME"`
	DBSSLMode                string        `mapstructure:"DB_SSLMODE"`
	ServerPort               string        `mapstructure:"SERVER_PORT"`
	GRPCServerPort           string        `mapstructure:"GRPC_SERVER_PORT"`
	HTTPServerPort           string        `mapstructure:"HTTP_SERVER_PORT"`
	TokenSymmetricKey        string        `mapstructure:"TOKEN_SYMMETRIC_KEY"`
	AccessTokenDuration      time.Duration `mapstructure:"ACCESS_TOKEN_DURATION"`
	RefreshTokenDuration     time.Duration `mapstructure:"REFRESH_TOKEN_DURATION"`
	VerificationCodeDuration time.Duration `mapstructure:"VERIFICATION_CODE_DURATION"`
	RedisServerAddr          string        `mapstructure:"REDIS_SERVER_ADDR"`
	ENV                      string        `mapstructure:"ENV"`
	EmailSenderName          string        `mapstructure:"EMAIL_SENDER_NAME"`
	EmailSenderAddress       string        `mapstructure:"EMAIL_SENDER_ADDRESS"`
	EmailSenderPassword      string        `mapstructure:"EMAIL_SENDER_PASSWORD"`
	// SMTP Config (Mailgun, Gmail, etc.)
	SMTPHost                 string        `mapstructure:"SMTP_HOST"`               // e.g., smtp.mailgun.org
	SMTPPort                 string        `mapstructure:"SMTP_PORT"`               // e.g., 587
	SMTPAuthAddress          string        `mapstructure:"SMTP_AUTH_ADDRESS"`       // For auth, usually same as SMTP_HOST
	// Twilio Config
	TwilioAccountSID         string        `mapstructure:"TWILIO_ACCOUNT_SID"`
	TwilioAuthToken          string        `mapstructure:"TWILIO_AUTH_TOKEN"`
	TwilioFromNumber         string        `mapstructure:"TWILIO_FROM_NUMBER"` // Or VerifyServiceSID if using Verify API
	PasswordResetOTPDuration time.Duration `mapstructure:"PASSWORD_RESET_OTP_DURATION"`

	// Password Reset Config
	PasswordResetTokenExpiry time.Duration `mapstructure:"PASSWORD_RESET_TOKEN_EXPIRY"`
	FrontendResetPasswordURL string        `mapstructure:"FRONTEND_RESET_PASSWORD_URL"`

	// GCS Config
	GCSBucketName      string `mapstructure:"GCS_BUCKET_NAME"`
	GCSCredentialsFile string `mapstructure:"GCS_CREDENTIALS_FILE"`

	// External Services
	ExchangeRateAPIKey string  `mapstructure:"EXCHANGE_RATE_API_KEY"`
	AiServiceURL        string  `mapstructure:"AI_SERVICE_URL"`
	AI_SCAN_SERVICE_URL string  `mapstructure:"AI_SCAN_SERVICE_URL"`
	OpenAIAPIKey       string  `mapstructure:"OPENAI_API_KEY"`
	OpenAIBaseURL      string  `mapstructure:"OPENAI_BASE_URL"`
	OpenAIModel        string  `mapstructure:"OPENAI_MODEL"`
	OpenAITemperature  float64 `mapstructure:"OPENAI_TEMPERATURE"`
	OpenAIMaxTokens    int     `mapstructure:"OPENAI_MAX_TOKENS"`

	// LiveKit Config
	LiveKitHost      string `mapstructure:"LIVEKIT_URL"`
	LiveKitAPIKey    string `mapstructure:"LIVEKIT_API_KEY"`
	LiveKitAPISecret string `mapstructure:"LIVEKIT_API_SECRET"`

	// Reloadly Gift Cards API Config
	ReloadlyClientID     string `mapstructure:"RELOADLY_CLIENT_ID"`
	ReloadlyClientSecret string `mapstructure:"RELOADLY_CLIENT_SECRET"`
	ReloadlyBaseURL      string `mapstructure:"RELOADLY_BASE_URL"`       // https://giftcards.reloadly.com (production) or https://giftcards-sandbox.reloadly.com (sandbox)
	ReloadlyAuthURL      string `mapstructure:"RELOADLY_AUTH_URL"`       // https://auth.reloadly.com/oauth/token
	ReloadlyEnabled      bool   `mapstructure:"RELOADLY_ENABLED"`

	// Alpaca Stock Trading API Config
	AlpacaAPIKey    string `mapstructure:"ALPACA_API_KEY"`
	AlpacaAPISecret string `mapstructure:"ALPACA_API_SECRET"`
	AlpacaBaseURL   string `mapstructure:"ALPACA_BASE_URL"`  // https://api.alpaca.markets (live) or https://paper-api.alpaca.markets (paper)
	AlpacaDataURL   string `mapstructure:"ALPACA_DATA_URL"`  // https://data.alpaca.markets
	AlpacaEnabled   bool   `mapstructure:"ALPACA_ENABLED"`
	AlpacaIsPaper   bool   `mapstructure:"ALPACA_IS_PAPER"`  // true for paper trading, false for live

	// Stripe Payment API Config
	StripeSecretKey      string `mapstructure:"STRIPE_SECRET_KEY"`
	StripePublishableKey string `mapstructure:"STRIPE_PUBLISHABLE_KEY"`
	StripeWebhookSecret  string `mapstructure:"STRIPE_WEBHOOK_SECRET"`
	StripeBaseURL        string `mapstructure:"STRIPE_BASE_URL"`  // https://api.stripe.com
	StripeEnabled        bool   `mapstructure:"STRIPE_ENABLED"`
}

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("app")
	viper.SetConfigType("env")

	viper.AutomaticEnv()
	err = viper.ReadInConfig()

	if err != nil {
		return
	}

	log.Println("Config loaded successfully")

	err = viper.Unmarshal(&config)
	if err == nil {
		log.Printf("GCS_BUCKET_NAME from config: %s", config.GCSBucketName)
		if envBucket := viper.GetString("GCS_BUCKET_NAME"); envBucket != config.GCSBucketName {
			log.Printf("WARNING: GCS_BUCKET_NAME overridden by environment: %s", envBucket)
		}
	}
	return
}

func (c *Config) GetAESKeyBytes() ([]byte, error) {
	return []byte(c.TokenSymmetricKey), nil
}

// GetReloadlyConfig returns Reloadly API configuration
func (c *Config) GetReloadlyConfig() (clientID, clientSecret, baseURL, authURL string, enabled bool) {
	clientID = c.ReloadlyClientID
	clientSecret = c.ReloadlyClientSecret
	baseURL = c.ReloadlyBaseURL
	if baseURL == "" {
		baseURL = "https://giftcards-sandbox.reloadly.com" // Default to sandbox
	}
	authURL = c.ReloadlyAuthURL
	if authURL == "" {
		authURL = "https://auth.reloadly.com/oauth/token"
	}
	enabled = c.ReloadlyEnabled
	return
}

// GetAlpacaConfig returns Alpaca API configuration
func (c *Config) GetAlpacaConfig() (apiKey, apiSecret, baseURL, dataURL string, enabled, isPaper bool) {
	apiKey = c.AlpacaAPIKey
	apiSecret = c.AlpacaAPISecret
	baseURL = c.AlpacaBaseURL
	if baseURL == "" {
		if c.AlpacaIsPaper {
			baseURL = "https://paper-api.alpaca.markets"
		} else {
			baseURL = "https://api.alpaca.markets"
		}
	}
	dataURL = c.AlpacaDataURL
	if dataURL == "" {
		dataURL = "https://data.alpaca.markets"
	}
	enabled = c.AlpacaEnabled
	isPaper = c.AlpacaIsPaper
	return
}

// GetStripeConfig returns Stripe API configuration
func (c *Config) GetStripeConfig() (secretKey, publishableKey, webhookSecret, baseURL string, enabled bool) {
	secretKey = c.StripeSecretKey
	publishableKey = c.StripePublishableKey
	webhookSecret = c.StripeWebhookSecret
	baseURL = c.StripeBaseURL
	if baseURL == "" {
		baseURL = "https://api.stripe.com"
	}
	enabled = c.StripeEnabled
	return
}
