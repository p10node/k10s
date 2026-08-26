// Package version records which build of k10s is running.
//
// The values are stamped at link time (`just build`, `just release`, and the
// release workflow all pass -ldflags). An unstamped build — `go run .`, a
// plain `go build` — reports "dev", which internal/update deliberately
// treats as older than every published release, so a source build can still
// install one.
package version

import (
	"fmt"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
)

// Stamped with:
//
//	-ldflags "-X k10s/internal/version.Version=v1.2.3
//	          -X k10s/internal/version.Commit=abc1234
//	          -X k10s/internal/version.Date=2026-08-26"
var (
	Version = ""
	Commit  = ""
	Date    = ""
)

// Dev is what an unstamped build reports.
const Dev = "dev"

// Current returns the running version. When the linker stamp is missing it
// falls back to the module version Go embeds for `go install k10s@vX`, so an
// install straight from the proxy still knows what it is.
func Current() string {
	if v := strings.TrimSpace(Version); v != "" {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" && !isPseudo(v) {
			return v
		}
	}
	return Dev
}

// pseudoVersion matches the version Go synthesises from VCS state for a
// build that has no tag — "v0.0.0-20260826114715-bb85f95437c1", or
// "v1.2.3-0.20260826114715-bb85f95437c1" past a release, each with an
// optional "+dirty". It names a commit, not a release, so Current reports
// "dev" for it: calling an uncommitted working tree "v0.0.0" would make
// /update compare against a version nobody published.
var pseudoVersion = regexp.MustCompile(`[-.][0-9]{14}-[0-9a-f]{12}(\+|$)`)

func isPseudo(v string) bool { return pseudoVersion.MatchString(v) }

// IsDev reports whether this build came from source rather than a release.
func IsDev() bool { return Current() == Dev }

// Revision returns the short commit hash: the stamp if present, otherwise
// the VCS revision Go records when it builds inside a git checkout.
func Revision() string {
	if c := short(Commit); c != "" {
		return c
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				return short(s.Value)
			}
		}
	}
	return ""
}

// String is the one-line report printed by `k10s --version` and shown at the
// top of the /update panel.
func String() string {
	var b strings.Builder
	b.WriteString("k10s " + Current())
	if r := Revision(); r != "" {
		b.WriteString(" (" + r + ")")
	}
	if d := strings.TrimSpace(Date); d != "" {
		b.WriteString(" built " + d)
	}
	fmt.Fprintf(&b, "  ·  %s/%s  ·  %s", runtime.GOOS, runtime.GOARCH, runtime.Version())
	return b.String()
}

func short(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
