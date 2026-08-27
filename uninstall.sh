#!/bin/sh
# k10s uninstaller — removes the binary, and the config only if asked.
#
#   curl -fsSL https://p10node.com/k10s/uninstall.sh | sh
#
# The mirror of install.sh: same shells, same flags where they overlap, same
# TTY-aware sudo rule. It only ever deletes a file named k10s (or the
# ~/.k10s directory with --purge), and it prints every path before touching
# it, so a --dry-run first costs nothing.
#
# Knobs (env or flag):
#   K10S_INSTALL_DIR=~/bin     --dir ~/bin    look only there
#   K10S_NO_SUDO=1             --no-sudo      never escalate
#   K10S_PURGE=1               --purge        also delete ~/.k10s
set -eu

BIN="k10s"
REPO="p10node/k10s"

INSTALL_DIR="${K10S_INSTALL_DIR:-}"
NO_SUDO="${K10S_NO_SUDO:-}"
PURGE="${K10S_PURGE:-}"
KEEP_CONFIG=""
DRY_RUN=""
ASSUME_YES=""

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
${B}k10s uninstaller${R}

  curl -fsSL https://p10node.com/k10s/uninstall.sh | sh
  curl -fsSL https://p10node.com/k10s/uninstall.sh | sh -s -- --purge

Options
  --purge          also delete ~/.k10s (config, theme choice)
  --keep-config    keep ~/.k10s and say nothing about it
  --dir <path>     only look in <path>, instead of PATH + the usual places
  --dry-run        print what would be removed, remove nothing
  -y, --yes        no confirmation prompt
  --no-sudo        never escalate; skip anything needing root
  -h, --help       this text
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --purge) PURGE=1; shift ;;
        --keep-config) KEEP_CONFIG=1; shift ;;
        --dir) INSTALL_DIR="${2:-}"; [ -n "$INSTALL_DIR" ] || die "--dir needs a path"; shift 2 ;;
        --dir=*) INSTALL_DIR="${1#*=}"; shift ;;
        --dry-run|-n) DRY_RUN=1; shift ;;
        -y|--yes) ASSUME_YES=1; shift ;;
        --no-sudo) NO_SUDO=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) die "unknown option: $1 (try --help)" ;;
    esac
done

[ -n "$PURGE" ] && [ -n "$KEEP_CONFIG" ] && die "--purge and --keep-config contradict each other"

# ---- find every copy ------------------------------------------------------

# A `go install` build, an install.sh build and a Homebrew-era leftover can
# all be on one machine, and the one earliest on PATH is the one that has
# been shadowing the others. Collecting all of them is the only way an
# uninstall actually uninstalls.
CANDIDATES=""

add_candidate() {
    [ -n "$1" ] || return 0
    [ -f "$1" ] || return 0
    # Resolve to a real path so /usr/local/bin/k10s reached twice — once via
    # PATH, once via the fixed list — is not reported or deleted twice.
    dir=$(CDPATH= cd -- "$(dirname -- "$1")" && pwd -P) || return 0
    p="$dir/$(basename -- "$1")"
    case " $CANDIDATES " in
        *" $p "*) return 0 ;;
    esac
    CANDIDATES="${CANDIDATES:+$CANDIDATES }$p"
}

if [ -n "$INSTALL_DIR" ]; then
    add_candidate "$INSTALL_DIR/$BIN"
else
    # Everything on PATH, not just the first hit.
    old_ifs=$IFS
    IFS=:
    for d in $PATH; do
        IFS=$old_ifs
        add_candidate "${d:-.}/$BIN"
        IFS=:
    done
    IFS=$old_ifs

    # The directories install.sh, `go install` and Homebrew write to, in case
    # none of them is on this shell's PATH.
    for d in /usr/local/bin /opt/homebrew/bin /usr/bin \
             "${XDG_BIN_HOME:-$HOME/.local/bin}" \
             "${GOBIN:-${GOPATH:-$HOME/go}/bin}"; do
        add_candidate "$d/$BIN"
    done
fi

