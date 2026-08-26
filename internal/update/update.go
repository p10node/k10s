// Package update finds newer k10s releases on GitHub and installs one over
// the running binary.
//
// It is deliberately self-contained: net/http plus the standard archive
// packages, no release-manager dependency. Two halves, split so the cheap
// one can run on startup without committing to the expensive one:
//
//	Check  — one GET against the releases API; answers "is there a newer
//	         one, and which file is mine?"
//	Apply  — download that file, verify its checksum, swap it in place of
//	         os.Executable() with a rename, which is atomic.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// DefaultRepo is the "owner/name" releases come from. Override it per user
// with $K10S_UPDATE_REPO, or per install with update.repo in the config file
// — a fork should not have to be recompiled to update itself.
const DefaultRepo = "p10node/k10s"

// RepoEnv names the environment override.
const RepoEnv = "K10S_UPDATE_REPO"

const defaultAPI = "https://api.github.com"

// CheckInterval is how long a successful check is remembered. Updates are
// not urgent, and a TUI that hits the network on every launch is a TUI whose
// update check people turn off.
const CheckInterval = 24 * time.Hour

// Due reports whether the startup check should run again. A zero time — no
// check has ever succeeded — is always due.
func Due(last time.Time) bool {
	return last.IsZero() || time.Since(last) >= CheckInterval
}

// Asset is one file attached to a release.
type Asset struct {
	Name string
	URL  string
	Size int64
}

// Release is a published release, already reduced to what an update needs.
type Release struct {
	Version string // normalised: "v" stripped, so "1.4.0"
	Tag     string // exactly as published, so "v1.4.0"
	Notes   string
	Page    string
	// Asset is the archive (or bare binary) built for this os/arch. Zero
	// when the release published nothing for this platform — Apply reports
	// that as an error rather than installing something else.
	Asset Asset
	// Sums is the checksums manifest, when the release publishes one.
	Sums Asset
	// Assets is everything published, kept so an unmatched platform can say
	// what *was* on offer.
	Assets []Asset
}

// HasAsset reports whether this release ships a build for the running
// platform.
func (r *Release) HasAsset() bool { return r != nil && r.Asset.URL != "" }

// Client reads the GitHub releases API. The zero value is usable: it talks
// to api.github.com about DefaultRepo with a 15-second timeout.
type Client struct {
	Repo string       // "owner/name"; empty falls back to env then DefaultRepo
	API  string       // API root; empty means api.github.com (tests point it elsewhere)
	HTTP *http.Client // empty means a client with Timeout below

	// GOOS/GOARCH override which asset is selected. Empty means the
	// running platform; tests set them to check the matcher.
	GOOS, GOARCH string

	// Target overrides the binary Apply replaces. Empty means the running
	// executable, which is the only value production uses — tests set it so
	// an install can be exercised without overwriting the test binary.
	Target string

	// Timeout applies when HTTP is nil.
	Timeout time.Duration
}

// Repository returns the repo the client will query, applying the
// environment override and the default.
func (c *Client) Repository() string {
	if c != nil && strings.TrimSpace(c.Repo) != "" {
		return strings.Trim(strings.TrimSpace(c.Repo), "/")
	}
	if v := strings.TrimSpace(os.Getenv(RepoEnv)); v != "" {
		return strings.Trim(v, "/")
	}
	return DefaultRepo
}

func (c *Client) api() string {
	if c != nil && c.API != "" {
		return strings.TrimSuffix(c.API, "/")
	}
	return defaultAPI
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	t := 15 * time.Second
	if c != nil && c.Timeout > 0 {
		t = c.Timeout
	}
	return &http.Client{Timeout: t}
}

func (c *Client) platform() (string, string) {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	if c != nil && c.GOOS != "" {
		goos = c.GOOS
	}
	if c != nil && c.GOARCH != "" {
		goarch = c.GOARCH
	}
	return goos, goarch
}

// ghRelease is the slice of the API response an update depends on.
type ghRelease struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	HTMLURL    string `json:"html_url"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// Latest returns the newest published release. Drafts and prereleases are
// excluded by the endpoint itself.
func (c *Client) Latest(ctx context.Context) (*Release, error) {
	repo := c.Repository()
	if !strings.Contains(repo, "/") {
		return nil, fmt.Errorf("update repo %q is not owner/name — set %s or update.repo in the config", repo, RepoEnv)
	}
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.api(), repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "k10s-updater")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// The two failures worth naming: a repo with no releases yet (404, and
	// "not found" alone sends people looking for a typo), and the anonymous
	// rate limit (403/429, which is transient and needs no action).
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%s has no published releases yet", repo)
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("github rate limit reached — try again later")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("github returned %s for %s", resp.Status, repo)
	}

	var gr ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&gr); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}
	if gr.TagName == "" {
		return nil, fmt.Errorf("%s: latest release has no tag", repo)
	}

	rel := &Release{
		Version: Normalize(gr.TagName),
		Tag:     gr.TagName,
		Notes:   strings.TrimSpace(gr.Body),
		Page:    gr.HTMLURL,
	}
	for _, a := range gr.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size})
	}
	goos, goarch := c.platform()
	if a, ok := PickAsset(rel.Assets, goos, goarch); ok {
		rel.Asset = a
	}
	if s, ok := PickChecksums(rel.Assets); ok {
		rel.Sums = s
	}
	return rel, nil
}

// Check fetches the latest release and reports whether it is newer than
// current. A release is returned either way, so a caller can show "you are
// on the latest" without a second request.
func (c *Client) Check(ctx context.Context, current string) (*Release, bool, error) {
	rel, err := c.Latest(ctx)
	if err != nil {
		return nil, false, err
	}
	return rel, Newer(current, rel.Version), nil
}

// AssetHint lists what the release did publish, for the error shown when
// nothing matched this platform.
func (r *Release) AssetHint() string {
	if r == nil || len(r.Assets) == 0 {
		return "the release publishes no assets"
	}
	names := make([]string, 0, len(r.Assets))
	for _, a := range r.Assets {
		names = append(names, a.Name)
	}
	return "published: " + strings.Join(names, ", ")
}
