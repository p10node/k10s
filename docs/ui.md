# UI

## Layout

```
TOP BANNER (no border, 4 rows incl. dashed rule)
┌ Resources ──[n/m]┐┌ <Resource> · <ns> ──────[ zoom ]┐┌ Actions ─┐
│ groups           ││   1 table row                   ││ [d] …    │
│ ▸ item         n ││ ▌ 2 selected row                ││ ───────  │
│                  ││   3 …          OR text view     ││ [D] Del  │
└──────────────────┘└─────────────────────────────────┘└──────────┘
┌ Command/Prompt ────────────────────[ CMD | AI · model ]┐
│ ❯ or ✦ input                                           │
└────────────────────────────────────────────────────────┘
status bar (toast · key hints)
```

Geometry (`layout()`): banner 4 rows, prompt 3, status 1, middle gets the
rest. Left/right panes are 22/24 cols (18/20 under 96 cols; 0 when zoomed).
Below 72×22 the UI is replaced by a "terminal too small" notice.

Neither side pane spends rows on a permanent search box — see *Search boxes*
below.

## Top banner (borderless)

Row 1: `⎈ k10s │ context │ ver │ nodes 2/3 ready … ns <name> ▾ │ theme <name> ⟳`

Identity on the left, two buttons on the right:

- the node counter turns warn-colored when any node is NotReady;
- **`ns <name> ▾`** opens the **Namespaces table in the main panel**, whose
  row `0` is `all` (real namespaces are then `1..N`, matching the sidebar
  count). `enter` there switches namespace and **returns to whichever kind
  you came from** — Services if you were looking at Services. `/ns` does the
  same;
- **`theme <name> ⟳`** opens the theme picker (the same one `/theme` shows).

Row 3: cluster totals — CPU and MEM gauges (average of per-node usage from
metrics-server). Per-node detail lives in Resources ▸ Nodes, not the banner.
Gauges color by threshold: ok < 60% ≤ warn < 85% ≤ err.

## Resources pane (left)

16 resource kinds grouped Workloads / Network / Config / Storage / Cluster /
Custom Resources, each with a row count for the current namespace. Selected
row: `▸` + accent.

Only the centre pane scrolls; use clicks, `ctrl+p` or `:search` to change
kind.

**Counts.** The live backend only watches kinds you have opened, so counts
for the rest come from a cheap background sweep (one `limit=1` request per
kind, reading `remainingItemCount`). A kind whose count isn't known yet shows
no badge rather than a misleading `0`, and a count already on screen is kept
while a newly-opened kind syncs. See [performance.md](performance.md).

**Filtering.** `tab` to this pane and type: every printable key filters by
name / short name / group, case-insensitive; `↑↓` moves, `enter`/`→` returns
to the table, `esc` clears. `:search <term>` and `ctrl+p` do the same without
focusing. The active filter shows in the panel **title** (`Resources · po`)
and the match counter in the top-right tag (`3/16`). The selection snaps to
the first match so the centre table follows.

## Main pane (center)

Two modes:

- **Table** — a left gutter carries the selection marker `▌` and a dim
  1-based **row number**, sized to the largest number present. Remaining
  columns are auto-sized; when space runs out, widths shrink to per-column
  minimums (first column min 18, NAMESPACE min 9) and then whole columns drop
  from the right. Cell colors by status (`CrashLoopBackOff` err, `Pending`
  warn, `Running` ok, `x/y` mismatch warn); a NAMESPACE column renders in
  accent2. Selected row: selection bg, bold on the identity column (NAME, or
  OBJECT for Events — found by header name, not a fixed index, since that
  column shifts right under `/ns all`).
- **Text** — describe / YAML / help render in place with a `[ close ]` tag
  and a bottom scroll bar (`n/total  %  hints`).
- **Logs** — its own viewer; see below.

`[ zoom ]` (or `z`) hides both side panes; `[ restore ]` / `z` / `esc` undoes.

### Sort order

