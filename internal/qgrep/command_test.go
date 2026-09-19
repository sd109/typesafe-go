package qgrep

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sd109/typesafe-go/pkg/sdk"
)

type fakeClient struct {
	response  *sdk.SystemOneResponse
	err       error
	request   sdk.SystemOneRequest
	called    bool
	closed    bool
	callCtx   context.Context
	optionCnt int
}

func (f *fakeClient) SystemOne(ctx context.Context, request sdk.SystemOneRequest, options ...sdk.RequestOption) (*sdk.SystemOneResponse, error) {
	f.called = true
	f.callCtx = ctx
	f.request = request
	f.optionCnt = len(options)
	return f.response, f.err
}

func (f *fakeClient) Close() { f.closed = true }

func TestCommandRendersTextAndBuildsRequest(t *testing.T) {
	fake := &fakeClient{response: &sdk.SystemOneResponse{Answers: map[string]sdk.Answer{
		"noul-1":   sdk.NoulAnswer{Noul: 0.9731},
		"choice-1": sdk.ChoiceAnswer{Choice: "meeting note", Confidence: 0.9214},
		"score-1":  sdk.ScoreAnswer{Score: 1.784, Confidence: 0.8832},
	}}}
	command := NewCommand(func() (Client, error) { return fake, nil })
	command.SetArgs([]string{
		"--noul", "Does this contain PII?",
		"--choice", "What is it? {log, meeting note, other}",
		"--score", "How old is it? [day, week, year]",
		"--model", "custom-model",
	})
	command.SetIn(strings.NewReader("document contents"))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !fake.called || !fake.closed {
		t.Fatalf("called=%v closed=%v", fake.called, fake.closed)
	}
	if fake.callCtx == nil || fake.request.State != "document contents" {
		t.Fatalf("request = %#v", fake.request)
	}
	if len(fake.request.Questions) != 3 || fake.optionCnt != 1 {
		t.Fatalf("questions=%d options=%d", len(fake.request.Questions), fake.optionCnt)
	}
	if strings.Count(output.String(), "├") < 3 {
		t.Fatalf("output is missing row separators: %q", output.String())
	}
	if !strings.Contains(output.String(), "│  TYPE  │    VALUE     │ CONFIDENCE │") ||
		!strings.Contains(output.String(), "│ noul   │ 0.9731       │            │") ||
		!strings.Contains(output.String(), "│ choice │ meeting note │ 0.9214     │") ||
		!strings.Contains(output.String(), "│ score  │ 1.7840       │ 0.8832     │") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestCommandRendersJSONInQueryOrder(t *testing.T) {
	fake := &fakeClient{response: &sdk.SystemOneResponse{Answers: map[string]sdk.Answer{
		"noul-1":   sdk.NoulAnswer{Noul: 0.5},
		"choice-1": sdk.ChoiceAnswer{Choice: "b", Confidence: 0.7},
	}}}
	command := NewCommand(func() (Client, error) { return fake, nil })
	command.SetArgs([]string{"--choice", "choose {a, b}", "--noul", "is it?", "--json"})
	command.SetIn(strings.NewReader("content"))
	var output bytes.Buffer
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := `[{"id":"noul-1","question":"is it?","type":"noul","answer":{"type":"noul","noul":0.5}},{"id":"choice-1","question":"choose {a, b}","type":"choice","answer":{"type":"choice","choice":"b","confidence":0.7,"probabilities":null}}]` + "\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestCommandRejectsEmptyInputBeforeCreatingClient(t *testing.T) {
	created := false
	command := NewCommand(func() (Client, error) {
		created = true
		return &fakeClient{}, nil
	})
	command.SetArgs([]string{"--noul", "is it?"})
	command.SetIn(strings.NewReader(" \n"))
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "stdin is empty") {
		t.Fatalf("error = %v", err)
	}
	if created {
		t.Fatal("client was created for invalid local input")
	}
}

func TestCommandPropagatesClientErrors(t *testing.T) {
	wantErr := errors.New("request failed")
	fake := &fakeClient{err: wantErr}
	command := NewCommand(func() (Client, error) { return fake, nil })
	command.SetArgs([]string{"--noul", "is it?"})
	command.SetIn(strings.NewReader("content"))
	if err := command.Execute(); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v", err)
	}
	if !fake.closed {
		t.Fatal("client was not closed")
	}
}

func TestCommandRequiresAQuestion(t *testing.T) {
	command := NewCommand(func() (Client, error) { return &fakeClient{}, nil })
	command.SetIn(strings.NewReader("content"))
	if err := command.Execute(); err == nil {
		t.Fatal("expected missing question error")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestCommandPropagatesOutputErrors(t *testing.T) {
	fake := &fakeClient{response: &sdk.SystemOneResponse{Answers: map[string]sdk.Answer{
		"noul-1": sdk.NoulAnswer{Noul: 0.5},
	}}}
	command := NewCommand(func() (Client, error) { return fake, nil })
	command.SetArgs([]string{"--noul", "is it?"})
	command.SetIn(strings.NewReader("content"))
	command.SetOut(failingWriter{})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "write output") {
		t.Fatalf("error = %v", err)
	}
}

func TestCommandRejectsMissingAnswer(t *testing.T) {
	fake := &fakeClient{response: &sdk.SystemOneResponse{Answers: map[string]sdk.Answer{}}}
	command := NewCommand(func() (Client, error) { return fake, nil })
	command.SetArgs([]string{"--noul", "is it?"})
	command.SetIn(strings.NewReader("content"))
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "noul-1") {
		t.Fatalf("error = %v", err)
	}
}

func TestCommandWithRealSDKClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != sdk.SystemOnePath || request.Header.Get(sdk.AuthorizationHeader) != "Bearer key" {
			t.Errorf("request = %s %s, authorization = %q", request.Method, request.URL, request.Header.Get(sdk.AuthorizationHeader))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["state"] != "document" || body["model"] != "custom-model" {
			t.Errorf("state/model = %#v/%#v", body["state"], body["model"])
		}
		questions, ok := body["questions"].(map[string]any)
		if !ok || questions["noul-1"] == nil {
			t.Errorf("questions = %#v", body["questions"])
		}
		writer.Header().Set("Content-Type", sdk.JSONContentType)
		_, _ = writer.Write([]byte(`{"model":"jev-latest","usage":{},"answers":{"noul-1":{"type":"noul","noul":0.75}}}`))
	}))
	defer server.Close()

	command := NewCommand(func() (Client, error) {
		return sdk.NewClient(sdk.WithAPIKey("key"), sdk.WithBaseURL(server.URL))
	})
	command.SetArgs([]string{"--noul", "does it contain PII?", "--model", "custom-model"})
	command.SetIn(strings.NewReader("document"))
	var output bytes.Buffer
	command.SetOut(&output)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "│ noul │ 0.7500 │            │") {
		t.Fatalf("output = %q", output.String())
	}
}
