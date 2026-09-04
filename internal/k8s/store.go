package k8s

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextinformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/p10node/k10s/internal/domain"
)

// Store is the real backend: client-go informers backing live table data,
// plus direct API calls for describe/YAML/logs/actions/metrics.
type Store struct {
	c *Client

	kubeconfigPath string
	// factory is the cluster-wide factory used for cluster-scoped kinds and
	// explicit all-namespaces views. namespacedFactories are created lazily
	// for a selected namespace so a RoleBinding-only identity never needs a
	// forbidden cluster-wide LIST/WATCH just to open its authorized namespace.
	factory             informers.SharedInformerFactory
	namespacedFactories map[string]informers.SharedInformerFactory
	apiext              apiextclientset.Interface
	apiextFactory       apiextinformers.SharedInformerFactory
	stop                chan struct{}

	// infMu guards started: which kinds have a running informer. Informers
	// are started lazily, per kind, the first time that kind is displayed.
	// loadErr retains the latest LIST/WATCH failure for an informer that has
	// not completed its first list, so the UI can distinguish Forbidden from
	// a request that is merely still in flight.
	infMu   sync.Mutex
	started map[informerKey]bool
	loadErr map[informerKey]error

	mu          sync.RWMutex
	podMetrics  map[string]metricSample // "ns/name" -> cpu/mem
	nodeMetrics map[string]metricSample // name -> cpu/mem

	// Custom resources have no informer (their GVRs are only known at
	// runtime), so they're swept on a background timer instead. crCache is
	// the only thing the render path ever reads.
	crMu      sync.Mutex
	crCache   []nsRow
	crAt      time.Time
	crRunning bool
	crErr     error

	// Sidebar badge counts for kinds whose informer isn't running, refreshed
	// off-thread so the sidebar can show a number without opening a watch.
	cntMu      sync.RWMutex
	cnt        map[string]int // "kind/namespace" -> count
	cntNS      string
	cntRunning bool
	cntKick    chan struct{}
	// cntWant is when each kind was last asked about by the render path.
	// Only kinds on screen are swept, so folding a sidebar group away stops
	// its requests — see counts.go.
	cntWant map[string]time.Time
	// cntGone remembers kinds this cluster will not count (no such API
	// group, or no permission) and when that was learnt, so a locked-down
	// cluster isn't asked the same forbidden question every 30 seconds.
	cntGone map[string]time.Time
}

type metricSample struct {
	cpuMilli int64
	memBytes int64
}

type informerKey struct {
	kind      string
	namespace string // empty means cluster-wide/all namespaces
}

// NewStore builds a Store against the given kubeconfig path/context (both
// may be empty to use defaults) and blocks until informer caches sync.
func NewStore(path, context string) (*Store, error) {
	c, err := New(path, context)
	if err != nil {
		return nil, err
	}
	return newStoreFromClient(c)
}

func newStoreFromClient(c *Client) (*Store, error) {
	apiext, err := apiextclientset.NewForConfig(c.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("apiextensions client: %w", err)
	}
	return newStoreFrom(c, apiext)
}

// newStoreFrom builds a Store from already-constructed clients. Split out
// from newStoreFromClient so tests can pass fake clientsets (fake.NewSimpleClientset
// et al) without needing a real *rest.Config to build the apiextensions
// client from.
func newStoreFrom(c *Client, apiext apiextclientset.Interface) (*Store, error) {
	s := &Store{
		c:              c,
		kubeconfigPath: c.ConfigPath,
		// resync period 0: we never register event handlers, so periodic
		// resyncs would be pure overhead. Freshness comes from the watch.
		factory:             informers.NewSharedInformerFactory(c.Clientset, 0),
		namespacedFactories: map[string]informers.SharedInformerFactory{},
		apiext:              apiext,
		apiextFactory:       apiextinformers.NewSharedInformerFactory(apiext, 0),
		stop:                make(chan struct{}),
		started:             map[informerKey]bool{},
		loadErr:             map[informerKey]error{},
		podMetrics:          map[string]metricSample{},
		nodeMetrics:         map[string]metricSample{},
		cnt:                 map[string]int{},
		cntKick:             make(chan struct{}, 1),
		cntWant:             map[string]time.Time{},
		cntGone:             map[string]time.Time{},
	}

	// No informers are registered here and nothing is awaited: construction
	// must be instant. Each kind's informer is started the first time that
	// kind is actually displayed (see ensure), and the UI repaints as caches
	// fill in. Eagerly listing every kind up front — secrets and events
	// across all namespaces especially — is what made startup crawl.
	go s.refreshMetricsLoop()

	return s, nil
}