Rows are sorted A→Z by name by default — informer caches return objects in
arbitrary order, so without this the table would reshuffle on every refresh.
Sorting is *natural*: `pod-2` comes before `pod-10`. Under `/ns all` rows
group by namespace first, then name.

**Events are the exception** and stay newest-first; sorting them by their
first column (TYPE) would bury the newest behind every `Normal` event.

### Shell

`s` opens an interactive shell **in the main panel**, not by handing the
whole terminal over: the resource list, header and status bar stay visible
while you are inside the pod. k10s runs a small terminal emulator (vt10x)
over the exec stream, feeds it the pod's output, and renders its screen into
the pane; the emulator is sized to the pane and resizes with it.

Every keystroke goes to the pod — `q`, `/`, `esc` and the action hotkeys all
belong to the program you are running. **`ctrl+]` detaches** (the telnet and
docker convention, chosen because nothing inside a shell wants it), as does
the `[ detach ]` button. When the shell exits on its own the panel returns
to the table.

Workloads resolve to one of their pods, the same way logs do. Backends
without a shell (the offline demo) say so in the status bar rather than
failing.

### Contexts

`/context` lists kubeconfig contexts in the main panel, numbered like a
table and marking the active one `current`. `enter` reconnects. Same idea as
the namespace chooser: full width beats a cramped popup for names this long.

### Busy state

Any action that has to wait — describe, YAML, logs, top, AI, and the
mutating ones — takes over the main panel with a spinner naming what is
running. This covers `enter` and double-click as well, and matters most for
actions whose only result is a toast (port-forward, cordon), where silence
used to look like nothing had happened.

### Log viewer

Opened with `l`, `enter` or a double-click on anything that has logs.

- **Newest at the bottom**, and the view opens pinned there.
- **Line numbers count up from the bottom**: the newest line is always `1`,
  so a number keeps meaning the same thing as new lines arrive.
- **Opens at the bottom already**, with the newest 500 lines and no loading
  flash — the view is where you want it from the first frame.
- **Following**: new lines append at the bottom while pinned. Scrolling up
  pauses ("⏸ paused"), and returning to the bottom — or pressing `End` —
  resumes ("● following"). While paused the view holds still even as new
  lines arrive underneath.
- **Scroll-back is endless**: reaching the oldest loaded line pulls the next
  500 older ones, then the next 500, and so on. The Kubernetes log API has no backwards cursor, so
  "older" means re-reading with a larger tail; the status line says
  `↑ for older` until the beginning is reached, then `start of log`.
- **Lines wrap** instead of being cut off with an ellipsis — a truncated log
  line is often exactly the part you needed. Continuation rows have no
  number.
- **Level tokens are coloured** (`ERROR`/`FATAL` red, `WARN` amber, `INFO`
  green, `DEBUG` dim) and the leading timestamp is dimmed; the message itself
  stays in the normal foreground, because that is what you actually read.

Kinds with no logs of their own fall back to **describe** rather than
reporting an error — there is nothing the user could do about "this kind has
no logs". Workloads (Deployment, StatefulSet, DaemonSet, Job) resolve to one
of their pods, preferring a running one.

### Loading state

While a kind's informer is still doing its first list, the pane shows a
centred spinner, an indeterminate bar and a one-line explanation — never
"no resources found", which would be a lie. The repaint tick runs faster
(150ms) while loading and backs off to 2s once synced.

## Search boxes

Neither pane keeps a search box on screen permanently:

- **Resources pane** — no box at all. It is type-to-filter, and the filter
  state lives in the title/tag.
- **Main pane** — the row-search box appears only while it's in use (focused,
  or holding a filter), so a table normally gets the full panel height. Open
  it with `f` — advertised as a `[ f to search ]` tag beside `[ zoom ]` — and
  the active filter also shows in the panel title.

## Actions pane (right)

Header shows the current target (`po/name`). The pane lists **only actions
that apply to the selected kind** — no dimmed rows. Risky actions (Drain,
Delete) sit below a rule and are drawn in the error color.

