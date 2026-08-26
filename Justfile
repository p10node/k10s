# k10s — task runner. `just` with no arguments lists every recipe.

binary := "k10s"

# Version stamped into the binary. Defaults to the nearest git tag, so a
# local build reports the same thing a release would; `just build version=v1.2.3`
# overrides it. An unstamped build reports "dev", which the self-updater
# treats as older than every release.
version := `git describe --tags --dirty 2>/dev/null || echo dev`
commit := `git rev-parse --short HEAD 2>/dev/null || echo none`
date := `date -u +%Y-%m-%d`

# What -ldflags carries: the version stamp read by internal/version.
stamp := "-X github.com/p10node/k10s/internal/version.Version=" + version + " -X github.com/p10node/k10s/internal/version.Commit=" + commit + " -X github.com/p10node/k10s/internal/version.Date=" + date

# Platforms `just release` builds for.
platforms := "darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64"

_default:
    @just --list

# ---- build / run ----------------------------------------------------------

# Build the TUI binary.
build:
    go build -ldflags="{{stamp}}" -o {{binary}} .

# Build a stripped binary (no debug info — roughly a third smaller).
build-release:
    go build -ldflags="-s -w {{stamp}}" -o {{binary}} .

# Print the version this tree would stamp.
version:
    @echo "{{version}} ({{commit}}) {{date}}"

# Build and run the TUI. Needs a real terminal (TTY).
run: build
    ./{{binary}}

# Run straight from source, skipping the binary.
dev:
    go run .

# Render one frame headlessly (no TTY/cluster); replay keys: `just shot 140 44 j,j,d`
shot w="140" h="44" keys="":
    go run ./cmd/shot {{w}} {{h}} "{{keys}}"

# ---- test -----------------------------------------------------------------

# Run every test.
test:
    go test ./...

# Verbose test output, per test case.
test-v:
    go test ./... -v

# Run tests matching a name, e.g. `just test-one RowCount`.
test-one pattern:
    go test ./... -run '{{pattern}}' -v

# Run under the race detector (slower; catches data races).
test-race:
    go test -race ./...

# Test coverage summary, plus a browsable HTML report.
cover:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out | tail -1
    go tool cover -html=coverage.out -o coverage.html
    @echo "report → coverage.html"

# ---- perf -----------------------------------------------------------------

# Benchmark the render hot paths (sidebar counts vs full row formatting).
bench:
    go test ./internal/k8s/ -bench Benchmark -run '^$' -benchmem

# Perf guards only: startup latency, per-frame cost, clean stderr.
test-perf:
    go test ./internal/k8s/ -run 'TestRowCount|TestNewStoreReturnsFast|TestSilenceLogging' -v
    go test ./internal/ui/ -run 'TestKeypressLatency|TestViewDoesNotBuildRows' -v

# ---- quality --------------------------------------------------------------

# Format every Go file in place.
fmt:
    gofmt -w .

# Fail if anything is unformatted (for CI).
fmt-check:
    @out=$(gofmt -l .); if [ -n "$out" ]; then echo "unformatted:"; echo "$out"; exit 1; fi

vet:
    go vet ./...

# Everything CI should enforce, in the order that fails fastest.
check: fmt-check vet test

# Tidy go.mod / go.sum.
tidy:
    go mod tidy

# ---- release --------------------------------------------------------------

# Cross-compile every platform into dist/, with a checksums.txt beside them.
release:
    #!/usr/bin/env bash
    # Archive names are what internal/update matches on:
    # k10s_<version>_<os>_<arch>.{tar.gz,zip}. Override the version with
    # `just --set version v1.2.3 release`.
    set -euo pipefail
    rm -rf dist && mkdir -p dist
    for p in {{platforms}}; do
      os="${p%/*}"; arch="${p#*/}"
      bin={{binary}}; [ "$os" = windows ] && bin={{binary}}.exe
      echo "→ $os/$arch"
      GOOS=$os GOARCH=$arch CGO_ENABLED=0 \
        go build -trimpath -ldflags="-s -w {{stamp}}" -o "dist/$bin" .
      name="{{binary}}_{{version}}_${os}_${arch}"
      if [ "$os" = windows ]; then
        (cd dist && zip -q "$name.zip" "$bin" && rm "$bin")
      else
        (cd dist && tar czf "$name.tar.gz" "$bin" && rm "$bin")
      fi
    done
    # sha256sum on Linux, shasum on macOS — same format either way, and it
    # is what internal/update's verifier parses. Globbing the archives only,
    # so the manifest doesn't list itself.
    (cd dist && (sha256sum {{binary}}_* 2>/dev/null || shasum -a 256 {{binary}}_*) > checksums.txt)
    ls -lh dist

# Tag this commit and push it; the release workflow publishes the build.
tag ver:
    git tag -a {{ver}} -m "k10s {{ver}}"
    git push origin {{ver}}

# ---- demo -----------------------------------------------------------------

# Keyboard only — vhs cannot record a mouse cursor, so the hero clip that
# shows clicking has to be a real screen capture. See docs/build-in-public.md.

# Record the reproducible demo GIF (needs charmbracelet/vhs + k10s on PATH).
demo: install
    vhs assets/demo.tape

# Needs charmbracelet/vhs, imagemagick and a JetBrainsMono Nerd Font
# (`brew install vhs imagemagick && brew install --cask font-jetbrains-mono-nerd-font`).
# Runs against the offline demo backend, so no real cluster ends up in the
# docs. The magick pass palettes the PNG — a terminal frame has ~250 colours,
# so it costs nothing visible and cuts the file to a third.

# Capture the README hero: a real terminal frame of the running TUI.
screenshot: build
    rm -f /tmp/k10s-screenshot-config.yaml
    vhs assets/screenshot.tape
    magick assets/screenshot.png -colors 256 PNG8:assets/screenshot.png
    @ls -lh assets/screenshot.png

# ---- logo -----------------------------------------------------------------

# Regenerate every logo SVG and re-export the PNGs. Needs python3 + rsvg-convert
# (`brew install librsvg`). The SVGs are generated, never hand-edited.
logo:
    python3 assets/logo/generate.py
    for s in 1024 512 256 128 64; do rsvg-convert -w $s assets/logo/mark.svg -o assets/logo/mark-$s.png; done
    for s in 180 48 32 16; do rsvg-convert -w $s assets/logo/favicon.svg -o assets/logo/favicon-$s.png; done
    rsvg-convert -w 904 assets/logo/wordmark-on-dark.svg -o assets/logo/wordmark-on-dark.png
    rsvg-convert -w 904 -b '#ffffff' assets/logo/wordmark-on-light.svg -o assets/logo/wordmark-on-light.png
    rsvg-convert -w 1280 assets/logo/social-preview.svg -o assets/logo/social-preview.png

# ---- misc -----------------------------------------------------------------

# Remove build, release and coverage artifacts.
clean:
    rm -f {{binary}} coverage.out coverage.html
    rm -rf dist

# Install the binary into GOBIN (or ~/go/bin), version stamp included.
install:
    go install -ldflags="{{stamp}}" .
