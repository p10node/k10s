package k8s

import (
	"context"
	"errors"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktesting "k8s.io/client-go/testing"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func drainPod(name string) *corev1.Pod {
	controller := true
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns", UID: types.UID(name + "-uid"), OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "rs", Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: "node"}}
}

func TestDrainWaitsForOriginalUID(t *testing.T) {
	p := drainPod("app")
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}, p)
	gets, evictions := 0, 0
	cs.PrependReactor("create", "pods", func(a ktesting.Action) (bool, runtime.Object, error) {
		evictions++
		ev := a.(ktesting.CreateAction).GetObject().(*policyv1.Eviction)
		if ev.DeleteOptions == nil || ev.DeleteOptions.Preconditions == nil || ev.DeleteOptions.Preconditions.UID == nil || *ev.DeleteOptions.Preconditions.UID != p.UID {
			t.Error("eviction missing original UID precondition")
		}
		return true, nil, nil
	})
	cs.PrependReactor("get", "pods", func(a ktesting.Action) (bool, runtime.Object, error) {
		gets++
		replacement := p.DeepCopy()
		replacement.UID = "replacement"
		return true, replacement, nil
	})
	if err := (&Store{c: &Client{Clientset: cs}}).Drain("node"); err != nil {
		t.Fatal(err)
	}
	if gets == 0 {
		t.Fatal("reported drained without checking original UID termination")
	}
	if evictions > 1 {
		t.Fatalf("evicted replacement: %d", evictions)
	}
}

func TestDrainPDBRetry(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		t.Run(fmtBool(replacement), func(t *testing.T) {
			p := drainPod("app")
			cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}, p)
			evictions := 0
			cs.PrependReactor("create", "pods", func(a ktesting.Action) (bool, runtime.Object, error) {
				evictions++
				if evictions == 1 {
					return true, nil, apierrors.NewTooManyRequests("PDB", 7)
				}
				return true, nil, cs.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), p.Namespace, p.Name)
			})
			waits := []time.Duration{}
			err := (&Store{c: &Client{Clientset: cs}}).drain(context.Background(), "node", func(ctx context.Context, d time.Duration) error {
				waits = append(waits, d)
				if _, ok := ctx.Deadline(); !ok {
					t.Error("missing operation deadline")
				}
				if replacement {
					q := p.DeepCopy()
					q.UID = "replacement"
					return cs.Tracker().Update(corev1.SchemeGroupVersion.WithResource("pods"), q, p.Namespace)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if replacement {
				want = 1
			}
			if evictions != want || !reflect.DeepEqual(waits, []time.Duration{7 * time.Second}) {
				t.Fatalf("evictions=%d waits=%v", evictions, waits)
			}
			for _, a := range cs.Actions() {
				if a.GetVerb() == "delete" {
					t.Fatal("DELETE bypassed PDB")
				}
			}
		})
	}
}
func fmtBool(v bool) string {
	if v {
		return "replacement"
	}
	return "retry"
}

func TestDrainTerminationCancellation(t *testing.T) {
	p := drainPod("app")
	p.Finalizers = []string{"hold"}
	now := metav1.Now()
	p.DeletionTimestamp = &now
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}, p)
	cs.PrependReactor("create", "pods", func(a ktesting.Action) (bool, runtime.Object, error) { return true, nil, nil })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waits := 0
	err := (&Store{c: &Client{Clientset: cs}}).drain(ctx, "node", func(ctx context.Context, d time.Duration) error { waits++; cancel(); return drainWait(ctx, time.Hour) })
	if waits != 1 || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "remains cordoned") {
		t.Fatalf("waits=%d err=%v", waits, err)
	}
}

func TestDrainForbiddenPartial(t *testing.T) {
	p := drainPod("app")
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}, p)
	cs.PrependReactor("create", "pods", func(a ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, p.Name, errors.New("denied"))
	})
	err := (&Store{c: &Client{Clientset: cs}}).Drain("node")
	if !apierrors.IsForbidden(err) || !strings.Contains(err.Error(), "remains cordoned") {
		t.Fatal(err)
	}
	for _, a := range cs.Actions() {
		if a.GetVerb() == "delete" {
			t.Fatal("DELETE bypass")
		}
		if pa, ok := a.(ktesting.PatchAction); ok && string(pa.GetPatch()) != `{"spec":{"unschedulable":true}}` {
			t.Fatal("unexpected patch")
		}
	}
}

