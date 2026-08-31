# Build in public

The playbook for sharing k10s while it is being built. The
[marketing plan](marketing.md) is *where* things get posted; this is *what*
gets posted, *how often*, and where the line is.

Building in public works here for one specific reason: **k10s is a tool for
people who read commit logs for fun.** Its audience is platform engineers and
Go developers. They are more convinced by "here is the bug that made startup
take four seconds, and here is the fix" than by any feature list.

## Four rules

1. **Ship the story with the code.** A post about work that is not merged is
   marketing. A post about work that shipped is a changelog with a plot.
2. **Publish the failures.** "The first live version was unusably slow" is the
   most credible sentence in this repo. Nobody trusts a build log where
   nothing ever went wrong.
3. **Never oversell the state.** The README says this has not been
   smoke-tested on a production cluster. Keep saying it until it is not true.
   The day it *is* true, that is a post.
4. **One artefact per post.** A frame, a GIF, a table, a diff. A post with no
   image is a note to yourself.

## Cadence

Sustainable beats frequent. Missing a self-imposed daily streak reads worse
than a reliable weekly one.

| Rhythm | What |
|---|---|
| **Weekly** (pick a day, keep it) | one substantive post: what shipped, what broke, what is next |
| **Per release** | tag → release notes → short post with the one-line highlight |
| **Ad hoc** | a frame worth showing (a new theme, a picker, a nice modal) |
| **Monthly** | numbers in public: stars, issues, downloads, what you learned |

## The five post shapes

Rotate these. Nobody wants four feature announcements in a row.

### 1. The war story (highest value)

The format that consistently outperforms everything else: *a real problem,
the wrong first version, the measurement, the fix, the guard against
regression.*

k10s already has the best possible example written up in
[performance.md](performance.md):

> **The k10s startup bug I want you to avoid**
>
> First live version: several seconds to open, visible input lag between
> kinds. Three causes, all mine:
>
> 1. the sidebar made a live API call per resource kind **on every frame** —
>    one `LIST` per CRD, sequentially, on the UI thread;
> 2. startup registered all 16 informers up front, so opening the app issued
>    16 cluster-wide `LIST`s, including every Secret and every Event;
> 3. then it blocked on `WaitForCacheSync` before drawing anything.
>
> Fix: informers start lazily per kind, the render path does zero I/O, counts
> are swept on a background timer. Startup now registers nothing and awaits
> nothing.
>
> The part I am proudest of is not the fix — it is the three tests that fail
> if anyone reintroduces it.

Every engineer reading that recognises themselves. That is why it converts.

### 2. The decision post

One design choice, the alternatives you rejected, why. k10s has several ready
to go:

- **Two command prefixes** — `/` acts on the cluster, `:` narrows the screen.
  Why two languages instead of one, and what it makes impossible to confuse.
- **The Actions pane is not in the tab cycle** — every action already has a
  hotkey and a clickable row, so a tab stop there leads nowhere.
- **A `Source` interface with two implementations** — the live cluster and the
  offline demo are interchangeable, which is why the UI can be developed with
  no cluster and demoed with no risk.
- **`ctrl+p` searches loaded kinds' objects only** — searching every kind's
  objects would mean a cluster-wide watch per kind, which is exactly the bug
  from post #1. A deliberate limit, stated in the footer, instead of a
  surprise.
- **Cmd+K cannot be bound.** macOS terminals eat Cmd and never write it to
  the TTY; Bubble Tea has no Super key. A short, genuinely useful post that
  nothing else on the internet explains clearly.

### 3. The frame post

Cheap, frequent, and it is what gets reshared. `just shot` generates a real
frame from the code in one command, so this costs nothing:

```bash
just shot 120 36                 # a clean frame
just shot 120 36 "esc,j,j,l"     # replay keys first, then render
```

Show the theme picker, the confirm modal, the metrics view, the log follower.
No mockups, ever — it is the actual renderer.

### 4. The failure / limit post

