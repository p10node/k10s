# Prompt & commands

The bottom panel has two modes, toggled with `ctrl+a` or by clicking the
right-hand tag:

- **CMD ❯** — `/` and `:` commands run inside k10s; **anything else runs as
  a shell command** and its output opens in the main panel as a text view
  (`$ date`, `$ kubectl get pods -o wide`, `$ helm list`). The line is run
  verbatim through your own `$SHELL`, so aliases, functions and PATH are
  the ones you expect, and the CLI name in `/settings` is a label for hints
  rather than something that rewrites what you typed.

  Two limits, both deliberate: a command is given **30 seconds**, and it is
  given **no terminal** — so anything interactive (`vim`, `top`, `kubectl
  exec -it`) waits and then times out. Use `s` on a pod for a real shell.
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

- **`/` — k10s itself.** Theme, settings, updates, help. Nothing here is
  about the cluster you are looking at.
- **`:` — the cluster, in the k9s vocabulary.** Name a resource to open its
  view (`:po`, `:deploy`), switch where you are (`:ns`, `:ctx`), or act on
  the view you are on (`:search`, `:filter`, `:scale`).

Typing either prefix pre-fills it and shows only that set. `↑↓` (or the
wheel) moves the highlight and **`enter` runs the highlighted command straight away** — no
second trip through the prompt. The exception is a command whose argument is
mandatory (`:scale`, `:search`, `:filter`), where enter completes it instead
so the argument can be typed; a resource command's argument is optional, so
enter runs it. `tab` always completes rather than runs.

| Command     | Args     | Effect                                                       |
|-------------|----------|--------------------------------------------------------------|
| `/theme`    |          | theme picker with live preview                               |
| `/settings` |          | CLI name, AI provider **and** the update check, in one dialog |
| `/mouse`    |          | toggle mouse capture — same as `ctrl+s`                      |
| `/update`   | `[skip]` | check GitHub for a newer k10s and install it over this binary |
| `/version`  |          | which build is running, and what the last check found        |
| `/demo`     |          | switch to k10s's built-in demo cluster (`k10s-demo`) — sample data; `:ctx` leaves it |
| `/setup`    |          | how to install kubectl and get a `~/.kube/config` — links, readable with no cluster connected |
| `/help`     |          | keybindings + commands text view                             |
| `:search`   | `<term>` | filter the resource list (left pane, by kind)                |
| `:filter`   | `<term>` | filter rows of the table currently open (main pane)          |
| `:scale`    | `<n>`    | scale the selected deployment/statefulset/replicaset         |
| `:ns`       | `[name]` | the Namespaces table; with a name, switch straight to it     |
| `:ctx`      | `[name]` | context picker; with a name, switch straight to it           |
| `:aliases`  |          | every `:` name in one text view                              |
| `:q`        |          | quit (`:quit`, `:qa` too)                                    |

Unknown commands toast `unknown command … — /help lists everything`.

`/ns` and `/context` are **gone**: namespace and context name things the
cluster has, like every other `:` command, and `:ns` / `:ctx` is where a k9s
user reaches for them. `/theme` still takes no name — it opens a chooser
showing what is actually available.

## Resource commands (`:po`, `:deploy`, …)

Every kind in the Resources pane has a `:` command, and answers to the same
three or four spellings k9s uses — the short form, the plural, the singular.
They are generated from the kind list the connected backend serves, so a
backend that adds a kind gets its command for free and the names can never
drift from what the pane shows.

| Group     | View                | Names                                                             |
|-----------|---------------------|-------------------------------------------------------------------|
| Workloads | Pods                | `:po` `:pods` `:pod`                                              |
|           | Deployments         | `:deploy` `:deployments` `:deployment` `:dp`                      |
|           | ReplicaSets         | `:rs` `:replicasets` `:replicaset`                                |
|           | StatefulSets        | `:sts` `:statefulsets` `:statefulset`                             |
|           | DaemonSets          | `:ds` `:daemonsets` `:daemonset`                                  |
|           | Jobs                | `:job` `:jobs`                                                    |
|           | CronJobs            | `:cj` `:cronjobs` `:cronjob`                                      |
|           | HPAs                | `:hpa` `:hpas` `:horizontalpodautoscalers`                        |
| Network   | Services            | `:svc` `:services` `:service`                                     |
|           | Endpoints           | `:ep` `:endpoints` `:endpoint`                                    |
|           | Ingresses           | `:ing` `:ingresses` `:ingress`                                    |
|           | NetworkPolicies     | `:netpol` `:netpols` `:networkpolicies` `:networkpolicy`          |
| Config    | ConfigMaps          | `:cm` `:configmaps` `:configmap`                                  |
|           | Secrets             | `:sec` `:secrets` `:secret`                                       |
|           | ResourceQuotas      | `:quota` `:quotas` `:resourcequotas` `:resourcequota`             |
|           | LimitRanges         | `:limits` `:limitranges` `:limitrange`                            |
|           | PDBs                | `:pdb` `:pdbs` `:poddisruptionbudgets`                            |
| Storage   | PVCs                | `:pvc` `:pvcs` `:persistentvolumeclaims`                          |
|           | PVs                 | `:pv` `:pvs` `:persistentvolumes` `:persistentvolume`             |
|           | StorageClasses      | `:sc` `:storageclasses` `:storageclass`                           |
| RBAC      | ServiceAccounts     | `:sa` `:serviceaccounts` `:serviceaccount`                        |
|           | Roles               | `:role` `:roles`                                                  |
|           | RoleBindings        | `:rb` `:rolebindings` `:rolebinding`                              |
|           | ClusterRoles        | `:crole` `:clusterroles` `:croles` `:clusterrole`                 |
|           | ClusterRoleBindings | `:crb` `:clusterrolebindings` `:crbs` `:clusterrolebinding`       |
| Cluster   | Nodes               | `:no` `:nodes` `:node`                                            |
|           | Namespaces          | `:ns` `:namespaces` `:namespace`                                  |
|           | Events              | `:ev` `:events` `:event`                                          |
| Custom    | CRDs                | `:crd` `:crds` `:customresourcedefinitions`                       |
|           | Custom Resources    | `:cr` `:customresources` `:crs` `:customresource`                 |

