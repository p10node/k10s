package k8s

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadedKubeconfigPathPreservesMergedKubeconfigList(t *testing.T) {
	t.Setenv("KUBECONFIG", "/tmp/one:/tmp/two")
	if got := loadedKubeconfigPath(""); got != "/tmp/one:/tmp/two" {
		t.Errorf("loadedKubeconfigPath = %q, want full KUBECONFIG list", got)
	}
	if got := loadedKubeconfigPath("/tmp/explicit"); got != "/tmp/explicit" {
		t.Errorf("explicit path = %q", got)
	}
}

// writeKubeconfig writes a kubeconfig file holding a single context, name,
// whose cluster is server, and returns the file's path.
func writeKubeconfig(t *testing.T, dir, name, server string) string {
	t.Helper()
	path := filepath.Join(dir, name+".yaml")
	body := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: %[1]s
  cluster:
    server: %[2]s
users:
- name: %[1]s
  user:
    token: %[1]s-token
contexts:
- name: %[1]s
  context:
    cluster: %[1]s
    user: %[1]s
current-context: %[1]s
`, name, server)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// One kubeconfig file per cluster, merged through $KUBECONFIG, is a common
// layout, and kubectl reads every file in the list. Switching context has to
// load them the same way the first connection did, not as a single file
// whose name happens to contain the list separator.
func TestSwitchContextWithKubeconfigList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/version" {
			fmt.Fprint(w, `{"major":"1","minor":"33","gitVersion":"v1.33.0"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	list := writeKubeconfig(t, dir, "alpha", srv.URL) + string(os.PathListSeparator) +
		writeKubeconfig(t, dir, "beta", srv.URL)
	t.Setenv("KUBECONFIG", list)

	s, err := NewStore("", "")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(s.Close)
	if got := s.ClusterInfo().Context; got != "alpha" {
		t.Fatalf("connected to %q, want alpha, the first file's current-context", got)
	}

	next, err := s.SwitchContext("beta")
	if err != nil {
		t.Fatalf("SwitchContext(beta): %v", err)
	}
	t.Cleanup(next.Close)
	info := next.ClusterInfo()
	if info.Context != "beta" || info.Server != srv.URL {
		t.Errorf("switched to %q on %q, want beta on %s", info.Context, info.Server, srv.URL)
	}
	// Plugins still get the whole list: that is what kubectl expects in
	// $KUBECONFIG.
	if info.Kubeconfig != list {
		t.Errorf("Kubeconfig = %q, want the $KUBECONFIG list %q", info.Kubeconfig, list)
	}
}
