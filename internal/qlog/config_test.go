package qlog

import (
	"testing"

	"github.com/spf13/pflag"
)

func newTestConfigFlags() (*ConfigOptions, *pflag.FlagSet) {
	options := NewConfigOptions()
	flags := pflag.NewFlagSet("qlog", pflag.ContinueOnError)
	options.AddFlags(flags)
	return options, flags
}

func TestNamespaceScopeUsesExplicitNamespace(t *testing.T) {
	options, flags := newTestConfigFlags()
	if err := flags.Parse([]string{"--namespace", "payments"}); err != nil {
		t.Fatal(err)
	}
	scope, err := options.NamespaceScope(flags)
	if err != nil {
		t.Fatal(err)
	}
	if scope != (NamespaceScope{Namespace: "payments"}) {
		t.Fatalf("scope = %#v", scope)
	}
}

func TestNamespaceScopeDefaultsWithoutKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", t.TempDir()+"/missing")
	options, flags := newTestConfigFlags()
	scope, err := options.NamespaceScope(flags)
	if err != nil {
		t.Fatal(err)
	}
	if scope != (NamespaceScope{Namespace: "default"}) {
		t.Fatalf("scope = %#v", scope)
	}
}

func TestNamespaceScopeRejectsNamespaceAndAllNamespaces(t *testing.T) {
	options, flags := newTestConfigFlags()
	if err := flags.Parse([]string{"--namespace", "payments", "--all-namespaces"}); err != nil {
		t.Fatal(err)
	}
	if _, err := options.NamespaceScope(flags); err == nil {
		t.Fatal("expected mutually exclusive namespace error")
	}
}

func TestNamespaceScopeAllNamespaces(t *testing.T) {
	options, flags := newTestConfigFlags()
	if err := flags.Parse([]string{"-A"}); err != nil {
		t.Fatal(err)
	}
	scope, err := options.NamespaceScope(flags)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.All || scope.Namespace != "" {
		t.Fatalf("scope = %#v", scope)
	}
}
