package qlog

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
)

// NamespaceScope describes the namespace set searched by a qlog invocation.
type NamespaceScope struct {
	// Namespace is the selected namespace, or metav1.NamespaceAll when All is
	// true.
	Namespace string
	All       bool
}

// ConfigOptions owns the kubectl-compatible client configuration flags used by
// qlog. ConfigFlags supplies kubeconfig, context, authentication, TLS, and
// namespace behavior; AllNamespaces is qlog's additional -A selection mode.
type ConfigOptions struct {
	ConfigFlags   *genericclioptions.ConfigFlags
	AllNamespaces bool
}

// NewConfigOptions creates options with kubectl-compatible persistent config.
func NewConfigOptions() *ConfigOptions {
	return &ConfigOptions{
		ConfigFlags: genericclioptions.NewConfigFlags(true),
	}
}

// AddFlags adds kubeconfig and namespace flags to a command flag set.
func (o *ConfigOptions) AddFlags(flags *pflag.FlagSet) {
	o.ConfigFlags.AddFlags(flags)
	flags.BoolVarP(&o.AllNamespaces, "all-namespaces", "A", false, "If present, search pods across all namespaces.")
}

// NamespaceScope resolves the effective namespace after flags have been
// parsed. The flag set is used to distinguish an explicit -n from a namespace
// inherited from the kubeconfig context.
func (o *ConfigOptions) NamespaceScope(flags *pflag.FlagSet) (NamespaceScope, error) {
	if o.AllNamespaces {
		if flags != nil && flags.Changed("namespace") {
			return NamespaceScope{}, fmt.Errorf("cannot specify both --namespace and --all-namespaces")
		}
		return NamespaceScope{All: true}, nil
	}

	if flags != nil && flags.Changed("namespace") {
		namespace := strings.TrimSpace(*o.ConfigFlags.Namespace)
		if namespace == "" {
			return NamespaceScope{}, fmt.Errorf("namespace must not be empty")
		}
		return NamespaceScope{Namespace: namespace}, nil
	}

	namespace, _, err := o.ConfigFlags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return NamespaceScope{}, fmt.Errorf("resolve namespace: %w", err)
	}
	if namespace == "" {
		namespace = "default"
	}
	return NamespaceScope{Namespace: namespace}, nil
}

// Client creates a typed Kubernetes client from the configured kubeconfig.
func (o *ConfigOptions) Client() (*kubernetes.Clientset, error) {
	config, err := o.ConfigFlags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("load Kubernetes client configuration: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}
	return client, nil
}
