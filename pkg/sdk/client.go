package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"strings"
	"time"
)

// Client is a concurrency-safe TypeSafe API client.
type Client struct {
	httpClient   *http.Client
	apiKey       string
	baseURL      string
	defaultModel string
	timeout      time.Duration
	headers      http.Header
	retry        RetryPolicy
	logger       *slog.Logger
}

// NewClient creates a TypeSafe API client.
func NewClient(options ...ClientOption) (*Client, error) {
	config, err := resolveConfig(options)
	if err != nil {
		return nil, err
	}
	return &Client{
		httpClient:   config.httpClient,
		apiKey:       config.apiKey,
		baseURL:      strings.TrimRight(config.baseURL, "/"),
		defaultModel: config.defaultModel,
		timeout:      config.timeout,
		headers:      config.headers,
		retry:        config.retry,
		logger:       config.logger,
	}, nil
}

// Close releases idle connections held by the underlying transport. It is safe
// to call more than once and does not prevent later use of the client.
func (c *Client) Close() {
	if c == nil || c.httpClient == nil {
		return
	}
	c.httpClient.CloseIdleConnections()
}

// SystemOne evaluates named questions against state.
func (c *Client) SystemOne(ctx context.Context, request SystemOneRequest, options ...RequestOption) (*SystemOneResponse, error) {
	result := new(SystemOneResponse)
	if err := c.do(ctx, http.MethodPost, SystemOnePath, request, result, options...); err != nil {
		return nil, err
	}
	return result, nil
}

// SystemOneInto evaluates named questions and decodes the response into dst.
// dst must be a non-nil pointer.
func (c *Client) SystemOneInto(ctx context.Context, request SystemOneRequest, dst any, options ...RequestOption) error {
	if dst == nil {
		return errors.New("response destination must not be nil")
	}
	value := reflectValue(dst)
	if !value {
		return errors.New("response destination must be a non-nil pointer")
	}
	return c.do(ctx, http.MethodPost, SystemOnePath, request, dst, options...)
}

// ListModels returns models available to the account.
func (c *Client) ListModels(ctx context.Context, options ...RequestOption) (*ListModelsResponse, error) {
	result := new(ListModelsResponse)
	if err := c.do(ctx, http.MethodGet, ModelsPath, nil, result, options...); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) do(ctx context.Context, method, path string, input any, output any, options ...RequestOption) error {
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	requestOptions := requestConfig{}
	for _, option := range options {
		if option != nil {
			if err := option(&requestOptions); err != nil {
				return err
			}
		}
	}
	if requestOptions.timeoutSet {
		if err := validateTimeout(requestOptions.timeout); err != nil {
			return err
		}
	}
	policy := c.retry
	if requestOptions.retry != nil {
		policy = *requestOptions.retry
	}
	if err := policy.Validate(); err != nil {
		return err
	}

	var body []byte
	var err error
	if input != nil {
		if request, ok := input.(SystemOneRequest); ok {
			request.Model = c.defaultModel
			if requestOptions.model != "" {
				request.Model = requestOptions.model
			}
			body, err = mergeRequestBody(request, requestOptions.extraBody)
		} else {
			body, err = json.Marshal(input)
		}
		if err != nil {
			return err
		}
	}
	endpoint, err := c.endpoint(path)
	if err != nil {
		return err
	}
	return retryLoop(ctx, policy, func(parent context.Context, attempt int) error {
		return c.doAttempt(parent, method, endpoint, body, output, requestOptions, attempt)
	})
}

