package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/p10node/k10s/internal/mock"
)

func TestNewSourceFallsBackWhenAPIServerIsUnreachable(t *testing.T) {
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
	defer src.Close()
	if _, ok := src.(*mock.Source); !ok {
		t.Fatalf("source = %T, want offline demo after failed API-server probe", src)
	}
	if !strings.Contains(warning, "mock mode") || !strings.Contains(warning, "server version") {
		t.Fatalf("warning = %q, want fallback reason", warning)
	}
}
