package k8s

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"

	"github.com/p10node/k10s/internal/domain"
)

func TestCheckReadOnlyAllowsOnlyReads(t *testing.T) {
	cases := []struct {
		method, path string
		allowed      bool
	}{
		{"GET", "/api/v1/namespaces/default/pods", true},
		{"GET", "/api/v1/namespaces/default/pods/web/log", true},
		{"GET", "/apis/metrics.k8s.io/v1beta1/nodes", true},
		{"HEAD", "/version", true},
		{"OPTIONS", "/api", true},
		// A pod named "exec" in a namespace named "pods" is still a read.
		{"GET", "/api/v1/namespaces/pods/pods/exec", true},
		{"POST", "/api/v1/namespaces/default/pods", false},
		{"PUT", "/apis/apps/v1/namespaces/default/deployments/web", false},
		{"PATCH", "/apis/apps/v1/namespaces/default/deployments/web/scale", false},
		{"DELETE", "/api/v1/namespaces/default/pods/web", false},
		{"POST", "/api/v1/namespaces/default/pods/web/eviction", false},
		{"POST", "/api/v1/namespaces/default/pods/web/exec", false},
		// WebSocket exec, attach and port-forward open with a GET.
		{"GET", "/api/v1/namespaces/default/pods/web/exec", false},
		{"GET", "/api/v1/namespaces/default/pods/web/attach", false},
		{"GET", "/api/v1/namespaces/default/pods/web/portforward", false},
		// proxy hands the request to whatever listens behind a pod, a
		// service or a node, where it is the kubelet.
		{"GET", "/api/v1/namespaces/default/pods/web/proxy", false},
		{"GET", "/api/v1/namespaces/default/pods/web:8080/proxy/admin/reset", false},
		{"GET", "/api/v1/namespaces/default/services/http:web:80/proxy/", false},
		{"HEAD", "/api/v1/nodes/node-1/proxy/healthz", false},
		{"GET", "/api/v1/nodes/node-1/proxy/exec/default/web/app", false},
		// An API server served under a path prefix, as some gateways do.
		{"GET", "/k8s/clusters/c-1/api/v1/nodes/node-1/proxy/stats/summary", false},
		{"GET", "/k8s/clusters/c-1/api/v1/namespaces/default/pods", true},
		// Objects that are merely named "proxy" stay readable.
		{"GET", "/api/v1/namespaces/default/pods/proxy", true},
		{"GET", "/api/v1/namespaces/default/pods/proxy/log", true},
		{"GET", "/api/v1/nodes/proxy", true},
		{"GET", "/apis/metrics.k8s.io/v1beta1/nodes/proxy", true},
	}
	for _, c := range cases {
		err := checkReadOnly(c.method, c.path)
		if c.allowed && err != nil {
			t.Errorf("%s %s refused: %v", c.method, c.path, err)
		}
		if !c.allowed && !errors.Is(err, domain.ErrReadOnly) {
			t.Errorf("%s %s = %v, want ErrReadOnly", c.method, c.path, err)
		}
	}
}

// recordingAPIServer answers every request with a 404 Status and remembers
// what reached it, so a test can prove a request never left the client.
func recordingAPIServer(t *testing.T) (*rest.Config, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
	}))
	t.Cleanup(srv.Close)
	cfg := &rest.Config{Host: srv.URL}
	guardReadOnly(cfg)
	return cfg, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

func TestReadOnlyClientSendsReadsAndNoWrites(t *testing.T) {
	cfg, seen := recordingAPIServer(t)
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if err := cs.CoreV1().Pods("default").Delete(ctx, "web", metav1.DeleteOptions{}); !errors.Is(err, domain.ErrReadOnly) {
		t.Errorf("delete: err = %v, want ErrReadOnly", err)
	}
	if _, err := cs.AppsV1().Deployments("default").UpdateScale(ctx, "web", &autoscalingv1.Scale{}, metav1.UpdateOptions{}); !errors.Is(err, domain.ErrReadOnly) {
		t.Errorf("scale: err = %v, want ErrReadOnly", err)
	}
	if _, err := cs.CoreV1().Pods("default").Get(ctx, "web", metav1.GetOptions{}); errors.Is(err, domain.ErrReadOnly) {
		t.Errorf("get was refused: %v", err)
	}

	got := seen()
	if len(got) != 1 || got[0] != "GET /api/v1/namespaces/default/pods/web" {
		t.Errorf("requests that reached the API server = %v, want only the GET", got)
	}
}

// exec and port-forward build their own transports from the config, over
// WebSockets or SPDY; the guard has to sit under those too.
func TestReadOnlyClientOpensNoExecOrPortForward(t *testing.T) {
	cfg, seen := recordingAPIServer(t)

	execURL, err := url.Parse(cfg.Host + "/api/v1/namespaces/default/pods/web/exec?command=sh&stdin=true&stdout=true&tty=true")
	if err != nil {
		t.Fatal(err)
	}
	executor, err := newExecutor(cfg, execURL)
	if err != nil {
		t.Fatal(err)
	}
	err = executor.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Tty: true,
	})
	if err == nil || !strings.Contains(err.Error(), domain.ErrReadOnly.Error()) {
		t.Errorf("exec: err = %v, want a read-only refusal", err)
	}

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	pfURL, err := url.Parse(cfg.Host + "/api/v1/namespaces/default/pods/web/portforward")
	if err != nil {
		t.Fatal(err)
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, pfURL)
	if _, _, err := dialer.Dial("portforward.k8s.io"); err == nil || !strings.Contains(err.Error(), domain.ErrReadOnly.Error()) {
		t.Errorf("port-forward: err = %v, want a read-only refusal", err)
	}

	if got := seen(); len(got) != 0 {
		t.Errorf("requests that reached the API server = %v, want none", got)
	}
}

// A GET through a proxy subresource reaches whatever listens behind it, so
// it is not known to be a read; client-go's own proxy helpers have to be
// stopped before they leave the machine.
func TestReadOnlyClientProxiesNothing(t *testing.T) {
	cfg, seen := recordingAPIServer(t)
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	proxies := map[string]func() error{
		"pod": func() error {
			_, err := cs.CoreV1().Pods("default").ProxyGet("http", "web", "8080", "admin/reset", nil).DoRaw(ctx)
			return err
		},
		"service": func() error {
			_, err := cs.CoreV1().Services("default").ProxyGet("http", "web", "80", "admin/reset", nil).DoRaw(ctx)
			return err
		},
		"node": func() error {
			return cs.CoreV1().RESTClient().Get().Resource("nodes").Name("node-1").
				SubResource("proxy").Suffix("healthz").Do(ctx).Error()
		},
	}
	for name, get := range proxies {
		if err := get(); !errors.Is(err, domain.ErrReadOnly) {
			t.Errorf("%s proxy: err = %v, want ErrReadOnly", name, err)
		}
	}

	if got := seen(); len(got) != 0 {
		t.Errorf("requests that reached the API server = %v, want none", got)
	}
}
