package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestAndResponseRoundTrip(t *testing.T) {
	request := SystemOneRequest{
		State: "message",
		Model: "jev-latest",
		Questions: map[string]Question{
			"yes":     NoulQuestion{Instructions: "is this true?", Criteria: NoulCriteria{"true": nil}},
			"tone":    ChoiceQuestion{Criteria: map[string]JSONContent{"calm": nil, "angry": "upset"}},
			"quality": ScoreQuestion{Criteria: []JSONContent{"bad", "good"}},
		},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["model"] != "jev-latest" {
		t.Fatalf("model = %#v", wire["model"])
	}
	if got := wire["questions"].(map[string]any)["yes"].(map[string]any)["type"]; got != NoulType {
		t.Fatalf("type = %#v", got)
	}

	var response SystemOneResponse
	if err := json.Unmarshal([]byte(`{"model":"jev-latest","usage":{},"answers":{"yes":{"type":"noul","noul":0.9},"future":{"type":"future","value":1}}}`), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Answers) != 1 {
		t.Fatalf("answers = %#v", response.Answers)
	}
	if _, ok := response.Answers["yes"].(NoulAnswer); !ok {
		t.Fatalf("answer type = %T", response.Answers["yes"])
	}
}

func TestResponseValidationPaths(t *testing.T) {
	cases := []struct {
		body string
		path string
	}{
		{`{"usage":{},"answers":{}}`, "model"},
		{`{"model":"m","usage":{},"answers":{"q":{"type":"noul"}}}`, "answers.q.noul"},
		{`{"model":"m","usage":{},"answers":{"q":"bad"}}`, "answers.q.type"},
	}
	for _, test := range cases {
		var response SystemOneResponse
		err := json.Unmarshal([]byte(test.body), &response)
		var validation *ResponseValidationError
		if !errors.As(err, &validation) || validation.FieldPath != test.path {
			t.Fatalf("body %s: error = %v, validation = %#v", test.body, err, validation)
		}
	}
}

func TestClientSystemOneAndMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != SystemOnePath {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		if got := r.Header.Get(AuthorizationHeader); got != "Bearer key" {
			t.Errorf("authorization = %q", got)
		}
		w.Header().Set(RequestIDHeader, "req-1")
		w.Header().Set("Content-Type", JSONContentType)
		_, _ = w.Write([]byte(`{"model":"jev-latest","usage":{"input_tokens":2},"answers":{"q":{"type":"noul","noul":0.8}}}`))
	}))
	defer server.Close()
	client, err := NewClient(WithAPIKey("key"), WithBaseURL(server.URL), WithRetryPolicy(RetryPolicy{MaxRetries: 0}))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	response, err := client.SystemOne(context.Background(), SystemOneRequest{State: "x", Questions: map[string]Question{"q": NoulQuestion{}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Metadata.RequestID != "req-1" || response.Metadata.StatusCode != http.StatusOK {
		t.Fatalf("metadata = %#v", response.Metadata)
	}
	if response.Answers["q"].(NoulAnswer).Noul != 0.8 {
		t.Fatalf("answer = %#v", response.Answers["q"])
	}
}

func TestAPIErrorDispatchAndRetryAfter(t *testing.T) {
	err := NewAPIError(http.StatusTooManyRequests, map[string]any{"message": "slow down"}, http.Header{RetryAfterMSHeader: []string{"125"}, RequestIDHeader: []string{"req"}}, "GET https://example.test/v1/models?token=secret")
	var rate *RateLimitError
	if !errors.As(err, &rate) {
		t.Fatalf("error type = %T", err)
	}
	if rate.RetryAfter != 125*time.Millisecond || rate.RequestID != "req" {
		t.Fatalf("rate error = %#v", rate)
	}
	if rate.Endpoint != "GET https://example.test/v1/models" {
		t.Fatalf("endpoint = %q", rate.Endpoint)
	}
	if rate.Error() != "GET https://example.test/v1/models: 429 slow down (request_id=req)" {
		t.Fatalf("error = %q", rate.Error())
	}
}

func TestClientRetriesTransientResponse(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"message":"retry"}`))
			return
		}
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()
	policy := RetryPolicy{MaxRetries: 1, BackoffInitial: 0, BackoffMax: 0, RetryTimeout: time.Second, RetryConnections: true, RetryTimeouts: true}
	client, err := NewClient(WithAPIKey("key"), WithBaseURL(server.URL), WithRetryPolicy(policy))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.ListModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d", calls.Load())
	}
}
