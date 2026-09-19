package sdk

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net/http"
	"time"
)

// RetryPolicy controls retries for transient API and network failures.
type RetryPolicy struct {
	MaxRetries        int
	BackoffInitial    time.Duration
	BackoffMax        time.Duration
	BackoffJitter     float64
	HTTPStatuses      map[int]struct{}
	RespectRetryAfter bool
	RetryConnections  bool
	RetryTimeouts     bool
	RetryTimeout      time.Duration
}

// DefaultRetryPolicy returns the Python SDK-compatible default policy.
func DefaultRetryPolicy() RetryPolicy {
	statuses := map[int]struct{}{http.StatusRequestTimeout: {}, http.StatusTooManyRequests: {}}
	for status := 500; status <= 599; status++ {
		statuses[status] = struct{}{}
	}
	return RetryPolicy{
		MaxRetries:        DefaultMaxRetries,
		BackoffInitial:    DefaultBackoffInitial,
		BackoffMax:        DefaultBackoffMax,
		BackoffJitter:     DefaultBackoffJitter,
		HTTPStatuses:      statuses,
		RespectRetryAfter: true,
		RetryConnections:  true,
		RetryTimeouts:     true,
		RetryTimeout:      DefaultRetryTimeout,
	}
}

// Validate validates retry counts and delays.
func (p RetryPolicy) Validate() error {
	if p.MaxRetries < 0 {
		return errors.New("max retries must be non-negative")
	}
	if p.BackoffInitial < 0 || p.BackoffMax < 0 {
		return errors.New("backoff durations must be non-negative")
	}
	if p.BackoffJitter < 0 || p.BackoffJitter > 1 || math.IsNaN(p.BackoffJitter) {
		return errors.New("backoff jitter must be between zero and one")
	}
	if p.RetryTimeout < 0 {
		return errors.New("retry timeout must be non-negative")
	}
	return nil
}

func (p RetryPolicy) effectiveStatuses() map[int]struct{} {
	if p.HTTPStatuses != nil {
		return p.HTTPStatuses
	}
	return DefaultRetryPolicy().HTTPStatuses
}
func (p RetryPolicy) retryable(err error) bool {
	var timeout *TimeoutError
	if errors.As(err, &timeout) {
		return p.RetryTimeouts
	}
	var connection *ConnectionError
	if errors.As(err, &connection) {
		return p.RetryConnections
	}
	var api *APIError
	if errors.As(err, &api) {
		_, ok := p.effectiveStatuses()[api.Status]
		return ok
	}
	return false
}

func (p RetryPolicy) delay(attempt int, err error) time.Duration {
	if p.RespectRetryAfter {
		var rate *RateLimitError
		if errors.As(err, &rate) && rate.HasRetryAfter {
			return rate.RetryAfter
		}
		var api *APIError
		if errors.As(err, &api) {
			if value, ok := parseRetryAfter(api.Headers); ok {
				return value
			}
		}
	}
	if p.BackoffInitial == 0 || p.BackoffMax == 0 {
		return 0
	}
	delay := p.BackoffInitial
	for i := 1; i < attempt && delay < p.BackoffMax; i++ {
		if delay > p.BackoffMax/2 {
			delay = p.BackoffMax
		} else {
			delay *= 2
		}
	}
	if delay > p.BackoffMax {
		delay = p.BackoffMax
	}
	if p.BackoffJitter > 0 {
		delay = time.Duration(float64(delay) * (1 - rand.Float64()*p.BackoffJitter))
	}
	return delay
}

func retryLoop(ctx context.Context, policy RetryPolicy, operation func(context.Context, int) error) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	started := time.Now()
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := operation(ctx, attempt)
		if err == nil {
			return nil
		}
		if attempt >= policy.MaxRetries || !policy.retryable(err) {
			return err
		}
		delay := policy.delay(attempt+1, err)
		if policy.RetryTimeout > 0 {
			remaining := policy.RetryTimeout - time.Since(started)
			if remaining <= delay {
				return err
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
