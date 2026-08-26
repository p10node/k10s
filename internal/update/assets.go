package update

import (
	"path"
	"strings"
)

// Release assets are matched by name rather than by a fixed convention,
// because the same release may be built by goreleaser, by `just release`, or
// by hand, and each spells the platform slightly differently. Matching needs
// one OS token and one architecture token, both delimited — otherwise the
// "arm" alias happily claims an "arm64" archive.

var osAliases = map[string][]string{
	"darwin":  {"darwin", "macos", "mac", "osx"},
	"linux":   {"linux"},
	"windows": {"windows", "win"},
	"freebsd": {"freebsd"},
}

var archAliases = map[string][]string{
	"amd64": {"amd64", "x86_64", "x64"},
	"arm64": {"arm64", "aarch64"},
	"386":   {"386", "i386", "x86"},
	"arm":   {"arm", "armv7", "armv6"},
}

// archiveExts are the wrappers Apply knows how to open. An asset with none
// of them is taken to be the bare binary.
var archiveExts = []string{".tar.gz", ".tgz", ".zip"}

// PickAsset returns the asset built for goos/goarch. It is exported so a
// test — and the /update panel, when nothing matches — can say exactly what
// was looked for.
func PickAsset(assets []Asset, goos, goarch string) (Asset, bool) {
	for _, a := range assets {
		if isChecksums(a.Name) || isSidecar(a.Name) {
			continue
		}
		n := normalizeName(a.Name)
		if hasToken(n, osAliases[goos]) && hasToken(n, archAliases[goarch]) {
			return a, true
		}
	}
	return Asset{}, false
}

// PickChecksums returns the release's checksum manifest, if it publishes one.
func PickChecksums(assets []Asset) (Asset, bool) {
	for _, a := range assets {
		if isChecksums(a.Name) {
			return a, true
		}
	}
	return Asset{}, false
}

// normalizeName lowercases and rewrites every separator to "-", then pads
// both ends, so a token test is a plain "-token-" substring search: "x86_64"
// and "x86-64" become the same thing and "arm" stops matching "arm64".
func normalizeName(name string) string {
	name = strings.ToLower(path.Base(name))
	var b strings.Builder
	b.WriteByte('-')
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
		} else {
			b.WriteByte('-')
		}
	}
	b.WriteByte('-')
	return b.String()
}

func hasToken(normalized string, aliases []string) bool {
	for _, a := range aliases {
		if strings.Contains(normalized, "-"+strings.ReplaceAll(a, "_", "-")+"-") {
			return true
		}
	}
	return false
}

func isChecksums(name string) bool {
	n := strings.ToLower(path.Base(name))
	return strings.Contains(n, "checksum") || strings.Contains(n, "sha256sum")
}

// isSidecar skips the files that sit *next to* a release binary — per-asset
// checksums, signatures, SBOMs, notes. They repeat the platform tokens in
// their own names, so without this they win by being listed first.
func isSidecar(name string) bool {
	n := strings.ToLower(path.Base(name))
	for _, ext := range []string{
		".sig", ".asc", ".pem", ".pub", ".crt", ".cert",
		".sha256", ".sha512", ".md5",
		".sbom", ".json", ".jsonl", ".txt", ".md", ".yaml", ".yml",
	} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return false
}

// archiveKind reports which wrapper an asset uses, or "" for a bare binary.
func archiveKind(name string) string {
	n := strings.ToLower(path.Base(name))
	for _, ext := range archiveExts {
		if strings.HasSuffix(n, ext) {
			if ext == ".tgz" {
				return ".tar.gz"
			}
			return ext
		}
	}
	return ""
}