Notable behavior:

- **Logs** (`l`) follows the stream live, like `kubectl logs -f`.
- **Shell** (`s`) opens a real interactive exec session **inside the main
  panel** — see below.
- **Port Forward** (`p`) starts a real forward on an ephemeral local port and
  toasts the address; pressing it again stops that forward.
- **Edit** (`e`) fetches live YAML into a temp file, hands the terminal to
  `$EDITOR`, and applies the result on exit.
- **Scale** (`c`) pre-fills the prompt with `:scale <current>` instead of
  guessing a number.
- Rows **light up under the pointer** and **flash** when clicked, so a click
  is acknowledged even when the action only produces a toast.
- **Cordon** (`o`, nodes only) toggles `spec.unschedulable`; the label flips
  between "Cordon" and "Uncordon" to match the node's state, and the STATUS
  cell gains a warn-colored `,SchedulingDisabled` suffix — the same
  convention `kubectl get nodes` uses.
- **Drain** (`u`, nodes only) cordons, then evicts every non-DaemonSet,
  non-mirror pod via the eviction API, reporting any that refused.

## Prompt

Bordered, mode tag on the right (`[ CMD ]` / `[ AI · model ]`, clickable),
and a `[ grow ]` / `[ shrink ]` button beside it.

Normally one input row. `ctrl+z` — or simply typing something that is not a
`/` or `:` command — grows it to half the screen and wraps the value across
the rows, because kubectl lines and AI prompts get long and a sideways-
scrolling one-liner hides most of what you typed. `esc` shrinks, `esc` again
leaves. See [commands.md](commands.md).

## Overlays

- **Confirm** (restart, delete, drain): centered, danger variant gets
  err-colored border/title; Enter·Confirm / Esc·Cancel buttons are clickable.
- **Search palette** (`ctrl+p`): one box over kinds and objects, each hit
  labelled with where it lives; the footer names how many kinds are matched
  by name only.
(Contexts and namespaces are not overlays — both list in the main panel.)
- **Theme picker** (`/theme`): each row carries a swatch of that theme's own
  colors, and moving the cursor **applies the theme immediately** so you see
  it before committing. `esc` restores whatever was active before.
- **Settings** (`/settings`, and on first run): one dialog with the command
  names and the AI provider block (radio, Base URL, Model, masked API Key).
  `↑↓` moves, `enter` ticks a name or starts editing a field, clicking a row
  does the same, `tab` reaches Save. Text fields show a blinking caret.

  The built-in command names are **stated, not chosen**: `kubectl`, `k8s`
  and `k` always work at the prompt, so there is nothing to tick and no way
  to end up with none enabled. One row adds a name of your own, which then
  becomes the one shown in hints; clearing it falls back to `kubectl`.
- **Command popup**: appears while the input starts with `/` (do something)
  or `:` (narrow what's on screen) — each prefix lists only its own set.
  `↑↓` moves the highlight, **`enter` runs the highlighted command outright**,
  `tab` completes instead, rows are clickable.

The pickers share a **Save** button reachable with `tab`.

## Focus model

`tab` cycles **Resources → Main → command box** in layout order, and
`shift+tab` walks it backwards. The prompt can also be entered with `/`, `:`
or a click, and left with esc.

The **Actions pane is not in the cycle** — every action already has a hotkey
and a clickable row, so a tab stop there would be a keystroke that leads
nowhere. Clicking a resource row or an action acts on it without moving
focus.

Clicking blank space in the centre pane focuses it. **While a popup is open
the wheel belongs to the popup** — letting it through meant scrolling a
picker quietly moved the table behind it. The side panes respond to clicks
(pick a kind, fire an action) but not to the wheel — scrolling the
resource list changes the entire view, which was too easy to trigger by
accident while reaching for the table.

**Double-clicking** a table row opens it, the same as `enter`: logs when the
kind has them, describe otherwise — or, on the Namespaces table, switching to
that namespace and showing its pods.
