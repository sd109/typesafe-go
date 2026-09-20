package qlog

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
)

func TestResolverSelectsAndDeduplicatesPods(t *testing.T) {
	client := fake.NewSimpleClientset(
		podObject("payments", "api-1", "pod-1", nil, nil),
		podObject("payments", "api-0", "pod-0", nil, nil),
		podObject("other", "api-0", "pod-2", nil, nil),
	)
	resolver := NewResolver(client)

	pods, err := resolver.Resolve(context.Background(), NamespaceScope{Namespace: "payments"}, []ResourceRef{
		{Kind: ResourcePod, Name: "api-1"},
		{Kind: ResourcePod, Name: "api-1"},
		{Kind: ResourcePod, Name: "api-0"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := podNames(pods); len(got) != 2 || got[0] != "api-0" || got[1] != "api-1" {
		t.Fatalf("pods = %v", got)
	}

	pods, err = resolver.Resolve(context.Background(), NamespaceScope{All: true}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := podKeys(pods); len(got) != 3 || got[0] != "other/api-0" || got[1] != "payments/api-0" || got[2] != "payments/api-1" {
		t.Fatalf("all pods = %v", got)
	}
}

func TestResolverRejectsInvalidSelectionModesAndMissingResources(t *testing.T) {
	resolver := NewResolver(fake.NewSimpleClientset())
	ctx := context.Background()
	if _, err := resolver.Resolve(ctx, NamespaceScope{Namespace: "default"}, nil, false); err == nil {
		t.Fatal("expected missing selection error")
	}
	if _, err := resolver.Resolve(ctx, NamespaceScope{Namespace: "default"}, []ResourceRef{{Kind: ResourcePod, Name: "api"}}, true); err == nil {
		t.Fatal("expected conflicting selection error")
	}
	if _, err := resolver.Resolve(ctx, NamespaceScope{Namespace: "default"}, []ResourceRef{{Kind: ResourcePod, Name: "api"}}, false); err == nil {
		t.Fatal("expected missing pod error")
	}
}

func TestResolverFiltersDirectControllerOwners(t *testing.T) {
	cases := []struct {
		caseName     string
		kind         ResourceKind
		controller   runtime.Object
		resourceName string
		pod          *corev1.Pod
		wrong        *corev1.Pod
	}{
		{
			caseName:     "replicaset",
			kind:         ResourceReplicaSet,
			controller:   &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-rs", Namespace: "ns", UID: "rs-uid"}, Spec: appsv1.ReplicaSetSpec{Selector: selector("app", "rs")}},
			resourceName: "api-rs",
			pod:          podObject("ns", "rs-pod", "pod-rs", owner("ReplicaSet", "api-rs", "rs-uid"), labels.Set{"app": "rs"}),
			wrong:        podObject("ns", "rs-wrong", "pod-wrong", owner("ReplicaSet", "other", "other-uid"), labels.Set{"app": "rs"}),
		},
		{
			caseName:     "statefulset",
			kind:         ResourceStatefulSet,
			resourceName: "db",
			controller:   &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "ns", UID: "sts-uid"}, Spec: appsv1.StatefulSetSpec{Selector: selector("app", "sts")}},
			pod:          podObject("ns", "sts-pod", "pod-sts", owner("StatefulSet", "db", "sts-uid"), labels.Set{"app": "sts"}),
			wrong:        podObject("ns", "sts-wrong", "pod-wrong", owner("StatefulSet", "other", "other-uid"), labels.Set{"app": "sts"}),
		},
		{
			caseName:     "daemonset",
			kind:         ResourceDaemonSet,
			resourceName: "agent",
			controller:   &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns", UID: "ds-uid"}, Spec: appsv1.DaemonSetSpec{Selector: selector("app", "ds")}},
			pod:          podObject("ns", "ds-pod", "pod-ds", owner("DaemonSet", "agent", "ds-uid"), labels.Set{"app": "ds"}),
			wrong:        podObject("ns", "ds-wrong", "pod-wrong", owner("DaemonSet", "other", "other-uid"), labels.Set{"app": "ds"}),
		},
	}

	for _, test := range cases {
		t.Run(test.caseName, func(t *testing.T) {
			client := fake.NewSimpleClientset(test.controller, test.pod, test.wrong)
			resolver := NewResolver(client)
			pods, err := resolver.Resolve(context.Background(), NamespaceScope{Namespace: "ns"}, []ResourceRef{{Kind: test.kind, Name: test.resourceName}}, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(pods) != 1 || pods[0].Name != test.pod.Name {
				t.Fatalf("pods = %v", podNames(pods))
			}
		})
	}
}

func TestResolverTraversesDeploymentReplicaSetsAndUsesUIDs(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "ns", UID: "deploy-uid"},
		Spec:       appsv1.DeploymentSpec{Selector: selector("app", "api")},
	}
	oldRS := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-old", Namespace: "ns", UID: "rs-old", Labels: map[string]string{"app": "api"}, OwnerReferences: []metav1.OwnerReference{*owner("Deployment", "api", "deploy-uid")}}, Spec: appsv1.ReplicaSetSpec{Selector: selector("app", "api")}}
	newRS := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-new", Namespace: "ns", UID: "rs-new", Labels: map[string]string{"app": "api"}, OwnerReferences: []metav1.OwnerReference{*owner("Deployment", "api", "deploy-uid")}}, Spec: appsv1.ReplicaSetSpec{Selector: selector("app", "api")}}
	wrongRS := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "ns", UID: "rs-other", Labels: map[string]string{"app": "api"}, OwnerReferences: []metav1.OwnerReference{*owner("Deployment", "other", "other-deploy")}}, Spec: appsv1.ReplicaSetSpec{Selector: selector("app", "api")}}
	client := fake.NewSimpleClientset(
		deployment, oldRS, newRS, wrongRS,
		podObject("ns", "api-old-pod", "pod-old", owner("ReplicaSet", "api-old", "rs-old"), labels.Set{"app": "api"}),
		podObject("ns", "api-new-pod", "pod-new", owner("ReplicaSet", "api-new", "rs-new"), labels.Set{"app": "api"}),
		podObject("ns", "wrong-pod", "pod-wrong", owner("ReplicaSet", "other", "rs-other"), labels.Set{"app": "api"}),
	)
	pods, err := NewResolver(client).Resolve(context.Background(), NamespaceScope{Namespace: "ns"}, []ResourceRef{{Kind: ResourceDeployment, Name: "api"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := podNames(pods); len(got) != 2 || got[0] != "api-new-pod" || got[1] != "api-old-pod" {
		t.Fatalf("pods = %v", got)
	}
}

func TestResolverMatchesNamedResourcesAcrossNamespaces(t *testing.T) {
	deploymentA := deploymentObject("ns-a", "api", "deploy-a", "rs-a")
	deploymentB := deploymentObject("ns-b", "api", "deploy-b", "rs-b")
	client := fake.NewSimpleClientset(
		deploymentA, deploymentB,
		replicaSetObject("ns-a", "api-rs", "rs-a", "deploy-a", "api"),
		replicaSetObject("ns-b", "api-rs", "rs-b", "deploy-b", "api"),
		podObject("ns-a", "api-a", "pod-a", owner("ReplicaSet", "api-rs", "rs-a"), labels.Set{"app": "api"}),
		podObject("ns-b", "api-b", "pod-b", owner("ReplicaSet", "api-rs", "rs-b"), labels.Set{"app": "api"}),
	)
	pods, err := NewResolver(client).Resolve(context.Background(), NamespaceScope{All: true}, []ResourceRef{{Kind: ResourceDeployment, Name: "api"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := podKeys(pods); len(got) != 2 || got[0] != "ns-a/api-a" || got[1] != "ns-b/api-b" {
		t.Fatalf("pods = %v", got)
	}
}

func TestResolverReportsEmptyWorkloadsInvalidSelectorsAndAPIErrors(t *testing.T) {
	empty := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "ns", UID: "empty-uid"}, Spec: appsv1.ReplicaSetSpec{Selector: selector("app", "empty")}}
	if _, err := NewResolver(fake.NewSimpleClientset(empty)).Resolve(context.Background(), NamespaceScope{Namespace: "ns"}, []ResourceRef{{Kind: ResourceReplicaSet, Name: "empty"}}, false); err == nil {
		t.Fatal("expected empty workload error")
	}

	invalid := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "invalid", Namespace: "ns", UID: "invalid-uid"}, Spec: appsv1.ReplicaSetSpec{Selector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "bad key", Operator: metav1.LabelSelectorOpExists}}}}}
	if _, err := NewResolver(fake.NewSimpleClientset(invalid)).Resolve(context.Background(), NamespaceScope{Namespace: "ns"}, []ResourceRef{{Kind: ResourceReplicaSet, Name: "invalid"}}, false); err == nil {
		t.Fatal("expected invalid selector error")
	}

	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "pods", func(_ clientgotesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("API unavailable")
	})
	if _, err := NewResolver(client).Resolve(context.Background(), NamespaceScope{Namespace: "ns"}, []ResourceRef{{Kind: ResourcePod, Name: "api"}}, false); err == nil {
		t.Fatal("expected API error")
	}
}

