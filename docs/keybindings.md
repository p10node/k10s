# Keybindings

## Global

| Key                    | Action                                                                                                                        |
|------------------------|-------------------------------------------------------------------------------------------------------------------------------|
| `ctrl+c`               | quit (always)                                                                                                                 |
| `q`                    | quit (when not typing in a search box or prompt)                                                                              |
| `tab` / `shift+tab`    | cycle panes Resources → Main → Actions                                                                                        |
| `←` `h` / `→`          | focus resource list / main pane                                                                                               |
| `enter` / double-click | open the selected item — logs if it has them, else describe. On the Namespaces table it switches namespace and shows its pods |
| `z`                    | zoom / restore main pane                                                                                                      |
| `esc`                  | close text view → restore zoom (in that order)                                                                                |
| `T` / `ctrl+t`         | next / previous theme                                                                                                         |
| `ctrl+p`               | **search everything** — kinds and objects in one box                                                                          |
| `ctrl+s`               | toggle mouse capture (copy-mode)                                                                                              |
| `/`                    | commands — namespace, context, theme, settings                                                                                |
| `:`                    | narrowing — search, filter, scale                                                                                             |
| `f`                    | find: focus the main table's row-search box                                                                                   |
| `ctrl+a`               | toggle AI prompt mode (also focuses prompt)                                                                                   |

## Movement

| Key                               | Action                                                  |
|-----------------------------------|---------------------------------------------------------|
| `↑` `↓`                           | move (all panes)                                        |
| `pgup` `pgdn` / `ctrl+b` `ctrl+f` | page                                                    |
| `g` / `G`                         | first / last                                            |
| wheel                             | scrolls the pane under the pointer; a popup takes it while open |

## Copy & select

k10s captures the mouse, which stops the terminal doing its own click-drag
selection. `ctrl+s` (or `/mouse`) releases it: drag-select and copy as usual,
and the status dot changes to `✂` while capture is off. Press it again to get
clicking back.

## Search

Three different boxes, deliberately distinct:

| Scope                  | How to open                  | What it searches                     |
|------------------------|------------------------------|--------------------------------------|
| Everything             | `ctrl+p`                     | resource kinds + objects (see below) |
| Rows of the open table | `f`, or `:filter <term>`     | every column of every visible row    |
| The resource list      | just type while it's focused | kind name / short name / group       |

**`ctrl+p` scope caveat:** kinds you have already opened are searched by
object name too. Kinds not yet loaded match by *name only* — searching their
objects would mean starting a cluster-wide watch for every kind, which is
what [performance.md](performance.md) exists to avoid. The palette footer
says how many kinds are in that state.

**On Cmd+K:** it cannot be bound. macOS terminals handle Cmd themselves and
never write it to the TTY, and bubbletea has no Super/Cmd key at all — only
Ctrl, Alt and Shift. In iTerm2, WezTerm or Ghostty you can map Cmd+K to
*send* `\x10` (which is ctrl+p); Terminal.app cannot remap it.

## Focus

`tab` cycles **Resources → Main → command box**, and `shift+tab` goes the
other way. The **Actions pane is not in the cycle**: every action already has
a hotkey and a clickable row, so a tab stop there would lead nowhere.

Clicking a resource row or an action acts on it without moving focus. The
Resources pane scrolls with the wheel **without changing what is selected**
— scrolling is looking, not picking, so the main panel stays where you left
it. The Actions pane does not scroll; it is never taller than its list.

## Resource list (left pane)

`tab` to focus it. Any printable key then filters the list; `↑↓` moves,
`enter` or `→` returns to the table, `esc` clears. The active filter appears
in the panel title, with the match counter in its top-right tag.

