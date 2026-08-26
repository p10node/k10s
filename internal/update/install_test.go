package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeBinary is what a release archive contains: something that passes the
// executable sniff (ELF magic) and is over the minimum size, without being
// an actual program.
func fakeBinary(marker string) []byte {
	b := append([]byte{0x7f, 'E', 'L', 'F'}, []byte(marker)...)
	return append(b, bytes.Repeat([]byte{0x90}, 2048)...)
}

func tarGz(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	// Sorted so the archive bytes — and therefore the checksum — are stable.
	for _, name := range sortedKeys(entries) {
		body := entries[name]
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipOf(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range sortedKeys(entries) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entries[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sortedKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func sum256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// release wires up a fake GitHub: the API endpoint, the asset download, and
// an optional checksums manifest, all on one test server.
type release struct {
	tag       string
	assetName string
	assetBody []byte
	sums      string // "" means the release publishes no manifest
	srv       *httptest.Server
}

func serveRelease(t *testing.T, r *release) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/k10s/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		assets := []map[string]any{
			{"name": r.assetName, "browser_download_url": r.srv.URL + "/dl/" + r.assetName, "size": len(r.assetBody)},
		}
		if r.sums != "" {
			assets = append(assets, map[string]any{"name": "checksums.txt", "browser_download_url": r.srv.URL + "/dl/checksums.txt"})
		}
		json.NewEncoder(w).Encode(map[string]any{"tag_name": r.tag, "assets": assets})
	})
	mux.HandleFunc("/dl/"+r.assetName, func(w http.ResponseWriter, _ *http.Request) {
		w.Write(r.assetBody)
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, r.sums)
	})
	r.srv = httptest.NewUnstartedServer(mux)
	r.srv.Start()
	t.Cleanup(r.srv.Close)
	return &Client{Repo: "acme/k10s", API: r.srv.URL, HTTP: r.srv.Client()}
}

