#!/bin/sh
# k10s installer — one prebuilt static binary, checksum-verified.
#
#   curl -fsSL https://p10node.com/k10s/install.sh | sh
#
# Everything it needs is a POSIX shell, curl or wget, and tar. The asset
# names and checksums.txt format it reads are the ones produced by
# `just release` / .github/workflows/release.yml, which is also what
# internal/update matches on — keep the three in step.
#
# Knobs (env or flag):
#   K10S_VERSION=v0.1.0        --version v0.1.0   pin a release (default: latest)
#   K10S_INSTALL_DIR=~/bin     --dir ~/bin        where the binary lands
#   K10S_NO_SUDO=1             --no-sudo          never escalate; use a user dir
set -eu

REPO="p10node/k10s"
BIN="k10s"

VERSION="${K10S_VERSION:-}"
INSTALL_DIR="${K10S_INSTALL_DIR:-}"
NO_SUDO="${K10S_NO_SUDO:-}"

# ---- output ---------------------------------------------------------------

if [ -t 2 ]; then
    B=$(printf '\033[1m'); DIM=$(printf '\033[2m'); RED=$(printf '\033[31m')
    GRN=$(printf '\033[32m'); YLW=$(printf '\033[33m'); R=$(printf '\033[0m')
else
    B=""; DIM=""; RED=""; GRN=""; YLW=""; R=""
fi

say()  { printf '%s\n' "$*" >&2; }
step() { printf '%s→%s %s\n' "$DIM" "$R" "$*" >&2; }
warn() { printf '%s!%s %s\n' "$YLW" "$R" "$*" >&2; }
die()  { printf '%serror%s %s\n' "$RED" "$R" "$*" >&2; exit 1; }

usage() {
    cat >&2 <<EOF
${B}k10s installer${R}

  curl -fsSL https://p10node.com/k10s/install.sh | sh
  curl -fsSL https://p10node.com/k10s/install.sh | sh -s -- --dir ~/bin

Options
  --version <tag>   install a specific release (default: the latest)
  --dir <path>      install into <path> instead of /usr/local/bin
  --no-sudo         never escalate; fall back to a directory you own
  -h, --help        this text
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --version) VERSION="${2:-}"; [ -n "$VERSION" ] || die "--version needs a tag"; shift 2 ;;
        --version=*) VERSION="${1#*=}"; shift ;;
        --dir) INSTALL_DIR="${2:-}"; [ -n "$INSTALL_DIR" ] || die "--dir needs a path"; shift 2 ;;
        --dir=*) INSTALL_DIR="${1#*=}"; shift ;;
        --no-sudo) NO_SUDO=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) die "unknown option: $1 (try --help)" ;;
    esac
done

# ---- platform -------------------------------------------------------------

detect_os() {
    os=$(uname -s 2>/dev/null || echo unknown)
    case "$os" in
        Darwin) echo darwin ;;
        Linux) echo linux ;;
        MINGW*|MSYS*|CYGWIN*|Windows_NT)
            die "Windows: grab k10s_<version>_windows_amd64.zip from
      https://github.com/$REPO/releases/latest and put k10s.exe on your PATH." ;;
        *) die "unsupported OS: $os — build from source: go install github.com/$REPO@latest" ;;
    esac
}

detect_arch() {
    arch=$(uname -m 2>/dev/null || echo unknown)
    case "$arch" in
        x86_64|amd64) echo amd64 ;;
        arm64|aarch64) echo arm64 ;;
        *) die "unsupported architecture: $arch — build from source: go install github.com/$REPO@latest" ;;
    esac
}

# ---- fetch ----------------------------------------------------------------

if command -v curl >/dev/null 2>&1; then
    HTTP=curl
elif command -v wget >/dev/null 2>&1; then
    HTTP=wget
else
    die "need curl or wget on PATH"
fi

command -v tar >/dev/null 2>&1 || die "need tar on PATH"

