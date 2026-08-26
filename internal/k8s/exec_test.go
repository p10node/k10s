package k8s

import (
	"net/url"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// A TTY and a separate stderr stream are mutually exclusive: ask for both and
// the SPDY transport upgrades the connection and then delivers nothing, so the
// shell panel opens blank and swallows every keystroke. Guarding the options
// builder is cheaper than rediscovering that against a live cluster.
func TestTTYExecNeverRequestsStderr(t *testing.T) {
	opts := ttyExecOptions("app", shellCmd)
	if !opts.TTY {
		t.Fatal("an interactive exec must ask for a TTY")
	}
	if opts.Stderr {
		t.Error("stderr must not be requested alongside a TTY — the SPDY stream goes silent")
	}
	if !opts.Stdin || !opts.Stdout {
		t.Error("an interactive exec needs both stdin and stdout")
	}
}

// The same invariant, checked where it actually matters: the query string the
// API server sees.
func TestExecURLHasNoStderrParam(t *testing.T) {
	c, err := rest.RESTClientFor(&rest.Config{
		Host: "https://example.invalid",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &scheme.Scheme.PrioritizedVersionsAllGroups()[0],
			NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		},
	})
	if err != nil {
		t.Fatalf("rest client: %v", err)
	}
	req := c.Post().Resource("pods").Namespace("default").Name("web-1").SubResource("exec")
	req.VersionedParams(ttyExecOptions("app", shellCmd), scheme.ParameterCodec)

	q := req.URL().Query()
	if q.Get("tty") != "true" {
		t.Errorf("tty = %q, want true", q.Get("tty"))
	}
	if got := q.Get("stderr"); got == "true" {
		t.Errorf("stderr = %q; requesting it with a TTY silences the SPDY stream", got)
	}
	if q.Get("stdin") != "true" || q.Get("stdout") != "true" {
		t.Errorf("stdin/stdout missing from %s", req.URL().RawQuery)
	}
}

// newExecutor must hand back a working executor, not nil, for a plain config —
// the fallback wrapper is easy to break by returning early on the first error.
func TestNewExecutorBuildsFallback(t *testing.T) {
	u, _ := url.Parse("https://example.invalid/api/v1/namespaces/default/pods/web-1/exec")
	ex, err := newExecutor(&rest.Config{Host: "https://example.invalid"}, u)
	if err != nil {
		t.Fatalf("newExecutor: %v", err)
	}
	if ex == nil {
		t.Fatal("newExecutor returned no executor")
	}
}
