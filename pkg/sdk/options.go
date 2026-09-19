package sdk

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type clientConfig struct {
	apiKey       string
	baseURL      string
	defaultModel string
	timeout      time.Duration
	headers      http.Header
	httpClient   *http.Client
	retry        RetryPolicy
	logger       *slog.Logger
}

// ClientOption configures a Client.
type ClientOption func(*clientConfig) error

// WithAPIKey sets the bearer API key.
func WithAPIKey(value string) ClientOption {
	return func(c *clientConfig) error { c.apiKey = strings.TrimSpace(value); return nil }
}

// WithBaseURL sets the API root URL.
func WithBaseURL(value string) ClientOption {
	return func(c *clientConfig) error { c.baseURL = strings.TrimSpace(value); return nil }
}

// WithDefaultModel sets the default model used by SystemOne.
func WithDefaultModel(value string) ClientOption {
	return func(c *clientConfig) error { c.defaultModel = strings.TrimSpace(value); return nil }
}

// WithModel is an alias for WithDefaultModel.
func WithModel(value string) ClientOption { return WithDefaultModel(value) }

// WithTimeout sets the default per-operation timeout.
func WithTimeout(value time.Duration) ClientOption {
	return func(c *clientConfig) error { c.timeout = value; return nil }
}

// WithHeaders adds default request headers.
func WithHeaders(value http.Header) ClientOption {
	return func(c *clientConfig) error { c.headers = mergeHeaders(c.headers, value); return nil }
}

// WithHTTPClient supplies the HTTP client used for requests.
func WithHTTPClient(value *http.Client) ClientOption {
	return func(c *clientConfig) error { c.httpClient = value; return nil }
}

// WithRetryPolicy sets the default retry policy.
func WithRetryPolicy(value RetryPolicy) ClientOption {
	return func(c *clientConfig) error { c.retry = cloneRetryPolicy(value); return nil }
}

// WithLogger supplies structured logging. A nil logger disables SDK logging.
func WithLogger(value *slog.Logger) ClientOption {
	return func(c *clientConfig) error { c.logger = value; return nil }
}

// RequestOption configures one API call.
type RequestOption func(*requestConfig) error
type requestConfig struct {
	model      string
	timeout    time.Duration
	timeoutSet bool
	retry      *RetryPolicy
	headers    http.Header
	extraBody  map[string]any
}

// WithRequestModel overrides the model for one SystemOne call.
func WithRequestModel(value string) RequestOption {
	return func(c *requestConfig) error { c.model = strings.TrimSpace(value); return nil }
}

// WithRequestTimeout overrides the timeout for one call.
func WithRequestTimeout(value time.Duration) RequestOption {
	return func(c *requestConfig) error { c.timeout, c.timeoutSet = value, true; return nil }
}

// WithRequestHeaders adds headers for one call.
func WithRequestHeaders(value http.Header) RequestOption {
	return func(c *requestConfig) error { c.headers = mergeHeaders(c.headers, value); return nil }
}

// WithRequestRetryPolicy overrides retries for one call.
func WithRequestRetryPolicy(value RetryPolicy) RequestOption {
	return func(c *requestConfig) error { cloned := cloneRetryPolicy(value); c.retry = &cloned; return nil }
}

// WithExtraBody shallow-merges additional SystemOne body fields.
func WithExtraBody(value map[string]any) RequestOption {
	return func(c *requestConfig) error { c.extraBody = cloneMap(value); return nil }
}

func resolveConfig(options []ClientOption) (clientConfig, error) {
	config := clientConfig{
		apiKey:       envOr(APIKeyEnv, ""),
		baseURL:      envOr(BaseURLEnv, DefaultBaseURL),
		defaultModel: envOr(DefaultModelEnv, DefaultModel),
		timeout:      DefaultTimeout,
		headers:      make(http.Header),
		retry:        DefaultRetryPolicy(),
		logger:       defaultLogger(),
		httpClient:   &http.Client{},
	}
	for _, option := range options {
		if option != nil {
			if err := option(&config); err != nil {
				return config, err
			}
		}
	}
	if config.apiKey == "" {
		return config, fmt.Errorf("no API key was provided; pass WithAPIKey or set %s", APIKeyEnv)
	}
	if config.defaultModel == "" {
		return config, errors.New("default model must not be empty")
	}
	if err := validateTimeout(config.timeout); err != nil {
		return config, err
	}
	if err := validateBaseURL(config.baseURL); err != nil {
		return config, err
	}
	if config.httpClient == nil {
		return config, errors.New("HTTP client must not be nil")
	}
	if err := config.retry.Validate(); err != nil {
		return config, err
	}
	config.headers = cloneHeader(config.headers)
	config.retry = cloneRetryPolicy(config.retry)
	return config, nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
func validateTimeout(value time.Duration) error {
	if value <= 0 {
		return errors.New("timeout must be positive")
	}
	return nil
}
func validateBaseURL(value string) error {
	if value == "" {
		return errors.New("base URL must not be empty")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid base URL %q", value)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("base URL must not contain a query or fragment")
	}
	return nil
}
func mergeHeaders(base, extra http.Header) http.Header {
	result := cloneHeader(base)
	if result == nil {
		result = make(http.Header)
	}
	for key, values := range extra {
		deleteHeaderFold(result, key)
		result[http.CanonicalHeaderKey(key)] = append([]string(nil), values...)
	}
	return result
}

func deleteHeaderFold(headers http.Header, name string) {
	for key := range headers {
		if strings.EqualFold(key, name) {
			delete(headers, key)
		}
	}
}
func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func cloneRetryPolicy(value RetryPolicy) RetryPolicy {
	if value.HTTPStatuses == nil {
		return value
	}
	statuses := make(map[int]struct{}, len(value.HTTPStatuses))
	for status := range value.HTTPStatuses {
		statuses[status] = struct{}{}
	}
	value.HTTPStatuses = statuses
	return value
}
func defaultLogger() *slog.Logger {
	level := new(slog.LevelVar)
	switch strings.ToLower(strings.TrimSpace(os.Getenv(LogLevelEnv))) {
	case "debug":
		level.Set(slog.LevelDebug)
	case "info":
		level.Set(slog.LevelInfo)
	case "warn", "warning":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelError + 1)
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

// FormatTimeout is provided for callers that want to display SDK durations
// consistently in diagnostics.
func FormatTimeout(value time.Duration) string {
	return strconv.FormatFloat(value.Seconds(), 'f', -1, 64) + "s"
}