**The first name is what the popup shows** — the short form, the same one the
sidebar and toasts use (`po/api-gateway`). The rest match while you type but
never take a row of their own, so one kind is one row.

A bare `:` matches more commands than fit above the prompt, so the popup
draws a screenful and **scrolls** — the tag counts where you are (`17/37`)
and `↑`/`↓` say which way there is more. Nothing is dropped from the list;
typing one more letter is still the fastest way through it.

Matching is against every spelling: `:dp` narrows to Deployments and then
closes, because there is nothing left to disambiguate. A name that is also
the start of a longer one is highlighted first, so `:pv` opens PVs rather
than PVCs and `:role` opens Roles rather than RoleBindings.

The table is generated from the kind list the backend serves, so it is
exactly what the Resources pane shows — nothing more. Kinds k9s has that
k10s does not model (`:rc`, `:csr`, `:pc`, `:leases`, Helm releases) and its
extra screens (`:popeye`, `:xray`, `:pulses`) have no command here, because
there is no view for them to open.

### The optional argument

```
:po                 Pods, as they are
:po kube-system     switch to that namespace and show Pods there
:po all             every namespace at once, NAMESPACE column and all
:svc api            "api" is not a namespace, so it filters the rows instead
```

A namespace argument only applies to namespaced kinds — `:no kube-system`
filters Node rows rather than switching, since Nodes ignore namespaces.
Arriving with no argument clears whatever row filter was left behind, so a
command always shows the whole view.

Two of them are also switchers, which is where the argument saves the most
time:

- `:ns` opens the namespace chooser (the same one the header's `ns …` button
  opens);
  **`:ns <name>`** switches namespace and leaves you on the view you were
  reading, rather than jumping to Pods.
- `:ctx` opens the context picker; **`:ctx <name>`** switches straight to
  that context — the same reconnect, without the picking. An unknown name
  toasts rather than reconnecting to nothing.

`:aliases` prints the whole table above in a text view, since four spellings
per kind is more than a popup should ever list.

## Growing the command box

`ctrl+z` grows the prompt to half the screen, and typing anything that is
not a `/` or `:` command grows it automatically — a kubectl line or an AI
question gets long, and a one-row field that scrolls sideways hides most of
it. The value wraps across the tall box so the whole thing is readable.
`esc` shrinks it; `esc` again leaves the prompt. There is a `[ grow ]` /
`[ shrink ]` button in the panel's top border too.

## Namespaces

The active namespace is a specific name or the sentinel `all`. The chooser is
reachable two ways — the **`ns <name> ▾` button** in the top-right of the
banner, or **`:ns`**. Both open the **Namespaces table in the
main panel**, whose first row is `all`. Pressing `enter` on a row switches to
that namespace *and* jumps to its Pods, which is the usual next step.

When you already know the name, **`:ns <name>`** skips the table and switches
outright, staying on the view you were reading. So does the namespace
argument on any resource command — `:po kube-system` does both at once.

What changes when you switch:
- **A specific namespace** — the table shows only that namespace's rows.
- **`all`** — every row, with a NAMESPACE column prepended (accent2-colored),
  the same idea as `kubectl get pods -A`. Rows group by namespace, then name.
- Every Resources-pane badge count updates too, not just the open table.
- Non-namespaced kinds (Nodes, Namespaces, CRDs) ignore the filter entirely.
- The row cursor resets to the top, since the row set changed completely.

## CLI name (`/settings`)

Which command you type for Kubernetes — `kubectl`, `k8s`, `k`, or your own —
used in hints and messages. It defaults to `kubectl` and is never asked for:
k10s opens straight into the cluster, and the status bar mentions
`/settings` once on the very first launch rather than a dialog standing in
the way.

It is a **label**: k10s reads the cluster through the API directly, and a
line you type at the prompt is run exactly as typed, so `kubectl get pods`
reaches whatever `kubectl` your PATH has, whatever this setting says. Change
it any time with `/settings`; it is stored as `cli:` in
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
