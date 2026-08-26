# Architecture

## Packages

```
main.go                 entrypoint: builds the live backend, falls back to the demo
cmd/shot/               headless renderer for dev (replays keys, prints one frame)
internal/domain/        the Source interface + shared types — the whole contract
internal/k8s/           live backend: client-go informers, describe/YAML/logs/exec/pf
internal/mock/          offline demo backend, same interface, no network
internal/ai/            OpenAI-compatible / Anthropic HTTP calls
internal/theme/         Theme struct + 7 palettes
internal/config/        ~/.k10s/config.yaml load/save
internal/version/       the -ldflags build stamp (version, commit, date)
internal/update/        self-update: GitHub releases, checksums, atomic swap
internal/ui/
  block.go              Block primitive, Panel chrome
  model.go              Model state, Update: key/mouse dispatch, async commands
  view.go               View: header, list, table/text, actions, prompt
  actions.go            the action table (id, hotkey, label, risky)
  commands.go           slash commands + /help text
  msgs.go               async result messages
  update.go             /update + /version: the check, the confirm, the install
  pickers.go/_view.go   onboarding + theme picker
  palette.go/_view.go   global search palette, namespace picker
  nspicker.go           namespace picker logic
  loading.go            spinner / indeterminate progress
```

Dependencies: `bubbletea` (loop), `lipgloss` (styling), `bubbles/textinput`
(prompt + inline edits), `bubblezone` (mouse hit-testing), `x/ansi`
(width-aware truncate), `client-go` + `kubectl` (cluster access, describe).

## The Source boundary (`internal/domain`)

`domain.Source` is the only thing `internal/ui` knows about a cluster.
Both `k8s.Store` and `mock.Source` implement it, and the UI cannot tell
which it has:

```go
type Source interface {
    Kinds() []Kind
    Rows(kind, ns string) (cols []string, rows [][]string)
    RowCount(kind, ns string) int
    ClusterInfo() ClusterInfo
    Nodes() []NodeInfo
    Contexts() []string
    Namespaces() []string
    SwitchContext(name string) (Source, error)
    Describe/YAML/Logs/LogsFollow/TopPod/TopNode(...)
    Delete/Restart/Scale/Cordon/Drain/Apply(...)
    Shell(...) (tea.ExecCommand, error)
    PortForward(...) (addr string, stop func(), err error)
    Close()
}
```

Everything crosses this boundary as already-formatted `[]string` rows, so the
UI never imports a Kubernetes type and the backends never import a UI one.

**Optional capabilities** are discovered by type assertion rather than
widening the interface, so the demo backend doesn't have to fake them:

| Assertion           | Used for                                                |
|---------------------|---------------------------------------------------------|
| `Synced(kind) bool` | loading spinner, adaptive repaint, palette row-scanning |

`RowCount` may return `domain.CountUnknown` (-1) when a backend genuinely
doesn't know yet; the sidebar then draws no badge rather than a false `0`.

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
→ overlaySuggestions / overlayConfig / overlayThemePicker / overlayOnboard
  / overlayPalette / overlayNSPicker / overlayConfirm  (stamped on top)
→ zone.Scan(root.String())                 — registers mouse zones, strips markers
```

`Model.mark(id, s)` wraps content in a bubblezone marker but returns `s`
unchanged while any overlay is open (`modalOpen()`), so an overlay never
slices a marker in half.

**The render path must not do I/O.** `View` runs on every keystroke and every
repaint tick; anything that could block belongs on a background goroutine.
See [performance.md](performance.md).

## Async work

Nothing that touches the network runs inline. Actions return a `tea.Cmd` that
does the work off-thread and posts a message back (`internal/ui/msgs.go`):

| Message                          | Produced by                                  |
|----------------------------------|----------------------------------------------|
| `textResultMsg`                  | describe / YAML / logs snapshot / top / AI   |
| `actionResultMsg`                | delete, restart, scale, cordon, drain, apply |
| `ctxSwitchMsg`                   | `/context` — rebuilds the whole client       |
| `logStartMsg` / `logLineMsg`     | `LogsFollow` streaming                       |
| `editFetchedMsg` / `editExitMsg` | the `$EDITOR` round trip                     |
| `portForwardMsg`                 | port-forward start                           |
| `tickMsg`                        | repaint timer; also advances the spinner     |

`ui.IsAsyncMsg` identifies these, which is how `cmd/shot` resolves an action
chain synchronously without a real event loop.

## Mouse

Every clickable element is a named zone: `res:<i>`, `row:<i>`, `act:<id>`,
`zoom`, `close`, `theme`, `nsfield`, `prompt`, `aimode`, `tablesearch`,
`sug:<i>`, `pal:<i>`, `nsp:<i>`, `thm:<i>`, `ob:<i>`, `cf:ok`/`cf:no`,
`cfg:*`. `handleMouse` checks overlay zones first (confirm → config → theme →
onboarding → palette → namespace → suggestions), then chrome, then lists.

Two fallbacks make panes feel solid rather than only their contents:
- the wheel scrolls **and focuses** whichever pane the pointer is over
  (`scrollPaneAt` → `paneAt`);
- a click that hits no zone still focuses the pane it landed in
  (`focusPaneAt`), blank space included.

`ctrl+s` toggles mouse capture off entirely so the terminal can do its own
selection — the only way to copy text out of a mouse-capturing TUI.

## Update dispatch order (`handleKey`)

1. `ctrl+c` always quits
2. confirm modal (enter/y confirm, esc/n cancel)
3. search palette / namespace picker
4. first-run onboarding, theme picker
5. AI settings modal (incl. inline edit sub-state)
6. prompt focused → suggestions navigation, then textinput
7. resource list focused → type-to-filter
8. main-pane row search → type-to-filter, but `globalShortcut` first so
   pane switching and ctrl-shortcuts still work while typing
9. global keys, then action hotkeys