// Kind keys that have an informer behind them.
const (
	kPods        = "pods"
	kDeployments = "deployments"
	kStatefulSet = "statefulsets"
	kDaemonSets  = "daemonsets"
	kJobs        = "jobs"
	kCronJobs    = "cronjobs"
	kServices    = "services"
	kIngresses   = "ingresses"
	kConfigMaps  = "configmaps"
	kSecrets     = "secrets"
	kPVCs        = "pvcs"
	kNodes       = "nodes"
	kNamespaces  = "namespaces"
	kEvents      = "events"
	kCRDs        = "crds"

	kReplicaSets = "replicasets"
	kHPAs        = "hpas"
	kEndpoints   = "endpoints"
	kNetPols     = "networkpolicies"
	kQuotas      = "resourcequotas"
	kLimitRanges = "limitranges"
	kPDBs        = "pdbs"
	kPVs         = "pvs"
	kStorageCls  = "storageclasses"
	kSAs         = "serviceaccounts"
	kRoles       = "roles"
	kRoleBinds   = "rolebindings"
	kClusterRole = "clusterroles"
	kClusterBind = "clusterrolebindings"
)

// register wires up the informer for kind without starting it. Calling a
// factory's typed accessor is what registers it; the factory caches by type,
// so repeat calls are cheap and return the same shared informer.
func register(factory informers.SharedInformerFactory, apiextFactory apiextinformers.SharedInformerFactory, kind string) cache.SharedIndexInformer {
	switch kind {
	case kPods:
		return factory.Core().V1().Pods().Informer()
	case kDeployments:
		return factory.Apps().V1().Deployments().Informer()
	case kStatefulSet:
		return factory.Apps().V1().StatefulSets().Informer()
	case kDaemonSets:
		return factory.Apps().V1().DaemonSets().Informer()
	case kJobs:
		return factory.Batch().V1().Jobs().Informer()
	case kCronJobs:
		return factory.Batch().V1().CronJobs().Informer()
	case kServices:
		return factory.Core().V1().Services().Informer()
	case kIngresses:
		return factory.Networking().V1().Ingresses().Informer()
	case kConfigMaps:
		return factory.Core().V1().ConfigMaps().Informer()
	case kSecrets:
		return factory.Core().V1().Secrets().Informer()
	case kPVCs:
		return factory.Core().V1().PersistentVolumeClaims().Informer()
	case kNodes:
		return factory.Core().V1().Nodes().Informer()
	case kNamespaces:
		return factory.Core().V1().Namespaces().Informer()
	case kEvents:
		return factory.Core().V1().Events().Informer()
	case kCRDs:
		return apiextFactory.Apiextensions().V1().CustomResourceDefinitions().Informer()
	case kReplicaSets:
		return factory.Apps().V1().ReplicaSets().Informer()
	case kHPAs:
		return factory.Autoscaling().V2().HorizontalPodAutoscalers().Informer()
	case kEndpoints:
		return factory.Core().V1().Endpoints().Informer()
	case kNetPols:
		return factory.Networking().V1().NetworkPolicies().Informer()
	case kQuotas:
		return factory.Core().V1().ResourceQuotas().Informer()
	case kLimitRanges:
		return factory.Core().V1().LimitRanges().Informer()
	case kPDBs:
		return factory.Policy().V1().PodDisruptionBudgets().Informer()
	case kPVs:
		return factory.Core().V1().PersistentVolumes().Informer()
	case kStorageCls:
		return factory.Storage().V1().StorageClasses().Informer()
	case kSAs:
		return factory.Core().V1().ServiceAccounts().Informer()
	case kRoles:
		return factory.Rbac().V1().Roles().Informer()
	case kRoleBinds:
		return factory.Rbac().V1().RoleBindings().Informer()
	case kClusterRole:
		return factory.Rbac().V1().ClusterRoles().Informer()
	case kClusterBind:
		return factory.Rbac().V1().ClusterRoleBindings().Informer()
	}
	return nil
}