// installTarget writes a stand-in for the running binary and points the
// client at it, so a real install can be exercised without the test binary
// replacing itself.
func installTarget(t *testing.T, c *Client) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "k10s")
	if err := os.WriteFile(path, fakeBinary("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	c.Target = path
	return path
}

func TestApplyInstallsTheNewBinaryOverTheOldOne(t *testing.T) {
	body := tarGz(t, map[string][]byte{"k10s": fakeBinary("NEW")})
	r := &release{tag: "v2.0.0", assetName: "k10s_2.0.0_linux_amd64.tar.gz", assetBody: body}
	c := serveRelease(t, r)
	c.GOOS, c.GOARCH = "linux", "amd64"
	r.sums = sum256(body) + "  " + r.assetName + "\n"
	target := installTarget(t, c)

	rel, newer, err := c.Check(context.Background(), "1.0.0")
	if err != nil || !newer {
		t.Fatalf("Check: rel=%v newer=%v err=%v", rel, newer, err)
	}

	var lastDone, lastTotal int64
	res, err := Apply(context.Background(), c, rel, func(done, total int64) {
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Version != "2.0.0" || res.Path != target {
		t.Errorf("Result = %+v, want 2.0.0 at %s", res, target)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("NEW")) {
		t.Error("the target still holds the old binary")
	}
	if fi, err := os.Stat(target); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o, want 755 — an installed binary has to stay runnable", fi.Mode().Perm())
	}
	if lastDone != int64(len(body)) || lastTotal != int64(len(body)) {
		t.Errorf("progress ended at %d/%d, want %d/%d", lastDone, lastTotal, len(body), len(body))
	}

	// Nothing may be left in the install directory but the binary itself.
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("install dir holds %v, want just k10s — temp files leaked", names)
	}
}

func TestApplyReadsAZipRelease(t *testing.T) {
	body := zipOf(t, map[string][]byte{"k10s.exe": fakeBinary("WINNEW")})
	r := &release{tag: "v2.0.0", assetName: "k10s_2.0.0_windows_amd64.zip", assetBody: body}
	c := serveRelease(t, r)
	c.GOOS, c.GOARCH = "windows", "amd64"
	target := installTarget(t, c)

	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(context.Background(), c, rel, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.ReadFile(target)
	if !bytes.Contains(got, []byte("WINNEW")) {
		t.Error("the zip's binary was not installed")
	}
}

func TestApplyAcceptsABareBinaryAsset(t *testing.T) {
	body := fakeBinary("BARE")
	r := &release{tag: "v2.0.0", assetName: "k10s-linux-amd64", assetBody: body}
	c := serveRelease(t, r)
	c.GOOS, c.GOARCH = "linux", "amd64"
	target := installTarget(t, c)

	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(context.Background(), c, rel, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.ReadFile(target)
	if !bytes.Contains(got, []byte("BARE")) {
		t.Error("the bare binary was not installed")
	}
}

func TestApplyRefusesAChecksumMismatch(t *testing.T) {
	body := tarGz(t, map[string][]byte{"k10s": fakeBinary("TAMPERED")})
	r := &release{tag: "v2.0.0", assetName: "k10s_2.0.0_linux_amd64.tar.gz", assetBody: body,
		sums: strings.Repeat("0", 64) + "  k10s_2.0.0_linux_amd64.tar.gz\n"}
	c := serveRelease(t, r)
	c.GOOS, c.GOARCH = "linux", "amd64"
	target := installTarget(t, c)

	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(context.Background(), c, rel, nil); err == nil {
		t.Fatal("Apply of a tampered asset succeeded")
	} else if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %v, want a checksum mismatch", err)
	}
	// A failed install must leave the working binary exactly as it was.
	got, _ := os.ReadFile(target)
	if !bytes.Contains(got, []byte("OLD")) {
		t.Error("a failed install damaged the existing binary")
	}
}

func TestApplyRefusesAnAssetMissingFromTheManifest(t *testing.T) {
	body := tarGz(t, map[string][]byte{"k10s": fakeBinary("NEW")})
	r := &release{tag: "v2.0.0", assetName: "k10s_2.0.0_linux_amd64.tar.gz", assetBody: body,
		sums: sum256(body) + "  some-other-file.tar.gz\n"}
	c := serveRelease(t, r)
	c.GOOS, c.GOARCH = "linux", "amd64"
	installTarget(t, c)

	rel, _ := c.Latest(context.Background())
	_, err := Apply(context.Background(), c, rel, nil)
	if err == nil || !strings.Contains(err.Error(), "does not list") {
		t.Errorf("error = %v, want a refusal to install unverified", err)
	}
}

func TestApplyRejectsSomethingThatIsNotABinary(t *testing.T) {
	// The realistic failure: a CDN or proxy serves an HTML error page with a
	// 200. Renaming that over a working k10s would break the install.
	body := []byte("<html><body>504 Gateway Timeout</body></html>")
	r := &release{tag: "v2.0.0", assetName: "k10s-linux-amd64", assetBody: body}
	c := serveRelease(t, r)
	c.GOOS, c.GOARCH = "linux", "amd64"
	target := installTarget(t, c)

	rel, _ := c.Latest(context.Background())
	if _, err := Apply(context.Background(), c, rel, nil); err == nil {
		t.Fatal("Apply installed an HTML page as the binary")
	}
	got, _ := os.ReadFile(target)
	if !bytes.Contains(got, []byte("OLD")) {
		t.Error("the existing binary was replaced anyway")
	}
}

func TestApplyRefusesWhenNothingWasBuiltForThePlatform(t *testing.T) {
	body := tarGz(t, map[string][]byte{"k10s": fakeBinary("NEW")})
	r := &release{tag: "v2.0.0", assetName: "k10s_2.0.0_linux_amd64.tar.gz", assetBody: body}
	c := serveRelease(t, r)
	c.GOOS, c.GOARCH = "plan9", "mips"
	installTarget(t, c)

	rel, _ := c.Latest(context.Background())
	_, err := Apply(context.Background(), c, rel, nil)
	if err == nil || !strings.Contains(err.Error(), "plan9/mips") {
		t.Errorf("error = %v, want it to name the missing platform", err)
	}
}

func TestApplyExplainsAnUnwritableInstallDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: every directory is writable")
	}
	body := tarGz(t, map[string][]byte{"k10s": fakeBinary("NEW")})
	r := &release{tag: "v2.0.0", assetName: "k10s_2.0.0_linux_amd64.tar.gz", assetBody: body}
	c := serveRelease(t, r)
	c.GOOS, c.GOARCH = "linux", "amd64"

	dir := filepath.Join(t.TempDir(), "readonly")
	if err := os.Mkdir(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	c.Target = filepath.Join(dir, "k10s")

	rel, _ := c.Latest(context.Background())
	_, err := Apply(context.Background(), c, rel, nil)
	if err == nil {
		t.Fatal("Apply into a read-only directory succeeded")
	}
	// The message has to say what to do about it, not just "permission denied".
	if !strings.Contains(err.Error(), "sudo") {
		t.Errorf("error = %v, want it to suggest sudo or a package manager", err)
	}
}

func TestApplyTakesTheLargestEntryWhenTheArchiveRenamedTheBinary(t *testing.T) {
	body := tarGz(t, map[string][]byte{
		"k10s_1.0.0_linux_amd64/LICENSE":     []byte("MIT"),
		"k10s_1.0.0_linux_amd64/README.md":   []byte("# k10s"),
		"k10s_1.0.0_linux_amd64/k10s-binary": fakeBinary("RENAMED"),
	})
	r := &release{tag: "v2.0.0", assetName: "k10s_2.0.0_linux_amd64.tar.gz", assetBody: body}
	c := serveRelease(t, r)
	c.GOOS, c.GOARCH = "linux", "amd64"
	target := installTarget(t, c)

	rel, _ := c.Latest(context.Background())
	if _, err := Apply(context.Background(), c, rel, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.ReadFile(target)
	if !bytes.Contains(got, []byte("RENAMED")) {
		t.Error("the largest archive entry was not installed")
	}
}

func TestApplyPrefersTheEntryNamedK10s(t *testing.T) {
	// Even when a larger file sits beside it — the name is the stronger
	// signal, and a big changelog must not be installed as the binary.
	body := tarGz(t, map[string][]byte{
		"k10s":         fakeBinary("RIGHT"),
		"CHANGELOG.md": bytes.Repeat([]byte("x"), 1<<16),
	})
	r := &release{tag: "v2.0.0", assetName: "k10s_2.0.0_linux_amd64.tar.gz", assetBody: body}
	c := serveRelease(t, r)
	c.GOOS, c.GOARCH = "linux", "amd64"
	target := installTarget(t, c)

	rel, _ := c.Latest(context.Background())
	if _, err := Apply(context.Background(), c, rel, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.ReadFile(target)
	if !bytes.Contains(got, []byte("RIGHT")) {
		t.Error("Apply did not prefer the entry named k10s")
	}
}

func TestFindSumHandlesSha256sumFormats(t *testing.T) {
	manifest := "aaaa  k10s_linux_amd64.tar.gz\n" +
		"bbbb *k10s_darwin_arm64.tar.gz\n" + // "*" marks binary mode
		"cccc  dist/k10s_windows_amd64.zip\n" // a path prefix from a dist dir
	for asset, want := range map[string]string{
		"k10s_linux_amd64.tar.gz":  "aaaa",
		"k10s_darwin_arm64.tar.gz": "bbbb",
		"k10s_windows_amd64.zip":   "cccc",
	} {
		got, ok := findSum(manifest, asset)
		if !ok || got != want {
			t.Errorf("findSum(%q) = %q,%v want %q,true", asset, got, ok, want)
		}
	}
	if _, ok := findSum(manifest, "missing.tar.gz"); ok {
		t.Error("findSum found a checksum for an asset that is not listed")
	}
}

func TestLooksExecutableAcceptsEveryPlatformWeShip(t *testing.T) {
	ok := map[string][]byte{
		"elf":    {0x7f, 'E', 'L', 'F'},
		"mach-o": {0xcf, 0xfa, 0xed, 0xfe},
		"fat":    {0xca, 0xfe, 0xba, 0xbe},
		"pe":     {'M', 'Z', 0x90, 0x00},
	}
	for name, head := range ok {
		if !looksExecutable(head) {
			t.Errorf("looksExecutable(%s) = false", name)
		}
	}
	for _, head := range [][]byte{[]byte("<htm"), []byte("{\"me"), {0, 0, 0, 0}} {
		if looksExecutable(head) {
			t.Errorf("looksExecutable(%q) = true", head)
		}
	}
}

func TestBinaryName(t *testing.T) {
	if got := BinaryName("windows"); got != "k10s.exe" {
		t.Errorf("BinaryName(windows) = %q", got)
	}
	if got := BinaryName("linux"); got != "k10s" {
		t.Errorf("BinaryName(linux) = %q", got)
	}
}
