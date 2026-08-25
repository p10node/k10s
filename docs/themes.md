# Themes

Cycle with `T` / `ctrl+t`, jump with `/theme <name>`, or click the
`theme <name> ⟳` label in the banner.

| # | Name               | Notes            |
|---|--------------------|------------------|
| 0 | `tokyo-night`      | default          |
| 1 | `catppuccin-mocha` |                  |
| 2 | `dracula`          |                  |
| 3 | `nord`             |                  |
| 4 | `gruvbox-dark`     |                  |
| 5 | `solarized-light`  | light background |
| 6 | `matrix`           | black + green    |

## Palette contract (`internal/theme/theme.go`)

```go
type Theme struct {
    Name     string
    Bg, Fg   lipgloss.Color // ground + primary text
    Subtle   lipgloss.Color // secondary text, hints
    Border   lipgloss.Color // resting borders, rules, disabled
    BorderOn lipgloss.Color // focused-pane border
    Accent   lipgloss.Color // selection bar, hotkeys, CMD caret
    Accent2  lipgloss.Color // context, counts, AI caret ✦
    Ok, Warn, Err lipgloss.Color // status + gauge thresholds
    SelBg, SelFg  lipgloss.Color // selected row
}
```

Every render derives colors from the active theme — there are no hardcoded
colors in `internal/ui`. Adding a theme = appending one struct to
`theme.Themes`; it immediately appears in the `T` cycle and `/theme`.
