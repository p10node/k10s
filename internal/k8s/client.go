// Package k8s talks to a real Kubernetes cluster: kubeconfig/contexts,
// informer-backed listing, describe/YAML/logs/exec/port-forward, actions
// (delete/cordon/drain/scale/restart/edit) and pod/node metrics.
package k8s

import (
	"fmt"
	"os"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/p10node/k10s/internal/domain"
)

// Client bundles every handle we need against one kubeconfig context.
type Client struct {
	RestConfig *rest.Config
	Clientset  kubernetes.Interface
	Dynamic    dynamic.Interface
	Discovery  discovery.DiscoveryInterface
	Mapper     *restmapper.DeferredDiscoveryRESTMapper
	Metrics    metricsv.Interface

	ConfigPath     string
	RawConfig      clientcmdapi.Config
	CurrentContext string
	Server         string
	Version        string

	// versionErr is what the API server said — or failed to say — when New
	// asked it for its version. It is the one request New makes, so it is
	// also the only evidence we have that there is a cluster at the other
	// end at all. See Reachable.
	versionErr error
}

// KubeconfigPath resolves the path k10s should load, honoring $KUBECONFIG
// and falling back to ~/.kube/config like kubectl does.
func KubeconfigPath() string {
	if p := os.Getenv("KUBECONFIG"); p != "" {
		return p
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	return rules.GetDefaultFilename()
}

func loadedKubeconfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return KubeconfigPath()
}

// New loads kubeconfig at path (empty = default resolution) and builds every
// client against the given context (empty = kubeconfig's current-context).
func New(path, context string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path != "" {
		rules.ExplicitPath = path
	}
	overrides := &clientcmd.ConfigOverrides{}
	if context != "" {
		overrides.CurrentContext = context
	}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)

	raw, err := loader.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	restCfg, err := loader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build client config: %w", err)
	}
	restCfg.QPS = 50
	restCfg.Burst = 100

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("clientset: %w", err)
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	// Discovery/RESTMapper calls are one-shot GETs (server version, API
	// groups) — bound them so a slow or unreachable API server can't hang
	// startup indefinitely. This timeout must NOT be applied to restCfg
	// itself: that config also backs the informers' long-lived list-watch
	// connections, which a blanket per-request timeout would kill.
	discoCfg := *restCfg
	discoCfg.Timeout = 10 * time.Second
	disco, err := discovery.NewDiscoveryClientForConfig(&discoCfg)
	if err != nil {
		return nil, fmt.Errorf("discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disco))

	metrics, err := metricsv.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("metrics client: %w", err)
	}

	cur := context
	if cur == "" {
		cur = raw.CurrentContext
	}

	c := &Client{
		RestConfig:     restCfg,
		Clientset:      cs,
		Dynamic:        dyn,
		Discovery:      disco,
		Mapper:         mapper,
		Metrics:        metrics,
		ConfigPath:     loadedKubeconfigPath(path),
		RawConfig:      raw,
		CurrentContext: cur,
		Server:         restCfg.Host,
	}
	if v, err := disco.ServerVersion(); err == nil {
		c.Version = v.GitVersion
	} else {
		c.versionErr = err
	}
	return c, nil
}

// Reachable reports whether there is actually a cluster behind this
// kubeconfig context: nil if the API server answered, the failure otherwise.
//
// A kubeconfig that parses is not a cluster. Every client above is built
// lazily and none of them dials, so a context left over from a cluster that
// was deleted — or one whose VPN is down — builds a perfectly healthy Client
// that can never answer. Without this check k10s would sit on an empty
// table instead of saying so.
//
// 401/403 are deliberately *not* unreachable: that is a live cluster
// refusing this user, a different problem with a different fix, and k10s can
// still show whatever they are allowed to see.
func (c *Client) Reachable() error {
	if c.versionErr == nil {
		return nil
	}
	if apierrors.IsUnauthorized(c.versionErr) || apierrors.IsForbidden(c.versionErr) {
		return nil
	}
	return c.versionErr
}

// Contexts lists every context name in the loaded kubeconfig, sorted.
//
// The sort is load-bearing, not cosmetic: kubeconfig contexts live in a Go
// map, and ranging a map yields a different order every call. An unsorted
// list would reshuffle between frames, so arrow keys in the chooser would
// land somewhere different each press.
func (c *Client) Contexts() []string {
	out := make([]string, 0, len(c.RawConfig.Contexts))
	for name := range c.RawConfig.Contexts {
		out = append(out, name)
	}
	domain.SortNames(out)
	return out
}

// DefaultNamespace is the namespace set for CurrentContext in kubeconfig,
// falling back to "default".
func (c *Client) DefaultNamespace() string {
	if ctx, ok := c.RawConfig.Contexts[c.CurrentContext]; ok && ctx.Namespace != "" {
		return ctx.Namespace
	}
	return "default"
}

// KubeContexts reads kubeconfig and nothing else: no client is built and no
// request is made, so it answers instantly even when the cluster is
// unreachable. Startup uses it to fill the context picker before (or
// instead of) a successful connection.
func KubeContexts(path string) (names []string, current string) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path != "" {
		rules.ExplicitPath = path
	}
	raw, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).RawConfig()
	if err != nil {
		return nil, ""
	}
	for name := range raw.Contexts {
		names = append(names, name)
	}
	domain.SortNames(names)
	return names, raw.CurrentContext
}
