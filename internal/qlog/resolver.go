package qlog

import (
	"context"
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

// KubernetesClient is the subset of a typed clientset needed by the resolver.
type KubernetesClient interface {
	CoreV1() corev1client.CoreV1Interface
	AppsV1() appsv1client.AppsV1Interface
}

// Resolver resolves supported resources into concrete pods using owner UIDs,
// not labels alone.
type Resolver struct {
	client KubernetesClient
}

// NewResolver creates a resolver backed by a typed Kubernetes client.
func NewResolver(client KubernetesClient) *Resolver {
	return &Resolver{client: client}
}

// Resolve resolves either explicit resource references or all pods in the
// namespace scope. The result is deduplicated and sorted by namespace/name.
func (r *Resolver) Resolve(ctx context.Context, scope NamespaceScope, refs []ResourceRef, allPods bool) ([]corev1.Pod, error) {
	if allPods && len(refs) > 0 {
		return nil, fmt.Errorf("--all-pods cannot be combined with resource arguments")
	}
	if !allPods && len(refs) == 0 {
		return nil, fmt.Errorf("a resource argument or --all-pods is required")
	}
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("Kubernetes resolver client is nil")
	}

	var pods []corev1.Pod
	var err error
	if allPods {
		pods, err = r.resolveAllPods(ctx, scope)
	} else {
		for _, ref := range refs {
			resolved, resolveErr := r.resolveRef(ctx, scope, ref)
			if resolveErr != nil {
				return nil, resolveErr
			}
			pods = append(pods, resolved...)
		}
	}
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		return nil, fmt.Errorf("no pods found")
	}
	return deduplicatePods(pods), nil
}

func (r *Resolver) resolveAllPods(ctx context.Context, scope NamespaceScope) ([]corev1.Pod, error) {
	namespace := scope.Namespace
	if scope.All {
		namespace = metav1.NamespaceAll
	}
	list, err := r.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods in %s: %w", displayNamespace(scope), err)
	}
	return append([]corev1.Pod(nil), list.Items...), nil
}

func (r *Resolver) resolveRef(ctx context.Context, scope NamespaceScope, ref ResourceRef) ([]corev1.Pod, error) {
	switch ref.Kind {
	case ResourcePod:
		return r.resolvePods(ctx, scope, ref)
	case ResourceDeployment:
		deployments, err := r.deployments(ctx, scope, ref.Name)
		if err != nil {
			return nil, err
		}
		return resolveNamedControllers(ctx, ref, deployments, r.deploymentPods)
	case ResourceReplicaSet:
		replicasets, err := r.replicasets(ctx, scope, ref.Name)
		if err != nil {
			return nil, err
		}
		return resolveNamedControllers(ctx, ref, replicasets, r.replicaSetPods)
	case ResourceStatefulSet:
		statefulsets, err := r.statefulsets(ctx, scope, ref.Name)
		if err != nil {
			return nil, err
		}
		return resolveNamedControllers(ctx, ref, statefulsets, r.statefulSetPods)
	case ResourceDaemonSet:
		daemonsets, err := r.daemonsets(ctx, scope, ref.Name)
		if err != nil {
			return nil, err
		}
		return resolveNamedControllers(ctx, ref, daemonsets, r.daemonSetPods)
	default:
		return nil, fmt.Errorf("unsupported resource kind %q", ref.Kind)
	}
}

func (r *Resolver) resolvePods(ctx context.Context, scope NamespaceScope, ref ResourceRef) ([]corev1.Pod, error) {
	if scope.All {
		list, err := r.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, namedResourceOptions(ref.Name))
		if err != nil {
			return nil, fmt.Errorf("list pod %q across all namespaces: %w", ref.Name, err)
		}
		items := filterPodsByName(list.Items, ref.Name)
		if len(items) == 0 {
			return nil, fmt.Errorf("pod %q was not found in any namespace", ref.Name)
		}
		return items, nil
	}

	pod, err := r.client.CoreV1().Pods(scope.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod %s/%s: %w", scope.Namespace, ref.Name, err)
	}
	return []corev1.Pod{*pod}, nil
}

