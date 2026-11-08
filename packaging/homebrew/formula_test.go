// Package homebrew has no Go code of its own: formula.sh writes the Homebrew
// formula, and these tests run it, so a change to the script, or to the
// archive names it reads, fails CI instead of the tap.
package homebrew

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sums is a checksums.txt the way release.yml writes it: sha256sum output
// for all five archives, Windows included.
const sums = `1111111111111111111111111111111111111111111111111111111111111111  k10s_v1.4.0_darwin_amd64.tar.gz
2222222222222222222222222222222222222222222222222222222222222222  k10s_v1.4.0_darwin_arm64.tar.gz
3333333333333333333333333333333333333333333333333333333333333333  k10s_v1.4.0_linux_amd64.tar.gz
4444444444444444444444444444444444444444444444444444444444444444  k10s_v1.4.0_linux_arm64.tar.gz
5555555555555555555555555555555555555555555555555555555555555555  k10s_v1.4.0_windows_amd64.zip
`

// formula runs formula.sh under shell and returns what it printed.
func formula(t *testing.T, shell, tag, manifest string) (string, error) {
	t.Helper()
	path, err := exec.LookPath(shell)
	if err != nil {
		t.Skipf("%s is not installed", shell)
	}
	file := filepath.Join(t.TempDir(), "checksums.txt")
	if err := os.WriteFile(file, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd := exec.Command(path, "formula.sh", tag, file)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return stderr.String(), err
	}
	return string(out), nil
}

func TestFormulaPointsEachPlatformAtItsArchive(t *testing.T) {
	out, err := formula(t, "bash", "v1.4.0", sums)
	if err != nil {
		t.Fatalf("formula.sh: %v\n%s", err, out)
	}
	base := "https://github.com/p10node/k10s/releases/download/v1.4.0/"
	for _, p := range []struct{ platform, sum string }{
		{"darwin_arm64", "2222"},
		{"darwin_amd64", "1111"},
		{"linux_arm64", "4444"},
		{"linux_amd64", "3333"},
	} {
		// The sha256 line has to follow its own url, or a Mac would check
		// one archive against another's digest.
		want := "url \"" + base + "k10s_v1.4.0_" + p.platform + ".tar.gz\"\n" +
			"      sha256 \"" + strings.Repeat(p.sum, 16) + "\""
		if !strings.Contains(out, want) {
			t.Errorf("formula has no\n%s\nin:\n%s", want, out)
		}
	}
	for _, want := range []string{
		"class K10s < Formula",
		`license "Apache-2.0"`,
		"strategy :github_latest",
		`bin.install "k10s"`,
		`shell_output("#{bin}/k10s --version")`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("formula is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "windows") {
		t.Error("the formula mentions the Windows archive, which Homebrew never installs")
	}
	// Homebrew reads the version out of the archive names, and brew audit
	// --strict fails a formula whose version line only repeats it.
	if strings.Contains(out, "\n  version ") {
		t.Error("the formula has a version line, which brew audit --strict calls redundant")
	}
}

// junior's rule for shell scripts: they run the same under bash and zsh.
func TestFormulaIsTheSameUnderBashAndZsh(t *testing.T) {
	fromBash, err := formula(t, "bash", "v1.4.0", sums)
	if err != nil {
		t.Fatalf("bash: %v\n%s", err, fromBash)
	}
	fromZsh, err := formula(t, "zsh", "v1.4.0", sums)
	if err != nil {
		t.Fatalf("zsh: %v\n%s", err, fromZsh)
	}
	if fromBash != fromZsh {
		t.Errorf("bash and zsh disagree:\n--- bash\n%s\n--- zsh\n%s", fromBash, fromZsh)
	}
}

func TestFormulaRefusesWhatItCannotPublish(t *testing.T) {
	drop := func(platform string) string {
		var keep []string
		for _, ln := range strings.Split(sums, "\n") {
			if !strings.Contains(ln, platform) {
				keep = append(keep, ln)
			}
		}
		return strings.Join(keep, "\n")
	}
	// A manifest that does list the tag's archives, so that a refused tag is
	// refused for being the wrong kind of tag, not for a missing archive.
	listing := func(tag string) string { return strings.ReplaceAll(sums, "v1.4.0", tag) }
	for _, c := range []struct {
		name, tag, manifest, why string
	}{
		{"a pre-release", "v1.4.0-rc.1", listing("v1.4.0-rc.1"), "not a release tag"},
		{"a tag without the v", "1.4.0", listing("1.4.0"), "not a release tag"},
		{"two version parts", "v1.4", listing("v1.4"), "not a release tag"},
		{"four version parts", "v1.4.0.1", listing("v1.4.0.1"), "not a release tag"},
		{"a second line in the tag", "v1.4.0\nv1.4.0", sums, "not a release tag"},
		{"an archive missing from the manifest", "v1.4.0", drop("linux_arm64"), "does not list k10s_v1.4.0_linux_arm64.tar.gz"},
		{"an archive listed twice", "v1.4.0", sums + strings.Repeat("6", 64) + "  k10s_v1.4.0_darwin_arm64.tar.gz\n", "does not list k10s_v1.4.0_darwin_arm64.tar.gz exactly once"},
		{"a short digest", "v1.4.0", strings.Replace(sums, strings.Repeat("3", 64), strings.Repeat("3", 63), 1), "not a sha256"},
		{"an upper-case digest", "v1.4.0", strings.Replace(sums, strings.Repeat("1", 64), strings.Repeat("A", 64), 1), "not a sha256"},
		{"an empty manifest", "v1.4.0", "", "does not list"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, err := formula(t, "bash", c.tag, c.manifest)
			if err == nil {
				t.Fatalf("formula.sh wrote a formula:\n%s", out)
			}
			if !strings.Contains(out, "formula.sh: ") || !strings.Contains(out, c.why) {
				t.Errorf("stderr = %q, want it to say %q", out, c.why)
			}
		})
	}
}
