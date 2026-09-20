package qlog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

func TestSelectContainersDefaultsAndOrdersAllContainers(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "api-0",
			Namespace:   "ns",
			Annotations: map[string]string{"kubectl.kubernetes.io/default-container": "worker"},
		},
		Spec: corev1.PodSpec{
			InitContainers:      []corev1.Container{{Name: "init", Image: "init:v1"}},
			Containers:          []corev1.Container{{Name: "api", Image: "api:v1"}, {Name: "worker", Image: "worker:v1"}},
			EphemeralContainers: []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debug", Image: "debug:v1"}}},
		},
	}
	selected, err := SelectContainers(pod, LogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected[0].Name != "worker" || selected[0].Type != ContainerRegular {
		t.Fatalf("default selection = %#v", selected)
	}
	selected, err = SelectContainers(pod, LogOptions{AllContainers: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := containerNames(selected); len(got) != 4 || got[0] != "init" || got[1] != "api" || got[2] != "worker" || got[3] != "debug" {
		t.Fatalf("all selection = %v", got)
	}
	if selected[0].Type != ContainerInit || selected[3].Type != ContainerEphemeral || selected[3].Image != "debug:v1" {
		t.Fatalf("container metadata = %#v", selected)
	}
}

func TestSelectContainersValidatesExplicitAndMissingContainers(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "ns"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}}}}
	if _, err := SelectContainers(pod, LogOptions{Container: "missing"}); err == nil {
		t.Fatal("expected missing container error")
	}
	if _, err := SelectContainers(pod, LogOptions{Container: "api", AllContainers: true}); err == nil {
		t.Fatal("expected container conflict")
	}
	if _, err := SelectContainers(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "ns"}}, LogOptions{}); err == nil {
		t.Fatal("expected no regular container error")
	}
}

func TestLogOptionsBuildPodLogOptions(t *testing.T) {
	options := LogOptions{Since: 1500 * time.Millisecond, Tail: 25}
	request, err := options.PodLogOptions("api")
	if err != nil {
		t.Fatal(err)
	}
	if request.Container != "api" || request.SinceSeconds == nil || *request.SinceSeconds != 2 || request.TailLines == nil || *request.TailLines != 25 {
		t.Fatalf("request = %#v", request)
	}
	if _, err := (LogOptions{Since: time.Second, SinceTime: "2026-01-01T00:00:00Z"}).PodLogOptions("api"); err == nil {
		t.Fatal("expected since conflict")
	}
	if _, err := (LogOptions{SinceTime: "not-a-time"}).PodLogOptions("api"); err == nil {
		t.Fatal("expected invalid since-time error")
	}
	if _, err := (LogOptions{Tail: -2}).PodLogOptions("api"); err == nil {
		t.Fatal("expected invalid tail error")
	}
}

func TestClientLogFetcherUsesPodLogSubresourceAndClosesStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/namespaces/ns/pods/api-0/log" {
			t.Errorf("path = %q", request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("container") != "api" || query.Get("tailLines") != "25" || query.Get("sinceSeconds") != "2" {
			t.Errorf("query = %v", query)
		}
		_, _ = writer.Write([]byte("line one\nline two\n"))
	}))
	defer server.Close()

	config := &rest.Config{
		Host:    server.URL,
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &schema.GroupVersion{Version: "v1"},
			NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		},
	}
	client, err := corev1client.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &ClientLogFetcher{pods: client}
	logs, err := fetcher.Fetch(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-0", Namespace: "ns"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "api:v1"}}},
	}, LogOptions{Since: 1500 * time.Millisecond, Tail: 25})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Logs != "line one\nline two\n" || logs[0].Image != "api:v1" {
		t.Fatalf("logs = %#v", logs)
	}
}

func containerNames(containers []ContainerInfo) []string {
	result := make([]string, len(containers))
	for index := range containers {
		result[index] = containers[index].Name
	}
	return result
}
