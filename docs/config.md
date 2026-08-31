# Config file

`internal/config` persists settings to `~/.k10s/config.yaml` (override with
`K10S_CONFIG=/path/to/file`, used by `cmd/shot` in tests so it never touches
a real user's file).

Custom themes live in the adjacent `themes/` folder (normally
`~/.k10s/themes`) and can be moved independently with `K10S_THEME_DIR`. Theme
files are documented in [themes.md](themes.md); only the selected theme name
is stored in `config.yaml`.

Command plugins are separate too: `~/.k10s/plugins.yaml` and YAML snippets
under `~/.k10s/plugins/`. They use the k9s plugin format and are documented in
[plugins.md](plugins.md); plugin definitions are never rewritten by k10s.

## What's saved

```yaml
# k10s configuration — edited in-app via /settings, /theme, T, :ns, :ctx
theme: "dracula"
context: "prod-eks-apse1"
namespace: "default"
cli: "k"
clis: "kubectl,k8s,k"
onboarded: true
collapsed: "Config,Storage,RBAC"
ai:
  provider: "anthropic"
  base_url: "https://api.anthropic.com/v1"
  model: "claude-sonnet-5"
  api_key: "sk-ant-api03-••••••••••••7f2a"
update:
  disabled: false
  repo: ""
  last_check: 1772064000
  skip: ""
```

Written with a hand-rolled flat-YAML reader/writer (`render`/`parse` in
`internal/config/config.go`) — the format is simple enough (top-level scalars
plus the nested `ai:` and `update:` blocks, all double-quoted) that pulling
in a YAML library isn't worth it. Values round-trip through Go string quoting only;
nothing fancier (lists, multi-line, anchors) is needed or supported.

File permissions are `0600` since the file can hold an API key; the parent
directory `~/.k10s` is created with `0755` on first save.

`collapsed` lists the Resources-pane groups folded away. It is written on
every save, and **`"-"` means "none, deliberately"** — an empty value would
be indistinguishable from a config written before the key existed, and k10s
would fold the default groups (`Config`, `Storage`, `RBAC`) back over
somebody who had just opened them. A file without the key at all still gets
those defaults, which is what makes the first run after an upgrade sensible.

## When it saves

Every mutation that should survive a restart calls `Model.saveConfig()`
immediately — there's no explicit "save" step and no dirty-flag debounce:

| Trigger                                                     | Field(s) saved                                                               |
|-------------------------------------------------------------|------------------------------------------------------------------------------|
| `T` / `ctrl+t` / click theme label                          | `theme`                                                                      |
| `:ctx <name>` / the context picker                          | `context` (as the namespace's address — see below)                           |
| `:ns <name>`                                                | `namespace`                                                                  |
| `/settings` → toggle provider (←→/click/enter)              | `ai.provider`, `ai.base_url`, `ai.model` (reset to that provider's defaults) |
| `/settings` → edit Base URL / Model / API Key, then `enter` | the edited field                                                             |
| `/theme` picker → Save / `enter`                            | `theme`                                                                      |
| `/settings` → Save                                          | `cli`                                                                        |
| namespace picker (click `ns …` in the banner)               | `namespace`                                                                  |
| folding a Resources group (`space`, `left`, click a header) | `collapsed`                                                                  |
| `/settings` → toggle the update check                       | `update.disabled`                                                            |
| a successful update check (startup or `/update`)            | `update.last_check`                                                          |

A failed save (e.g. unwritable home directory) surfaces as a status-bar
toast — `config save failed: …` — and never blocks the UI.

## Load on startup

`New()` calls `loadConfig()` once: matches `theme` against
the combined built-in and custom theme names, applies `cli` and (for the matching context) the
`namespace` directly, and fills the AI config (provider/url/model/key) from
whatever is present. A missing file is not an error — the built-in defaults
(tokyo-night, the kubeconfig context's own namespace, `kubectl`, Anthropic
preset) apply.

`context` is **not** a "connect here" pin. k10s always opens on
**kubeconfig's current-context** — the cluster the `kubectl` in the next
terminal is talking to. A TUI that quietly opened a different cluster than
the shell beside it is a good way to run the right command in the wrong
place, and switching context inside k10s does not write to kubeconfig, so
the two would drift apart with nothing on screen saying so.

What the key records is **which context the saved namespace belongs to**.
On startup the namespace is restored only when `context` matches the
context being opened; on any other cluster that namespace might not even
exist, so the context's own default namespace wins and nothing is pinned.
While a context switch is in flight the pair is written against the context
being switched *to*, so quitting mid-switch doesn't leave the two halves
pointing at different clusters.

To open a different cluster, change kubeconfig (`kubectl config
use-context …`) — or switch inside k10s with `:ctx <name>` for the session.

`onboarded` records that k10s has been run before. There is no first-run
dialog any more — every setting has a working default, so a form in front of
the cluster was only something to dismiss — and all this flag decides is
whether the status bar mentions `/settings` once on the very first launch.
It is written as soon as that launch happens.

The `update:` block is read the same way, and the field is deliberately
spelled `disabled` rather than `enabled`: absent means the check runs, so a
config file written before the updater existed still gets it, and the zero
value is the useful default. `last_check` throttles the startup check to once
a day; `repo` overrides where releases come from; `skip` silences one version
without turning the check off. Details in [update.md](update.md).

## Current limits

- `last_check` is a plain timestamp with no jitter, so a fleet of machines
  that all launched together will all re-check together. Harmless at this
  scale; it is the thing to change if the anonymous rate limit ever bites.
- The API key sits in the file as plain text (masked only in the UI). If
  that's not acceptable once this is wired to a real key, the fix is to swap
  `api_key` for a keychain reference (macOS Keychain / libsecret / Windows
  Credential Manager) behind the same `AI.APIKey` field.
