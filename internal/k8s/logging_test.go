package k8s

import (
	"io"
	"os"
	"strings"
	"testing"

	apiextfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

// TestSilenceLoggingKeepsStderrClean guards a display-corruption bug: in a
// full-screen TUI, anything client-go writes to stderr paints over the
// rendered frame. Reflectors log on every failed list/watch, so a cluster
// with restricted RBAC or a dropped connection used to scribble all over the
// UI. After SilenceLogging, stderr must stay clean even in that worst case.
func TestSilenceLoggingKeepsStderrClean(t *testing.T) {
	SilenceLogging()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = orig })

	failList := func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.NewServiceUnavailable("simulated unreachable API server")
	}
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("list", "*", failList)
	apiext := apiextfake.NewSimpleClientset()
	apiext.PrependReactor("list", "*", failList)

	c := &Client{
		RestConfig: &rest.Config{Host: "https://unreachable"},
		Clientset:  cs,
		Dynamic:    dynamicfake.NewSimpleDynamicClient(scheme.Scheme),
		Metrics:    metricsfake.NewSimpleClientset(),
	}
	s, err := newStoreFrom(c, apiext)
	if err != nil {
		t.Fatalf("newStoreFrom: %v", err)
	}
	s.Close()

	os.Stderr = orig
	w.Close()
	out, _ := io.ReadAll(r)

	for _, noise := range []string{"Failed to watch", "reflector", "UnhandledError"} {
		if strings.Contains(string(out), noise) {
			t.Errorf("client-go logged %q to stderr — this paints over the TUI. Captured:\n%s",
				noise, truncate(string(out), 600))
		}
	}
	t.Logf("stderr bytes captured: %d", len(out))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
