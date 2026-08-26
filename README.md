# k10s

Clickable Kubernetes TUI — Bubble Tea + Lip Gloss + BubbleZone.

**Status: live.** `k10s` talks to your current kubeconfig context via
client-go (informer-backed listing, real describe/YAML/logs — including
`-f` follow — exec, port-forward, and the mutating actions) and falls back
to the offline demo in `internal/mock` when no cluster is reachable.

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
| `f` (Main focused, table mode)  | find: search rows of the open table · `esc` clears           |
| `z`                             | zoom / restore center pane                                   |
| `:` / `/`                       | open the command prompt (slash suggestions popup)            |
| `ctrl+a`                        | toggle AI prompt mode ✦                                      |
| `T` / `ctrl+t`                  | cycle themes · `/theme` opens a live-preview picker          |
| `ctrl+p`                        | search everything — kinds and objects in one box             |
| `enter`                         | open the item: logs if it has them, otherwise describe       |
| `ctrl+s`                        | copy-mode: release the mouse so you can select & copy        |
| `d y l s p r c e`               | describe · yaml · logs · shell (in-panel) · pf · restart · scale · edit |
| `m`                             | top / metrics (pods, nodes)                                  |
| `o` / `u`                       | cordon-uncordon / drain — shown only when Nodes is selected  |
| `D`                             | delete (red confirm modal)                                   |
| click / wheel                   | anywhere in a pane selects it, blank space included          |
| `q` (outside search) / `ctrl+c` | quit                                                         |

## Commands

Two prefixes: **`/` does something**, **`:` narrows what's on screen**.
Typing either shows only its own set, and `enter` runs the highlighted
command straight away.

`/ns` · `/context` · `/theme` · `/settings` (CLI name + AI provider) ·
`/help`

`:search <term>` · `:filter <term>` · `:scale <n>` · `:mouse`

## Structure

```
main.go                 entrypoint — builds the real k8s.Store, falls back to mock.Source
cmd/shot/               headless renderer (dev) — always mock-backed
internal/domain/        Source interface + shared types (Kind, ClusterInfo, ...)
internal/k8s/           real backend: client-go informers, describe/YAML/logs/exec/pf/actions
internal/mock/          offline demo backend implementing the same Source interface
internal/ai/            OpenAI-compatible / Anthropic HTTP calls for AI mode
internal/theme/         palettes (7 themes)
internal/config/        ~/.k10s/config.yaml load/save
internal/ui/block.go    Block primitives + Panel (border+title+tag)
internal/ui/model.go    state, key/mouse handling, async action plumbing
internal/ui/view.go     header / list / table / actions / prompt / modals
```

Every Block is padded to exactly W display cells, so horizontal joins and
modal overlays never drift — even with BubbleZone markers embedded.

## Config

Theme, context, namespace and AI settings (provider/URL/model/key) persist to
`~/.k10s/config.yaml` (override with `K10S_CONFIG=path`) — saved on every
change, loaded on startup. Details in [docs/config.md](docs/config.md).

## Done

1. ~~client-go + informer cache → replaces `internal/mock`, auto refresh.~~
   Both backends now implement `domain.Source`; `main.go` picks the real one
   when a cluster is reachable, `mock.Source` otherwise.
2. ~~Streams: logs follow, exec PTY, port-forward.~~ `l` follows logs live,
   `s` opens a real interactive exec session (raw TTY, resize-aware), `f`
   starts/stops a real port-forward.
3. ~~Real AI calls per `/settings`~~ — `internal/ai` calls the OpenAI-compatible
   or Anthropic endpoint with cluster/resource context injected.
4. ~~Persist theme/pane/AI settings~~ — done, see [docs/config.md](docs/config.md).

## Notes

- `d`/`y`/`m` (describe/YAML/top) and the mutating actions (delete, rollout
  restart, `/scale`, cordon, drain, edit) go through the real Kubernetes API
  when connected; `c` (scale) now opens the prompt pre-filled with
  `/scale <n>` instead of just toasting.
- `cmd/shot` (the headless dev renderer) is always mock-backed, so layout
  iteration never needs a live cluster.
