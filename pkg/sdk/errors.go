package sdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// APIError represents an unsuccessful HTTP response.
type APIError struct {
	Status    int
	Body      any
	Headers   http.Header
	Endpoint  string
	RequestID string
	Message   string
}

func (e *APIError) Error() string {
	message := e.Message
	if message == "" {
		message = formatBody(e.Body)
	}
	result := strconv.Itoa(e.Status)
	if message != "" {
		result += " " + message
	}
	if e.Endpoint != "" {
		result = e.Endpoint + ": " + result
	}
	if e.RequestID != "" {
		result += " (request_id=" + e.RequestID + ")"
	}
	return result
}

// BadRequestError indicates a 400 response.
type BadRequestError struct{ *APIError }

// AuthenticationError indicates a 401 response.
type AuthenticationError struct{ *APIError }

// PermissionDeniedError indicates a 403 response.
type PermissionDeniedError struct{ *APIError }

// NotFoundError indicates a 404 response.
type NotFoundError struct{ *APIError }

// UnprocessableEntityError indicates a 422 response.
type UnprocessableEntityError struct{ *APIError }

// InternalServerError indicates a 5xx response.
type InternalServerError struct{ *APIError }

func (e *BadRequestError) Unwrap() error          { return e.APIError }
func (e *AuthenticationError) Unwrap() error      { return e.APIError }
func (e *PermissionDeniedError) Unwrap() error    { return e.APIError }
func (e *NotFoundError) Unwrap() error            { return e.APIError }
func (e *UnprocessableEntityError) Unwrap() error { return e.APIError }
func (e *InternalServerError) Unwrap() error      { return e.APIError }

// RateLimitError indicates a 429 response and may contain a server-suggested delay.
type RateLimitError struct {
	*APIError
	RetryAfter    time.Duration
	HasRetryAfter bool
}

func (e *RateLimitError) Unwrap() error { return e.APIError }

// ConnectionError indicates a request failed without receiving an HTTP response.
type ConnectionError struct{ Cause error }

func (e *ConnectionError) Error() string {
	if e.Cause == nil {
		return "Connection error"
	}
	return "Connection error: " + e.Cause.Error()
}
func (e *ConnectionError) Unwrap() error { return e.Cause }

// TimeoutError indicates that an operation exceeded its configured timeout.
type TimeoutError struct {
	*ConnectionError
	Timeout time.Duration
}

func (e *TimeoutError) Error() string {
	if e.Timeout > 0 {
		return fmt.Sprintf("Request timed out (timeout=%s).", e.Timeout)
	}
	return "Request timed out."
}
func (e *TimeoutError) Unwrap() error { return e.ConnectionError }

// ResponseValidationError indicates a successful response did not match its schema.
type ResponseValidationError struct {
	Status    int
	Body      any
	Headers   http.Header
	Endpoint  string
	RequestID string
	FieldPath string
	Cause     error
}

func (e *ResponseValidationError) Error() string {
	message := "Invalid response data"
	if e.FieldPath != "" {
		message += fmt.Sprintf(" at %q", e.FieldPath)
	}
	message += "."
	if e.Endpoint != "" {
		message = e.Endpoint + ": " + strconv.Itoa(e.Status) + " " + message
	} else if e.Status != 0 {
		message = strconv.Itoa(e.Status) + " " + message
	}
	if e.RequestID != "" {
		message += " (request_id=" + e.RequestID + ")"
	}
	return message
}
func (e *ResponseValidationError) Unwrap() error { return e.Cause }

// NewAPIError creates the concrete API error corresponding to status.
func NewAPIError(status int, body any, headers http.Header, endpoint string) error {
	base := &APIError{
		Status:   status,
		Body:     body,
		Headers:  cloneHeader(headers),
		Endpoint: sanitizeEndpoint(endpoint),
	}
	base.RequestID = headerValue(base.Headers, RequestIDHeader)
	base.Message = extractMessage(body)
	switch status {
	case http.StatusBadRequest:
		return &BadRequestError{APIError: base}
	case http.StatusUnauthorized:
		return &AuthenticationError{APIError: base}
	case http.StatusForbidden:
		return &PermissionDeniedError{APIError: base}
	case http.StatusNotFound:
		return &NotFoundError{APIError: base}
	case http.StatusUnprocessableEntity:
		return &UnprocessableEntityError{APIError: base}
	case http.StatusTooManyRequests:
		delay, ok := parseRetryAfter(base.Headers)
		return &RateLimitError{
			APIError:      base,
			RetryAfter:    delay,
			HasRetryAfter: ok,
		}
	default:
		if status >= 500 {
			return &InternalServerError{APIError: base}
		}
		return base
	}
}