func TestDrainSkipsAndEmptyNode(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmtBool(empty), func(t *testing.T) {
			objects := []runtime.Object{&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}}
			if !empty {
				ds := drainPod("daemon")
				ds.OwnerReferences[0].Kind = "DaemonSet"
				mirror := drainPod("mirror")
				mirror.OwnerReferences = nil
				mirror.Annotations = map[string]string{corev1.MirrorPodAnnotationKey: "static"}
				sibling := drainPod("sibling")
				sibling.Spec.NodeName = "other"
				sibling.OwnerReferences = nil
				objects = append(objects, ds, mirror, sibling)
			}
			cs := fake.NewSimpleClientset(objects...)
			if err := (&Store{c: &Client{Clientset: cs}}).Drain("node"); err != nil {
				t.Fatal(err)
			}
			acts := cs.Actions()
			if len(acts) != 2 || acts[0].GetVerb() != "list" || acts[1].GetVerb() != "patch" {
				t.Fatalf("unexpected API log: %v", acts)
			}
			if acts[0].(ktesting.ListAction).GetListRestrictions().Fields.String() != "spec.nodeName=node" {
				t.Fatal("missing node selector")
			}
		})
	}
}

func TestDrainAcceptedWaitsThenGone(t *testing.T) {
	p := drainPod("app")
	p.Finalizers = []string{"hold"}
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}, p)
	cs.PrependReactor("create", "pods", func(a ktesting.Action) (bool, runtime.Object, error) { return true, nil, nil })
	waits := 0
	err := (&Store{c: &Client{Clientset: cs}}).drain(context.Background(), "node", func(ctx context.Context, d time.Duration) error {
		waits++
		if waits > 1 {
			t.Fatal("did not observe termination")
		}
		return cs.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), p.Namespace, p.Name)
	})
	if err != nil || waits != 1 {
		t.Fatalf("waits=%d err=%v", waits, err)
	}
	verbs := []string{}
	for _, a := range cs.Actions() {
		verbs = append(verbs, a.GetVerb()+"/"+a.GetResource().Resource+"/"+a.GetSubresource())
	}
	want := []string{"list/pods/", "patch/nodes/", "create/pods/eviction", "get/pods/", "get/pods/"}
	if !reflect.DeepEqual(verbs, want) {
		t.Fatalf("API log=%v", verbs)
	}
}

func TestDrainGoneAtEviction(t *testing.T) {
	p := drainPod("gone")
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}, p)
	cs.PrependReactor("create", "pods", func(a ktesting.Action) (bool, runtime.Object, error) {
		if err := cs.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), p.Namespace, p.Name); err != nil {
			t.Fatal(err)
		}
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, p.Name)
	})
	if err := (&Store{c: &Client{Clientset: cs}}).Drain("node"); err != nil {
		t.Fatal(err)
	}
}

// Manual context expiry keeps deadline tests independent of scheduler/timers.
type drainExpiredContext struct {
	context.Context
	expired bool
	done    chan struct{}
}

func (c *drainExpiredContext) Err() error {
	if c.expired {
		return context.DeadlineExceeded
	}
	return nil
}
func (c *drainExpiredContext) Done() <-chan struct{}       { return c.done }
func (c *drainExpiredContext) Deadline() (time.Time, bool) { return time.Now().Add(time.Minute), true }

func TestDrainDeadlineDuringPDB(t *testing.T) {
	p := drainPod("blocked")
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}, p)
	attempts := 0
	cs.PrependReactor("create", "pods", func(a ktesting.Action) (bool, runtime.Object, error) {
		attempts++
		return true, nil, apierrors.NewTooManyRequests("PDB", 3600)
	})
	ctx := &drainExpiredContext{Context: context.Background(), done: make(chan struct{})}
	err := (&Store{c: &Client{Clientset: cs}}).drain(ctx, "node", func(c context.Context, d time.Duration) error {
		if d != time.Hour {
			t.Fatalf("retry delay=%v", d)
		}
		ctx.expired = true
		close(ctx.done)
		return drainWait(c, d)
	})
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "remains cordoned") || attempts != 1 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}