func resolveNamedControllers[T metav1.Object](ctx context.Context, ref ResourceRef, objects []T, resolve func(context.Context, T) ([]corev1.Pod, error)) ([]corev1.Pod, error) {
	if len(objects) == 0 {
		return nil, fmt.Errorf("%s %q was not found in the selected namespace scope", ref.Kind, ref.Name)
	}
	var pods []corev1.Pod
	for _, object := range objects {
		resolved, err := resolve(ctx, object)
		if err != nil {
			return nil, err
		}
		pods = append(pods, resolved...)
	}
	if len(pods) == 0 {
		return nil, fmt.Errorf("%s %q resolved to no pods", ref.Kind, ref.Name)
	}
	return pods, nil
}

func (r *Resolver) deployments(ctx context.Context, scope NamespaceScope, name string) ([]*appsv1.Deployment, error) {
	if scope.All {
		list, err := r.client.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, namedResourceOptions(name))
		if err != nil {
			return nil, fmt.Errorf("list deployment %q across all namespaces: %w", name, err)
		}
		items := make([]*appsv1.Deployment, 0)
		for index := range list.Items {
			if list.Items[index].Name == name {
				items = append(items, &list.Items[index])
			}
		}
		return items, nil
	}
	item, err := r.client.AppsV1().Deployments(scope.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment %s/%s: %w", scope.Namespace, name, err)
	}
	return []*appsv1.Deployment{item}, nil
}

func (r *Resolver) replicasets(ctx context.Context, scope NamespaceScope, name string) ([]*appsv1.ReplicaSet, error) {
	if scope.All {
		list, err := r.client.AppsV1().ReplicaSets(metav1.NamespaceAll).List(ctx, namedResourceOptions(name))
		if err != nil {
			return nil, fmt.Errorf("list replicaset %q across all namespaces: %w", name, err)
		}
		items := make([]*appsv1.ReplicaSet, 0)
		for index := range list.Items {
			if list.Items[index].Name == name {
				items = append(items, &list.Items[index])
			}
		}
		return items, nil
	}
	item, err := r.client.AppsV1().ReplicaSets(scope.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get replicaset %s/%s: %w", scope.Namespace, name, err)
	}
	return []*appsv1.ReplicaSet{item}, nil
}

func (r *Resolver) statefulsets(ctx context.Context, scope NamespaceScope, name string) ([]*appsv1.StatefulSet, error) {
	if scope.All {
		list, err := r.client.AppsV1().StatefulSets(metav1.NamespaceAll).List(ctx, namedResourceOptions(name))
		if err != nil {
			return nil, fmt.Errorf("list statefulset %q across all namespaces: %w", name, err)
		}
		items := make([]*appsv1.StatefulSet, 0)
		for index := range list.Items {
			if list.Items[index].Name == name {
				items = append(items, &list.Items[index])
			}
		}
		return items, nil
	}
	item, err := r.client.AppsV1().StatefulSets(scope.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get statefulset %s/%s: %w", scope.Namespace, name, err)
	}
	return []*appsv1.StatefulSet{item}, nil
}

func (r *Resolver) daemonsets(ctx context.Context, scope NamespaceScope, name string) ([]*appsv1.DaemonSet, error) {
	if scope.All {
		list, err := r.client.AppsV1().DaemonSets(metav1.NamespaceAll).List(ctx, namedResourceOptions(name))
		if err != nil {
			return nil, fmt.Errorf("list daemonset %q across all namespaces: %w", name, err)
		}
		items := make([]*appsv1.DaemonSet, 0)
		for index := range list.Items {
			if list.Items[index].Name == name {
				items = append(items, &list.Items[index])
			}
		}
		return items, nil
	}
	item, err := r.client.AppsV1().DaemonSets(scope.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get daemonset %s/%s: %w", scope.Namespace, name, err)
	}
	return []*appsv1.DaemonSet{item}, nil
}