// desiredInformerScope maps a view namespace to the namespace the API
// LIST/WATCH should use. With no namespace argument, legacy internal callers
// mean all namespaces; Rows always passes the actual view namespace.
func (s *Store) desiredInformerScope(kind string, ns []string) string {
	k := findKind(kind)
	if k == nil || !k.Namespaced {
		return metav1.NamespaceAll
	}
	if len(ns) == 0 || ns[0] == domain.AllNamespaces {
		return metav1.NamespaceAll
	}
	return effectiveNS(ns[0])
}

// accessScopeLocked reuses an already-running all-namespaces cache when one
// exists (important for admin sessions and existing tests), but never starts
// that broader cache on behalf of a namespace-scoped view.
func (s *Store) accessScopeLocked(kind, desired string) string {
	if desired != metav1.NamespaceAll && s.started[informerKey{kind: kind, namespace: metav1.NamespaceAll}] {
		// A started informer is not necessarily usable: RBAC-denied initial
		// LISTs remain unsynced while client-go retries. Reuse only a cache
		// that actually completed its initial list; otherwise allow the
		// narrower, authorized namespace informer to start.
		if inf := register(s.factory, s.apiextFactory, kind); inf != nil && inf.HasSynced() {
			return metav1.NamespaceAll
		}
	}
	return desired
}

func (s *Store) factoryLocked(namespace string) informers.SharedInformerFactory {
	if namespace == metav1.NamespaceAll {
		return s.factory
	}
	if factory := s.namespacedFactories[namespace]; factory != nil {
		return factory
	}
	factory := informers.NewSharedInformerFactoryWithOptions(s.c.Clientset, 0, informers.WithNamespace(namespace))
	s.namespacedFactories[namespace] = factory
	return factory
}

// ensure starts the informer backing kind if it isn't running yet, and
// returns immediately — it never waits for the cache to sync. Callers get
// whatever is cached so far (possibly nothing on the first frame); the UI
// polls and repaints as data arrives.
func (s *Store) ensure(kind string, ns ...string) {
	s.infMu.Lock()
	defer s.infMu.Unlock()
	desired := s.desiredInformerScope(kind, ns)
	scope := s.accessScopeLocked(kind, desired)
	key := informerKey{kind: kind, namespace: scope}
	if s.started[key] {
		return
	}
	factory := s.factoryLocked(scope)
	inf := register(factory, s.apiextFactory, kind)
	if inf == nil {
		return
	}
	// Reflector sends both initial LIST failures and later WATCH failures to
	// this handler. It must be installed before Start; client-go rejects a
	// handler change after the informer is running.
	_ = inf.SetWatchErrorHandler(func(_ *cache.Reflector, err error) {
		s.infMu.Lock()
		s.loadErr[key] = err
		s.infMu.Unlock()
	})
	s.started[key] = true
	// Start is idempotent and launches only newly-registered informers.
	if kind == kCRDs {
		s.apiextFactory.Start(s.stop)
	} else {
		factory.Start(s.stop)
	}
}

// isStarted reports whether kind's informer has been started, without
// starting it. The sidebar uses this so drawing a badge never causes a new
// cluster-wide watch for a kind the user hasn't opened.
func (s *Store) isStarted(kind string, ns ...string) bool {
	s.infMu.Lock()
	defer s.infMu.Unlock()
	desired := s.desiredInformerScope(kind, ns)
	scope := s.accessScopeLocked(kind, desired)
	return s.started[informerKey{kind: kind, namespace: scope}]
}

