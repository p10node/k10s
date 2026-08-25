# Keybindings

## Global

| Key                 | Action                                         |
|---------------------|------------------------------------------------|
| `ctrl+c`            | quit (always)                                  |
| `q`                 | quit (when not typing in search/prompt)        |
| `tab` / `shift+tab` | cycle panes Resources → Main → Actions         |
| `←` `h` / `→`       | focus resource list / main pane                |
| `z`                 | zoom / restore main pane                       |
| `esc`               | close text view → restore zoom (in that order) |
| `T` / `ctrl+t`      | next / previous theme                          |
| `:`                 | open prompt                                    |
| `/`                 | main pane (table mode) focused: search its rows — else open prompt pre-filled with `/` |
| `ctrl+a`            | toggle AI prompt mode (also focuses prompt)    |

## Movement

| Key                               | Action                                                  |
|-----------------------------------|---------------------------------------------------------|
| `↑` `↓`                           | move (all panes)                                        |
| `j` `k`                           | move (main pane; in the list pane letters go to search) |
| `pgup` `pgdn` / `ctrl+b` `ctrl+f` | page                                                    |
| `g` / `G`                         | first / last                                            |
| wheel                             | scroll focused pane                                     |

## Resource list (focused)

Any printable key → search box; `backspace` edits; `esc` clears;
`enter`/`→` jumps to the main pane.

## Table row search (`/` from the main pane, table mode)

Any printable key filters every visible row (all columns, case-insensitive);
`↑`/`↓` move over the filtered set; `backspace` edits; `esc` clears and
exits; `enter` exits keeping the filter; `tab`/`shift+tab` still cycle panes.
Same result from the prompt via `/filter <term>`.

## Actions (main or actions pane focused)

| Key | Action                         | Kinds                                 |
|-----|--------------------------------|---------------------------------------|
| `d` | Describe                       | all                                   |
| `y` | YAML                           | all except events                     |
| `l` | Logs                           | pods, workloads, jobs                 |
| `s` | Shell                          | pods                                  |
| `f` | Port Forward                   | pods, services                        |
| `r` | Rollout Restart                | deployments, statefulsets, daemonsets |
| `c` | Scale                          | workloads                             |
| `e` | Edit                           | most                                  |
| `m` | Top (metrics)                  | pods, nodes                           |
| `o` | Cordon / **Uncordon** (toggle, label follows state) | nodes         |
| `u` | **Drain** — confirm modal, cordons + evicts | nodes                    |
| `D` | **Delete** — red confirm modal | most                                  |

Confirm modals: `enter`/`y` confirm · `esc`/`n` cancel.

## Mouse

Click: resource rows, table rows, every action, `[ zoom ]`/`[ close ]`,
theme name in banner, prompt row, mode tag, search box, suggestion rows,
modal buttons, provider radios. Wheel scrolls.
