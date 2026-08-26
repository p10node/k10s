package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyAgainstRealDistArtifacts installs a real `just release` build.
// Skipped unless dist/ has been populated, so it is a one-command way to
// check that the Justfile's archive names and checksums.txt format are the
// ones internal/update actually parses.
func TestApplyAgainstRealDistArtifacts(t *testing.T) {
	dist, err := filepath.Abs("../../dist")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dist)
	if err != nil {
		t.Skip("no dist/ — run `just release` first")
	}
	var archive string
	for _, e := range entries {
		if strings.Contains(e.Name(), "darwin_arm64") || strings.Contains(e.Name(), "linux_amd64") {
			archive = e.Name()
		}
	}
	if archive == "" {
		t.Skip("dist/ has no archive for a platform this test can install")
	}
	goos, goarch := "darwin", "arm64"
	if strings.Contains(archive, "linux") {
		goos, goarch = "linux", "amd64"
	}

	files := http.FileServer(http.Dir(dist))
	mux := http.NewServeMux()
	mux.Handle("/dl/", http.StripPrefix("/dl/", files))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/repos/acme/k10s/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v99.0.0",
			"assets": []map[string]any{
				{"name": archive, "browser_download_url": srv.URL + "/dl/" + archive},
				{"name": "checksums.txt", "browser_download_url": srv.URL + "/dl/checksums.txt"},
			},
		})
	})

	target := filepath.Join(t.TempDir(), "k10s")
	if err := os.WriteFile(target, fakeBinary("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Client{Repo: "acme/k10s", API: srv.URL, HTTP: srv.Client(), GOOS: goos, GOARCH: goarch, Target: target}

	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel.Asset.Name != archive || rel.Sums.Name != "checksums.txt" {
		t.Fatalf("asset=%q sums=%q — the release naming and the matcher disagree", rel.Asset.Name, rel.Sums.Name)
	}
	if _, err := Apply(context.Background(), c, rel, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// The proof: the installed file runs and reports the stamped version.
	if goos != "darwin" && goos != "linux" {
		return
	}
	out, err := exec.Command(target, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("running the installed binary: %v\n%s", err, out)
	}
	fmt.Printf("installed: %s", out)
	if !strings.HasPrefix(string(out), "k10s v") {
		t.Errorf("installed binary reported %q", out)
	}
}
