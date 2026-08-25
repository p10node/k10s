# UI

## Layout

```
TOP BANNER (no border, 4 rows incl. dashed rule)
┌ Resources ─┐┌ <Resource> · <ns> ──────[ zoom ]┐┌ Actions ─┐
│ groups     ││ table OR text view              ││ [d] …    │
│ ▸ item   n ││                                 ││ ───────  │
│ / search   ││ ─────────────────────────────── ││ [D] Del  │
└────────────┘│ / row search…                   │└──────────┘
              └─────────────────────────────────┘
┌ Command/Prompt ────────────────────[ CMD | AI · model ]┐
│ ❯ or ✦ input                                           │
└────────────────────────────────────────────────────────┘
status bar (toast · key hints)
```

Geometry (`layout()`): banner 4 rows, prompt 3, status 1, middle gets the
rest. Left/right panes are 22/24 cols (18/20 under 96 cols; 0 when zoomed).
Below 72×22 the UI is replaced by a "terminal too small" notice.

## Top banner (borderless)

Row 1: `⎈ k10s │ context │ ver │ ns │ nodes 2/3 ready │ … theme <name> ⟳`
— node counter turns warn-colored when any node is NotReady; theme label is
clickable. Row 3: cluster totals — CPU and MEM gauges (avg of node %, mocked
capacity 16 cores / 64 GiB per node). Per-node detail lives in
Resources ▸ Nodes, not the banner.

Gauges color by threshold: ok < 60% ≤ warn < 85% ≤ err.

`ns` reflects the active namespace filter — a specific name, or `all`. See
[commands.md](commands.md#namespaces) for how `/ns` drives it.

## Resources pane (left)

16 resource kinds grouped Workloads / Network / Config / Storage / Cluster /
Custom Resources, each with a row count that reflects the current namespace
filter (so switching `/ns` updates every badge, not just the open table).
Selected row: `▸` + accent. The bottom two rows are a separator plus the
**kind search box**: when this pane has focus every printable key filters the
list instantly (name/short/group substring, case-insensitive); counter shows
`matches/total`; backspace edits; esc clears; the selection snaps to the
first match so the center table follows.

## Main pane (center)

Two modes:
- **Table** — columns auto-sized; when space runs out, widths shrink to
  per-column minimums (first column min 18, NAMESPACE min 9) and then whole
  columns drop from the right. Cell colors by status (`CrashLoopBackOff` err,
  `Pending` warn, `Running` ok, `x/y` mismatch warn); a NAMESPACE column (see
  below) renders in accent2. Selected row: `▌` bar + selection bg, bold on
  the identity column (NAME, or OBJECT for Events — found by header name, not
  a fixed index, since that column shifts right under `/ns all`).
- **Text** — describe / yaml / logs / AI answers / help render in place with
  a `[ close ]` tag and a bottom scroll bar (`n/total  %  hints`). ERROR/WARN
  lines are tinted. No row-search box in this mode (below).

`[ zoom ]` (or `z`) hides both side panes; `[ restore ]` / `z` / `esc` undoes.

**Row search** — table mode only, bottom two rows of the panel (its own
search box, separate from the Resources-pane kind search above). Press `/`
while the main pane is focused to activate it — that overrides the global
`/` prompt-shortcut for exactly this one context, matching the `/`-to-search
convention from `less`/`vim`. Any letter filters every column of every row
(case-insensitive substring across the joined cells); `↑`/`↓` still move the
selection over the filtered set; `esc` clears and exits, `enter` exits
keeping the filter. Selection snaps to the top match on every keystroke.
Switching resource kind always clears it. Same effect from the prompt via
`/filter <term>` (see commands.md) without needing to focus the box first.

## Actions pane (right)

Header shows the current target (`po/name`). 12 actions with hotkeys; actions
not applicable to the selected resource kind are dimmed and refuse with a
toast. Risky actions (Drain, Delete) are separated by a rule and drawn in the
error color.

**Top (metrics)** — `m`, pods and nodes: opens a canned `kubectl top`-style
text view (per-container CPU/MEM for a pod; capacity/allocatable/usage for a
node).

**Cordon** — `o`, nodes only: toggles the node's schedulable state. The
label itself flips between "Cordon" and "Uncordon" depending on current
state (checked live via `mock.NodeCordoned`), and the node's STATUS cell in
the Nodes table gains/loses a `,SchedulingDisabled` suffix, colored warn —
same convention `kubectl get nodes` uses.

**Drain** — `u`, nodes only: confirm modal (danger-styled, like Delete)
explaining pods will be rescheduled and DaemonSet pods won't be evicted; on
confirm it force-cordons the node (`mock.SetCordon(name, true)`).

## Prompt

Bordered, one input row, mode tag on the right (`[ CMD ]` / `[ AI · model ]`,
clickable). See commands.md.

## Modals

- **Confirm** (restart, delete): centered, danger variant gets err-colored
  border/title; Enter·Confirm / Esc·Cancel buttons are clickable.
- **AI settings** (`/config`): 4 rows — provider radio (OpenAI-compatible /
  Anthropic), Base URL, Model, API Key (masked). ↑↓ select, enter edits
  inline via textinput, ←→/click toggles provider (auto-fills that
  provider's default URL+model).
- **Slash suggestions**: popup above the prompt while input starts with `/`;
  rows clickable to autofill.

## Focus model

`tab`/`shift+tab` cycle Resources → Main → Actions. Prompt is entered via
`:`/`/`/click and left with esc/enter. Focused pane gets accent border+title.
