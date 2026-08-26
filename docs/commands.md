# Prompt & commands

The bottom panel has two modes, toggled with `ctrl+a` or by clicking the
right-hand tag:

- **CMD ❯** — commands run; anything else is echoed as
  `$ <cli> <text>` and jumps to a resource if one is named. k10s does not
  shell out to run arbitrary kubectl commands — the echo is a read-only
  convenience, and `<cli>` is whatever name you picked in `/settings`.
- **AI ✦** — plain text goes to the configured model over HTTP; the answer
  opens as a text view in the main pane (scroll/zoom/close like describe).
  Commands still work in this mode.

## Search palette (`ctrl+p`)

One box that finds both resource kinds and individual objects. Kinds you have
opened are searched by object name; kinds not yet loaded match by name only,
because scanning their objects would mean starting a cluster-wide watch per
kind. The footer states how many kinds are in that reduced state.

`↑↓` moves, `enter` jumps to the kind (and row), `esc` closes.

Cmd+K cannot be bound to this — see [keybindings.md](keybindings.md#search).

## Two command prefixes

- **`/` — open a chooser.** Namespace, context, theme, settings, help.
- **`:` — act on what is on screen**, usually with an argument. Search,
  filter, scale, mouse capture.

Typing either prefix pre-fills it and shows only that set. `↑↓` moves the
highlight and **`enter` runs the highlighted command straight away** — no
second trip through the prompt. The exception is a command still missing its
argument (`:scale`, `:search`, `:filter`), where enter completes it instead
so the argument can be typed. `tab` always completes rather than runs.

| Command     | Args     | Effect                                                       |
|-------------|----------|--------------------------------------------------------------|
| `/ns`       |          | choose a namespace — opens the Namespaces table              |
| `/context`  |          | choose a kube context — opens a picker; switching reconnects |
| `/theme`    |          | theme picker with live preview                               |
| `/settings` |          | CLI name, AI provider **and** the update check, in one dialog |
| `/update`   | `[skip]` | check GitHub for a newer k10s and install it over this binary |
| `/version`  |          | which build is running, and what the last check found        |
| `/scale`    | `<n>`    | scale the selected deployment/statefulset                    |
| `/help`     |          | keybindings + commands text view                             |
| `:search`   | `<term>` | filter the resource list (left pane, by kind)                |
| `:filter`   | `<term>` | filter rows of the table currently open (main pane)          |
| `:mouse`    |          | toggle mouse capture — same as `ctrl+s`                      |

Unknown commands toast `unknown command … — /help lists everything`.

**None of `/ns`, `/context` or `/theme` takes a name.** Each opens a chooser
showing what is actually available.

## Growing the command box

`ctrl+z` grows the prompt to half the screen, and typing anything that is
not a `/` or `:` command grows it automatically — a kubectl line or an AI
question gets long, and a one-row field that scrolls sideways hides most of
it. The value wraps across the tall box so the whole thing is readable.
`esc` shrinks it; `esc` again leaves the prompt. There is a `[ grow ]` /
`[ shrink ]` button in the panel's top border too.

## Namespaces

The active namespace is a specific name or the sentinel `all`. There is one
route to change it, reachable two ways — the **`ns <name> ▾` button** in the
top-right of the banner, or **`/ns`**. Both open the **Namespaces table in
the main panel**, whose first row is `all`. Pressing `enter` on a row
switches to that namespace *and* jumps to its Pods, which is the usual next
step.

What changes when you switch:
- **A specific namespace** — the table shows only that namespace's rows.
- **`all`** — every row, with a NAMESPACE column prepended (accent2-colored),
  the same idea as `kubectl get pods -A`. Rows group by namespace, then name.
- Every Resources-pane badge count updates too, not just the open table.
- Non-namespaced kinds (Nodes, Namespaces, CRDs) ignore the filter entirely.
- The row cursor resets to the top, since the row set changed completely.

## CLI name (`/settings`)

k10s asks once, on first run, which command you type for Kubernetes —
`kubectl`, `k8s`, `k`, or your own — and uses it in command echoes and hints.
It is cosmetic: k10s talks to the API directly and never executes it.
Reopen the picker any time with `/settings`; it is stored as `cli:` in
[config.md](config.md).

## Self-update (`/update`)

`/update` asks GitHub for the newest release, confirms — same modal as
delete and drain, since it replaces the binary you are running — verifies
the download against the release checksums, installs it, then offers to
restart into it. `/version` reports what is running and where updates come
from without touching the network.

The same check also runs once a day at startup and only speaks up when there
is something newer, as a toast plus a clickable `⇧ 1.4.0` badge in the status
bar. Three levels of "not now":

| | |
|---|---|
| `esc` on the dialog | not this time |
| `/update skip` | stop mentioning *this* release; the check stays on |
| `/settings` → `UPDATES` → `off` | stop checking at all; `/update` still works |

Outside the TUI: `k10s update` and `k10s --version`. Full behaviour in
[update.md](update.md).

## AI settings (`/settings`)

| Field    | Notes                                                                                                       |
|----------|-------------------------------------------------------------------------------------------------------------|
| Provider | radio: **OpenAI-compatible** / **Anthropic**; switching auto-fills that provider's default Base URL + Model |
| Base URL | e.g. `https://api.anthropic.com/v1` — inline editable                                                       |
| Model    | e.g. `claude-sonnet-5` / `gpt-5` — inline editable                                                          |
| API Key  | inline editable, always displayed masked                                                                    |

`↑↓` moves, `enter` selects a radio or starts editing a field, `tab` reaches
**Save**, `esc` closes keeping what is set. Every field is persisted as soon
as you commit an edit or toggle the provider — see [config.md](config.md).

The API key field never pre-fills with the stored secret; leaving it empty
keeps the existing key rather than clearing it.

Requests are real (`internal/ai`): OpenAI-compatible posts to
`/chat/completions` with a Bearer token, Anthropic posts to `/messages` with
`x-api-key` + `anthropic-version`. The current context, namespace, resource
kind and selected object are injected into the system prompt so answers can
refer to what's on screen. Errors surface the server's own message.
