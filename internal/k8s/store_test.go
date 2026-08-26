package k8s

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"github.com/p10node/k10s/internal/domain"
)

// newTestStore builds a Store against fake clientsets (no real cluster, no
// network) — the same construction path newStoreFromClient uses in
// production, just with fakes swapped in for the client set. This is what
// was missing before: every prior test of this package only exercised the
// mock backend, so a Store-specific bug (blocking startup, an expensive
// RowCount) had no unit test to catch it.
func newTestStore(t *testing.T, objs ...runtime.Object) *Store {
	t.Helper()
	cs := fake.NewSimpleClientset(objs...)
	dyn := dynamicfake.NewSimpleDynamicClient(scheme.Scheme, objs...)
	apiext := apiextfake.NewSimpleClientset()
	metrics := metricsfake.NewSimpleClientset()

	c := &Client{
		RestConfig:     &rest.Config{Host: "https://fake"},
		Clientset:      cs,
		Dynamic:        dyn,
		Metrics:        metrics,
		CurrentContext: "test-context",
		Server:         "https://fake",
		Version:        "v1.31.0",
	}
	s, err := newStoreFrom(c, apiext)
	if err != nil {
		t.Fatalf("newStoreFrom: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func newTestStoreWithCRDs(t *testing.T, crds []runtime.Object, objs ...runtime.Object) *Store {
	t.Helper()
	cs := fake.NewSimpleClientset(objs...)
	dyn := dynamicfake.NewSimpleDynamicClient(scheme.Scheme, objs...)
	apiext := apiextfake.NewSimpleClientset(crds...)
	metrics := metricsfake.NewSimpleClientset()

	c := &Client{
		RestConfig:     &rest.Config{Host: "https://fake"},
		Clientset:      cs,
		Dynamic:        dyn,
		Metrics:        metrics,
		CurrentContext: "test-context",
	}
	s, err := newStoreFrom(c, apiext)
	if err != nil {
		t.Fatalf("newStoreFrom: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

// syncKinds opens each kind (starting its lazy informer) and waits for the
// initial list to land, so assertions see data instead of an empty cache.
// Production code never waits like this — the UI repaints as caches fill.
func syncKinds(t *testing.T, s *Store, kinds ...string) {
	t.Helper()
	for _, k := range kinds {
		s.ensure(k)
	}
	deadline := time.Now().Add(5 * time.Second)
	for _, k := range kinds {
		for !s.Synced(k) {
			if time.Now().After(deadline) {
				t.Fatalf("informer for %q never synced", k)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func pod(ns, name, node string, ready bool) *corev1.Pod {
	status := corev1.ConditionTrue
	if !ready {
		status = corev1.ConditionFalse
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: corev1.PodSpec{
			NodeName:   node,
			Containers: []corev1.Container{{Name: "app", Image: "example/app:1"}},
		},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Ready: ready, RestartCount: 0}},
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
		},
	}
}

func deployment(ns, name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "example/app:1"}},
			}},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: replicas, AvailableReplicas: replicas, UpdatedReplicas: replicas},
	}
}

func node(name string, unschedulable bool) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.NodeSpec{Unschedulable: unschedulable},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	}
}

func TestStoreRowsNamespaceFiltering(t *testing.T) {
	s := newTestStore(t,
		pod("default", "web-1", "node-a", true),
		pod("kube-system", "coredns-1", "node-a", true),
	)
	syncKinds(t, s, kPods)

	cols, rows := s.Rows("pods", "default")
	if len(rows) != 1 || rows[0][0] != "web-1" {
		t.Fatalf("ns=default: got cols=%v rows=%v, want exactly web-1", cols, rows)
	}
	if got := s.RowCount("pods", "default"); got != 1 {
		t.Fatalf("RowCount(default) = %d, want 1", got)
	}

	cols, rows = s.Rows("pods", domain.AllNamespaces)
	if cols[0] != "NAMESPACE" {
		t.Fatalf("ns=all: expected NAMESPACE column prepended, got %v", cols)
	}
	if len(rows) != 2 {
		t.Fatalf("ns=all: got %d rows, want 2: %v", len(rows), rows)
	}
	if got := s.RowCount("pods", domain.AllNamespaces); got != 2 {
		t.Fatalf("RowCount(all) = %d, want 2", got)
	}

	_, rows = s.Rows("pods", "kube-system")
	if len(rows) != 1 || rows[0][0] != "coredns-1" {
		t.Fatalf("ns=kube-system: got %v, want exactly coredns-1", rows)
	}
}

