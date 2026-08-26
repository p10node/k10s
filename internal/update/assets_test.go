package update

import "testing"

func assets(names ...string) []Asset {
	out := make([]Asset, 0, len(names))
	for _, n := range names {
		out = append(out, Asset{Name: n, URL: "https://example.test/" + n, Size: 1})
	}
	return out
}

func TestPickAssetMatchesCommonNamingSchemes(t *testing.T) {
	all := assets(
		"checksums.txt",
		"k10s_1.4.0_darwin_amd64.tar.gz",
		"k10s_1.4.0_darwin_arm64.tar.gz",
		"k10s_1.4.0_linux_amd64.tar.gz",
		"k10s_1.4.0_linux_arm64.tar.gz",
		"k10s_1.4.0_windows_amd64.zip",
	)
	cases := []struct {
		goos, goarch, want string
	}{
		{"darwin", "arm64", "k10s_1.4.0_darwin_arm64.tar.gz"},
		{"darwin", "amd64", "k10s_1.4.0_darwin_amd64.tar.gz"},
		{"linux", "arm64", "k10s_1.4.0_linux_arm64.tar.gz"},
		{"windows", "amd64", "k10s_1.4.0_windows_amd64.zip"},
	}
	for _, c := range cases {
		got, ok := PickAsset(all, c.goos, c.goarch)
		if !ok {
			t.Fatalf("PickAsset(%s/%s): no match", c.goos, c.goarch)
		}
		if got.Name != c.want {
			t.Errorf("PickAsset(%s/%s) = %s, want %s", c.goos, c.goarch, got.Name, c.want)
		}
	}
}

func TestPickAssetAcceptsAlternateSpellings(t *testing.T) {
	// goreleaser's defaults, uname spellings, and a bare binary all have to
	// resolve — a release built by a different tool is still a release.
	cases := []struct {
		name, goos, goarch string
	}{
		{"k10s-v1.0.0-macos-x86_64.tar.gz", "darwin", "amd64"},
		{"k10s.Darwin.aarch64.tar.gz", "darwin", "arm64"},
		{"k10s-linux-x64.tgz", "linux", "amd64"},
		{"k10s_windows_amd64.exe", "windows", "amd64"},
		{"k10s-linux-amd64", "linux", "amd64"},
	}
	for _, c := range cases {
		if _, ok := PickAsset(assets(c.name), c.goos, c.goarch); !ok {
			t.Errorf("PickAsset(%q) for %s/%s: no match", c.name, c.goos, c.goarch)
		}
	}
}

func TestPickAssetDoesNotConfuseArmWithArm64(t *testing.T) {
	// "arm" is a substring of "arm64": a plain substring matcher would hand
	// a 32-bit Raspberry Pi a 64-bit binary.
	if a, ok := PickAsset(assets("k10s_linux_arm64.tar.gz"), "linux", "arm"); ok {
		t.Errorf("PickAsset(arm) matched %s, want no match", a.Name)
	}
	if a, ok := PickAsset(assets("k10s_linux_arm64.tar.gz"), "linux", "arm64"); !ok {
		t.Error("PickAsset(arm64) found nothing")
	} else if a.Name != "k10s_linux_arm64.tar.gz" {
		t.Errorf("got %s", a.Name)
	}
}

func TestPickAssetSkipsManifestsAndSignatures(t *testing.T) {
	// These carry the platform tokens in their own names, so they have to be
	// excluded explicitly or they win by being listed first.
	all := assets(
		"k10s_linux_amd64.tar.gz.sha256",
		"k10s_linux_amd64.tar.gz.sig",
		"k10s_linux_amd64.sbom.json",
		"k10s_linux_amd64.tar.gz",
	)
	got, ok := PickAsset(all, "linux", "amd64")
	if !ok || got.Name != "k10s_linux_amd64.tar.gz" {
		t.Errorf("PickAsset = %+v (ok=%v), want the .tar.gz", got, ok)
	}
}

func TestPickAssetReportsNoMatchForAnUnbuiltPlatform(t *testing.T) {
	if a, ok := PickAsset(assets("k10s_linux_amd64.tar.gz"), "windows", "arm64"); ok {
		t.Errorf("PickAsset matched %s for an unbuilt platform", a.Name)
	}
}

func TestPickChecksums(t *testing.T) {
	for _, name := range []string{"checksums.txt", "k10s_1.0.0_SHA256SUMS", "CHECKSUMS.TXT"} {
		if _, ok := PickChecksums(assets("k10s_linux_amd64.tar.gz", name)); !ok {
			t.Errorf("PickChecksums did not find %q", name)
		}
	}
	if _, ok := PickChecksums(assets("k10s_linux_amd64.tar.gz")); ok {
		t.Error("PickChecksums found a manifest where there is none")
	}
}

func TestArchiveKind(t *testing.T) {
	for in, want := range map[string]string{
		"k10s.tar.gz": ".tar.gz",
		"k10s.tgz":    ".tar.gz",
		"k10s.zip":    ".zip",
		"k10s":        "",
		"k10s.exe":    "",
	} {
		if got := archiveKind(in); got != want {
			t.Errorf("archiveKind(%q) = %q, want %q", in, got, want)
		}
	}
}
