package update

import (
	"os"
	"path/filepath"
	"strings"
)

// A package manager that installed k10s keeps its own record of which
// version sits where, and upgrades it itself. A self-update written over its
// copy would leave that record naming a build that is no longer there, so
// self-update steps aside for one and names the command that does the job.

// Owner is the package manager an installed binary belongs to.
type Owner struct {
	Name    string // "Homebrew"
	Upgrade string // the command that upgrades it: "brew upgrade k10s"
}

// Hint is what `k10s update` and /update say instead of installing.
func (o *Owner) Hint() string {
	return "installed by " + o.Name + ", which updates it: run " + o.Upgrade
}

// ManagedError is Apply's refusal to touch a binary a package manager owns.
// Apply returns it before it writes anything, the directory check's
// temporary file included.
type ManagedError struct{ Owner *Owner }

func (e *ManagedError) Error() string { return e.Owner.Hint() }

// InstalledBy reports the package manager that owns the running binary, or
// nil when there is none and self-update may replace it.
func InstalledBy() *Owner {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	return OwnerOf(exe)
}

// OwnerOf reports the package manager whose install tree holds path, once
// symlinks are resolved: Homebrew links <prefix>/bin/k10s to the binary in
// its Cellar, and on macOS os.Executable reports the link it was started
// through. It returns nil for anything else.
func OwnerOf(path string) *Owner {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	parts := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	if inHomebrewKeg(parts) {
		return &Owner{Name: "Homebrew", Upgrade: "brew upgrade k10s"}
	}
	return nil
}

// inHomebrewKeg reports whether parts name a file inside a k10s keg,
// <prefix>/Cellar/k10s/<version>/..., which is where `brew install` puts the
// binary under every prefix: /opt/homebrew, /usr/local,
// /home/linuxbrew/.linuxbrew or a custom one. Whole path segments are
// compared, so a directory that only has "Cellar" in its name is not a keg.
func inHomebrewKeg(parts []string) bool {
	for i := 0; i+3 < len(parts); i++ {
		if parts[i] == "Cellar" && parts[i+1] == "k10s" && isKegVersion(parts[i+2]) {
			return true
		}
	}
	return false
}

// isKegVersion matches the names Homebrew gives a keg: the formula version,
// with a "_1" revision suffix when there is one, or HEAD-<commit>.
func isKegVersion(s string) bool {
	if strings.HasPrefix(s, "HEAD") {
		return true
	}
	return s != "" && s[0] >= '0' && s[0] <= '9'
}
