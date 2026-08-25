# k10s

Clickable Kubernetes TUI — Bubble Tea + Lip Gloss + BubbleZone.

**Status: MOCK v4.** All data is fake (`internal/mock`). No cluster API calls yet.

Full docs in [docs/](docs/README.md) — architecture, UI, keybindings,
commands, themes, mock data, dev workflow, roadmap.

## Run

```bash
go run .                            # real TUI (TTY, mouse enabled)
go run ./cmd/shot 140 44 ""         # headless render to stdout
go run ./cmd/shot 140 44 "j,j,d"    # replay keys before rendering
```

## Layout

```
TOP BANNER (no border) — context · ver · ns · nodes  /  total CPU + MEM gauges
┌ Resources ─┐┌ Pods · default ──────[ zoom ]┐┌ Actions ┐
│ ▸ Pods     ││ NAME READY STATUS ...        ││ [d] …   │
│ / search   ││ / row search…                ││ [D] Del │
└────────────┘└──────────────────────────────┘└─────────┘
┌ Command / Prompt ───────────────────[ CMD | AI · model ]┐
│ ❯ / ✦ _                                                 │
└─────────────────────────────────────────────────────────┘
status bar
```

## Keys

| Key                             | Action                                                       |
|---------------------------------|--------------------------------------------------------------|
| `tab` / `shift+tab`             | cycle panes                                                  |
| `↑↓` (`j` `k` in main)          | move · `pgup/pgdn`, `g`/`G`                                  |
| type (Resources focused)        | instant kind search/filter · `esc` clears                    |
| `/` (Main focused, table mode)  | search rows of the open table · `esc` clears                 |
| `z`                             | zoom / restore center pane                                   |
| `:` / `/` (elsewhere)           | open prompt (slash suggestions popup)                        |
| `ctrl+a`                        | toggle AI prompt mode ✦                                      |
| `T` / `ctrl+t`                  | cycle themes                                                 |
| `d y l s f r c e`               | describe · yaml · logs · shell · pf · restart · scale · edit |
| `m`                             | top / metrics (pods, nodes)                                  |
| `o`                             | cordon / uncordon (nodes)                                    |
| `u`                             | drain (nodes, confirm modal)                                 |
| `D`                             | delete (red confirm modal)                                   |
| `q` (outside search) / `ctrl+c` | quit                                                         |

## Slash commands

`/context [name]` · `/ns [name]` (try `all`) · `/theme [name]` · `/config`
(AI provider: OpenAI-compatible or Anthropic, base URL, model, API key) ·
`/ai <prompt>` · `/search <term>` (resource kinds) · `/filter <term>` (table
rows) · `/crd` · `/dr` · `/help`

## Structure

```
main.go                 entrypoint
cmd/shot/               headless renderer (dev)
internal/theme/         palettes (7 themes)
internal/mock/          fake data + describe/logs/yaml/AI answers
internal/config/        ~/.k10s/config.yaml load/save
internal/ui/block.go    Block primitives + Panel (border+title+tag)
internal/ui/model.go    state, key/mouse handling
internal/ui/view.go     header / list / table / actions / prompt / modals
```

Every Block is padded to exactly W display cells, so horizontal joins and
modal overlays never drift — even with BubbleZone markers embedded.

## Config

Theme, context, namespace and AI settings (provider/URL/model/key) persist to
`~/.k10s/config.yaml` (override with `K10S_CONFIG=path`) — saved on every
change, loaded on startup. Details in [docs/config.md](docs/config.md).

## Next (after mock approval)

1. client-go + informer cache → replaces `internal/mock`, auto refresh.
2. Streams: logs follow, exec PTY, port-forward.
3. Real AI calls per `/config`; cluster context injected into prompts.
4. ~~Persist theme/pane/AI settings~~ — done, see [docs/config.md](docs/config.md).
