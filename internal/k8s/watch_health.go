package k8s

import (
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sync"
	"time"
)

// Health is independent of HasSynced: that bit is historical, not liveness.
// Each informer generation owns its health, so evicted watchers cannot publish
// errors into a later cache of the same namespace/kind.
type watchHealth struct {
	mu      sync.Mutex
	err     error
	pending *time.Timer
}

// A closed watch is routine (the server expires them every few minutes) and
// the reflector re-establishes it at once. Health degrades only when that
// re-establishment is not prompt, so an empty view does not flash an error.
var watchReconnectGrace = 3 * time.Second

func (h *watchHealth) set(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cancelPendingLocked()
	h.err = err
}
func (h *watchHealth) get() error { h.mu.Lock(); defer h.mu.Unlock(); return h.err }
func (h *watchHealth) cancelPendingLocked() {
	if h.pending != nil {
		h.pending.Stop()
		h.pending = nil
	}
}

// degradeAfter publishes err unless set reports a newer outcome first.
func (h *watchHealth) degradeAfter(d time.Duration, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cancelPendingLocked()
	var t *time.Timer
	t = time.AfterFunc(d, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.pending != t {
			return
		}
		h.pending = nil
		h.err = err
	})
	h.pending = t
}

func healthyInformer[L runtime.Object](obj runtime.Object, list func(context.Context, metav1.ListOptions) (L, error), open func(context.Context, metav1.ListOptions) (watch.Interface, error), h *watchHealth) cache.SharedIndexInformer {
	lw := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			result, err := list(ctx, o)
			if err != nil {
				h.set(err)
			}
			return result, err
		},
		WatchFuncWithContext: func(ctx context.Context, o metav1.ListOptions) (watch.Interface, error) {
			w, err := open(ctx, o)
			if err != nil {
				h.set(err)
				return nil, err
			}
			h.set(nil) // successful watch establishment after LIST/reconnect is recovery
			out := &healthWatch{upstream: w, events: make(chan watch.Event), done: make(chan struct{})}
			go out.run(ctx, h)
			return out, nil
		},
	}
	return cache.NewSharedIndexInformer(traditionalListWatch{lw}, obj, 0, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
}

// Use authoritative LIST then WATCH, including with fake clients that cannot
// emit streaming-list initial-events bookmarks.
type traditionalListWatch struct{ *cache.ListWatch }

func (traditionalListWatch) IsWatchListSemanticsUnSupported() bool { return true }

type healthWatch struct {
	upstream watch.Interface
	events   chan watch.Event
	done     chan struct{}
	once     sync.Once
}

func (w *healthWatch) Stop()                          { w.once.Do(func() { close(w.done); w.upstream.Stop() }) }
func (w *healthWatch) ResultChan() <-chan watch.Event { return w.events }
func (w *healthWatch) run(ctx context.Context, h *watchHealth) {
	defer close(w.events)
	defer w.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case event, ok := <-w.upstream.ResultChan():
			if !ok {
				// Stop closes done before the upstream and shutdown cancels
				// ctx, so both are visible here. Only a close the reflector
				// did not ask for is a disconnect.
				select {
				case <-w.done:
				case <-ctx.Done():
				default:
					h.degradeAfter(watchReconnectGrace, fmt.Errorf("watch disconnected; cached rows may be stale"))
				}
				return
			}
			if event.Type == watch.Error {
				h.set(apierrors.FromObject(event.Object))
			}
			select {
			case w.events <- event:
			case <-ctx.Done():
				return
			case <-w.done:
				return
			}
		}
	}
}

