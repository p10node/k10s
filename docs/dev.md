# Development

## Build & run

```bash
go build ./...          # go 1.24+ (module pins bubbletea v1.3, lipgloss v1.1)
go run .                # real TUI — needs a TTY; alt screen + mouse enabled
go vet ./...
```

Note: bubblezone is pinned to `v1.0.0` (proxy-resolvable; this machine's git
rewrites github https → ssh, so pseudo-versions that bypass the proxy fail).

## Headless renderer — `cmd/shot`

Renders one frame without a TTY, with truecolor forced
(`lipgloss.SetColorProfile(termenv.TrueColor)`):

```bash
go run ./cmd/shot <width> <height> "<keys>"
go run ./cmd/shot 140 44 ""                    # main screen
go run ./cmd/shot 140 44 "j,j,d"               # 2×down, describe
go run ./cmd/shot 140 44 "left,sec"            # focus list, type "sec"
go run ./cmd/shot 140 44 ":,/config,enter"     # AI settings modal
go run ./cmd/shot 140 44 "ctrl+a,hello,enter"  # AI answer view
```

Key tokens are comma-separated. Multi-char tokens are sent as one rune batch
(handy for typing into search/prompt). Special names: `tab enter esc up down
left right pgup pgdown ctrl+a backspace` (see `special` map in
`cmd/shot/main.go`).

## Width invariant

Every rendered line must be exactly the terminal width — overlays and joins
rely on it. Check with:

```bash
go run ./cmd/shot 140 44 "<keys>" | sed -e $'s/\x1b\\[[0-9;]*m//g' \
  | awk '{ if (length($0) != 140) print NR": "length($0) }'
```

(That awk counts bytes; for a strict check strip ANSI and measure display
cells — wide glyphs like `⎈` count 1 cell but multiple bytes.)

If a line goes long, the usual culprit is a style nested inside another
style's `Render` (the inner reset drops the outer background) — use `padBG`
and sibling runs instead, never nesting.

## Review page

The "k10s" artifact (mock-v2) is generated from shot captures: ANSI →
HTML spans, one `<figure>` per scene, all 7 themes. Regenerate by re-running
the captures and the converter (scratchpad scripts `ans2html.py`,
`build_page.py`) and republishing the same file path.
