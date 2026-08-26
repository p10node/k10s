package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// releaseJSON renders what the GitHub API returns, so the test exercises the
// real decoder rather than a hand-built Release.
func releaseJSON(tag string, assetNames ...string) string {
	type asset struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	}
	body := map[string]any{
		"tag_name": tag,
		"name":     "k10s " + tag,
		"body":     "- faster startup\n- fixed the thing",
		"html_url": "https://github.test/releases/" + tag,
	}
	var as []asset
	for _, n := range assetNames {
		as = append(as, asset{Name: n, URL: "https://dl.test/" + n, Size: 42})
	}
	body["assets"] = as
	b, _ := json.Marshal(body)
	return string(b)
}

func apiServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "/repos/acme/k10s/releases/latest"; r.URL.Path != want {
			t.Errorf("requested %s, want %s", r.URL.Path, want)
		}
		if got := r.Header.Get("Accept"); !strings.Contains(got, "github") {
			t.Errorf("Accept = %q, want the GitHub media type", got)
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testClient(t *testing.T, srv *httptest.Server) *Client {
	return &Client{Repo: "acme/k10s", API: srv.URL, HTTP: srv.Client(), GOOS: "linux", GOARCH: "amd64"}
}

func TestCheckFindsANewerRelease(t *testing.T) {
	srv := apiServer(t, http.StatusOK, releaseJSON("v1.4.0",
		"checksums.txt", "k10s_1.4.0_linux_amd64.tar.gz", "k10s_1.4.0_darwin_arm64.tar.gz"))

	rel, newer, err := testClient(t, srv).Check(context.Background(), "1.3.9")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !newer {
		t.Error("newer = false, want true for 1.4.0 over 1.3.9")
	}
	if rel.Version != "1.4.0" || rel.Tag != "v1.4.0" {
		t.Errorf("Version/Tag = %q/%q, want 1.4.0/v1.4.0", rel.Version, rel.Tag)
	}
	if rel.Asset.Name != "k10s_1.4.0_linux_amd64.tar.gz" {
		t.Errorf("Asset = %q, want the linux/amd64 archive", rel.Asset.Name)
	}
	if rel.Sums.Name != "checksums.txt" {
		t.Errorf("Sums = %q, want checksums.txt", rel.Sums.Name)
	}
	if !strings.Contains(rel.Notes, "faster startup") {
		t.Errorf("Notes = %q, want the release body", rel.Notes)
	}
}

func TestCheckReportsNoUpdateWhenCurrent(t *testing.T) {
	srv := apiServer(t, http.StatusOK, releaseJSON("v1.4.0", "k10s_1.4.0_linux_amd64.tar.gz"))

	// The release still comes back, so the UI can say which version that is
	// without a second request.
	rel, newer, err := testClient(t, srv).Check(context.Background(), "v1.4.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if newer {
		t.Error("newer = true for the version we are already running")
	}
	if rel == nil {
		t.Fatal("Check returned no release")
	}
}

func TestCheckExplainsAnEmptyRepo(t *testing.T) {
	srv := apiServer(t, http.StatusNotFound, `{"message":"Not Found"}`)

	_, _, err := testClient(t, srv).Check(context.Background(), "1.0.0")
	if err == nil {
		t.Fatal("Check of a repo with no releases: want an error")
	}
	// "404 Not Found" alone sends people hunting for a typo in the repo name.
	if !strings.Contains(err.Error(), "no published releases") {
		t.Errorf("error = %q, want it to say the repo has no releases", err)
	}
}

func TestCheckNamesTheRateLimit(t *testing.T) {
	srv := apiServer(t, http.StatusForbidden, `{"message":"API rate limit exceeded"}`)

	_, _, err := testClient(t, srv).Check(context.Background(), "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error = %v, want it to name the rate limit", err)
	}
}

func TestLatestFlagsAnUnbuiltPlatform(t *testing.T) {
	srv := apiServer(t, http.StatusOK, releaseJSON("v1.4.0", "k10s_1.4.0_linux_amd64.tar.gz"))
	c := testClient(t, srv)
	c.GOOS, c.GOARCH = "windows", "arm64"

	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.HasAsset() {
		t.Errorf("HasAsset = true, want false for windows/arm64")
	}
	if !strings.Contains(rel.AssetHint(), "k10s_1.4.0_linux_amd64.tar.gz") {
		t.Errorf("AssetHint = %q, want it to list what was published", rel.AssetHint())
	}
}

func TestRepositoryResolutionOrder(t *testing.T) {
	t.Setenv(RepoEnv, "")
	if got := (&Client{}).Repository(); got != DefaultRepo {
		t.Errorf("Repository() = %q, want the default %q", got, DefaultRepo)
	}

	t.Setenv(RepoEnv, "forked/k10s")
	if got := (&Client{}).Repository(); got != "forked/k10s" {
		t.Errorf("Repository() = %q, want the environment override", got)
	}
	// An explicit field beats the environment: the config file is a
	// deliberate choice, the environment is ambient.
	if got := (&Client{Repo: "explicit/k10s"}).Repository(); got != "explicit/k10s" {
		t.Errorf("Repository() = %q, want the explicit repo", got)
	}
}

func TestLatestRejectsAMalformedRepo(t *testing.T) {
	_, err := (&Client{Repo: "not-owner-slash-name"}).Latest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "owner/name") {
		t.Errorf("error = %v, want it to explain the owner/name form", err)
	}
}

func TestDueThrottlesTheStartupCheck(t *testing.T) {
	if !Due(time.Time{}) {
		t.Error("Due(zero) = false; a first run must check")
	}
	if Due(time.Now().Add(-time.Hour)) {
		t.Error("Due(an hour ago) = true; the check should be throttled")
	}
	if !Due(time.Now().Add(-CheckInterval - time.Minute)) {
		t.Error("Due(older than the interval) = false")
	}
}