# What internal/config calls the config path: $K10S_CONFIG wins, otherwise
# ~/.k10s/config.yaml. With $K10S_CONFIG set, only that file is ours to
# delete — the directory around it belongs to whoever chose the path.
CONFIG_DIR=""
CONFIG_FILE=""
if [ -n "${K10S_CONFIG:-}" ]; then
    [ -f "$K10S_CONFIG" ] && CONFIG_FILE="$K10S_CONFIG"
elif [ -d "$HOME/.k10s" ]; then
    CONFIG_DIR="$HOME/.k10s"
fi
CONFIG_TARGET="${CONFIG_DIR:-$CONFIG_FILE}"

if [ -z "$CANDIDATES" ]; then
    say "${GRN}✓${R} no $BIN binary found — nothing to remove"
    if [ -n "$CONFIG_TARGET" ] && [ -z "$KEEP_CONFIG" ]; then
        say ""
        say "  Config is still there: ${B}$CONFIG_TARGET${R}"
        say "  Remove it with: ${B}rm -rf $CONFIG_TARGET${R}"
    fi
    exit 0
fi

# ---- confirm --------------------------------------------------------------

say "${B}k10s uninstall${R}"
say ""
for p in $CANDIDATES; do
    v=$("$p" --version 2>/dev/null | awk 'NR==1 {print $2}' || true)
    say "  $p${v:+  ${DIM}$v${R}}"
done
[ -n "$PURGE" ] && [ -n "$CONFIG_TARGET" ] && say "  $CONFIG_TARGET${DIM}  (config)${R}"
say ""

if [ -n "$DRY_RUN" ]; then
    say "${YLW}dry run${R} — nothing removed"
    exit 0
fi

# Piped into sh, stdin is the script itself: there is no one to answer, so
# asking would either hang or silently read a line of this file. Only prompt
# when a real terminal is attached.
if [ -z "$ASSUME_YES" ] && [ -t 0 ]; then
    printf 'Remove? [y/N] ' >&2
    read -r reply || reply=""
    case "$reply" in
        y|Y|yes|YES) ;;
        *) say "aborted"; exit 1 ;;
    esac
fi

# ---- remove ---------------------------------------------------------------

removed=0
failed=0

for p in $CANDIDATES; do
    dir=$(dirname -- "$p")
    sudo_prefix=""
    if [ ! -w "$dir" ]; then
        if [ -z "$NO_SUDO" ] && [ -t 0 ] && command -v sudo >/dev/null 2>&1; then
            sudo_prefix=sudo
            step "removing $p (needs sudo)"
        else
            # No TTY means no password prompt can be answered, so this is a
            # an instruction to the user rather than a failure to hide.
            warn "skipped $p — not writable. Remove it with:"
            say "    sudo rm $p"
            failed=$((failed + 1))
            continue
        fi
    else
        step "removing $p"
    fi
    if $sudo_prefix rm -f "$p"; then
        removed=$((removed + 1))
    else
        warn "could not remove $p"
        failed=$((failed + 1))
    fi
done

if [ -n "$PURGE" ] && [ -n "$CONFIG_TARGET" ]; then
    step "removing $CONFIG_TARGET"
    rm -rf "$CONFIG_TARGET" || warn "could not remove $CONFIG_TARGET"
fi

say ""
if [ "$removed" -gt 0 ]; then
    say "${GRN}✓${R} removed $removed binar$([ "$removed" -eq 1 ] && echo y || echo ies)"
fi
[ "$failed" -gt 0 ] && warn "$failed left in place — see the commands above"

if [ -z "$PURGE" ] && [ -n "$CONFIG_TARGET" ] && [ -z "$KEEP_CONFIG" ]; then
    say ""
    say "  Config kept: ${B}$CONFIG_TARGET${R}"
    say "  Delete it too: ${B}rm -rf $CONFIG_TARGET${R}  (or rerun with --purge)"
fi

say ""
say "  Reinstall any time: ${B}curl -fsSL https://p10node.com/k10s/install.sh | sh${R}"
say "  Bug or missing feature? https://github.com/$REPO/issues"

[ "$failed" -gt 0 ] && exit 1
exit 0
