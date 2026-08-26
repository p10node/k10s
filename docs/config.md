# Config file

`internal/config` persists settings to `~/.k10s/config.yaml` (override with
`K10S_CONFIG=/path/to/file`, used by `cmd/shot` in tests so it never touches
a real user's file).

## What's saved

```yaml
# k10s configuration — edited in-app via /config, /settings, T, /ns, /context
theme: "dracula"
context: "prod-eks-apse1"
namespace: "default"
cli: "k"
clis: "kubectl,k8s,k"
onboarded: true
ai:
  provider: "anthropic"
  base_url: "https://api.anthropic.com/v1"
  model: "claude-sonnet-5"
  api_key: "sk-ant-api03-••••••••••••7f2a"
```

Written with a hand-rolled flat-YAML reader/writer (`render`/`parse` in
`internal/config/config.go`) — the format is simple enough (top-level scalars
plus one nested `ai:` block, all double-quoted) that pulling in a YAML
library isn't worth it. Values round-trip through Go string quoting only;
nothing fancier (lists, multi-line, anchors) is needed or supported.

File permissions are `0600` since the file can hold an API key; the parent
directory `~/.k10s` is created with `0755` on first save.

## When it saves

Every mutation that should survive a restart calls `Model.saveConfig()`
immediately — there's no explicit "save" step and no dirty-flag debounce:

| Trigger                                                     | Field(s) saved                                                               |
|-------------------------------------------------------------|------------------------------------------------------------------------------|
| `T` / `ctrl+t` / click theme label                          | `theme`                                                                      |
| `/context [name]`                                           | `context`                                                                    |
| `/ns <name>`                                                | `namespace`                                                                  |
| `/settings` → toggle provider (←→/click/enter)              | `ai.provider`, `ai.base_url`, `ai.model` (reset to that provider's defaults) |
| `/settings` → edit Base URL / Model / API Key, then `enter` | the edited field                                                             |
| `/theme` picker → Save / `enter`                            | `theme`                                                                      |
| `/settings` (or first-run onboarding) → Save                | `cli`, `onboarded`                                                           |
| namespace picker (click `ns …` in the banner)               | `namespace`                                                                  |

A failed save (e.g. unwritable home directory) surfaces as a status-bar
toast — `config save failed: …` — and never blocks the UI.

## Load on startup

`New()` calls `loadConfig()` once: matches `theme` against
`theme.Themes[i].Name`, applies `namespace` and `cli` directly, and fills the
AI config (provider/url/model/key) from whatever is present. A missing file
is not an error — the built-in defaults (tokyo-night, the kubeconfig's own
namespace, `kubectl`, Anthropic preset) apply.

`context` is handled differently: it names a kubeconfig context, and
switching to it means rebuilding the whole client. If the saved context
differs from the one already connected, `Init()` issues an async context
switch rather than blocking startup on it.

`onboarded` is what suppresses the first-run CLI picker. It is tracked
separately from `cli` so that choosing the default value still counts as
having answered.

## Current limits

- The API key sits in the file as plain text (masked only in the UI). If
  that's not acceptable once this is wired to a real key, the fix is to swap
  `api_key` for a keychain reference (macOS Keychain / libsecret / Windows
  Credential Manager) behind the same `AI.APIKey` field.
