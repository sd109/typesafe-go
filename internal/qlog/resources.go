package qlog

import (
	"fmt"
	"strings"
)

// ResourceKind is a supported Kubernetes resource kind that can resolve to
// pods.
type ResourceKind string

const (
	ResourcePod         ResourceKind = "pod"
	ResourceDeployment  ResourceKind = "deployment"
	ResourceReplicaSet  ResourceKind = "replicaset"
	ResourceStatefulSet ResourceKind = "statefulset"
	ResourceDaemonSet   ResourceKind = "daemonset"
)

// ResourceRef identifies a named resource supplied on the command line.
type ResourceRef struct {
	Kind ResourceKind
	Name string
}

var resourceAliases = map[string]ResourceKind{
	"pod":          ResourcePod,
	"pods":         ResourcePod,
	"po":           ResourcePod,
	"deployment":   ResourceDeployment,
	"deployments":  ResourceDeployment,
	"deploy":       ResourceDeployment,
	"replicaset":   ResourceReplicaSet,
	"replicasets":  ResourceReplicaSet,
	"rs":           ResourceReplicaSet,
	"statefulset":  ResourceStatefulSet,
	"statefulsets": ResourceStatefulSet,
	"sts":          ResourceStatefulSet,
	"daemonset":    ResourceDaemonSet,
	"daemonsets":   ResourceDaemonSet,
	"ds":           ResourceDaemonSet,
}

// ParseResourceRef parses kind/name syntax. A bare name is a pod reference.
func ParseResourceRef(value string) (ResourceRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ResourceRef{}, fmt.Errorf("resource must not be empty")
	}

	parts := strings.Split(value, "/")
	if len(parts) == 1 {
		return ResourceRef{Kind: ResourcePod, Name: parts[0]}, nil
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ResourceRef{}, fmt.Errorf("resource %q must use kind/name syntax", value)
	}

	kind, ok := resourceAliases[strings.ToLower(parts[0])]
	if !ok {
		return ResourceRef{}, fmt.Errorf("unsupported resource kind %q", parts[0])
	}
	return ResourceRef{Kind: kind, Name: parts[1]}, nil
}
