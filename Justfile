# k10s — task runner. `just` with no arguments lists every recipe.

binary := "k10s"

_default:
    @just --list

# ---- build / run ----------------------------------------------------------

# Build the TUI binary.
build:
    go build -o {{binary}} .

# Build a stripped binary (no debug info — roughly a third smaller).
build-release:
    go build -ldflags="-s -w" -o {{binary}} .

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

# ---- misc -----------------------------------------------------------------

# Remove build and coverage artifacts.
clean:
    rm -f {{binary}} coverage.out coverage.html

# Install the binary into GOBIN (or ~/go/bin).
install:
    go install .
