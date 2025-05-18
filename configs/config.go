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
	AiServiceURL       string  `mapstructure:"AI_SERVICE_URL"`
	OpenAIAPIKey       string  `mapstructure:"OPENAI_API_KEY"`
	OpenAIBaseURL      string  `mapstructure:"OPENAI_BASE_URL"`
	OpenAIModel        string  `mapstructure:"OPENAI_MODEL"`
	OpenAITemperature  float64 `mapstructure:"OPENAI_TEMPERATURE"`
	OpenAIMaxTokens    int     `mapstructure:"OPENAI_MAX_TOKENS"`

	// LiveKit Config
	LiveKitHost      string `mapstructure:"LIVEKIT_URL"`
	LiveKitAPIKey    string `mapstructure:"LIVEKIT_API_KEY"`
	LiveKitAPISecret string `mapstructure:"LIVEKIT_API_SECRET"`
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
	return
}

func (c *Config) GetAESKeyBytes() ([]byte, error) {
	return []byte(c.TokenSymmetricKey), nil
}