Publish the [known limits](roadmap.md#known-limits) as content instead of
hiding them at the bottom of a doc:

> Updates in k10s are checksum-verified, not signature-verified.
> `checksums.txt` proves the download matches what the release published. It
> proves nothing about *who* published it. Signing with cosign is the fix, and
> it goes in before anyone should trust this anywhere that matters.

Saying this yourself, first, is how a solo project earns the benefit of the
doubt.

### 5. The release post

Tag, then post the highlight and one frame. Keep it to three lines. Link the
release, not the repo — download counts are one of the few honest metrics
available.

The channel post is three lines; the release page itself is the long form.
Write it as `docs/releases/<tag>.md` *before* tagging — the workflow picks it
up and puts GitHub's generated commit list underneath.

## The demo GIF

The most important asset the project does not have yet. Read this before
recording anything.

**The differentiator is the mouse, and no terminal recorder captures a mouse
cursor.** `vhs` and `asciinema` replay keystrokes into a pty; the pointer
never exists. So there are two clips, with two tools:

### The hero clip — real screen capture, 20–30s

Record the actual screen (Kap or QuickTime on macOS, Peek on Linux), because
the cursor moving to a row and clicking it *is* the pitch. Storyboard:

| t | Beat |
|---|---|
| 0–3s | `k10s` opens instantly on the Pods table — no spinner, no wait |
| 3–7s | **mouse** clicks the CrashLoopBackOff row; the Actions pane changes |
| 7–11s | **click** `[l] Logs` — the log follower starts streaming in-pane |
| 11–15s | `ctrl+p`, type three letters, jump to a Service |
| 15–20s | `/theme` — arrow through themes, whole UI recolouring live |
| 20–26s | `ctrl+a`, ask "why is billing-worker crashing?", answer opens |
| 26–30s | back to the hero frame, hold still on it |

Rules: 100×28 or larger, font size ~16 so it is legible in a Reddit preview,
**always the offline demo backend** (`k10s demo` — it is opt-in now, plain
`k10s` shows "No cluster"), under 10MB or GitHub will not inline it,
no cut that requires the viewer to read a status bar to follow along.

### The reproducible clip — vhs, for docs and release notes

Keyboard-only, but it is scripted, diffable and rerecordable after every UI
change. Save as `assets/demo.tape` and run `vhs assets/demo.tape`:

```tape
Output assets/demo.gif
Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 700
Set Padding 20

Type "k10s demo"    Enter    Sleep 3s
Escape         Sleep 1s          # dismiss first-run onboarding
Down Down Down Sleep 1s          # to the CrashLoopBackOff pod
Type "l"       Sleep 3s          # logs, following
Escape         Sleep 1s
Ctrl+P         Sleep 1s
Type "svc"     Sleep 2s   Enter  Sleep 2s
Type "/theme"  Enter  Sleep 1s
Down Sleep 1s  Down Sleep 1s  Down Sleep 1s
Escape         Sleep 1s
Ctrl+A         Sleep 1s
Type "why is billing-worker crashing?"   Enter   Sleep 4s
```

Re-record it whenever the UI changes materially. A GIF showing an old layout
is worse than no GIF.

## Turning the repo into content automatically

The work already produces the material; the trick is not losing it.

- **Write commit subjects as if they were headlines.** The last four here
  (`feat: implement automated update checking and in-app self-updating`) are
  already usable as post titles. `git log --oneline` is then a content
  backlog.
- **`--generate-notes` on every release** (already wired in
  `.github/workflows/release.yml`) means release notes write themselves from
  those subjects — one more reason the subjects matter. That generated list
  is the floor, not the release post: drop a hand-written
  `docs/releases/<tag>.md` in first and the workflow puts it above the list.
  [`v0.1.0`](releases/v0.1.0.md) is the shape — positioning line, one frame,
  install, what-you-get, and an honest *known limits* section.
- **Keep a `NOTES.md` scratch file** of "that was annoying" moments while
  coding. Every one of them is a post. They are impossible to recover a week
  later.
- **When you fix a slow path, record the number before and after.** The
  before-number is the post; without it there is nothing to show.

## What never goes public

- Real cluster names, namespaces, hostnames or customer identifiers in any
  frame. **Run `k10s demo` for every published screenshot** — the mock
  backend is what it exists for, and since it is opt-in you now have to ask
  for it explicitly. (Check `internal/mock/data.go` first: demo data that
  *looks* internal is the same problem as data that is.)
- Colleagues' names, employers or Slack messages without asking them first.
  Thanking a team is warm; identifying individuals is theirs to consent to.
- API keys, kubeconfigs, bearer tokens — including in a terminal scrollback
  that happens to be in frame.
- Anything about an employer's infrastructure that the employer has not said
  is fine to share.

## When the criticism arrives

It will, and mostly it is useful.

- **"Why not just k9s?"** — answer it warmly, the same way, every time. The
  [prepared answer](marketing.md#answering-the-one-question-everyone-asks) is
  the honest one: k10s exists *because of* k9s and recommends it.
- **"This is AI slop / vibe-coded."** — the defence is the repo: a headless
  renderer, a documented width invariant, perf regression tests, and a
  roadmap that lists its own known limits. Point at those instead of arguing.
- **A real bug report.** Best possible outcome. Reproduce it, fix it, ship it,
  and reply with the commit. A stranger's bug fixed within a day is how the
  second contributor decides to bother.
- **Someone asks for a feature that is out of scope.** Say so plainly and add
  it to the roadmap under *possible next steps*. "No, and here is where I
  wrote down that you asked" keeps goodwill.

## The one thing to remember

The most compelling thing about k10s is not the mouse support or the AI. It is
that it was built so that a specific group of colleagues would stop dreading
the part of their day where they have to go look at a cluster.

Lead with that. It is true, it is unusual, and no feature list competes with
it.
