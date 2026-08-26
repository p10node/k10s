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
  Namespaces table (enter switches and shows pods); `/ns` and `/context`
  open compact popups. Neither asks you to type a name.
- **Theme picker** — `/theme` previews live before committing.
- **First-run onboarding** — pick the CLI name shown in hints
  (`kubectl`/`k8s`/`k`/custom); changeable later with `/settings`.
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

- More kinds (ReplicaSets, HPAs, NetworkPolicies, RBAC).
- Column sorting by clicking a header.
- Multi-select for bulk delete.
- Log filtering / grep within the follow stream.
- Saved views (kind + namespace + filter) as named shortcuts.