// Pre-register the instrumented informer so generated typed listers share it.
func (s *Store) registerHealth(factory informers.SharedInformerFactory, key informerKey) {
	h := &watchHealth{}
	s.health[key] = h
	ns := key.namespace
	switch key.kind {
	case kPods:
		client := s.c.Clientset.CoreV1().Pods(ns)
		factory.InformerFor(&corev1.Pod{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.Pod{}, client.List, client.Watch, h)
		})
	case kDeployments:
		client := s.c.Clientset.AppsV1().Deployments(ns)
		factory.InformerFor(&appsv1.Deployment{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&appsv1.Deployment{}, client.List, client.Watch, h)
		})
	case kStatefulSet:
		client := s.c.Clientset.AppsV1().StatefulSets(ns)
		factory.InformerFor(&appsv1.StatefulSet{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&appsv1.StatefulSet{}, client.List, client.Watch, h)
		})
	case kDaemonSets:
		client := s.c.Clientset.AppsV1().DaemonSets(ns)
		factory.InformerFor(&appsv1.DaemonSet{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&appsv1.DaemonSet{}, client.List, client.Watch, h)
		})
	case kJobs:
		client := s.c.Clientset.BatchV1().Jobs(ns)
		factory.InformerFor(&batchv1.Job{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&batchv1.Job{}, client.List, client.Watch, h)
		})
	case kCronJobs:
		client := s.c.Clientset.BatchV1().CronJobs(ns)
		factory.InformerFor(&batchv1.CronJob{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&batchv1.CronJob{}, client.List, client.Watch, h)
		})
	case kServices:
		client := s.c.Clientset.CoreV1().Services(ns)
		factory.InformerFor(&corev1.Service{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.Service{}, client.List, client.Watch, h)
		})
	case kIngresses:
		client := s.c.Clientset.NetworkingV1().Ingresses(ns)
		factory.InformerFor(&networkingv1.Ingress{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&networkingv1.Ingress{}, client.List, client.Watch, h)
		})
	case kConfigMaps:
		client := s.c.Clientset.CoreV1().ConfigMaps(ns)
		factory.InformerFor(&corev1.ConfigMap{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.ConfigMap{}, client.List, client.Watch, h)
		})
	case kSecrets:
		client := s.c.Clientset.CoreV1().Secrets(ns)
		factory.InformerFor(&corev1.Secret{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.Secret{}, client.List, client.Watch, h)
		})
	case kPVCs:
		client := s.c.Clientset.CoreV1().PersistentVolumeClaims(ns)
		factory.InformerFor(&corev1.PersistentVolumeClaim{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.PersistentVolumeClaim{}, client.List, client.Watch, h)
		})
	case kNodes:
		client := s.c.Clientset.CoreV1().Nodes()
		factory.InformerFor(&corev1.Node{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.Node{}, client.List, client.Watch, h)
		})
	case kNamespaces:
		client := s.c.Clientset.CoreV1().Namespaces()
		factory.InformerFor(&corev1.Namespace{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.Namespace{}, client.List, client.Watch, h)
		})
	case kEvents:
		client := s.c.Clientset.CoreV1().Events(ns)
		factory.InformerFor(&corev1.Event{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.Event{}, client.List, client.Watch, h)
		})
	case kReplicaSets:
		client := s.c.Clientset.AppsV1().ReplicaSets(ns)
		factory.InformerFor(&appsv1.ReplicaSet{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&appsv1.ReplicaSet{}, client.List, client.Watch, h)
		})
	case kHPAs:
		client := s.c.Clientset.AutoscalingV2().HorizontalPodAutoscalers(ns)
		factory.InformerFor(&autoscalingv2.HorizontalPodAutoscaler{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&autoscalingv2.HorizontalPodAutoscaler{}, client.List, client.Watch, h)
		})
	case kEndpoints:
		client := s.c.Clientset.CoreV1().Endpoints(ns)
		factory.InformerFor(&corev1.Endpoints{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer { //nolint:staticcheck // Keep the existing Endpoints resource view compatible with older clusters.
			return healthyInformer(&corev1.Endpoints{}, client.List, client.Watch, h) //nolint:staticcheck // Matches the resource requested by this view.
		})
	case kNetPols:
		client := s.c.Clientset.NetworkingV1().NetworkPolicies(ns)
		factory.InformerFor(&networkingv1.NetworkPolicy{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&networkingv1.NetworkPolicy{}, client.List, client.Watch, h)
		})
	case kQuotas:
		client := s.c.Clientset.CoreV1().ResourceQuotas(ns)
		factory.InformerFor(&corev1.ResourceQuota{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.ResourceQuota{}, client.List, client.Watch, h)
		})
	case kLimitRanges:
		client := s.c.Clientset.CoreV1().LimitRanges(ns)
		factory.InformerFor(&corev1.LimitRange{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.LimitRange{}, client.List, client.Watch, h)
		})
	case kPDBs:
		client := s.c.Clientset.PolicyV1().PodDisruptionBudgets(ns)
		factory.InformerFor(&policyv1.PodDisruptionBudget{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&policyv1.PodDisruptionBudget{}, client.List, client.Watch, h)
		})
	case kPVs:
		client := s.c.Clientset.CoreV1().PersistentVolumes()
		factory.InformerFor(&corev1.PersistentVolume{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.PersistentVolume{}, client.List, client.Watch, h)
		})
	case kStorageCls:
		client := s.c.Clientset.StorageV1().StorageClasses()
		factory.InformerFor(&storagev1.StorageClass{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&storagev1.StorageClass{}, client.List, client.Watch, h)
		})
	case kSAs:
		client := s.c.Clientset.CoreV1().ServiceAccounts(ns)
		factory.InformerFor(&corev1.ServiceAccount{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&corev1.ServiceAccount{}, client.List, client.Watch, h)
		})
	case kRoles:
		client := s.c.Clientset.RbacV1().Roles(ns)
		factory.InformerFor(&rbacv1.Role{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&rbacv1.Role{}, client.List, client.Watch, h)
		})
	case kRoleBinds:
		client := s.c.Clientset.RbacV1().RoleBindings(ns)
		factory.InformerFor(&rbacv1.RoleBinding{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&rbacv1.RoleBinding{}, client.List, client.Watch, h)
		})
	case kClusterRole:
		client := s.c.Clientset.RbacV1().ClusterRoles()
		factory.InformerFor(&rbacv1.ClusterRole{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&rbacv1.ClusterRole{}, client.List, client.Watch, h)
		})
	case kClusterBind:
		client := s.c.Clientset.RbacV1().ClusterRoleBindings()
		factory.InformerFor(&rbacv1.ClusterRoleBinding{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&rbacv1.ClusterRoleBinding{}, client.List, client.Watch, h)
		})
	case kCRDs:
		client := s.apiext.ApiextensionsV1().CustomResourceDefinitions()
		s.apiextFactory.InformerFor(&apiextv1.CustomResourceDefinition{}, func(apiextclient.Interface, time.Duration) cache.SharedIndexInformer {
			return healthyInformer(&apiextv1.CustomResourceDefinition{}, client.List, client.Watch, h)
		})
	}
}
