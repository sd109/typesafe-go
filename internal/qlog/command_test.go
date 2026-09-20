package qlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/sd109/typesafe-go/pkg/sdk"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type testResolver struct {
	pods []corev1.Pod
	err  error
}

func (r testResolver) Resolve(context.Context, NamespaceScope, []ResourceRef, bool) ([]corev1.Pod, error) {
	return r.pods, r.err
}

type testFetcher struct {
	logs      map[string][]ContainerLog
	err       error
	calls     int
	active    int
	maxActive int
	mu        sync.Mutex
	delay     time.Duration
}

func (f *testFetcher) Fetch(ctx context.Context, pod *corev1.Pod, _ LogOptions) ([]ContainerLog, error) {
	f.mu.Lock()
	f.calls++
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	}()
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(f.delay):
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.logs[pod.Name], nil
}

type testTypeSafeClient struct {
	response *sdk.SystemOneResponse
	err      error
	requests []sdk.SystemOneRequest
	closed   bool
	mu       sync.Mutex
}

func (f *testTypeSafeClient) SystemOne(_ context.Context, request sdk.SystemOneRequest, _ ...sdk.RequestOption) (*sdk.SystemOneResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, request)
	return f.response, f.err
}

func (f *testTypeSafeClient) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

func TestCommandHelpGroupsQlogAndKubernetesFlags(t *testing.T) {
	command := NewCommand(CommandDependencies{})
	command.SetArgs([]string{"--help"})
	var output bytes.Buffer
	command.SetOut(&output)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	qlogIndex := strings.Index(text, "Qlog options:")
	kubernetesIndex := strings.Index(text, "Kubernetes configuration:")
	if qlogIndex < 0 || kubernetesIndex < 0 || qlogIndex > kubernetesIndex {
		t.Fatalf("grouped help = %q", text)
	}
	if !strings.Contains(text, "--server") || !strings.Contains(text, "--noul") {
		t.Fatalf("help is missing grouped flags: %q", text)
	}
}

func TestCommandFetchesLogsBuildsStructuredStateAndRendersTable(t *testing.T) {
	pod := testPod("ns", "api-0")
	fetcher := &testFetcher{logs: map[string][]ContainerLog{
		"api-0": {{ContainerInfo: ContainerInfo{Name: "api", Type: ContainerRegular, Image: "api:v1"}, Logs: "error: failed\n"}},
	}}
	client := &testTypeSafeClient{response: &sdk.SystemOneResponse{Answers: map[string]sdk.Answer{
		"noul-1": sdk.NoulAnswer{Noul: 0.97},
	}}}
	command := newTestCommand([]corev1.Pod{pod}, fetcher, client)
	command.SetArgs([]string{"--namespace", "ns", "pod/api-0", "--noul", "Do these logs contain errors?"})
	var output bytes.Buffer
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 || !client.closed {
		t.Fatalf("requests=%d closed=%v", len(client.requests), client.closed)
	}
	state, ok := client.requests[0].State.(map[string]any)
	if !ok || state["namespace"] != "ns" || state["resource"] != "pod/api-0" {
		t.Fatalf("state = %#v", client.requests[0].State)
	}
	containers, ok := state["containers"].([]any)
	if !ok || len(containers) != 1 {
		t.Fatalf("containers = %#v", state["containers"])
	}
	container := containers[0].(map[string]any)
	if container["name"] != "api" || container["type"] != ContainerRegular || container["image"] != "api:v1" || container["logs"] != "error: failed\n" {
		t.Fatalf("container = %#v", container)
	}
	tableOutput := output.String()
	if !strings.Contains(tableOutput, "RESOURCE") || !strings.Contains(tableOutput, "0.9700") {
		t.Fatalf("output = %q", tableOutput)
	}
	for _, omittedColumn := range []string{"NAMESPACE", "CONTAINERS", "TYPE", "QUESTION"} {
		if strings.Contains(tableOutput, omittedColumn) {
			t.Fatalf("table output included omitted column %q: %q", omittedColumn, tableOutput)
		}
	}
	if strings.Contains(tableOutput, "api:v1") || strings.Contains(tableOutput, "pod/api-0") {
		t.Fatalf("table output included JSON-only details: %q", tableOutput)
	}
}

