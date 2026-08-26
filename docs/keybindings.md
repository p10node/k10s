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
| `:`                    | narrowing — search, filter, mouse capture                                                                                     |
| `f`                    | find: focus the main table's row-search box                                                                                   |
| `ctrl+a`               | toggle AI prompt mode (also focuses prompt)                                                                                   |

## Movement

| Key                               | Action                                                  |
|-----------------------------------|---------------------------------------------------------|
| `↑` `↓`                           | move (all panes)                                        |
| `pgup` `pgdn` / `ctrl+b` `ctrl+f` | page                                                    |
| `g` / `G`                         | first / last                                            |
| wheel                             | scrolls the centre pane; a popup takes it while open    |

## Copy & select

k10s captures the mouse, which stops the terminal doing its own click-drag
selection. `ctrl+s` (or `:mouse`) releases it: drag-select and copy as usual,
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

Clicking a resource row or an action acts on it without moving focus, and
neither side pane scrolls with the wheel.

## Resource list (left pane)

`tab` to focus it. Any printable key then filters the list; `↑↓` moves,
`enter` or `→` returns to the table, `esc` clears. The active filter appears
in the panel title, with the match counter in its top-right tag.
`:search <term>` and `ctrl+p` do the same without focusing.

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
| `c` | Scale — opens `/scale <n>`                  | deployments, statefulsets             |
| `e` | Edit — `$EDITOR` on live YAML               | most                                  |
| `m` | Top (metrics-server)                        | pods, nodes                           |
| `o` | Cordon / **Uncordon** (label follows state) | nodes                                 |
| `u` | **Drain** — confirm modal, cordons + evicts | nodes                                 |
| `D` | **Delete** — red confirm modal              | most                                  |

Confirm modals: `enter`/`y` confirm · `esc`/`n` cancel.

A kind with no logs falls back to **describe** instead of erroring.

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

Typing anything that is not a `/` or `:` command grows the box on its own.

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
| Context picker | `/context`      | type to filter · `↑↓` · `enter` · `esc`                         |
| Theme picker   | `/theme`        | `↑↓` previews live · `tab` Save · `enter` apply · `esc` cancels |
| Settings       | `/settings`     | `↑↓` · `enter` select/edit · `tab` Save                         |
| Command popup  | type `/` or `:` | `↑↓` move · `enter` **runs it** · `tab` completes               |

## Mouse

Click: resource rows, table rows, every action, `[ zoom ]`/`[ close ]`, the
`ns …` and `theme …` buttons top-right, prompt row, mode tag, suggestion
rows, palette and picker rows, modal buttons, provider radios.

Actions light up under the pointer and **flash** when clicked, so a click is
acknowledged even when the action only produces a toast.

Double-clicking a table row opens it, the same as `enter`.

Clicking blank space in the centre pane focuses it. The wheel scrolls only
the centre pane.