func TestStoreRowCountMatchesRowsAcrossKinds(t *testing.T) {
	s := newTestStore(t,
		pod("default", "web-1", "node-a", true),
		deployment("default", "web", 2),
		node("node-a", false),
	)
	syncKinds(t, s, kPods, kDeployments, kNodes)

	for _, tc := range []struct{ kind, ns string }{
		{"pods", "default"},
		{"pods", domain.AllNamespaces},
		{"deployments", "default"},
		{"nodes", ""},
	} {
		_, rows := s.Rows(tc.kind, tc.ns)
		if got := s.RowCount(tc.kind, tc.ns); got != len(rows) {
			t.Errorf("RowCount(%q, %q) = %d, want %d (len(Rows))", tc.kind, tc.ns, got, len(rows))
		}
	}
}

func TestStoreDeploymentReadyColumn(t *testing.T) {
	s := newTestStore(t, deployment("default", "web", 3))
	syncKinds(t, s, kDeployments)

	cols, rows := s.Rows("deployments", "default")
	readyIdx := -1
	for i, c := range cols {
		if c == "READY" {
			readyIdx = i
		}
	}
	if readyIdx < 0 {
		t.Fatalf("no READY column in %v", cols)
	}
	if len(rows) != 1 || rows[0][readyIdx] != "3/3" {
		t.Fatalf("READY = %v, want 3/3 (rows=%v)", rows, rows)
	}
}

func TestStoreDelete(t *testing.T) {
	s := newTestStore(t, deployment("default", "web", 1))

	if err := s.Delete("deployments", "default", "web"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	gvr, _, _ := s.gvrFor("deployments")
	_, err := s.c.Dynamic.Resource(gvr).Namespace("default").Get(context.Background(), "web", metav1.GetOptions{})
	if !errors.IsNotFound(err) {
		t.Fatalf("expected NotFound after Delete, got %v", err)
	}
}

func TestStoreCordonAndDrain(t *testing.T) {
	s := newTestStore(t, node("node-a", false), pod("default", "web-1", "node-a", true))

	if err := s.Cordon("node-a", true); err != nil {
		t.Fatalf("Cordon: %v", err)
	}
	got, err := s.c.Clientset.CoreV1().Nodes().Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get node: %v", err)
	}
	if !got.Spec.Unschedulable {
		t.Fatalf("Cordon(true) did not set Spec.Unschedulable")
	}

	if err := s.Cordon("node-a", false); err != nil {
		t.Fatalf("Cordon(false): %v", err)
	}
	got, _ = s.c.Clientset.CoreV1().Nodes().Get(context.Background(), "node-a", metav1.GetOptions{})
	if got.Spec.Unschedulable {
		t.Fatalf("Cordon(false) left Spec.Unschedulable = true")
	}
}

func TestStoreRestartPatchesAnnotation(t *testing.T) {
	s := newTestStore(t, deployment("default", "web", 1))

	if err := s.Restart("deployments", "default", "web"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	got, err := s.c.Clientset.AppsV1().Deployments("default").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get deployment: %v", err)
	}
	if _, ok := got.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"]; !ok {
		t.Fatalf("Restart did not set restartedAt annotation: %+v", got.Spec.Template.Annotations)
	}
}

