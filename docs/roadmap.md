# Roadmap

## Done

### Real cluster access

1. **client-go + informer cache** — `internal/k8s` replaced the mock as the
   default backend. Both implement `domain.Source`; the UI is unchanged and
   untouched by which one it has. See [backends.md](backends.md).
2. **Streams** — logs follow (`l`), a real interactive exec session with a
   raw resize-aware TTY (`s`), and real SPDY port-forward (`p`).
3. **Real AI** — `internal/ai` posts to the OpenAI-compatible or Anthropic
   endpoint per `/settings`, with cluster context injected into the prompt.
4. **Config file** — see [config.md](config.md).
5. **Self-update** — `internal/update` installs the newest GitHub release
   over the running binary: checksum-verified, atomic rename, offer to
   restart. A once-a-day startup check reports a newer version and nothing
   else. `internal/version` carries the `-ldflags` stamp. See
   [update.md](update.md).

### Performance

The first live version was unusably slow. Fixed by lazy per-kind informers,
a render path that does no I/O, and cheap background counts — startup went
from seconds to microseconds. The causes and the regression guards are
written up in [performance.md](performance.md), because each fix is easy to
undo by accident.

### UX

- **Search** — `ctrl+p` palette over kinds and objects; `f` finds rows in
  the open table; the resource list is type-to-filter. Neither side pane
  spends rows on a permanent search box any more.
- **Namespace / context pickers** — `ns … ▾` in the banner opens the
  Namespaces table (enter switches and shows pods); `:ns` and `:ctx`
  open compact popups. Neither asks you to type a name.
- **Theme picker** — `/theme` previews live before committing.
- **No setup screen** — first run opens the cluster. The CLI name shown in
  hints (`kubectl`/`k8s`/`k`/custom) defaults to `kubectl` and is changed
  with `/settings`.
- **Copy mode** — `ctrl+s` releases the mouse so the terminal can select
  text, the only way to copy out of a mouse-capturing TUI.
- **Table** — dim row numbers in the gutter, natural A→Z sort by default
  (events stay newest-first), an honest loading state instead of
  "no resources found".
- **Actions pane** — lists only the actions that apply to the selected kind.
- **Panes** — clicking blank space or scrolling selects a pane; `enter` or a
  double-click opens logs, falling back to describe.
- **Command prefixes** — `/` for the cluster, `:` for k10s itself, each with
  its own popup listing exactly what it can do.

## Known limits

- **Cmd+K cannot be bound.** macOS terminals consume Cmd themselves and never
  write it to the TTY; bubbletea has no Super/Cmd key at all. iTerm2, WezTerm
  and Ghostty can be configured to *send* `\x10` (ctrl+p) for Cmd+K;
  Terminal.app cannot.
- **The palette's object search covers loaded kinds only.** Kinds not yet
  opened match by name. Searching their objects would mean watching the whole
  cluster — see [performance.md](performance.md).
- **Updates are checksum-verified, not signature-verified.** `checksums.txt`
  proves the download matches what the release published; it proves nothing
  about who published it. Shipping a public key and checking a cosign or
  minisign signature is the fix, and is worth doing before this is used
  anywhere that matters.
- **No releases published yet.** Updates come from `p10node/k10s`
  (`update.DefaultRepo`), but this tree has no tags, so `/update` reports
  "no published releases" until `just tag v0.1.0` pushes one. A fork can
  point elsewhere with `K10S_UPDATE_REPO` / `update.repo`.
- **The API key is stored as plain text** in `~/.k10s/config.yaml` (mode
  0600), masked only in the UI. Swapping in an OS keychain behind the same
  `AI.APIKey` field is the fix if that's not acceptable.
- **Untested against a real cluster by its author.** Every live path is
  implemented against the real client-go/kubectl APIs and covered by tests
  using fake clientsets, but no run against an actual API server has been
  performed here. Smoke-test on a `kind` cluster before trusting it in prod.

## Open decisions

| # | Question                        | Current        |
|---|---------------------------------|----------------|
| 1 | Pane ratio — widen either side? | 22 / auto / 24 |
| 2 | Default theme                   | tokyo-night    |

## Possible next steps

The backlog now lives in [plan.md](plan.md) as task cards — each one carries
its own goal, files, design notes and acceptance criteria, so it can be picked
up (by a person or an agent) without further context. Twenty-six cards across
four phases:

- **P0 — trust & correctness.** A kind-cluster e2e suite, a container picker
  (logs currently always read `Containers[0]`, which is the wrong container on
  any pod with a sidecar), per-object events, CLI flags, and a read-only mode.
- **P1 — daily driver.** Column sort, multi-select bulk actions, log
  grep/`--previous`, a port-forward manager, secret decode, saved views.
- **P2 — differentiators.** An owner tree, an AI that actually reads describe
  and events (with redaction before anything leaves the machine), a pulse
  dashboard, `can-i`, diff-before-apply, Helm.
- **P3 — security, polish, distribution.** Signature verification, keychain,
  custom keybindings and columns, export, packaging.
