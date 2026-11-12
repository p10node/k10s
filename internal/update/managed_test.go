package update

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubBinary writes a stand-in binary at path, directories and all.
func stubBinary(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fakeBinary("KEEP"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// symlinkOrSkip makes path a symlink to target, or skips the test where
// symlinks cannot be made (Windows without the privilege).
func symlinkOrSkip(t *testing.T, target, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}
	return path
}

// brewInstall lays out what `brew install p10node/tap/k10s` leaves under a
// prefix: the binary in the keg, Cellar/k10s/<version>/bin/k10s, and
// bin/k10s, a relative link to it, the way Homebrew writes one.
func brewInstall(t *testing.T, prefix, version string) (binLink, kegBinary string) {
	t.Helper()
	kegBinary = stubBinary(t, filepath.Join(prefix, "Cellar", "k10s", version, "bin", "k10s"))
	binLink = symlinkOrSkip(t, filepath.Join("..", "Cellar", "k10s", version, "bin", "k10s"), filepath.Join(prefix, "bin", "k10s"))
	return binLink, kegBinary
}

func TestOwnerOfFindsAHomebrewKegUnderEveryPrefix(t *testing.T) {
	for _, prefix := range []string{
		"opt/homebrew",              // macOS on Apple silicon
		"usr/local",                 // macOS on Intel
		"home/linuxbrew/.linuxbrew", // Linux
	} {
		t.Run(prefix, func(t *testing.T) {
			binLink, kegBinary := brewInstall(t, filepath.Join(t.TempDir(), filepath.FromSlash(prefix)), "0.1.8")
			// macOS reports os.Executable as the link k10s was started
			// through; Linux reads /proc/self/exe, which is the keg itself.
			for _, path := range []string{binLink, kegBinary} {
				owner := OwnerOf(path)
				if owner == nil {
					t.Fatalf("OwnerOf(%s) = nil, want Homebrew", path)
				}
				if owner.Name != "Homebrew" || owner.Upgrade != "brew upgrade k10s" {
					t.Errorf("OwnerOf(%s) = %+v, want Homebrew and brew upgrade k10s", path, *owner)
				}
			}
		})
	}
}

func TestOwnerOfReadsEveryKegName(t *testing.T) {
	// A formula revision and a --HEAD build are kegs too.
	for _, version := range []string{"0.1.8", "0.1.8_1", "10.2.0", "HEAD-1a2b3c4"} {
		path := stubBinary(t, filepath.Join(t.TempDir(), "opt", "homebrew", "Cellar", "k10s", version, "bin", "k10s"))
		if OwnerOf(path) == nil {
			t.Errorf("OwnerOf(%s) = nil, want Homebrew", path)
		}
	}
}

func TestOwnerOfFollowsALinkIntoAKeg(t *testing.T) {
	root := t.TempDir()
	_, kegBinary := brewInstall(t, filepath.Join(root, "opt", "homebrew"), "0.1.8")
	// Someone's own ~/bin link to the brewed binary: updating through it
	// would still write into the keg.
	mine := symlinkOrSkip(t, kegBinary, filepath.Join(root, "home", "u", "bin", "k10s"))
	if OwnerOf(mine) == nil {
		t.Errorf("OwnerOf(%s) = nil for a link into a keg", mine)
	}
}

func TestOwnerOfLeavesEveryOtherInstallAlone(t *testing.T) {
	for _, path := range []string{
		"home/u/.local/bin/k10s", // install.sh with no sudo to ask for
		"usr/local/bin/k10s",     // install.sh with sudo: a file, not a link
		"opt/homebrew/bin/k10s",  // install.sh into a writable Homebrew bin
		"home/u/go/bin/k10s",     // go install
		// "Cellar" inside a name is not the Cellar.
		"home/u/MyCellar/k10s/0.1.8/bin/k10s",
		"home/u/Cellar-old/k10s/0.1.8/bin/k10s",
		"home/u/wine.Cellar/k10s/0.1.8/bin/k10s",
		// Nor is a Cellar without a k10s keg in it.
		"opt/homebrew/Cellar/k10s-nightly/0.1.8/bin/k10s",
		"opt/homebrew/Cellar/k10s/bin/k10s",
		"opt/homebrew/Cellar/k10s",
	} {
		full := stubBinary(t, filepath.Join(t.TempDir(), filepath.FromSlash(path)))
		if owner := OwnerOf(full); owner != nil {
			t.Errorf("OwnerOf(%s) = %+v, want nil", path, *owner)
		}
	}
}

func TestOwnerOfResolvesALinkOutOfACellarLookalike(t *testing.T) {
	root := t.TempDir()
	plain := stubBinary(t, filepath.Join(root, "home", "u", ".local", "bin", "k10s"))
	// The file Apply would replace is the link's target, which no package
	// manager owns.
	path := symlinkOrSkip(t, plain, filepath.Join(root, "x", "Cellar", "k10s", "0.1.8", "bin", "k10s"))
	if owner := OwnerOf(path); owner != nil {
		t.Errorf("OwnerOf(%s) = %+v, want nil: it resolves to %s", path, *owner, plain)
	}
}

func TestApplyLeavesAHomebrewKegUntouched(t *testing.T) {
	binLink, kegBinary := brewInstall(t, filepath.Join(t.TempDir(), "opt", "homebrew"), "0.1.8")
	// Nothing listens on port 1: had Apply gone on to download, it would
	// fail with a different error.
	rel := &Release{
		Version: "9.9.9",
		Tag:     "v9.9.9",
		Asset:   Asset{Name: "k10s_v9.9.9_darwin_arm64.tar.gz", URL: "http://127.0.0.1:1/k10s.tar.gz"},
		Sums:    Asset{Name: "checksums.txt", URL: "http://127.0.0.1:1/checksums.txt"},
	}
	// The refusal has to come before the writability probe, which writes a
	// temporary file into the directory. The probe deletes that file again,
	// so only a probe that reports being called can tell.
	var probed []string
	probe := probeWritable
	probeWritable = func(dir string) error {
		probed = append(probed, dir)
		return probe(dir)
	}
	t.Cleanup(func() { probeWritable = probe })

	for _, target := range []string{binLink, kegBinary} {
		c := &Client{Target: target, GOOS: "darwin", GOARCH: "arm64"}
		_, err := Apply(context.Background(), c, rel, nil)
		var managed *ManagedError
		if !errors.As(err, &managed) {
			t.Fatalf("Apply(%s) error = %v, want a *ManagedError", target, err)
		}
		if want := "installed by Homebrew, which updates it: run brew upgrade k10s"; err.Error() != want {
			t.Errorf("Apply error = %q, want %q", err, want)
		}
	}
	if len(probed) != 0 {
		t.Errorf("Apply probed %s for writability before refusing the keg", strings.Join(probed, ", "))
	}

	got, err := os.ReadFile(kegBinary)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("KEEP")) {
		t.Error("the keg's binary was rewritten")
	}
	// And nothing else was left in the keg's bin.
	entries, err := os.ReadDir(filepath.Dir(kegBinary))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("the keg's bin holds %s, want just k10s", strings.Join(names, ", "))
	}
}