// Synced reports whether the informer for kind has finished its initial
// list, so the UI can tell "no objects" apart from "still loading".
func (s *Store) Synced(kind string) bool {
	return s.SyncedFor(kind, domain.AllNamespaces)
}

// SyncedFor is the namespace-aware form used by the UI. Custom Resources
// are polling-backed rather than informer-backed, so a completed sweep —
// including a successful empty result — is their synchronization signal.
func (s *Store) SyncedFor(kind, ns string) bool {
	if kind == "customresources" {
		s.crMu.Lock()
		defer s.crMu.Unlock()
		return !s.crAt.IsZero()
	}

	s.infMu.Lock()
	defer s.infMu.Unlock()
	desired := s.desiredInformerScope(kind, []string{ns})
	scope := s.accessScopeLocked(kind, desired)
	if !s.started[informerKey{kind: kind, namespace: scope}] {
		return false
	}
	factory := s.factoryLocked(scope)
	inf := register(factory, s.apiextFactory, kind)
	synced := inf != nil && inf.HasSynced()
	if synced {
		// HasSynced means the informer recovered and completed an authoritative
		// list. Do not leave an earlier transient error stuck on screen.
		delete(s.loadErr, informerKey{kind: kind, namespace: scope})
	}
	return synced
}

// LoadErrorFor reports why the selected kind/scope has not completed its
// first load. It is deliberately an optional Source capability: the in-memory
// demo has no asynchronous list/watch and therefore no load error to expose.
func (s *Store) LoadErrorFor(kind, ns string) error {
	if kind == "customresources" {
		s.crMu.Lock()
		defer s.crMu.Unlock()
		return s.crErr
	}

	s.infMu.Lock()
	defer s.infMu.Unlock()
	desired := s.desiredInformerScope(kind, []string{ns})
	scope := s.accessScopeLocked(kind, desired)
	key := informerKey{kind: kind, namespace: scope}
	if !s.started[key] {
		return nil
	}
	inf := register(s.factoryLocked(scope), s.apiextFactory, kind)
	if inf != nil && inf.HasSynced() {
		delete(s.loadErr, key)
		return nil
	}
	return s.loadErr[key]
}

// factoryFor returns the informer factory serving kind in the requested
// namespace, starting that scoped informer lazily if needed.
func (s *Store) factoryFor(kind string, ns ...string) informers.SharedInformerFactory {
	s.ensure(kind, ns...)
	s.infMu.Lock()
	defer s.infMu.Unlock()
	desired := s.desiredInformerScope(kind, ns)
	scope := s.accessScopeLocked(kind, desired)
	return s.factoryLocked(scope)
}

func (s *Store) startedScopes(kind string) []string {
	s.infMu.Lock()
	defer s.infMu.Unlock()
	var scopes []string
	for key := range s.started {
		if key.kind == kind {
			scopes = append(scopes, key.namespace)
		}
	}
	return scopes
}

func (s *Store) refreshMetricsLoop() {
	s.refreshMetrics()
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.refreshMetrics()
		}
	}
}

func (s *Store) refreshMetrics() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Pod metrics are only rendered in the pods table, and the cluster-wide
	// list is expensive — skip it entirely until pods have been opened.
	pm := map[string]metricSample{}
	for _, namespace := range s.startedScopes(kPods) {
		if list, err := s.c.Metrics.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{}); err == nil {
			for _, m := range list.Items {
				var cpu, mem int64
				for _, cnt := range m.Containers {
					if q, ok := cnt.Usage[corev1.ResourceCPU]; ok {
						cpu += q.MilliValue()
					}
					if q, ok := cnt.Usage[corev1.ResourceMemory]; ok {
						mem += q.Value()
					}
				}
				pm[m.Namespace+"/"+m.Name] = metricSample{cpuMilli: cpu, memBytes: mem}
			}
		}
	}

	nm := map[string]metricSample{}
	if list, err := s.c.Metrics.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{}); err == nil {
		for _, m := range list.Items {
			var cpu, mem int64
			if q, ok := m.Usage[corev1.ResourceCPU]; ok {
				cpu = q.MilliValue()
			}
			if q, ok := m.Usage[corev1.ResourceMemory]; ok {
				mem = q.Value()
			}
			nm[m.Name] = metricSample{cpuMilli: cpu, memBytes: mem}
		}
	}

	s.mu.Lock()
	s.podMetrics, s.nodeMetrics = pm, nm
	s.mu.Unlock()
}

