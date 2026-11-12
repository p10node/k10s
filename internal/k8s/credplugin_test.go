package k8s

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// execCredential is what a plugin prints to hand client-go a bearer token.
const execCredential = `{"apiVersion":"client.authentication.k8s.io/v1beta1","kind":"ExecCredential","status":{"token":"plugin-token"}}`

// writeExecPlugin writes a shell script that stands in for an exec
// credential plugin and returns its path. name is the base name — "tsh" —
// because that is what k10s calls the plugin in its errors.
func writeExecPlugin(t *testing.T, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake plugin is a shell script")
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeExecKubeconfig writes a kubeconfig whose single context signs in
// through the exec plugin at command, and points $KUBECONFIG at it.
func writeExecKubeconfig(t *testing.T, name, server, ca, command, interactiveMode string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "exec.yaml")
	body := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: %[1]s
  cluster:
    server: %[2]s
    certificate-authority-data: %[3]s
users:
- name: %[1]s
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: %[4]s
      args: [kube, credentials]
      interactiveMode: %[5]s
contexts:
- name: %[1]s
  context:
    cluster: %[1]s
    user: %[1]s
current-context: %[1]s
`, name, server, ca, command, interactiveMode)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", path)
}

// captureStderrForTest does what main's k8s.CaptureStderr does, scoped to
// one test: the plugin's stderr goes through a tap, and the terminal comes
// back afterwards.
func captureStderrForTest(t *testing.T) {
	t.Helper()
	prev := tap
	tt := &stderrTap{}
	if err := tt.start(); err != nil {
		t.Fatal(err)
	}
	tap = tt
	t.Cleanup(func() {
		os.Stderr = tt.terminal
		_ = tt.pipe.Close()
		tap = prev
	})
}

// The plugin is run, its token is used, and it is run without the terminal:
// the TUI owns stdin, so client-go is told it is unavailable, as kubectl
// does when stdin is spoken for.
func TestExecPluginRunsWithoutTheTerminal(t *testing.T) {
	srv, ca, tokens := tlsAPIServer(t)
	plugin := writeExecPlugin(t, "tsh", "printf '%s' '"+execCredential+"'")
	writeExecKubeconfig(t, "tp", srv.URL, ca, plugin, "IfAvailable")

	c, err := New("", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Reachable(); err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	if got := tokens(); len(got) == 0 || got[len(got)-1] != "plugin-token" {
		t.Errorf("bearer tokens the API server saw = %q, want the plugin's", got)
	}
	ep := c.RestConfig.ExecProvider
	if ep == nil || !ep.StdinUnavailable || ep.StdinUnavailableMessage == "" {
		t.Errorf("ExecProvider = %+v, want stdin marked unavailable with a reason", ep)
	}
	if p := CredentialPlugin("", ""); p != "tsh" {
		t.Errorf("CredentialPlugin = %q, want tsh", p)
	}
}

// A plugin that declares it must have a terminal (interactiveMode: Always)
// is refused before it runs, and the refusal is reported as the plugin's
// failure rather than the cluster's.
func TestExecPluginThatInsistsOnATerminalIsRefusedNotRun(t *testing.T) {
	srv, ca, _ := tlsAPIServer(t)
	mark := filepath.Join(t.TempDir(), "ran")
	plugin := writeExecPlugin(t, "tsh", "touch '"+mark+"'; printf '%s' '"+execCredential+"'")
	writeExecKubeconfig(t, "tp", srv.URL, ca, plugin, "Always")

	c, err := New("", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = c.Reachable()
	var pe *CredentialPluginError
	if !errors.As(err, &pe) {
		t.Fatalf("Reachable = %v, want a CredentialPluginError", err)
	}
	if pe.Plugin != "tsh" || !strings.Contains(err.Error(), "interactive") {
		t.Errorf("error = %q, want it to name tsh and the interactive-mode refusal", err)
	}
	if _, statErr := os.Stat(mark); statErr == nil {
		t.Error("the plugin ran; it should have been refused before that")
	}
}

// What the plugin wrote to stderr is the error the user reads, not the exit
// code: "Having problems with relogin…" is actionable, "exit code 1" is not.
func TestExecPluginsLastWordsReachTheUser(t *testing.T) {
	captureStderrForTest(t)
	srv, ca, _ := tlsAPIServer(t)
	const said = "Having problems with relogin, please use 'tsh login/tsh kube login' manually"
	plugin := writeExecPlugin(t, "tsh", `echo "ERROR: `+said+`" >&2; exit 1`)
	writeExecKubeconfig(t, "tp", srv.URL, ca, plugin, "IfAvailable")

	c, err := New("", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = c.Reachable()
	if err == nil {
		t.Fatal("Reachable = nil, want the plugin's failure")
	}
	if want := "credential plugin tsh: " + said; err.Error() != want {
		t.Errorf("error = %q\n          want %q", err, want)
	}
	// Still client-go's error underneath, for anything that asks.
	if !strings.Contains(errors.Unwrap(err).Error(), "exit code 1") {
		t.Errorf("Unwrap = %v, want client-go's exec error", errors.Unwrap(err))
	}
}

// A silent plugin still gets client-go's clause, cut down to the part about
// the plugin.
func TestCredentialPluginErrorWithoutLastWords(t *testing.T) {
	err := &CredentialPluginError{
		Plugin: "aws",
		Err:    errors.New(`Get "https://x/version": getting credentials: exec: executable aws failed with exit code 255`),
	}
	if want := "credential plugin aws: exec: executable aws failed with exit code 255"; err.Error() != want {
		t.Errorf("Error = %q, want %q", err, want)
	}
}

// The plugin's running time is spent inside the probe's timeout, and
// client-go cannot interrupt it. A slow plugin therefore used to turn into
// "No cluster: context deadline exceeded" even though it succeeded; the
// retry, with its credentials now cached, is the network alone.
func TestSlowExecPluginDoesNotCountAgainstTheServer(t *testing.T) {
	prev := discoveryTimeout
	discoveryTimeout = 300 * time.Millisecond
	t.Cleanup(func() { discoveryTimeout = prev })

	srv, ca, tokens := tlsAPIServer(t)
	plugin := writeExecPlugin(t, "tsh", "sleep 1; printf '%s' '"+execCredential+"'")
	writeExecKubeconfig(t, "tp", srv.URL, ca, plugin, "IfAvailable")

	c, err := New("", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Reachable(); err != nil {
		t.Fatalf("Reachable = %v, want the retry to have reached the server", err)
	}
	if c.Version == "" {
		t.Error("Version empty after a successful probe")
	}
	if got := tokens(); len(got) == 0 || got[len(got)-1] != "plugin-token" {
		t.Errorf("bearer tokens the API server saw = %q, want the plugin's", got)
	}
}

// Without a plugin in the way a timeout is what it says: the server did not
// answer, and asking again would only double the wait.
func TestProbeTimeoutWithoutAPluginIsNotRetried(t *testing.T) {
	prev := discoveryTimeout
	discoveryTimeout = 200 * time.Millisecond
	t.Cleanup(func() { discoveryTimeout = prev })

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(600 * time.Millisecond)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("KUBECONFIG", writeKubeconfig(t, t.TempDir(), "slow", srv.URL))

	c, err := New("", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = c.Reachable()
	if err == nil || !requestTimedOut(err) {
		t.Fatalf("Reachable = %v, want a timeout", err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("server hit %d times, want 1: no plugin, so no retry", n)
	}
}

func TestCredentialPluginNamesTheContextsPlugin(t *testing.T) {
	raw := clientcmdapi.Config{
		CurrentContext: "tp",
		Contexts: map[string]*clientcmdapi.Context{
			"tp":   {AuthInfo: "tp"},
			"kind": {AuthInfo: "kind"},
			"lost": {AuthInfo: "nobody"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"tp":   {Exec: &clientcmdapi.ExecConfig{Command: "/usr/local/bin/tsh"}},
			"kind": {Token: "t"},
		},
	}
	for ctx, want := range map[string]string{"": "tsh", "tp": "tsh", "kind": "", "lost": "", "absent": ""} {
		if got := credentialPlugin(raw, ctx); got != want {
			t.Errorf("credentialPlugin(%q) = %q, want %q", ctx, got, want)
		}
	}
}
