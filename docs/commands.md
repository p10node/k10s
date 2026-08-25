# Prompt & commands

The bottom panel has two modes, toggled with `ctrl+a` or by clicking the
right-hand tag:

- **CMD ❯** — plain text is treated as a kubectl command (mock: echoes it and
  jumps to a resource if one is named). Slash commands work.
- **AI ✦** — plain text goes to the configured model; the answer opens as a
  text view in the main pane (scroll/zoom/close like describe). Slash
  commands still work in this mode.

## Slash commands

Typing `/` shows a suggestion popup filtered by prefix; click a row to fill.
Note: typing bare `/` (no `:` first) while the main pane is focused on a
table instead activates that table's own row-search box — see
[ui.md](ui.md#main-pane-center). Prefix with `:` to always reach the prompt
regardless of focus.

| Command    | Args       | Effect                                                  |
|------------|------------|----------------------------------------------------------|
| `/context` | `[name]`   | switch kube context — no arg cycles the known list      |
| `/ns`      | `[name]`   | switch namespace — `all` shows every namespace at once; no arg cycles |
| `/theme`   | `[name]`   | switch theme — no arg cycles                             |
| `/config`  |            | open AI settings modal                                   |
| `/ai`      | `<prompt>` | one-shot AI question regardless of mode                  |
| `/search`  | `<term>`   | filter the resource list (left pane, by kind)             |
| `/filter`  | `<term>`   | filter rows of the table currently open (main pane)        |
| `/crd`     |            | jump to CustomResourceDefinitions                         |
| `/dr`      |            | jump to Custom Resource instances                          |
| `/help`    |            | keybindings + commands text view                          |

Unknown commands toast `unknown command … — /help lists everything`.

## Namespaces

`mock.Cluster.Namespace` is one of: a specific namespace name, or the
sentinel `all`. `/ns` (no argument) cycles through every namespace the mock
knows about — `default`, `kube-system`, `monitoring`, `staging`, `argocd`,
`cert-manager` — and ends on `all`; `/ns <name>` jumps straight there,
case-insensitively matching `all`.

What changes when you switch:
- **A specific namespace** (including `default`) — the table shows only rows
  tagged with that namespace. Most kinds only have data under `default`;
  `kube-system`/`monitoring`/`staging` show a handful of cross-namespace
  extras (coredns, grafana, a staging deploy in CrashLoopBackOff…) added
  specifically to make switching namespaces show something different.
  `argocd`/`cert-manager` only have Custom Resource instances.
- **`all`** — every row from every namespace, with a NAMESPACE column
  prepended (accent2-colored) — same idea as `kubectl get pods -A`.
- Every Resources-pane badge count updates too, not just the open table.
- Non-namespaced kinds (Nodes, Namespaces, CRDs) ignore the filter entirely.

This is real filtering over mock data (see mock-data.md), not just a label —
switching namespace changes which rows exist, matching how the real thing
will behave once client-go is wired in.

## AI settings (`/config`)

| Field    | Notes                                                                                                       |
|----------|-------------------------------------------------------------------------------------------------------------|
| Provider | radio: **OpenAI-compatible** / **Anthropic**; switching auto-fills that provider's default Base URL + Model |
| Base URL | e.g. `https://api.anthropic.com/v1` — inline editable                                                       |
| Model    | e.g. `claude-sonnet-5` / `gpt-5` — inline editable                                                          |
| API Key  | inline editable, always displayed masked (`sk-ant-api••••7f2a`)                                             |

Every field is persisted to `~/.k10s/config.yaml` as soon as you commit an
edit (enter) or toggle the provider — see [config.md](config.md). The AI
*answer* itself is still mock-only (`mock.AIAnswer`, no network call);
persistence covers settings, not inference.