func extractMessage(body any) string {
	switch value := body.(type) {
	case string:
		return value
	case json.RawMessage:
		return string(value)
	case map[string]any:
		if value, ok := value["error"].(string); ok {
			return value
		}
		if value, ok := value["error"].(map[string]any); ok {
			if message, ok := value["message"].(string); ok {
				return message
			}
		}
		if value, ok := value["message"].(string); ok {
			return value
		}
		if value, ok := value["detail"].(string); ok {
			return value
		}
		if value, ok := value["detail"].(map[string]any); ok {
			if message, ok := value["message"].(string); ok {
				return message
			}
		}
		if detail, ok := value["detail"].([]any); ok {
			parts := make([]string, 0, len(detail))
			for _, item := range detail {
				entry, ok := item.(map[string]any)
				if !ok {
					continue
				}
				message, ok := entry["msg"].(string)
				if !ok {
					continue
				}
				path := ""
				if locations, ok := entry["loc"].([]any); ok {
					segments := make([]string, 0, len(locations))
					for _, location := range locations {
						if text, ok := location.(string); ok && text != "body" {
							segments = append(segments, text)
						} else if number, ok := location.(float64); ok {
							segments = append(segments, strconv.Itoa(int(number)))
						}
					}
					path = strings.Join(segments, ".")
				}
				if path != "" {
					parts = append(parts, path+": "+message)
				} else {
					parts = append(parts, message)
				}
			}
			return strings.Join(parts, "; ")
		}
	}
	return ""
}

func formatBody(body any) string {
	if body == nil {
		return "status code (no body)"
	}
	message := ""
	if extracted := extractMessage(body); extracted != "" {
		message = extracted
	} else {
		switch value := body.(type) {
		case string:
			message = value
		case json.RawMessage:
			message = string(value)
		default:
			encoded, err := json.Marshal(body)
			if err != nil {
				message = fmt.Sprint(body)
			} else {
				message = string(encoded)
			}
		}
	}
	runes := []rune(message)
	if len(runes) > 200 {
		return string(runes[:200]) + "…"
	}
	return message
}

func parseRetryAfter(headers http.Header) (time.Duration, bool) {
	if raw := strings.TrimSpace(headerValue(headers, RetryAfterMSHeader)); raw != "" {
		if value, err := strconv.ParseFloat(raw, 64); err == nil && finiteNonnegative(value) {
			if delay, ok := durationFromFloat(value, time.Millisecond); ok {
				return delay, true
			}
		}
	}
	if raw := strings.TrimSpace(headerValue(headers, RetryAfterHeader)); raw != "" {
		if value, err := strconv.ParseFloat(raw, 64); err == nil && finiteNonnegative(value) {
			if delay, ok := durationFromFloat(value, time.Second); ok {
				return delay, true
			}
		}
		if date, err := http.ParseTime(raw); err == nil {
			delay := time.Until(date)
			if delay < 0 {
				delay = 0
			}
			return delay, true
		}
	}
	return 0, false
}

func finiteNonnegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}
func durationFromFloat(value float64, unit time.Duration) (time.Duration, bool) {
	if value > float64(math.MaxInt64)/float64(unit) {
		return 0, false
	}
	return time.Duration(value * float64(unit)), true
}

func sanitizeEndpoint(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	parts := strings.SplitN(endpoint, " ", 2)
	if len(parts) != 2 {
		return endpoint
	}
	u, err := url.Parse(parts[1])
	if err != nil {
		return endpoint
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	return parts[0] + " " + u.String()
}

func decodeErrorBody(data []byte) any {
	if len(data) == 0 {
		return nil
	}
	var decoded any
	if json.Unmarshal(data, &decoded) == nil {
		return decoded
	}
	return strings.ToValidUTF8(string(data), "�")
}

func enrichValidationError(err error, status int, body any, headers http.Header, endpoint string) error {
	var validation *ResponseValidationError
	if errors.As(err, &validation) {
		validation.Status = status
		validation.Body = body
		validation.Headers = cloneHeader(headers)
		validation.Endpoint = sanitizeEndpoint(endpoint)
		validation.RequestID = headerValue(headers, RequestIDHeader)
	}
	return err
}

func headerValue(header http.Header, name string) string {
	for key, values := range header {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}
