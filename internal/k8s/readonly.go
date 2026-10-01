package k8s

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"k8s.io/client-go/rest"

	"github.com/p10node/k10s/internal/domain"
)

// readOnly is set once by main, from --readonly, before any client exists.
var readOnly atomic.Bool

// SetReadOnly makes every client built after it refuse requests that could
// change the cluster or open a session into it. Clients that already exist
// keep their transport, so main calls it before the first connection.
func SetReadOnly(on bool) { readOnly.Store(on) }

// guardReadOnly puts readOnlyTransport under every client built from cfg:
// the clientset, the dynamic client, discovery, metrics, and the exec and
// port-forward streams, which client-go builds from the same config.
func guardReadOnly(cfg *rest.Config) {
	cfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return readOnlyTransport{next: rt}
	})
}

// readOnlyTransport refuses a request before it leaves the machine unless it
// can only read. It is the backstop behind the UI, which hides and refuses
// the same actions: whichever path a request takes, the API server never
// receives a write.
type readOnlyTransport struct{ next http.RoundTripper }

func (t readOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := checkReadOnly(req.Method, req.URL.Path); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(req)
}

// checkReadOnly allows GET, HEAD and OPTIONS, except where one opens a
// session: exec, attach and port-forward upgrade to a stream over GET when
// they use WebSockets, and proxy passes the request on to whatever listens
// behind it. Every other method can change something.
func checkReadOnly(method, path string) error {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		if sub := streamSubresource(path); sub != "" {
			return fmt.Errorf("%w: %s is not allowed", domain.ErrReadOnly, sub)
		}
		return nil
	}
	return fmt.Errorf("%w: %s %s is not allowed", domain.ErrReadOnly, method, path)
}

// streamSubresource names the session a GET to path would open, such as
// "pod exec" or "node proxy", and returns "" when path only reads:
//
//   - exec, attach and portforward of a pod,
//     .../namespaces/<ns>/pods/<name>/<sub>;
//   - proxy of a pod or a service, .../namespaces/<ns>/<kind>/<name>/proxy,
//     or of a node, .../v1/nodes/<name>/proxy, followed by the path it
//     forwards. Behind a node proxy is the kubelet, whose own API includes
//     exec, so nothing sent through a proxy is known to only read.
//
// Anchoring on "namespaces", and on "v1" for nodes, which have none, keeps an
// object that happens to be named "exec" or "proxy" readable.
func streamSubresource(path string) string {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	n := len(segs)
	if n >= 5 && segs[n-5] == "namespaces" && segs[n-3] == "pods" {
		switch segs[n-1] {
		case "exec", "attach", "portforward":
			return "pod " + segs[n-1]
		}
	}
	// The forwarded path follows proxy, so it can sit anywhere.
	for i := 3; i < n; i++ {
		if segs[i] != "proxy" {
			continue
		}
		switch kind := segs[i-2]; {
		case (kind == "pods" || kind == "services") && i >= 4 && segs[i-4] == "namespaces":
			return strings.TrimSuffix(kind, "s") + " proxy"
		case kind == "nodes" && segs[i-3] == "v1":
			return "node proxy"
		}
	}
	return ""
}
