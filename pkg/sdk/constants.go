package sdk

import "time"

const (
	APIKeyEnv       = "TYPESAFE_API_KEY"
	BaseURLEnv      = "TYPESAFE_BASE_URL"
	DefaultModelEnv = "TYPESAFE_DEFAULT_MODEL"
	LogLevelEnv     = "TYPESAFE_LOG_LEVEL"

	DefaultBaseURL = "https://api.typesafe.ai"
	DefaultModel   = "jev-latest"

	SystemOnePath = "/v1/systemone"
	ModelsPath    = "/v1/models"

	JSONContentType = "application/json"

	AuthorizationHeader = "Authorization"
	AcceptHeader        = "Accept"
	ContentTypeHeader   = "Content-Type"
	UserAgentHeader     = "User-Agent"
	SDKHeader           = "X-TypeSafe-SDK"
	RuntimeHeader       = "X-TypeSafe-Runtime"
	RetryCountHeader    = "X-TypeSafe-Retry-Count"
	RequestIDHeader     = "x-typesafe-request-id"
	RetryAfterHeader    = "retry-after"
	RetryAfterMSHeader  = "retry-after-ms"

	SDKName = "typesafe-sdk-go"

	NoulType   = "noul"
	ChoiceType = "choice"
	ScoreType  = "score"
)

const (
	DefaultTimeout        = 10 * time.Second
	DefaultRetryTimeout   = 30 * time.Second
	DefaultMaxRetries     = 2
	DefaultBackoffInitial = 500 * time.Millisecond
	DefaultBackoffMax     = 5 * time.Second
	DefaultBackoffJitter  = 0.25
)
