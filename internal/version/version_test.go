package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestCurrentPrefersTheLinkerStamp(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })

	Version = "v1.4.0"
	if got := Current(); got != "v1.4.0" {
		t.Errorf("Current() = %q, want the stamped version", got)
	}
	// Whitespace creeps in through shell quoting in build recipes.
	Version = "  v1.4.1\n"
	if got := Current(); got != "v1.4.1" {
		t.Errorf("Current() = %q, want it trimmed", got)
	}
}

func TestCurrentReportsDevForAnUnstampedBuild(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = ""

	// `go test` builds have no stamp, and Go's own VCS pseudo-version names a
	// commit rather than a release — either way this is a source build.
	if got := Current(); got != Dev {
		t.Errorf("Current() = %q, want %q", got, Dev)
	}
	if !IsDev() {
		t.Error("IsDev() = false for an unstamped build")
	}
}

func TestIsPseudoRecognisesGosVCSVersions(t *testing.T) {
	pseudo := []string{
		"v0.0.0-20260826114715-bb85f95437c1",
		"v0.0.0-20260826114715-bb85f95437c1+dirty",
		"v1.2.3-0.20260826114715-bb85f95437c1",
	}
	for _, v := range pseudo {
		if !isPseudo(v) {
			t.Errorf("isPseudo(%q) = false", v)
		}
	}
	// A real release — including a prerelease — must not be mistaken for one.
	for _, v := range []string{"v1.4.0", "v1.4.0-rc.1", "v1.4.0+build.7", "dev"} {
		if isPseudo(v) {
			t.Errorf("isPseudo(%q) = true", v)
		}
	}
}

func TestStringNamesTheBuildAndThePlatform(t *testing.T) {
	oldV, oldC, oldD := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = oldV, oldC, oldD })

	Version, Commit, Date = "v1.4.0", "abcdef1234567890", "2026-08-26"
	got := String()
	for _, want := range []string{"k10s v1.4.0", "abcdef1", "2026-08-26", runtime.GOOS + "/" + runtime.GOARCH} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
	// A full 40-character hash is unreadable in a status line.
	if strings.Contains(got, "abcdef1234567890") {
		t.Errorf("String() = %q, want the commit shortened", got)
	}
}