func (s *Store) podMetric(ns, name string) (metricSample, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.podMetrics[ns+"/"+name]
	return m, ok
}

func (s *Store) nodeMetric(name string) (metricSample, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.nodeMetrics[name]
	return m, ok
}

func (s *Store) Kinds() []domain.Kind { return Kinds() }

// Ping reports whether there is a cluster behind this store — see
// Client.Reachable. Nothing here dials: the answer was settled by the single
// version request New already made, so this is free to call.
func (s *Store) Ping() error { return s.c.Reachable() }

func (s *Store) ClusterInfo() domain.ClusterInfo {
	info := domain.ClusterInfo{
		Context: s.c.CurrentContext, Kubeconfig: s.c.ConfigPath,
		Server: s.c.Server, Version: s.c.Version,
	}
	if current := s.c.RawConfig.Contexts[s.c.CurrentContext]; current != nil {
		info.Cluster = current.Cluster
		info.User = current.AuthInfo
		if auth := s.c.RawConfig.AuthInfos[current.AuthInfo]; auth != nil {
			info.Groups = strings.Join(auth.ImpersonateGroups, ",")
		}
	}
	return info
}

func (s *Store) Nodes() []domain.NodeInfo {
	nodes, _ := s.nodeLister().List(labels.Everything())
	out := make([]domain.NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodeInfo(n, s))
	}
	return out
}

func (s *Store) DefaultNamespace() string { return s.c.DefaultNamespace() }

func (s *Store) Contexts() []string { return s.c.Contexts() }

// Namespaces lists every namespace, sorted — informer caches return objects
// in arbitrary order, and anything the user scrolls must be stable.
func (s *Store) Namespaces() []string {
	nss, _ := s.nsLister().List(labels.Everything())
	listForbidden := false

	// The first command that mentions a namespace is often what starts the
	// lazy namespace informer. A lister is necessarily empty on that exact
	// call, so use one bounded direct LIST as the cold-cache fallback instead
	// of misclassifying the namespace as row-filter text.
	if !s.Synced(kNamespaces) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if list, err := s.c.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{}); err == nil {
			nss = make([]*corev1.Namespace, len(list.Items))
			for i := range list.Items {
				nss[i] = &list.Items[i]
			}
		} else if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			listForbidden = true
		}
		cancel()
	}

	out := make([]string, 0, len(nss)+1)
	seen := make(map[string]bool, len(nss)+1)
	for _, n := range nss {
		if !seen[n.Name] {
			out = append(out, n.Name)
			seen[n.Name] = true
		}
	}
	// A RoleBinding-only user may not list the cluster-scoped Namespace
	// object, but the kubeconfig's current namespace is still valid and must
	// be recognized by :kind <namespace>.
	if current := s.DefaultNamespace(); listForbidden && current != "" && current != domain.AllNamespaces && !seen[current] {
		out = append(out, current)
	}
	domain.SortNames(out)
	return out
}