func TestDrainDeadlineDuringTermination(t *testing.T) {
	p := drainPod("terminating")
	p.Finalizers = []string{"hold"}
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}, p)
	cs.PrependReactor("create", "pods", func(a ktesting.Action) (bool, runtime.Object, error) { return true, nil, nil })
	ctx := &drainExpiredContext{Context: context.Background(), done: make(chan struct{})}
	err := (&Store{c: &Client{Clientset: cs}}).drain(ctx, "node", func(c context.Context, d time.Duration) error {
		ctx.expired = true
		close(ctx.done)
		return drainWait(c, d)
	})
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "ns/terminating") || !strings.Contains(err.Error(), "remains cordoned") {
		t.Fatal(err)
	}
}

func TestDrainPreflightListFailureAndCancellation(t *testing.T) {
	for _, cancelOnList := range []bool{false, true} {
		t.Run(fmtBool(cancelOnList), func(t *testing.T) {
			cs := fake.NewSimpleClientset()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			denied := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("list denied"))
			cs.PrependReactor("list", "pods", func(a ktesting.Action) (bool, runtime.Object, error) {
				if cancelOnList {
					cancel()
					return true, &corev1.PodList{}, nil
				}
				return true, nil, denied
			})
			err := (&Store{c: &Client{Clientset: cs}}).drain(ctx, "node", drainWait)
			if cancelOnList && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if !cancelOnList && !apierrors.IsForbidden(err) {
				t.Fatal(err)
			}
			if len(cs.Actions()) != 1 {
				t.Fatalf("preflight mutated: %v", cs.Actions())
			}
		})
	}
}

func TestDrainCanceledBeforeMutation(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (&Store{c: &Client{Clientset: cs}}).drain(ctx, "node", drainWait)
	if !errors.Is(err, context.Canceled) || len(cs.Actions()) != 0 {
		t.Fatalf("err=%v actions=%v", err, cs.Actions())
	}
}

func TestDrainPreflightNoMutation(t *testing.T) {
	for _, reason := range []string{"unmanaged", "UID"} {
		t.Run(reason, func(t *testing.T) {
			p := drainPod("z-blocker")
			switch reason {
			case "UID":
				p.UID = ""
			case "unmanaged":
				p.OwnerReferences = nil
			}
			cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}, drainPod("a-safe"), p)
			cs.PrependReactor("create", "pods", func(a ktesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("unexpected eviction")
			})
			s := &Store{c: &Client{Clientset: cs}}
			err := s.Drain("node")
			if err == nil || !strings.Contains(err.Error(), reason) || !strings.Contains(err.Error(), "ns/z-blocker") {
				t.Errorf("want named %s blocker, got %v", reason, err)
			}
			for _, a := range cs.Actions() {
				if a.GetVerb() != "list" && a.GetVerb() != "get" {
					t.Errorf("preflight mutated: %s %s", a.GetVerb(), a.GetResource().Resource)
				}
			}
		})
	}
}

func TestDrainEvictsEmptyDirPods(t *testing.T) {
	p := drainPod("scratch")
	p.Spec.Volumes = []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}, p)
	evictions := 0
	cs.PrependReactor("create", "pods", func(a ktesting.Action) (bool, runtime.Object, error) {
		evictions++
		return true, nil, cs.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), p.Namespace, p.Name)
	})
	if err := (&Store{c: &Client{Clientset: cs}}).Drain("node"); err != nil {
		t.Fatal(err)
	}
	if evictions != 1 {
		t.Fatalf("evictions=%d", evictions)
	}
}

// Neither pod terminates until both evictions have been issued: a sequential
// drain would wait on the first pod until the deadline.
func TestDrainEvictsConcurrently(t *testing.T) {
	a, b := drainPod("a"), drainPod("b")
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}, a, b)
	var mu sync.Mutex
	evicted := map[string]bool{}
	both := make(chan struct{})
	cs.PrependReactor("create", "pods", func(act ktesting.Action) (bool, runtime.Object, error) {
		ev := act.(ktesting.CreateAction).GetObject().(*policyv1.Eviction)
		mu.Lock()
		defer mu.Unlock()
		evicted[ev.Name] = true
		if len(evicted) == 2 {
			close(both)
		}
		return true, nil, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := (&Store{c: &Client{Clientset: cs}}).drain(ctx, "node", func(ctx context.Context, d time.Duration) error {
		select {
		case <-both:
		case <-ctx.Done():
			return ctx.Err()
		}
		mu.Lock()
		defer mu.Unlock()
		for _, p := range []*corev1.Pod{a, b} {
			if err := cs.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), p.Namespace, p.Name); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(evicted) != 2 {
		t.Fatalf("evicted=%v", evicted)
	}
}