# fetch <url> <dest>
fetch() {
    if [ "$HTTP" = curl ]; then
        curl -fsSL --retry 3 --retry-delay 1 -o "$2" "$1"
    else
        wget -q --tries=3 -O "$2" "$1"
    fi
}

# fetch_stdout <url>
fetch_stdout() {
    if [ "$HTTP" = curl ]; then
        curl -fsSL --retry 3 --retry-delay 1 "$1"
    else
        wget -q --tries=3 -O - "$1"
    fi
}

# Three ways to name the newest release, in order of preference:
#
#   1. the /releases/latest redirect — its Location carries the tag, and it
#      costs no API quota (unauthenticated api.github.com is rate limited per
#      IP, and CI runners share IPs).
#   2. the API, for the wget path where a redirect cannot be inspected.
#   3. releases.atom — also quota-free, and the only one of the three that
#      sees a pre-release. GitHub's "latest" is stable-only, so while the
#      newest release is a pre-release the first two hops find nothing and
#      this is what answers.
#
# The order is the policy: a stable release always wins over a pre-release.
latest_tag() {
    if [ "$HTTP" = curl ]; then
        url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
            "https://github.com/$REPO/releases/latest" 2>/dev/null || true)
        case "$url" in
            */tag/*) printf '%s\n' "${url##*/tag/}"; return 0 ;;
        esac
    fi

    tag=$(fetch_stdout "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null |
        sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
    if [ -n "$tag" ]; then
        printf '%s\n' "$tag"
        return 0
    fi

    tag=$(fetch_stdout "https://github.com/$REPO/releases.atom" 2>/dev/null |
        sed -n 's|.*releases/tag/\([^"<]*\).*|\1|p' | head -1)
    [ -n "$tag" ] || return 1
    warn "no stable release yet — installing the pre-release $tag"
    printf '%s\n' "$tag"
}

# ---- checksum -------------------------------------------------------------

# sha256 <file> — prints the bare hex digest, or nothing if no tool exists.
sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | cut -d' ' -f1
    elif command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 "$1" | sed 's/.*= *//'
    fi
}

# ---- install target -------------------------------------------------------

SUDO=""

pick_install_dir() {
    if [ -n "$INSTALL_DIR" ]; then
        # An explicit --dir is a promise, not a suggestion: create it, and
        # escalate only if the user did not forbid it.
        if [ ! -d "$INSTALL_DIR" ] && ! mkdir -p "$INSTALL_DIR" 2>/dev/null; then
            if [ -z "$NO_SUDO" ] && command -v sudo >/dev/null 2>&1; then
                SUDO=sudo
            else
                die "cannot create $INSTALL_DIR"
            fi
        fi
        if [ -d "$INSTALL_DIR" ] && [ ! -w "$INSTALL_DIR" ]; then
            if [ -z "$NO_SUDO" ] && command -v sudo >/dev/null 2>&1; then
                SUDO=sudo
            else
                die "$INSTALL_DIR is not writable"
            fi
        fi
        return
    fi

    for d in /usr/local/bin /opt/homebrew/bin; do
        if [ -d "$d" ] && [ -w "$d" ]; then
            INSTALL_DIR="$d"; return
        fi
    done

    if [ -z "$NO_SUDO" ] && [ -t 0 ] && command -v sudo >/dev/null 2>&1; then
        INSTALL_DIR=/usr/local/bin
        SUDO=sudo
        return
    fi

    # Piped into sh with no TTY, sudo would hang on its password prompt, so a
    # directory the user owns is the only honest default.
    INSTALL_DIR="${XDG_BIN_HOME:-$HOME/.local/bin}"
    mkdir -p "$INSTALL_DIR" 2>/dev/null || die "cannot create $INSTALL_DIR"
}

on_path() {
    case ":$PATH:" in
        *":$1:"*) return 0 ;;
        *) return 1 ;;
    esac
}

# ---- run ------------------------------------------------------------------

