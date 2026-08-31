# Plugins

k10s supports the core k9s `plugins.yaml` format: scope a command to one or
more resource views, assign a shortcut, and run it against the selected row.
Applicable plugins appear in the Actions pane and can also be clicked.

## Locations

k10s loads these at startup, in order:

1. `~/.k10s/plugins.yaml`
2. every `.yaml` or `.yml` file recursively under `~/.k10s/plugins/`

Files in the directory override an earlier plugin with the same name. Use
`K10S_PLUGINS` and `K10S_PLUGIN_DIR` to override the two locations. When
`K10S_CONFIG` points elsewhere, the default plugin locations move beside that
config file too, which keeps tests and portable installations isolated.

Restart k10s after editing a plugin file. A malformed plugin is skipped and the
status bar reports its filename and validation error; the TUI still starts.

## Example

```yaml
plugins:
  pod-logs:
    shortCut: Ctrl-L
    description: Pod logs via kubectl
    scopes:
      - po
    command: kubectl
    background: false
    args:
      - logs
      - -f
      - $NAME
      - -n
      - $NAMESPACE
      - --context
      - $CONTEXT

  restart-deployment:
    shortCut: Shift-R
    description: Rollout restart
    scopes: [deploy]
    command: kubectl
    confirm: true
    dangerous: true
    args:
      - rollout
      - restart
      - deployment/$NAME
      - -n
      - $NAMESPACE
      - --context
      - $CONTEXT
```

The complete installable example is at `examples/plugins.yaml`.

## Fields

| Field | Meaning |
|---|---|
| `shortCut` | k9s notation such as `Ctrl-L`, `Shift-R`, `Alt-X`, or `x` |
| `description` | label shown in the Actions pane and confirmation dialog |
| `scopes` | resource aliases such as `po`, `deploy`, `svc`; `all` matches every view |
| `command` | executable to run directly |
| `args` | argument list, with variables expanded before execution |
| `background` | start detached and return to k10s immediately |
| `confirm` | show a confirmation dialog before running |
| `dangerous` | render the plugin and confirmation with danger styling |
| `override` | let the plugin replace a built-in keybinding |

Without `override: true`, built-in k10s keys win. For shell syntax, pipes, or
redirection, explicitly run a shell just as in k9s:

```yaml
command: sh
args:
  - -c
  - kubectl get pod "$NAME" -n "$NAMESPACE" -o json | jq .status
```

Foreground commands temporarily leave the alternate screen and inherit the
terminal, so interactive tools such as `fzf` work. Background commands have no
attached terminal or visible output.

## Variables

Arguments support the following k9s-compatible variables:

- `$NAME` — selected object name
- `$NAMESPACE` — selected row namespace (including rows in the all-namespaces view)
- `$CONTEXT` — active kube context; `$CLUSTER` — that context's kubeconfig cluster
- `$RESOURCE_NAME` — current k10s resource key, such as `pods`
- `$FILTER` — current table filter
- `$KUBECONFIG`, `$USER`, and `$GROUPS` — active kubeconfig path and context identity
- `$COL-<COLUMN>` — value from a displayed column, for example `$COL-STATUS`
- `$RESOURCE_GROUP`, `$RESOURCE_VERSION`, `$CONTAINER`, and `$POD` — accepted for
  compatibility; currently empty because the table model does not expose them

Unknown variables such as `$HOME` are preserved, so a configured `sh -c`
command can expand them normally.

## Supported file shapes

Like current k9s releases, k10s accepts:

- a complete `plugins:` document;
- a map of plugin names (useful in files under `plugins/`); or
- one plugin snippet, named after its filename.

The newer k9s-only fields `pipes`, `inputs`, and `overwriteOutput` are not yet
implemented. Unknown YAML fields are ignored, but they have no effect in k10s.