func (s *Store) SwitchContext(name string) (domain.Source, error) {
	target := name
	if target == "" {
		ctxs := s.c.Contexts()
		for i, c := range ctxs {
			if c == s.c.CurrentContext && i+1 < len(ctxs) {
				target = ctxs[i+1]
				break
			}
		}
		if target == "" && len(ctxs) > 0 {
			target = ctxs[0]
		}
	} else {
		// allow substring match like the old mock behaviour
		found := false
		for _, c := range s.c.Contexts() {
			if c == target {
				found = true
				break
			}
		}
		if !found {
			for _, c := range s.c.Contexts() {
				if strings.Contains(c, name) {
					target = c
					found = true
					break
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("no such context %q", name)
		}
	}
	ns, err := NewStore(s.kubeconfigPath, target)
	if err != nil {
		return nil, err
	}
	return ns, nil
}

func (s *Store) Close() {
	close(s.stop)
}

// gvrFor resolves the GroupVersionResource for a builtin kind key or a CRD
// plural name, via the REST mapper / CRD lister.
func (s *Store) gvrFor(kind string) (schema.GroupVersionResource, bool, error) {
	switch kind {
	case "pods":
		return schema.GroupVersionResource{Version: "v1", Resource: "pods"}, true, nil
	case "deployments":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, true, nil
	case "statefulsets":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, true, nil
	case "daemonsets":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, true, nil
	case "jobs":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, true, nil
	case "cronjobs":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, true, nil
	case "services":
		return schema.GroupVersionResource{Version: "v1", Resource: "services"}, true, nil
	case "ingresses":
		return schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}, true, nil
	case "configmaps":
		return schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, true, nil
	case "secrets":
		return schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, true, nil
	case "pvcs":
		return schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}, true, nil
	case "nodes":
		return schema.GroupVersionResource{Version: "v1", Resource: "nodes"}, false, nil
	case "namespaces":
		return schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}, false, nil
	case "events":
		return schema.GroupVersionResource{Version: "v1", Resource: "events"}, true, nil
	case "crds":
		return schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}, false, nil
	case "replicasets":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, true, nil
	case "hpas":
		return schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}, true, nil
	case "endpoints":
		return schema.GroupVersionResource{Version: "v1", Resource: "endpoints"}, true, nil
	case "networkpolicies":
		return schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}, true, nil
	case "resourcequotas":
		return schema.GroupVersionResource{Version: "v1", Resource: "resourcequotas"}, true, nil
	case "limitranges":
		return schema.GroupVersionResource{Version: "v1", Resource: "limitranges"}, true, nil
	case "pdbs":
		return schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}, true, nil
	case "pvs":
		return schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumes"}, false, nil
	case "storageclasses":
		return schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}, false, nil
	case "serviceaccounts":
		return schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}, true, nil
	case "roles":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}, true, nil
	case "rolebindings":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}, true, nil
	case "clusterroles":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}, false, nil
	case "clusterrolebindings":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}, false, nil
	}
	// CRDs / custom resource instances: resolve via discovered CRD by plural
	// name embedded as "cr:<group>:<version>:<resource>:<namespaced>".
	if gvr, namespaced, ok := decodeCRGVR(kind); ok {
		return gvr, namespaced, nil
	}
	return schema.GroupVersionResource{}, false, fmt.Errorf("unknown kind %q", kind)
}

func decodeCRGVR(s string) (schema.GroupVersionResource, bool, bool) {
	parts := strings.SplitN(s, "|", 5)
	if len(parts) != 5 || parts[0] != "cr" {
		return schema.GroupVersionResource{}, false, false
	}
	return schema.GroupVersionResource{Group: parts[1], Version: parts[2], Resource: parts[3]}, parts[4] == "1", true
}

func encodeCRGVR(gvr schema.GroupVersionResource, namespaced bool) string {
	n := "0"
	if namespaced {
		n = "1"
	}
	return strings.Join([]string{"cr", gvr.Group, gvr.Version, gvr.Resource, n}, "|")
}

func servedStorageVersion(crd *apiextv1.CustomResourceDefinition) string {
	for _, v := range crd.Spec.Versions {
		if v.Storage {
			return v.Name
		}
	}
	if len(crd.Spec.Versions) > 0 {
		return crd.Spec.Versions[0].Name
	}
	return ""
}

func quantityMi(q resource.Quantity) int64 { return q.Value() / (1024 * 1024) }

var _ = cache.MetaNamespaceKeyFunc
