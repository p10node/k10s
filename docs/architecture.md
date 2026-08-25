# Architecture

## Packages

```
main.go                 entrypoint: zone.NewGlobal, tea.NewProgram(AltScreen, MouseCellMotion)
cmd/shot/               headless renderer for dev (replays keys, prints one frame)
internal/theme/         Theme struct + 7 palettes
internal/mock/          fake cluster data, canned describe/logs/yaml/AI/help text
internal/ui/
  block.go              Block primitive, Panel chrome
  model.go              Model state, Update: key/mouse dispatch, commands
  view.go               View: header, list, table/text, actions, prompt, overlays
```

Dependencies: `bubbletea` (loop), `lipgloss` (styling), `bubbles/textinput`
(prompt + inline edits), `bubblezone` (mouse hit-testing), `x/ansi`
(width-aware truncate).

## Block primitive (`internal/ui/block.go`)

The whole frame is composed from `Block{W, H, Lines}` rectangles instead of
`lipgloss.JoinHorizontal`:

- **Invariant: every line is padded to exactly W visible cells.** Joins
  (`HJoin`, `VJoin`) then never re-measure, which matters because bubblezone
  embeds invisible markers that break naive width math.
- `padBG` pads with an independently-rendered background run — styles are
  never nested, so a nested SGR reset can't drop the outer background.
- `Overlay(o, x, y)` stamps a block onto another (modals, popups) using
  `ansi.Truncate`/`ansi.TruncateLeft` to slice styled lines safely.
- `Panel` draws the border manually so the title sits in the top border and a
  right-aligned tag (`[ zoom ]`, `[ CMD ]`, …) can live there too.
  `PanelOpts.BorderCol` overrides border color (danger modals).

## Render pipeline (`View`)

```
viewHeader (borderless banner)
HJoin(viewList, viewMain, viewActions)     — or viewMain alone when zoomed
viewPrompt
viewStatus
→ overlaySuggestions / overlayConfig / overlayConfirm (stamped on top)
→ zone.Scan(root.String())                 — registers mouse zones, strips markers
```

`Model.mark(id, s)` wraps content in a bubblezone marker but returns `s`
unchanged while a modal is open, so an overlay never slices a marker in half.

## Mouse

Every clickable element is a named zone: `res:<i>`, `row:<i>`, `act:<id>`,
`zoom`, `close`, `theme`, `prompt`, `aimode`, `searchbox`, `sug:<i>`,
`cf:ok`/`cf:no`, `cfg:*`. `handleMouse` checks modal zones first (confirm →
config → suggestions), then chrome, then lists. Wheel scrolls the focused pane.

## Update dispatch order (`handleKey`)

1. `ctrl+c` always quits
2. confirm modal (enter/y confirm, esc/n cancel)
3. AI settings modal (incl. inline edit sub-state)
4. prompt focused → textinput
5. resource list focused → type-to-filter (arrows navigate, esc clears)
6. global keys, then action hotkeys
