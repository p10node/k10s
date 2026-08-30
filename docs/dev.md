# Development

## Build & run

`just` (with no arguments) lists every recipe. The common ones:

```bash
just build            # → ./k10s, version stamped from git describe
just build-release    # → ./k10s, stripped (~a third smaller)
just run              # build + run — needs a TTY
just dev              # go run . without producing a binary
just version          # print the version this tree would stamp
```

`just build` passes the version stamp through `-ldflags`; `just dev` and a
bare `go build` do not, so those report `dev`. That is deliberate — `dev`
compares older than every release, so a source build can still exercise
`/update`. See [update.md](update.md).

Go 1.26+. k10s uses your current kubeconfig context, and falls back to the
offline demo backend when no cluster is reachable, so it always starts.

## Tests

```bash
just check       # fmt-check + vet + test — run this before committing
just test        # go test ./...
just test-v      # verbose, per test case
just test-one X  # go test -run X -v
just test-race   # race detector
just cover       # coverage summary + coverage.html
just test-perf   # only the performance regression guards
just bench       # the render hot-path benchmarks
```

The live backend is tested against **fake clientsets**
(`k8s.io/client-go/kubernetes/fake` and friends), not a real cluster, so the
whole suite runs offline. `newTestStore` builds a `Store` through the same
constructor production uses, with fakes swapped in.

Two things to know when writing `internal/k8s` tests:

- Informers are lazy, so a test that asserts on rows must call
  `syncKinds(t, s, kPods, …)` first — it opens those kinds and waits for the
  initial list. Production code never waits like that; the UI repaints as
  caches fill.
- Fake clientsets panic on unregistered list kinds. `countKind` recovers from
  panics for the same reason production does: a background sweep must never
  take down the TUI.

`internal/update` has one test that is skipped by default: `RealDist`
installs a real cross-compiled build, which is how the release naming and
the asset matcher are kept in step.

```bash
just release && go test ./internal/update/ -run RealDist -v
```

Performance guards are listed in [performance.md](performance.md). They fail
if the render path starts doing I/O again — treat a failure there as a real
regression, not a flaky test.

## Headless renderer — `cmd/shot`

Renders one frame without a TTY, with truecolor forced
(`lipgloss.SetColorProfile(termenv.TrueColor)`). It is always mock-backed, so
layout work never needs a cluster:

```bash
just shot                            # 140x44, main screen
just shot 140 44 "j,j,d"             # 2×down, describe
just shot 140 44 "left,sec"          # focus list, type "sec"
just shot 140 44 ":,/config,enter"   # AI settings modal
just shot 140 44 "ctrl+p,web"        # search palette
go run ./cmd/shot 120 40 "esc,f,web" # or call it directly
```

Key tokens are comma-separated. Multi-char tokens are sent as one rune batch
(handy for typing into search/prompt). Special names: `tab enter esc up down
left right pgup pgdown ctrl+a ctrl+s ctrl+p shift+tab backspace` (see the
`special` map in `cmd/shot/main.go`).

Because real actions are async `tea.Cmd`s, `shot` runs the returned command
and feeds the result back (`drain`), following the chain only while the
message is one of the UI's own async messages (`ui.IsAsyncMsg`) — otherwise a
self-perpetuating command like the cursor blink would loop forever.

**Note:** `shot` gets a throwaway `K10S_CONFIG` per run, so it always starts
as a first run — which now opens straight into the cluster, no overlay to
dismiss first.

## Width invariant

Every rendered line must be exactly the terminal width — overlays and joins
rely on it. Check with:

```bash
go run ./cmd/shot 140 44 "<keys>" | sed -e $'s/\x1b\\[[0-9;]*m//g' \
  | awk '{ if (length($0) != 140) print NR": "length($0) }'
```

(That awk counts bytes; for a strict check strip ANSI and measure display
cells — wide glyphs like `⎈` count 1 cell but multiple bytes.)

Two tests in `internal/ui/block_test.go` guard it without a terminal:

- `TestPanelTopBorderNeverExceedsItsWidth` — the top border is the only line
  whose length depends on the title and the tag, so it is checked directly
  across widths, long titles and oversized tags. A title is truncated against
  what the tag leaves behind, and a tag with no room at all is dropped rather
  than cut (it carries bubblezone markers, so truncating it would corrupt its
  click target).
- `TestLongTextTitleKeepsEveryRowAtTerminalWidth` — the same invariant through
  the real render path: `logs -f <pod>` at 100 columns, every row measured.

If a line goes long, the usual culprit is a style nested inside another
style's `Render` (the inner reset drops the outer background) — use `padBG`
and sibling runs instead, never nesting.

## Adding a resource kind

1. Add it to `builtinKinds` in `internal/k8s/kinds.go` (key, columns,
   allowed actions).
2. Register its informer in `Store.register` and add a lazy accessor in
   `listers.go`.
3. Add a `…Rows()` formatter in `rows.go` and wire it into `Rows` and
   `RowCount`.
4. Map it in `gvrFor` (for delete/YAML) and `kindToGK` (for describe).
5. Mirror it in `internal/mock/data.go` so the demo and tests cover it.

`RowCount` must not format rows or start a watch — see
[performance.md](performance.md).

## Cutting a release

```bash
just tag v1.4.0        # tags, pushes, and the workflow publishes the build
just release           # or build the same archives locally into dist/
```

`.github/workflows/release.yml` runs the tests, cross-compiles
`darwin/{amd64,arm64}`, `linux/{amd64,arm64}` and `windows/amd64`, writes
`checksums.txt`, and publishes everything with generated notes.

The archive names (`k10s_<version>_<os>_<arch>.tar.gz`, `.zip` on Windows)
and the `sha256sum`-format manifest are what the self-updater matches on and
verifies against — changing either means changing `internal/update/assets.go`
with it. `docs/update.md` has the details, and the `RealDist` test above is
what catches a mismatch.

Releases are looked for in `p10node/k10s` (`update.DefaultRepo` in
`internal/update/update.go`). Until the first tag is pushed, `/update`
reports that the repo has no published releases — that is the expected
answer, not a failure.
