package qlog

import "testing"

func TestParseResourceRef(t *testing.T) {
	cases := []struct {
		input string
		kind  ResourceKind
		name  string
	}{
		{"api-0", ResourcePod, "api-0"},
		{"pods/api-0", ResourcePod, "api-0"},
		{"deploy/api", ResourceDeployment, "api"},
		{"rs/api-123", ResourceReplicaSet, "api-123"},
		{"statefulsets/db", ResourceStatefulSet, "db"},
		{"ds/agents", ResourceDaemonSet, "agents"},
	}
	for _, test := range cases {
		t.Run(test.input, func(t *testing.T) {
			got, err := ParseResourceRef(test.input)
			if err != nil {
				t.Fatal(err)
			}
			want := ResourceRef{Kind: test.kind, Name: test.name}
			if got != want {
				t.Fatalf("reference = %#v, want %#v", got, want)
			}
		})
	}
}

func TestParseResourceRefRejectsMalformedOrUnsupportedValues(t *testing.T) {
	for _, input := range []string{"", "/api", "pod/", "pod/a/b", "service/api"} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseResourceRef(input); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