OS=$(detect_os)
ARCH=$(detect_arch)

if [ -z "$VERSION" ]; then
    step "resolving the latest release"
    VERSION=$(latest_tag)
    [ -n "$VERSION" ] || die "could not resolve the latest release — pass --version <tag>"
fi

BASE="https://github.com/$REPO/releases/download/$VERSION"

TMP=$(mktemp -d 2>/dev/null || mktemp -d -t k10s)
trap 'rm -rf "$TMP"' EXIT INT TERM

# The archive name is read out of checksums.txt rather than guessed, so a
# release built with a different version-string convention still resolves.
ASSET=""
if fetch "$BASE/checksums.txt" "$TMP/checksums.txt" 2>/dev/null; then
    ASSET=$(grep -E "_${OS}_${ARCH}\.tar\.gz\$" "$TMP/checksums.txt" |
        awk '{print $NF; exit}')
fi
[ -n "$ASSET" ] || ASSET="${BIN}_${VERSION}_${OS}_${ARCH}.tar.gz"

step "downloading ${B}$ASSET${R}"
fetch "$BASE/$ASSET" "$TMP/$ASSET" 2>/dev/null ||
    die "no build for $OS/$ARCH in $VERSION — see https://github.com/$REPO/releases"

want=""
[ -f "$TMP/checksums.txt" ] && want=$(awk -v f="$ASSET" '$NF == f {print $1; exit}' "$TMP/checksums.txt")
if [ -n "$want" ]; then
    got=$(sha256 "$TMP/$ASSET")
    if [ -z "$got" ]; then
        warn "no sha256 tool found (sha256sum/shasum/openssl) — checksum not verified"
    elif [ "$got" != "$want" ]; then
        die "checksum mismatch for $ASSET
      want $want
      got  $got"
    else
        step "checksum ${GRN}ok${R}"
    fi
else
    warn "no checksum published for $ASSET — installing unverified"
fi

tar xzf "$TMP/$ASSET" -C "$TMP" || die "could not unpack $ASSET"
[ -f "$TMP/$BIN" ] || die "$ASSET did not contain a $BIN binary"

pick_install_dir
DEST="$INSTALL_DIR/$BIN"

# Just "k10s v0.1.0 (abc1234)" — the full stamp also carries the build date,
# platform and Go version, which is more than a one-line summary needs.
short_version() { "$1" --version 2>/dev/null | awk 'NR==1 {print $1, $2, $3}'; }

had=""
[ -x "$DEST" ] && had=$(short_version "$DEST" || true)

[ -n "$SUDO" ] && step "installing into ${B}$INSTALL_DIR${R} (needs sudo)"
# install(1) replaces the file rather than writing through it, so a running
# k10s keeps its own inode and does not get corrupted mid-flight.
$SUDO install -m 755 "$TMP/$BIN" "$DEST" ||
    die "could not install into $INSTALL_DIR"

new=$(short_version "$DEST" || true)
[ -n "$new" ] || new="$BIN $VERSION"

say ""
if [ -n "$had" ] && [ "$had" != "$new" ]; then
    printf '%s✓%s %s → %s%s%s\n' "$GRN" "$R" "$had" "$B" "$new" "$R" >&2
else
    printf '%s✓%s %s%s%s installed → %s\n' "$GRN" "$R" "$B" "$new" "$R" "$DEST" >&2
fi

if on_path "$INSTALL_DIR"; then
    say ""
    say "  ${B}k10s${R}            your current kubeconfig context"
    say "  ${B}k10s --version${R}  which build is this"
    say "  ${B}k10s update${R}     upgrade in place, later on"
else
    say ""
    warn "$INSTALL_DIR is not on your PATH. Add it:"
    say ""
    say "    export PATH=\"$INSTALL_DIR:\$PATH\""
    say ""
    say "  …in ~/.zshrc or ~/.bashrc, then run ${B}k10s${R}."
fi
