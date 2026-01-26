module lazervaultGo

go 1.25.4

// Local microservice proto package imports
replace (
	accounts-service => ../../microservices/accounts-service/accounts-microservice
	auth-service => ../../microservices/auth-service/auth-microservice
	github.com/lazervault/kafka-client => ../../infrastructure/shared-libs/kafka-client
	github.com/lazervault/shared/auth-interceptor => ../../microservices/shared/auth-interceptor
	github.com/lazervault/shared/degradation => ../../microservices/shared/degradation
	github.com/lazervault/shared/errors => ../../microservices/shared/errors
	github.com/lazervault/shared/retry => ../../microservices/shared/retry
	github.com/lazervault/shared/transaction-pin-client => ../../microservices/shared/transaction-pin-client
)

require (
	accounts-service v0.0.0
	github.com/gin-contrib/cors v1.7.6
	github.com/go-redis/redis/v8 v8.11.5
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.4
	github.com/hibiken/asynq v0.25.1
	github.com/lazervault/shared/auth-interceptor v0.0.0
	github.com/lazervault/shared/degradation v0.0.0
	github.com/lazervault/shared/errors v0.0.0
	github.com/redis/go-redis/v9 v9.17.2
	github.com/rs/zerolog v1.34.0
	github.com/segmentio/kafka-go v0.4.49
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/viper v1.20.1
	go.uber.org/zap v1.27.1
	golang.org/x/time v0.11.0
	google.golang.org/genproto/googleapis/api v0.0.0-20251222181119-0a764e51fe1b
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/bytedance/sonic v1.14.0 // indirect
	github.com/bytedance/sonic/loader v0.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.27.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/goccy/go-yaml v1.18.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.17.8 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.3 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc v1.0.6 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/jwx/v2 v2.1.6 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/quic-go/quic-go v0.54.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/segmentio/asm v1.2.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.0 // indirect
	go.uber.org/mock v0.5.0 // indirect
	golang.org/x/arch v0.20.0 // indirect
	golang.org/x/mod v0.30.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/tools v0.39.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251222181119-0a764e51fe1b // indirect
)

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gin-gonic/gin v1.11.0
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/google/uuid v1.6.0
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/sagikazarmark/locafero v0.7.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.12.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/subosito/gotenv v1.6.0
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.46.0
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/grpc v1.78.0
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gorm.io/gorm v1.25.12
)