func (r *Resolver) deploymentPods(ctx context.Context, deployment *appsv1.Deployment) ([]corev1.Pod, error) {
	selector, err := labelSelector(deployment.Spec.Selector, ResourceDeployment)
	if err != nil {
		return nil, fmt.Errorf("deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
	}
	replicasets, err := r.client.AppsV1().ReplicaSets(deployment.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, fmt.Errorf("list ReplicaSets for deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
	}
	ownedReplicaSets := make(map[types.UID]struct{})
	for index := range replicasets.Items {
		if controlledBy(&replicasets.Items[index], deployment.UID) {
			ownedReplicaSets[replicasets.Items[index].UID] = struct{}{}
		}
	}
	if len(ownedReplicaSets) == 0 {
		return nil, nil
	}
	pods, err := r.client.CoreV1().Pods(deployment.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, fmt.Errorf("list pods for deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
	}
	result := make([]corev1.Pod, 0)
	for index := range pods.Items {
		controller := metav1.GetControllerOf(&pods.Items[index])
		if controller != nil {
			if _, ok := ownedReplicaSets[controller.UID]; ok {
				result = append(result, pods.Items[index])
			}
		}
	}
	return result, nil
}

func (r *Resolver) replicaSetPods(ctx context.Context, replicaSet *appsv1.ReplicaSet) ([]corev1.Pod, error) {
	return r.directControllerPods(ctx, replicaSet.Namespace, replicaSet.Name, replicaSet.UID, replicaSet.Spec.Selector, ResourceReplicaSet)
}

func (r *Resolver) statefulSetPods(ctx context.Context, statefulSet *appsv1.StatefulSet) ([]corev1.Pod, error) {
	return r.directControllerPods(ctx, statefulSet.Namespace, statefulSet.Name, statefulSet.UID, statefulSet.Spec.Selector, ResourceStatefulSet)
}

func (r *Resolver) daemonSetPods(ctx context.Context, daemonSet *appsv1.DaemonSet) ([]corev1.Pod, error) {
	return r.directControllerPods(ctx, daemonSet.Namespace, daemonSet.Name, daemonSet.UID, daemonSet.Spec.Selector, ResourceDaemonSet)
}

func (r *Resolver) directControllerPods(ctx context.Context, namespace, name string, uid types.UID, rawSelector *metav1.LabelSelector, kind ResourceKind) ([]corev1.Pod, error) {
	selector, err := labelSelector(rawSelector, kind)
	if err != nil {
		return nil, fmt.Errorf("%s %s/%s: %w", kind, namespace, name, err)
	}
	pods, err := r.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, fmt.Errorf("list pods for %s %s/%s: %w", kind, namespace, name, err)
	}
	result := make([]corev1.Pod, 0)
	for index := range pods.Items {
		if controlledBy(&pods.Items[index], uid) {
			result = append(result, pods.Items[index])
		}
	}
	return result, nil
}

func labelSelector(raw *metav1.LabelSelector, kind ResourceKind) (labels.Selector, error) {
	if raw == nil {
		return nil, fmt.Errorf("%s has no pod selector", kind)
	}
	selector, err := metav1.LabelSelectorAsSelector(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid pod selector: %w", err)
	}
	return selector, nil
}

func controlledBy(object metav1.Object, uid types.UID) bool {
	controller := metav1.GetControllerOf(object)
	return controller != nil && controller.UID == uid
}

func namedResourceOptions(name string) metav1.ListOptions {
	return metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("metadata.name", name).String()}
}

func filterPodsByName(items []corev1.Pod, name string) []corev1.Pod {
	result := make([]corev1.Pod, 0)
	for index := range items {
		if items[index].Name == name {
			result = append(result, items[index])
		}
	}
	return result
}

func deduplicatePods(pods []corev1.Pod) []corev1.Pod {
	seen := make(map[string]struct{}, len(pods))
	result := make([]corev1.Pod, 0, len(pods))
	for _, pod := range pods {
		key := string(pod.UID)
		if key == "" {
			key = pod.Namespace + "\x00" + pod.Name
		} else {
			key = pod.Namespace + "\x00" + key
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, pod)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].UID < result[j].UID
	})
	return result
}

func displayNamespace(scope NamespaceScope) string {
	if scope.All {
		return "all namespaces"
	}
	return scope.Namespace
}
