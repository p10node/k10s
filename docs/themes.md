# Themes

`T` / `ctrl+t` cycles forward/backward. `/theme` and clicking the
`theme <name> ⟳` label in the banner open the **picker**.

The picker shows each theme with a swatch of its own colors and applies the
highlighted one **immediately**, so you judge it on the real UI rather than
from a name. `tab` reaches the Save button, `enter` commits from anywhere,
and `esc` restores whatever was active before you opened it.

| # | Name               | Notes            |
|---|--------------------|------------------|
| 0 | `tokyo-night`      | default          |
| 1 | `catppuccin-mocha` |                  |
| 2 | `dracula`          |                  |
| 3 | `nord`             |                  |
| 4 | `gruvbox-dark`     |                  |
| 5 | `solarized-light`  | light background |
| 6 | `matrix`           | black + green    |

## Custom themes

k10s loads custom `.yaml` and `.yml` files at startup and appends every valid
theme to the normal theme picker and `T` / `ctrl+t` cycle. The default folder
is `~/.k10s/themes` (the `themes` folder beside `config.yaml`). If
`K10S_CONFIG` points elsewhere, the folder follows it; `K10S_THEME_DIR`
overrides the theme folder directly.

### Install the demo theme

From a cloned repository on macOS or Linux:

```bash
mkdir -p ~/.k10s/themes
install -m 0644 examples/themes/rose-pine.yaml ~/.k10s/themes/rose-pine.yaml
k10s
```

Without cloning:

```bash
mkdir -p ~/.k10s/themes
curl -fsSL https://raw.githubusercontent.com/p10node/k10s/main/examples/themes/rose-pine.yaml \
  -o ~/.k10s/themes/rose-pine.yaml
k10s
```

On Windows PowerShell:

```powershell
New-Item -ItemType Directory -Force "$HOME/.k10s/themes" | Out-Null
Invoke-WebRequest `
  https://raw.githubusercontent.com/p10node/k10s/main/examples/themes/rose-pine.yaml `
  -OutFile "$HOME/.k10s/themes/rose-pine.yaml"
k10s
```

Open `/theme`, preview `rose-pine`, then press `enter` to save it. Themes are
read at startup, so restart k10s after adding or editing a file. To uninstall
one, delete its file and restart; if it was selected, k10s safely falls back
to `tokyo-night`.

### Create a theme

Copy [the demo](../examples/themes/rose-pine.yaml), give it a unique name,
and replace the colors:

```yaml
name: my-theme
bg: "#101828"
fg: "#f2f4f7"
subtle: "#98a2b3"
border: "#344054"
border_on: "#53b1fd"
accent: "#53b1fd"
accent2: "#b692f6"
ok: "#32d583"
warn: "#fec84b"
err: "#f97066"
sel_bg: "#1d2939"
sel_fg: "#ffffff"
```

All fields are required. Colors must use quoted `#RRGGBB` values. Names must
start with a lowercase letter or number and contain only lowercase letters,
numbers, `-`, or `_`. Names must not duplicate a built-in or another custom
theme. Parsing is strict: a bad or unknown field is reported in the status
bar, that file is skipped, and other valid custom themes still load.

### Palette contract (`internal/theme/theme.go`)

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
colors in `internal/ui`. Built-in themes live in `theme.Themes`; users add the
same palette as YAML without recompiling. Both immediately appear in the `T`
cycle and `/theme`, and the picker generates its swatch from the palette.
