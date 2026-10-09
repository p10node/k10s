package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/p10node/k10s/internal/domain"
)

// fakeIDToken is an unsigned JWT. The oidc auth-provider reads only its exp
// claim, to decide whether to refresh; checking the signature is the API
// server's job.
func fakeIDToken(t *testing.T, sub string, exp time.Time) string {
	t.Helper()
	part := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return part(map[string]string{"alg": "RS256", "typ": "JWT"}) + "." +
		part(map[string]any{"sub": sub, "exp": exp.Unix()}) + ".c2ln"
}

// tlsAPIServer is an https API server that answers /version, 404s the rest
// and records the bearer token each request carried. client-go only applies
// auth-provider credentials over TLS. It returns the server, its CA as
// kubeconfig certificate-authority-data, and the tokens seen so far.
func tlsAPIServer(t *testing.T) (*httptest.Server, string, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var tokens []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		tokens = append(tokens, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/version" {
			fmt.Fprint(w, `{"major":"1","minor":"33","gitVersion":"v1.33.0"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
	}))
	t.Cleanup(srv.Close)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	return srv, base64.StdEncoding.EncodeToString(ca), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), tokens...)
	}
}

// writeAuthProviderKubeconfig writes a kubeconfig file holding a single
// context, name, whose user logs in through the given auth-provider.
func writeAuthProviderKubeconfig(t *testing.T, path, name, server, ca, provider string, cfg map[string]string) {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "apiVersion: v1\nkind: Config\nclusters:\n- name: %[1]s\n  cluster:\n    server: %[2]s\n    certificate-authority-data: %[3]s\n", name, server, ca)
	fmt.Fprintf(&b, "users:\n- name: %s\n  user:\n    auth-provider:\n      name: %s\n      config:\n", name, provider)
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "        %s: %q\n", k, cfg[k])
	}
	fmt.Fprintf(&b, "contexts:\n- name: %[1]s\n  context:\n    cluster: %[1]s\n    user: %[1]s\ncurrent-context: %[1]s\n", name)
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A kubeconfig that logs in through `auth-provider: oidc`, which kubectl
// still reads, failed before sending anything with `clientset: no Auth
// Provider found for name "oidc"`: client-go only knows the providers a
// program imports.
func TestOIDCAuthProviderKubeconfigConnects(t *testing.T) {
	srv, ca, tokens := tlsAPIServer(t)
	idToken := fakeIDToken(t, "alice", time.Now().Add(time.Hour))
	path := filepath.Join(t.TempDir(), "oidc.yaml")
	writeAuthProviderKubeconfig(t, path, "oidc", srv.URL, ca, "oidc", map[string]string{
		"client-id": "k10s", "idp-issuer-url": "https://idp.invalid", "id-token": idToken,
	})
	t.Setenv("KUBECONFIG", path)

	c, err := New("", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Reachable(); err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	if got := tokens(); len(got) == 0 || got[len(got)-1] != idToken {
		t.Errorf("bearer tokens the API server saw = %q, want the kubeconfig's id-token", got)
	}
}

// An expired id-token is refreshed against the identity provider and the new
// one is written back to the kubeconfig file that holds the user, as kubectl
// does. In read-only mode too: the refresh is a POST, but it goes to the
// identity provider through the plugin's own client, not the guarded one.
func TestOIDCAuthProviderRefreshesExpiredToken(t *testing.T) {
	for _, ro := range []bool{false, true} {
		t.Run(fmt.Sprintf("readonly=%v", ro), func(t *testing.T) {
			SetReadOnly(ro)
			t.Cleanup(func() { SetReadOnly(false) })

			fresh := fakeIDToken(t, "alice", time.Now().Add(time.Hour))
			var mu sync.Mutex
			var refreshes []string
			var idp *httptest.Server
			idp = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/.well-known/openid-configuration":
					fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":%q}`, idp.URL, idp.URL+"/token")
				case "/token":
					_ = r.ParseForm()
					mu.Lock()
					refreshes = append(refreshes, r.Method+" "+r.PostForm.Get("grant_type")+" "+r.PostForm.Get("refresh_token"))
					mu.Unlock()
					fmt.Fprintf(w, `{"access_token":"unused","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-2","id_token":%q}`, fresh)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(idp.Close)

			srv, ca, tokens := tlsAPIServer(t)
			// One file per cluster, merged through $KUBECONFIG; the oidc user
			// lives in the second file.
			dir := t.TempDir()
			other := writeKubeconfig(t, dir, "other", "https://other.invalid")
			oidcPath := filepath.Join(dir, "oidc.yaml")
			writeAuthProviderKubeconfig(t, oidcPath, "oidc", srv.URL, ca, "oidc", map[string]string{
				"client-id": "k10s", "idp-issuer-url": idp.URL, "refresh-token": "rt-1",
				"id-token": fakeIDToken(t, "alice", time.Now().Add(-time.Hour)),
			})
			otherBefore, err := os.ReadFile(other)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("KUBECONFIG", other+string(os.PathListSeparator)+oidcPath)

			c, err := New("", "oidc")
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if err := c.Reachable(); err != nil {
				t.Fatalf("Reachable: %v", err)
			}
			_, err = c.Clientset.CoreV1().Pods("default").Get(context.Background(), "web", metav1.GetOptions{})
			if err == nil || errors.Is(err, domain.ErrReadOnly) {
				t.Errorf("get pod: err = %v, want the API server's 404", err)
			}

			mu.Lock()
			got := append([]string(nil), refreshes...)
			mu.Unlock()
			if len(got) != 1 || got[0] != "POST refresh_token rt-1" {
				t.Errorf("token requests to the identity provider = %q, want one refresh with rt-1", got)
			}
			for _, tok := range tokens() {
				if tok != fresh {
					t.Errorf("the API server saw bearer %q, want only the refreshed id-token", tok)
				}
			}
			saved, err := os.ReadFile(oidcPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(saved), fresh) || !strings.Contains(string(saved), "rt-2") {
				t.Errorf("the refreshed tokens were not written back to %s:\n%s", oidcPath, saved)
			}
			if otherAfter, _ := os.ReadFile(other); string(otherAfter) != string(otherBefore) {
				t.Errorf("%s changed, but the oidc user lives in the other file", other)
			}
		})
	}
}

// The gcp and azure providers were removed from client-go. With their stubs
// registered, a kubeconfig that still names one gets client-go's own pointer
// to what replaced it instead of "no Auth Provider found".
func TestRemovedAuthProviderIsStillRecognised(t *testing.T) {
	srv, ca, _ := tlsAPIServer(t)
	path := filepath.Join(t.TempDir(), "gcp.yaml")
	writeAuthProviderKubeconfig(t, path, "gke", srv.URL, ca, "gcp", map[string]string{"access-token": "x"})
	t.Setenv("KUBECONFIG", path)

	_, err := New("", "")
	if err == nil || strings.Contains(err.Error(), "no Auth Provider found") {
		t.Errorf("New with a gcp auth-provider: err = %v, want client-go's removal notice", err)
	}
}
