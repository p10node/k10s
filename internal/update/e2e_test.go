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
	"runtime"
	"strings"
	"testing"
)

func TestApplyArtifactRequiredRejectsMissingDist(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestApplyAgainstRealDistArtifacts$", "-test.v")
	cmd.Env = append(os.Environ(), "K10S_UPDATE_E2E_REQUIRED=1", "K10S_UPDATE_E2E_DIST="+filepath.Join(t.TempDir(), "missing"), "K10S_UPDATE_E2E_VERSION=v99.0.0", "K10S_UPDATE_E2E_COMMIT=abcdef0")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("required artifact gate passed without artifacts:\n%s", out)
	}
	if !strings.Contains(string(out), "required artifact") {
		t.Fatalf("unexpected failure: %v\n%s", err, out)
	}
}

func TestApplyArtifactRequiredRejectsMissingPlatform(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestApplyAgainstRealDistArtifacts$", "-test.v")
	cmd.Env = append(os.Environ(), "K10S_UPDATE_E2E_REQUIRED=1", "K10S_UPDATE_E2E_DIST="+t.TempDir(), "K10S_UPDATE_E2E_VERSION=v99.0.0", "K10S_UPDATE_E2E_COMMIT=abcdef0")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("required artifact gate passed without platform archive:\n%s", out)
	}
	if !strings.Contains(string(out), "required artifact") {
		t.Fatalf("unexpected failure: %v\n%s", err, out)
	}
}

func TestApplyArtifactRequiresExactBuildIdentity(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	if err := os.WriteFile(source, []byte(`package main
import "fmt"
func main() { fmt.Println("k10s v99.0.0 (abcdef0) built fixture") }
`), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, BinaryName(runtime.GOOS))
	build := exec.Command("go", "build", "-o", binary, source)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v\n%s", err, out)
	}
	payload, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("k10s_v99.0.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	archive := tarGz(t, map[string][]byte{BinaryName(runtime.GOOS): payload})
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, name), archive, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), []byte(sum256(archive)+"  "+name+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, version, commit string
		ok                    bool
	}{
		{"correct", "v99.0.0", "abcdef0", true},
		{"stale-version", "v98.0.0", "abcdef0", false},
		{"stale-commit", "v99.0.0", "1234567", false},
		{"missing-version", "", "abcdef0", false},
		{"missing-commit", "v99.0.0", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestApplyAgainstRealDistArtifacts$", "-test.v")
			cmd.Env = append(os.Environ(), "K10S_UPDATE_E2E_REQUIRED=1", "K10S_UPDATE_E2E_DIST="+dist, "K10S_UPDATE_E2E_VERSION="+tc.version, "K10S_UPDATE_E2E_COMMIT="+tc.commit)
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.ok {
				t.Fatalf("identity gate err=%v, want success=%v:\n%s", err, tc.ok, out)
			}
		})
	}
}

func TestUpdateReleaseWorkflowRequiresFreshArtifactGate(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)
	build, gate, publish := strings.Index(workflow, "- name: build every platform"), strings.Index(workflow, "- name: verify fresh updater artifact"), strings.Index(workflow, "- name: publish")
	if build < 0 || gate <= build || publish <= gate {
		t.Fatal("release must build, require fresh artifact e2e, then publish")
	}
	step := workflow[gate:publish]
	for _, want := range []string{"K10S_UPDATE_E2E_REQUIRED: \"1\"", "K10S_UPDATE_E2E_VERSION: ${{ github.ref_name }}", "git rev-parse HEAD", "K10S_UPDATE_E2E_COMMIT", "go test -count=1 ./internal/update -run '^TestApplyAgainstRealDistArtifacts$'"} {
		if !strings.Contains(step, want) {
			t.Errorf("artifact gate missing %q", want)
		}
	}
	if strings.Contains(step, "continue-on-error") || strings.Contains(step, "|| true") {
		t.Fatal("artifact gate may not be optional")
	}
	if !strings.Contains(workflow[build:gate], "rm -rf dist") {
		t.Fatal("release may reuse stale dist")
	}
}

// TestApplyAgainstRealDistArtifacts installs and executes a fresh local release.
// Set K10S_UPDATE_E2E_VERSION and K10S_UPDATE_E2E_COMMIT to the build stamps.
// K10S_UPDATE_E2E_REQUIRED=1 turns missing inputs into failures (release CI);
// K10S_UPDATE_E2E_DIST optionally overrides ../../dist for isolated fixtures.
func TestApplyAgainstRealDistArtifacts(t *testing.T) {
	required := os.Getenv("K10S_UPDATE_E2E_REQUIRED") == "1"
	dist := os.Getenv("K10S_UPDATE_E2E_DIST")
	if dist == "" {
		dist = "../../dist"
	}
	unavailable := func(reason string) {
		t.Helper()
		if required {
			t.Fatalf("required artifact: %s", reason)
		}
		t.Skip(reason)
	}
	entries, err := os.ReadDir(dist)
	if err != nil {
		unavailable(fmt.Sprintf("cannot read dist %s: %v — run `just release` first", dist, err))
	}
	expectedVersion := os.Getenv("K10S_UPDATE_E2E_VERSION")
	expectedCommit := os.Getenv("K10S_UPDATE_E2E_COMMIT")
	if expectedVersion == "" || expectedCommit == "" {
		unavailable("set K10S_UPDATE_E2E_VERSION and K10S_UPDATE_E2E_COMMIT to the fresh build identity")
	}
	if len(expectedCommit) > 7 {
		expectedCommit = expectedCommit[:7]
	}
	// Only this machine's own archive can be installed and then executed,
	// so the platform comes from the running test binary rather than from
	// whichever matching name dist/ happens to list last.
	goos, goarch := runtime.GOOS, runtime.GOARCH
	var archive string
	for _, e := range entries {
		if strings.Contains(e.Name(), goos+"_"+goarch) {
			archive = e.Name()
			break
		}
	}
	if archive == "" {
		unavailable(fmt.Sprintf("dist/ has no %s_%s archive to install", goos, goarch))
	}

	files := http.FileServer(http.Dir(dist))
	mux := http.NewServeMux()
	mux.Handle("/dl/", http.StripPrefix("/dl/", files))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/repos/acme/k10s/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"tag_name": expectedVersion,
			"assets": []map[string]any{
				{"name": archive, "browser_download_url": srv.URL + "/dl/" + archive},
				{"name": "checksums.txt", "browser_download_url": srv.URL + "/dl/checksums.txt"},
			},
		}); err != nil {
			t.Errorf("encode release: %v", err)
		}
	})

	target := filepath.Join(t.TempDir(), BinaryName(goos))
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

	// The proof: the installed file runs and reports this build's exact identity.
	out, err := exec.Command(target, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("running the installed binary: %v\n%s", err, out)
	}
	t.Logf("installed: %s", out)
	fields := strings.Fields(string(out))
	if len(fields) < 3 || fields[0] != "k10s" || fields[1] != expectedVersion || fields[2] != "("+expectedCommit+")" {
		t.Fatalf("installed binary identity %q, want k10s %s (%s)", out, expectedVersion, expectedCommit)
	}
}
