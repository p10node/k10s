package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/p10node/k10s/internal/k8s"
	"github.com/p10node/k10s/internal/update"
)

// An API server that answers with an error is not a cluster, and k10s says
// so rather than quietly swapping in sample data: newSource returns no
// source at all plus the reason, and the UI draws the "No cluster" panel.
// Fake rows are reachable only through the demo context, which is labelled
// as the demo it is.
func TestNewSourceReportsNoClusterWhenAPIServerIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unreachable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "kubeconfig")
	cfg := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"unreachable": {Server: server.URL},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"repro": {},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"unreachable": {Cluster: "unreachable", AuthInfo: "repro"},
		},
		CurrentContext: "unreachable",
	}
	if err := clientcmd.WriteToFile(cfg, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", path)

	src, warning := newSource("")
	if src != nil {
		src.Close()
		t.Fatalf("source = %T, want none after a failed API-server probe", src)
	}
	if warning == "" {
		t.Fatal("warning is empty, want the reason the cluster did not answer")
	}
	if strings.Contains(warning, "mock") || strings.Contains(warning, "demo") {
		t.Fatalf("warning = %q, want a failure reason, not a fallback to sample data", warning)
	}
}

func TestReadOnlyFlagIsTakenOutWherever(t *testing.T) {
	cases := []struct {
		in   []string
		rest []string
		ro   bool
	}{
		{nil, []string{}, false},
		{[]string{"demo"}, []string{"demo"}, false},
		{[]string{"--readonly"}, []string{}, true},
		{[]string{"--readonly", "demo"}, []string{"demo"}, true},
		{[]string{"demo", "--read-only"}, []string{"demo"}, true},
		{[]string{"--version"}, []string{"--version"}, false},
	}
	for _, c := range cases {
		rest, ro := readOnlyFlag(c.in)
		if ro != c.ro || strings.Join(rest, " ") != strings.Join(c.rest, " ") {
			t.Errorf("readOnlyFlag(%q) = %q, %v; want %q, %v", c.in, rest, ro, c.rest, c.ro)
		}
	}
}

// `k10s update` on a Homebrew install says how Homebrew upgrades it, and
// does nothing else: no release check, no download, nothing written.
func TestSelfUpdateStepsAsideForAPackageManager(t *testing.T) {
	t.Setenv("K10S_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	// Were the guard missing, this would stop the release check before it
	// reached GitHub, with a different error, and still write nothing.
	t.Setenv(update.RepoEnv, "not-a-repo")

	err := selfUpdate(&update.Owner{Name: "Homebrew", Upgrade: "brew upgrade k10s"})
	var managed *update.ManagedError
	if !errors.As(err, &managed) {
		t.Fatalf("selfUpdate error = %v, want a *update.ManagedError", err)
	}
	if want := "installed by Homebrew, which updates it: run brew upgrade k10s"; err.Error() != want {
		t.Errorf("selfUpdate error = %q, want %q", err, want)
	}
}

// A credential plugin's own words survive the trim: they are the actionable
// part, and nothing in them is advice aimed at another tool.
func TestNoClusterReasonKeepsACredentialPluginsWords(t *testing.T) {
	err := &k8s.CredentialPluginError{
		Plugin: "tsh",
		Said:   "Having problems with relogin, please use 'tsh login/tsh kube login' manually",
		Err:    errors.New("getting credentials: exec: executable tsh failed with exit code 1"),
	}
	want := "credential plugin tsh: Having problems with relogin, please use 'tsh login/tsh kube login' manually"
	if got := noClusterReason(err); got != want {
		t.Errorf("noClusterReason = %q\n              want %q", got, want)
	}
}