func TestCommandRendersOrderedJSONForMultiplePods(t *testing.T) {
	pods := []corev1.Pod{testPod("ns", "api-1"), testPod("ns", "api-0")}
	fetcher := &testFetcher{logs: map[string][]ContainerLog{
		"api-0": {{ContainerInfo: ContainerInfo{Name: "api", Type: ContainerRegular, Image: "api:v1"}, Logs: "zero"}},
		"api-1": {{ContainerInfo: ContainerInfo{Name: "api", Type: ContainerRegular, Image: "api:v1"}, Logs: "one"}},
	}}
	client := &testTypeSafeClient{response: &sdk.SystemOneResponse{Answers: map[string]sdk.Answer{
		"noul-1": sdk.NoulAnswer{Noul: 0.5},
	}}}
	command := newTestCommand(pods, fetcher, client)
	command.SetArgs([]string{"--namespace", "ns", "--all-pods", "--output", "json", "--noul", "is it?"})
	var output bytes.Buffer
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var items []struct {
		Resource  string            `json:"resource"`
		Questions []json.RawMessage `json:"questions"`
	}
	if err := json.Unmarshal(output.Bytes(), &items); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(items) != 2 || items[0].Resource != "pod/api-0" || items[1].Resource != "pod/api-1" {
		t.Fatalf("results are not sorted by pod: %#v", items)
	}
	for _, item := range items {
		if len(item.Questions) != 1 {
			t.Fatalf("questions for %s = %d, want 1", item.Resource, len(item.Questions))
		}
	}
	if strings.Contains(output.String(), `"logs"`) {
		t.Fatalf("output leaked log content: %s", output.String())
	}
}

func TestCommandDryRunDoesNotFetchLogsOrCreateTypeSafeClient(t *testing.T) {
	fetcher := &testFetcher{}
	clientCreated := false
	command := NewCommand(CommandDependencies{
		Config:                  NewConfigOptions(),
		KubernetesClientFactory: func(*ConfigOptions) (KubernetesClient, error) { return fake.NewSimpleClientset(), nil },
		ResolverFactory: func(KubernetesClient) PodResolver {
			return testResolver{pods: []corev1.Pod{testPod("ns", "api-0")}}
		},
		LogFetcherFactory: func(KubernetesClient) LogFetcher { return fetcher },
		TypeSafeClientFactory: func() (TypeSafeClient, error) {
			clientCreated = true
			return &testTypeSafeClient{}, nil
		},
	})
	command.SetArgs([]string{"--namespace", "ns", "--all-pods", "--dry-run", "--output", "json"})
	var output bytes.Buffer
	command.SetOut(&output)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if clientCreated || fetcher.calls != 0 || !strings.Contains(output.String(), `"containers"`) || strings.Contains(output.String(), `"logs"`) {
		t.Fatalf("dry-run client=%v calls=%d output=%q", clientCreated, fetcher.calls, output.String())
	}
}

func TestCommandWithRealSDKClientSendsOneStructuredRequestPerPod(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		requests = append(requests, body)
		mu.Unlock()
		writer.Header().Set("Content-Type", sdk.JSONContentType)
		_, _ = writer.Write([]byte(`{"model":"jev-latest","usage":{},"answers":{"noul-1":{"type":"noul","noul":0.5}}}`))
	}))
	defer server.Close()

	fetcher := &testFetcher{logs: map[string][]ContainerLog{
		"api-0": {{ContainerInfo: ContainerInfo{Name: "api", Type: ContainerRegular, Image: "api:v1"}, Logs: "zero"}},
		"api-1": {{ContainerInfo: ContainerInfo{Name: "api", Type: ContainerRegular, Image: "api:v1"}, Logs: "one"}},
	}}
	command := NewCommand(CommandDependencies{
		Config:                  NewConfigOptions(),
		KubernetesClientFactory: func(*ConfigOptions) (KubernetesClient, error) { return fake.NewSimpleClientset(), nil },
		ResolverFactory: func(KubernetesClient) PodResolver {
			return testResolver{pods: []corev1.Pod{testPod("ns", "api-1"), testPod("ns", "api-0")}}
		},
		LogFetcherFactory: func(KubernetesClient) LogFetcher { return fetcher },
		TypeSafeClientFactory: func() (TypeSafeClient, error) {
			return sdk.NewClient(sdk.WithAPIKey("key"), sdk.WithBaseURL(server.URL))
		},
	})
	command.SetArgs([]string{"--namespace", "ns", "--all-pods", "--noul", "is it?"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("request count = %d", len(requests))
	}
	seen := make(map[string]bool)
	for _, body := range requests {
		state, ok := body["state"].(map[string]any)
		if !ok {
			t.Fatalf("state is not an object: %#v", body["state"])
		}
		seen[state["resource"].(string)] = true
		if _, ok := state["containers"].([]any); !ok {
			t.Fatalf("containers = %#v", state["containers"])
		}
	}
	if !seen["pod/api-0"] || !seen["pod/api-1"] {
		t.Fatalf("resources = %#v", seen)
	}
}