func selector(key, value string) *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: map[string]string{key: value}}
}

func owner(kind, name, uid string) *metav1.OwnerReference {
	controller := true
	return &metav1.OwnerReference{APIVersion: "apps/v1", Kind: kind, Name: name, UID: types.UID(uid), Controller: &controller}
}

func podObject(namespace, name, uid string, ownerRef *metav1.OwnerReference, podLabels labels.Set) *corev1.Pod {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(uid), Labels: podLabels}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "example/app:v1"}}}}
	if ownerRef != nil {
		pod.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}
	return pod
}

func deploymentObject(namespace, name, uid, replicaSetUID string) *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(uid)}, Spec: appsv1.DeploymentSpec{Selector: selector("app", "api")}}
}

func replicaSetObject(namespace, name, uid, deploymentUID, app string) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(uid), Labels: map[string]string{"app": app}, OwnerReferences: []metav1.OwnerReference{*owner("Deployment", "api", deploymentUID)}}, Spec: appsv1.ReplicaSetSpec{Selector: selector("app", app)}}
}

func podNames(pods []corev1.Pod) []string {
	result := make([]string, len(pods))
	for index := range pods {
		result[index] = pods[index].Name
	}
	return result
}

func podKeys(pods []corev1.Pod) []string {
	result := make([]string, len(pods))
	for index := range pods {
		result[index] = pods[index].Namespace + "/" + pods[index].Name
	}
	return result
}
