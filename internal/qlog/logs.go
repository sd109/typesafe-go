package qlog

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

// ContainerType identifies the source section of a pod container.
type ContainerType string

const (
	ContainerInit      ContainerType = "init"
	ContainerRegular   ContainerType = "regular"
	ContainerEphemeral ContainerType = "ephemeral"
)

// ContainerInfo describes a selected container without its logs.
type ContainerInfo struct {
	Name  string        `json:"name"`
	Type  ContainerType `json:"type"`
	Image string        `json:"image"`
}

// ContainerLog contains metadata and the complete finite log stream for one
// selected container.
type ContainerLog struct {
	ContainerInfo
	Logs string `json:"logs"`
}

// LogOptions contains qlog's supported finite pod-log filters.
type LogOptions struct {
	Since         time.Duration
	SinceTime     string
	Tail          int64
	Container     string
	AllContainers bool
}

// Validate checks combinations and values that are independent of a pod.
func (o LogOptions) Validate() error {
	if o.Since < 0 {
		return fmt.Errorf("--since must not be negative")
	}
	if o.Since > 0 && strings.TrimSpace(o.SinceTime) != "" {
		return fmt.Errorf("at most one of --since or --since-time may be specified")
	}
	if o.Tail < -1 {
		return fmt.Errorf("--tail must be greater than or equal to -1")
	}
	if o.Container != "" && o.AllContainers {
		return fmt.Errorf("--container and --all-containers cannot be used together")
	}
	if strings.TrimSpace(o.SinceTime) != "" {
		if _, err := time.Parse(time.RFC3339, o.SinceTime); err != nil {
			return fmt.Errorf("invalid --since-time %q: %w", o.SinceTime, err)
		}
	}
	return nil
}

// PodLogOptions returns a Kubernetes log request for one container.
func (o LogOptions) PodLogOptions(container string) (*corev1.PodLogOptions, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	options := &corev1.PodLogOptions{Container: container}
	if o.Since > 0 {
		seconds := int64(math.Ceil(o.Since.Seconds()))
		options.SinceSeconds = &seconds
	}
	if strings.TrimSpace(o.SinceTime) != "" {
		parsed, err := time.Parse(time.RFC3339, o.SinceTime)
		if err != nil {
			return nil, fmt.Errorf("invalid --since-time %q: %w", o.SinceTime, err)
		}
		sinceTime := metav1.NewTime(parsed)
		options.SinceTime = &sinceTime
	}
	if o.Tail != -1 {
		tail := o.Tail
		options.TailLines = &tail
	}
	return options, nil
}

// SelectContainers returns the containers kubectl-style defaults select for a
// pod, in stable init, regular, ephemeral order.
func SelectContainers(pod *corev1.Pod, options LogOptions) ([]ContainerInfo, error) {
	if pod == nil {
		return nil, fmt.Errorf("pod must not be nil")
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}
	all := allContainerInfos(pod)
	if options.AllContainers {
		if len(all) == 0 {
			return nil, fmt.Errorf("pod %s/%s has no containers", pod.Namespace, pod.Name)
		}
		return all, nil
	}
	if options.Container != "" {
		for _, container := range all {
			if container.Name == options.Container {
				return []ContainerInfo{container}, nil
			}
		}
		return nil, fmt.Errorf("container %q is not valid for pod %s/%s", options.Container, pod.Namespace, pod.Name)
	}

	if defaultName := pod.Annotations["kubectl.kubernetes.io/default-container"]; defaultName != "" {
		for _, container := range all {
			if container.Name == defaultName {
				return []ContainerInfo{container}, nil
			}
		}
	}
	for _, container := range all {
		if container.Type == ContainerRegular {
			return []ContainerInfo{container}, nil
		}
	}
	return nil, fmt.Errorf("pod %s/%s has no regular containers", pod.Namespace, pod.Name)
}

func allContainerInfos(pod *corev1.Pod) []ContainerInfo {
	result := make([]ContainerInfo, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers)+len(pod.Spec.EphemeralContainers))
	for _, container := range pod.Spec.InitContainers {
		result = append(result, ContainerInfo{Name: container.Name, Type: ContainerInit, Image: container.Image})
	}
	for _, container := range pod.Spec.Containers {
		result = append(result, ContainerInfo{Name: container.Name, Type: ContainerRegular, Image: container.Image})
	}
	for _, container := range pod.Spec.EphemeralContainers {
		result = append(result, ContainerInfo{Name: container.Name, Type: ContainerEphemeral, Image: container.Image})
	}
	return result
}

// LogFetcher retrieves finite logs for a pod.
type LogFetcher interface {
	Fetch(ctx context.Context, pod *corev1.Pod, options LogOptions) ([]ContainerLog, error)
}

// ClientLogFetcher uses the typed CoreV1 pod log subresource.
type ClientLogFetcher struct {
	pods corev1client.PodsGetter
}

// NewLogFetcher creates a log fetcher from a typed CoreV1 client.
func NewLogFetcher(client KubernetesClient) LogFetcher {
	return &ClientLogFetcher{pods: client.CoreV1()}
}

// Fetch reads each selected container sequentially and closes every stream.
func (f *ClientLogFetcher) Fetch(ctx context.Context, pod *corev1.Pod, options LogOptions) ([]ContainerLog, error) {
	if f == nil || f.pods == nil {
		return nil, fmt.Errorf("pod log client is nil")
	}
	selected, err := SelectContainers(pod, options)
	if err != nil {
		return nil, err
	}
	result := make([]ContainerLog, 0, len(selected))
	for _, container := range selected {
		requestOptions, err := options.PodLogOptions(container.Name)
		if err != nil {
			return nil, err
		}
		stream, err := f.pods.Pods(pod.Namespace).GetLogs(pod.Name, requestOptions).Stream(ctx)
		if err != nil {
			return nil, fmt.Errorf("fetch logs for pod %s/%s container %q: %w", pod.Namespace, pod.Name, container.Name, err)
		}
		logs, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read logs for pod %s/%s container %q: %w", pod.Namespace, pod.Name, container.Name, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close logs for pod %s/%s container %q: %w", pod.Namespace, pod.Name, container.Name, closeErr)
		}
		result = append(result, ContainerLog{ContainerInfo: container, Logs: string(logs)})
	}
	return result, nil
}
