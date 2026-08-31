package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
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