func (c *Client) doAttempt(parent context.Context, method, endpoint string, body []byte, output any, options requestConfig, attempt int) error {
	requestContext := parent
	cancel := func() {}
	timeout := c.timeout
	if options.timeoutSet {
		timeout = options.timeout
	}
	if timeout > 0 {
		requestContext, cancel = context.WithTimeout(parent, timeout)
	}
	defer cancel()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(requestContext, method, endpoint, reader)
	if err != nil {
		return &ConnectionError{Cause: err}
	}
	headers := mergeHeaders(c.headers, options.headers)
	setProtectedHeaders(headers, c.apiKey, attempt)
	for key, values := range headers {
		request.Header[key] = append([]string(nil), values...)
	}
	if body != nil {
		setHeaderFold(request.Header, ContentTypeHeader, JSONContentType)
	}
	if c.logger != nil {
		c.logger.Debug("sending TypeSafe request", "method", method, "url", sanitizeEndpoint(method+" "+endpoint), "attempt", attempt+1, "headers", redactHeaders(headers))
	}
	started := time.Now()
	response, err := c.httpClient.Do(request)
	if err != nil {
		if parent.Err() != nil {
			return parent.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return &TimeoutError{
				ConnectionError: &ConnectionError{Cause: err},
				Timeout:         timeout,
			}
		}
		return &ConnectionError{Cause: err}
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		if parent.Err() != nil {
			return parent.Err()
		}
		return &ConnectionError{Cause: readErr}
	}
	requestID := headerValue(response.Header, RequestIDHeader)
	if c.logger != nil {
		c.logger.Debug("received TypeSafe response", "method", method, "url", sanitizeEndpoint(method+" "+endpoint), "status", response.StatusCode, "elapsed_ms", time.Since(started).Milliseconds(), "request_id", requestID)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return NewAPIError(response.StatusCode, decodeErrorBody(data), response.Header, method+" "+endpoint)
	}
	if err := json.Unmarshal(data, output); err != nil {
		validation := enrichValidationError(err, response.StatusCode, decodeErrorBody(data), response.Header, method+" "+endpoint)
		var responseValidation *ResponseValidationError
		if !errors.As(validation, &responseValidation) {
			validation = &ResponseValidationError{
				Status:    response.StatusCode,
				Body:      decodeErrorBody(data),
				Headers:   cloneHeader(response.Header),
				Endpoint:  sanitizeEndpoint(method + " " + endpoint),
				RequestID: requestID,
				Cause:     validation,
			}
		}
		return validation
	}
	setResponseMetadata(output, ResponseMetadata{
		StatusCode: response.StatusCode,
		Headers:    response.Header,
		RequestID:  requestID,
		RawBody:    data,
	})
	return nil
}

func (c *Client) endpoint(path string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid base URL %q", c.baseURL)
	}
	joined := strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(path, "/")
	base.Path, base.RawPath, base.RawQuery, base.Fragment = joined, "", "", ""
	return base.String(), nil
}

func setProtectedHeaders(headers http.Header, apiKey string, attempt int) {
	setHeaderFold(headers, AuthorizationHeader, "Bearer "+apiKey)
	setHeaderFold(headers, AcceptHeader, JSONContentType)
	setHeaderFold(headers, UserAgentHeader, SDKName+"/"+Version)
	setHeaderFold(headers, SDKHeader, SDKName+"/"+Version)
	setHeaderFold(headers, RuntimeHeader, "go/"+runtime.Version()+" ("+runtime.GOOS+"; "+runtime.GOARCH+")")
	if attempt > 0 {
		setHeaderFold(headers, RetryCountHeader, fmt.Sprint(attempt))
	} else {
		deleteHeaderFold(headers, RetryCountHeader)
	}
}

func setHeaderFold(headers http.Header, name, value string) {
	deleteHeaderFold(headers, name)
	headers[http.CanonicalHeaderKey(name)] = []string{value}
}

func setResponseMetadata(output any, metadata ResponseMetadata) {
	metadata = metadata.clone()
	switch value := output.(type) {
	case *SystemOneResponse:
		value.Metadata = metadata
	case *ListModelsResponse:
		value.Metadata = metadata
	}
}

func redactHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		value := strings.Join(values, ",")
		lower := strings.ToLower(key)
		if lower == "authorization" || lower == "proxy-authorization" || lower == "cookie" || lower == "set-cookie" || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || lower == "x-api-key" || lower == "api-key" {
			value = "***"
		}
		result[key] = value
	}
	return result
}

func reflectValue(value any) bool {
	// Keep reflection isolated to the public destination check and avoid
	// accepting typed nil pointers that would panic during decoding.
	v := reflect.ValueOf(value)
	return v.IsValid() && v.Kind() == reflect.Pointer && !v.IsNil()
}