`space` folds or unfolds the group under the cursor and `left` folds it;
clicking a `▾`/`▸` header does either. Config, Storage and RBAC start folded.
While a search is running, `space` is a search character and folding is
ignored — see [ui.md](ui.md#folding-groups).

`:search <term>` and `ctrl+p` filter it without focusing, and a resource
command (`:po`, `:deploy`, `:ns` — see [commands.md](commands.md)) jumps
straight to a kind without touching the filter at all.

The wheel scrolls the pane and nothing else: the selection and the main panel
stay put, and `↑`/`↓` in the panel title say which way there is more. Moving
the selection with `↑↓` drags the window along far enough to keep it visible,
group header included.

## Table row search (`f` from the main pane)

Any printable key filters every visible row (all columns, case-insensitive);
`↑`/`↓` move over the filtered set; `backspace` edits; `esc` clears and
exits; `enter` exits keeping the filter. The box only occupies rows while
it's in use.

Shortcuts keep working while typing: `tab` (to the command box),
`shift+tab` (back to the table), `pgup`/`pgdn`, `ctrl+p`, `ctrl+s`. Only bare
letters go into the search text.

## Actions (main or actions pane focused)

The Actions pane lists **only the actions that apply to the selected kind** —
nothing greyed out.

| Key | Action                                      | Kinds                                 |
|-----|---------------------------------------------|---------------------------------------|
| `d` | Describe                                    | all                                   |
| `y` | YAML                                        | all except events                     |
| `l` | Logs (follows live)                         | pods, workloads, jobs                 |
| `s` | Shell — real interactive exec               | pods                                  |
| `p` | Port Forward — real, toggles                | pods, services                        |
| `r` | Rollout Restart                             | deployments, statefulsets, daemonsets |
| `c` | Scale — opens `:scale <n>`                  | deployments, statefulsets, replicasets|
| `e` | Edit — `$EDITOR` on live YAML               | most                                  |
| `m` | Top (metrics-server)                        | pods, nodes                           |
| `o` | Cordon / **Uncordon** (label follows state) | nodes                                 |
| `u` | **Drain** — confirm modal, cordons + evicts | nodes                                 |
| `D` | **Delete** — red confirm modal              | most                                  |

Confirm modals: `enter`/`y` confirm · `esc`/`n` cancel.

A kind with no logs falls back to **describe** instead of erroring.

## No cluster

With nothing connected the main panel is the **No cluster** panel and the
action keys above have nothing to act on, so two of them are rebound while it
is on screen:

| Key       | Action                                                    |
|-----------|-----------------------------------------------------------|
| `r`       | retry the connection (instead of Rollout Restart)         |
| `enter`   | same — retry                                              |
| `:ctx`    | pick another kubeconfig context                           |
| `/setup`  | installing kubectl, getting a `~/.kube/config` — links     |
| `/demo`   | try the UI on k10s's sample cluster (labelled `DEMO`)      |

Everything else still works: the sidebar, the prompt, `/help`, the theme
picker. See [cluster-setup.md](cluster-setup.md) and
[ui.md](ui.md#no-cluster).

Applicable command plugins from `~/.k10s/plugins.yaml` are listed after the
built-in actions. Their configured shortcut or a click runs them. Built-in
keys win unless the plugin sets `override: true`; see [plugins.md](plugins.md).

## Shell

| Key       | Action                                              |
|-----------|-----------------------------------------------------|
| every key | goes to the pod — the shell owns the keyboard       |
| `ctrl+]`  | detach and return to the table                      |

The shell renders inside the main panel, so the rest of k10s stays visible.

## Command box

| Key      | Action                                                        |
|----------|---------------------------------------------------------------|
| `ctrl+z` | grow to half the screen / shrink back                         |
| `esc`    | shrink if grown, otherwise leave the prompt                   |
| `↑↓`     | move through the suggestion popup                             |
| `enter`  | run the highlighted command · `tab` completes it instead      |

Typing anything that is not a `/` or `:` command grows the box on its own.

`/` lists the choosers; `:` lists the k9s-style resource commands (`:po`,
`:deploy`, `:svc`, `:ns` …) plus the ones that act on the open view. Full
reference in [commands.md](commands.md), and `:aliases` prints it in-app.

## Log viewer

| Key            | Action                                                   |
|----------------|-----------------------------------------------------------|
| `↑↓` / wheel   | scroll; going up pauses following                        |
| `pgup`/`pgdn`  | page; nearing the top loads older entries                |
| `End` / `G`    | jump to the newest line and resume following             |
| `esc`          | close                                                    |

Newest is line `1` at the bottom, numbers count upward, long lines wrap, and
level tokens are coloured.

## Overlays

| Overlay        | Open with       | Keys                                                            |
|----------------|-----------------|-----------------------------------------------------------------|
| Search palette | `ctrl+p`        | `↑↓` move · `enter` open · `esc` close                          |
| Context picker | `:ctx`          | type to filter · `↑↓` · `enter` · `esc`                         |
| Theme picker   | `/theme`        | `↑↓` previews live · `tab` Save · `enter` apply · `esc` cancels |
| Settings       | `/settings`     | `↑↓` · `enter` select/edit or toggle · `←→` toggle · `tab` Save  |
| Update confirm | `/update`       | `enter` install · `esc` cancel                                   |
| Command popup  | type `/` or `:` | `↑↓` move · `enter` **runs it** · `tab` completes               |

## Mouse

Click: resource rows, table rows, every action, `[ zoom ]`/`[ close ]`, the
`ns …` and `theme …` buttons top-right, the `⇧ 1.4.0` update badge in the
status bar, prompt row, mode tag, suggestion rows, palette and picker rows,
modal buttons, provider radios, the update-check toggle.

Actions light up under the pointer and **flash** when clicked, so a click is
acknowledged even when the action only produces a toast.

Double-clicking a table row opens it, the same as `enter`.

Clicking blank space in the centre pane focuses it. The wheel scrolls
the centre pane.
