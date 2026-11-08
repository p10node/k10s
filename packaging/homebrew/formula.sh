#!/usr/bin/env bash
# Prints the Homebrew formula for one k10s release, the Formula/k10s.rb that
# p10node/homebrew-tap serves to `brew install p10node/tap/k10s`:
#
#   packaging/homebrew/formula.sh vX.Y.Z checksums.txt > k10s.rb
#   just brew-formula vX.Y.Z checksums.txt
#
# The formula installs the prebuilt archives the release publishes, checked
# against the sha256 that the release's checksums.txt lists for each, so an
# install takes seconds and needs no Go. .github/workflows/homebrew.yml runs
# this for every full release and pushes the result to the tap.
#
# The archive names and the sha256sum lines of checksums.txt are the ones
# release.yml writes and that internal/update and install.sh read: keep the
# four in step.
#
# Written for bash 3.2 (macOS's /bin/bash) and zsh alike: no arrays, no [[ ]],
# every expansion quoted.
set -eu

repo="p10node/k10s"

die() {
    printf 'formula.sh: %s\n' "$*" >&2
    exit 1
}

[ "$#" -eq 2 ] || die "usage: formula.sh <tag> <checksums.txt>"
tag=$1
sums=$2

# Full releases only. A pre-release in the tap would be offered to every
# Homebrew user as an upgrade.
case "$tag" in
    '' | *[!v0-9.]*) die "not a release tag: $tag (want vMAJOR.MINOR.PATCH)" ;;
esac
printf '%s\n' "$tag" | grep -Eq '^v[0-9]+[.][0-9]+[.][0-9]+$' ||
    die "not a release tag: $tag (want vMAJOR.MINOR.PATCH)"

[ -f "$sums" ] || die "no checksums file at $sums"

# sha <os>_<arch> prints the sha256 that checksums.txt lists for that
# platform's archive. It fails unless the file lists the archive exactly
# once, with a well-formed digest.
sha() {
    name="k10s_${tag}_$1.tar.gz"
    sum=$(awk -v name="$name" '
        $2 == name || $2 == ("*" name) { print $1; n++ }
        END { exit (n == 1 ? 0 : 1) }
    ' "$sums") || die "$sums does not list $name exactly once"
    case "$sum" in
        *[!0-9a-f]*) die "$sums: not a sha256 for $name: $sum" ;;
    esac
    [ "${#sum}" -eq 64 ] || die "$sums: not a sha256 for $name: $sum"
    printf '%s\n' "$sum"
}

# Each on its own line, so set -e stops the script on the first failure: a
# failing $(...) inside the here-document below would only leave a hole.
darwin_arm64=$(sha darwin_arm64)
darwin_amd64=$(sha darwin_amd64)
linux_arm64=$(sha linux_arm64)
linux_amd64=$(sha linux_amd64)

base="https://github.com/$repo/releases/download/$tag"

# No version line: Homebrew reads the version out of the archive names, and
# brew audit --strict calls a version line that repeats it redundant.
cat <<EOF
# Written by packaging/homebrew/formula.sh in $repo, which
# .github/workflows/homebrew.yml runs for every release: edit the script, not
# this file, or the next release overwrites the change.
class K10s < Formula
  desc "Kubernetes terminal UI you can click"
  homepage "https://github.com/$repo"
  license "Apache-2.0"

  livecheck do
    url :stable
    strategy :github_latest
  end

  on_macos do
    on_arm do
      url "$base/k10s_${tag}_darwin_arm64.tar.gz"
      sha256 "$darwin_arm64"
    end
    on_intel do
      url "$base/k10s_${tag}_darwin_amd64.tar.gz"
      sha256 "$darwin_amd64"
    end
  end

  on_linux do
    on_arm do
      url "$base/k10s_${tag}_linux_arm64.tar.gz"
      sha256 "$linux_arm64"
    end
    on_intel do
      url "$base/k10s_${tag}_linux_amd64.tar.gz"
      sha256 "$linux_amd64"
    end
  end

  def install
    bin.install "k10s"
  end

  test do
    assert_match "k10s v#{version}", shell_output("#{bin}/k10s --version")
  end
end
EOF