func TestStoreScale(t *testing.T) {
	s := newTestStore(t, deployment("default", "web", 1))

	n, err := s.Scale("deployments", "default", "web", 5)
	if err != nil {
		t.Fatalf("Scale: %v", err)
	}
	if n != 5 {
		t.Fatalf("Scale returned %d, want 5", n)
	}
	got, err := s.c.Clientset.AppsV1().Deployments("default").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get deployment: %v", err)
	}
	if got.Spec.Replicas == nil || *got.Spec.Replicas != 5 {
		t.Fatalf("Spec.Replicas = %v, want 5", got.Spec.Replicas)
	}
}

func TestStoreCRDRows(t *testing.T) {
	crd := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "widgets.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextv1.CustomResourceDefinitionNames{Kind: "Widget", Plural: "widgets"},
			Scope: apiextv1.NamespaceScoped,
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
		},
	}
	s := newTestStoreWithCRDs(t, []runtime.Object{crd})
	syncKinds(t, s, kCRDs)

	cols, rows := s.Rows("crds", "")
	if len(rows) != 1 {
		t.Fatalf("got %d CRD rows, want 1: %v", len(rows), rows)
	}
	kindIdx := -1
	for i, c := range cols {
		if c == "KIND" {
			kindIdx = i
		}
	}
	if kindIdx < 0 || rows[0][kindIdx] != "Widget" {
		t.Fatalf("CRD row KIND = %v (cols=%v), want Widget", rows[0], cols)
	}
}

// TestNewStoreReturnsFastWhenCachesNeverSync is the startup-lag guard.
//
// A cluster that is slow, huge, or partially unreachable can leave informer
// caches unsynced indefinitely. The TUI must still come up: startup gets a
// short grace period for the initial list and then proceeds, with informers
// continuing to fill in the background. Previously this blocked for 20s and
// then failed outright, which is what made `k10s` appear to hang on launch.
//
// The fake clientset here fails every List, so caches can never sync — the
// worst case.
func TestNewStoreReturnsFastWhenCachesNeverSync(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("list", "*", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.NewServiceUnavailable("simulated unreachable API server")
	})
	apiext := apiextfake.NewSimpleClientset()
	apiext.PrependReactor("list", "*", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.NewServiceUnavailable("simulated unreachable API server")
	})

	c := &Client{
		RestConfig:     &rest.Config{Host: "https://unreachable"},
		Clientset:      cs,
		Dynamic:        dynamicfake.NewSimpleDynamicClient(scheme.Scheme),
		Metrics:        metricsfake.NewSimpleClientset(),
		CurrentContext: "broken-context",
	}

	type result struct {
		s   *Store
		err error
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		s, err := newStoreFrom(c, apiext)
		done <- result{s, err}
	}()

	select {
	case r := <-done:
		elapsed := time.Since(start)
		if r.err != nil {
			t.Fatalf("newStoreFrom returned an error (%v) — an unsynced cache must not prevent the UI from starting", r.err)
		}
		if r.s == nil {
			t.Fatal("newStoreFrom returned a nil Store")
		}
		t.Cleanup(r.s.Close)
		// Startup registers no informers and awaits nothing, so this should
		// be microseconds. Keep the bound tight enough that reintroducing a
		// blocking cache-sync wait fails the test.
		if elapsed > 250*time.Millisecond {
			t.Errorf("startup took %v — construction must not block on cluster I/O", elapsed)
		}
		t.Logf("startup with unsyncable caches: %v", elapsed)

		// A kind nobody has opened has no informer, so its count is unknown
		// rather than a misleading zero.
		if got := r.s.RowCount("pods", domain.AllNamespaces); got != domain.CountUnknown {
			t.Errorf("RowCount for an unopened kind = %d, want CountUnknown", got)
		}
		if _, rows := r.s.Rows("pods", domain.AllNamespaces); len(rows) != 0 {
			t.Errorf("Rows on an unsynced store returned %d rows, want 0", len(rows))
		}
	case <-time.After(15 * time.Second):
		t.Fatal("newStoreFrom never returned with unsyncable caches — startup must not block indefinitely")
	}
}