func TestValidateCommandFlagsEnforcesTableWidthLikeQgrep(t *testing.T) {
	if err := validateCommandFlags("table", 4, 0); err == nil || !strings.Contains(err.Error(), "table width must be positive") {
		t.Fatalf("error = %v", err)
	}
	if err := validateCommandFlags("json", 4, 0); err != nil {
		t.Fatalf("JSON should ignore table width: %v", err)
	}
	if err := validateCommandFlags("table", 4, 42); err != nil {
		t.Fatalf("positive table width error = %v", err)
	}
}

func TestCommandRejectsInvalidOutputBeforeCreatingKubernetesClient(t *testing.T) {
	created := false
	command := NewCommand(CommandDependencies{
		Config: NewConfigOptions(),
		KubernetesClientFactory: func(*ConfigOptions) (KubernetesClient, error) {
			created = true
			return fake.NewSimpleClientset(), nil
		},
	})
	command.SetArgs([]string{"--output", "yaml", "--noul", "is it?", "pod/api"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "unsupported output format") {
		t.Fatalf("error = %v", err)
	}
	if created {
		t.Fatal("Kubernetes client was created for invalid local flags")
	}
}

func TestCommandStopsWithoutPartialOutputOnWorkerError(t *testing.T) {
	fetcher := &testFetcher{err: errors.New("log unavailable")}
	client := &testTypeSafeClient{response: &sdk.SystemOneResponse{Answers: map[string]sdk.Answer{
		"noul-1": sdk.NoulAnswer{Noul: 0.5},
	}}}
	command := newTestCommand([]corev1.Pod{testPod("ns", "api-0")}, fetcher, client)
	command.SetArgs([]string{"--namespace", "ns", "pod/api-0", "--noul", "is it?"})
	var output bytes.Buffer
	command.SetOut(&output)
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "log unavailable") {
		t.Fatalf("error = %v", err)
	}
	if output.Len() != 0 || len(client.requests) != 0 {
		t.Fatalf("partial output=%q requests=%d", output.String(), len(client.requests))
	}
}

func TestProcessPodsHonorsParallelism(t *testing.T) {
	pods := []corev1.Pod{testPod("ns", "one"), testPod("ns", "two"), testPod("ns", "three"), testPod("ns", "four")}
	fetcher := &testFetcher{delay: 20 * time.Millisecond, logs: map[string][]ContainerLog{
		"one":   {{ContainerInfo: ContainerInfo{Name: "api", Type: ContainerRegular}, Logs: "one"}},
		"two":   {{ContainerInfo: ContainerInfo{Name: "api", Type: ContainerRegular}, Logs: "two"}},
		"three": {{ContainerInfo: ContainerInfo{Name: "api", Type: ContainerRegular}, Logs: "three"}},
		"four":  {{ContainerInfo: ContainerInfo{Name: "api", Type: ContainerRegular}, Logs: "four"}},
	}}
	client := &testTypeSafeClient{response: &sdk.SystemOneResponse{Answers: map[string]sdk.Answer{"noul-1": sdk.NoulAnswer{Noul: 0.5}}}}
	_, err := processPods(context.Background(), pods, LogOptions{}, 2, map[string]sdk.Question{"noul-1": sdk.NoulQuestion{Instructions: "is it?"}}, fetcher, client)
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.maxActive > 2 {
		t.Fatalf("max active fetches = %d", fetcher.maxActive)
	}
}

func newTestCommand(pods []corev1.Pod, fetcher LogFetcher, client TypeSafeClient) *cobra.Command {
	return NewCommand(CommandDependencies{
		Config:                  NewConfigOptions(),
		KubernetesClientFactory: func(*ConfigOptions) (KubernetesClient, error) { return fake.NewSimpleClientset(), nil },
		ResolverFactory:         func(KubernetesClient) PodResolver { return testResolver{pods: pods} },
		LogFetcherFactory:       func(KubernetesClient) LogFetcher { return fetcher },
		TypeSafeClientFactory:   func() (TypeSafeClient, error) { return client, nil },
	})
}

func testPod(namespace, name string) corev1.Pod {
	return corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "api:v1"}}}}
}
