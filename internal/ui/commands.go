package ui

import "strings"

// SlashCommand describes one prompt command for the suggestion popup.
//
// Two prefixes, split by what the command acts on:
//
//	"/"  pickers and settings — choose a namespace, context, theme, or open
//	     the settings dialog. These show you something to pick from.
//	":"  commands that act on what is already on screen, usually with an
//	     argument — search, filter, scale, mouse capture.
//
// Both open the same popup, filtered to their own set, so typing either
// prefix shows exactly what it can do.
type SlashCommand struct {
	Name string
	Args string
	Desc string
}

// clusterCommands are the "/" set: each opens something to choose from.
var clusterCommands = []SlashCommand{
	{"/ns", "", "choose a namespace"},
	{"/context", "", "choose a kube context — reconnects"},
	{"/theme", "", "theme picker with live preview"},
	{"/settings", "", "CLI name, AI provider, update check"},
	{"/update", "[skip]", "check for a newer k10s and install it"},
	{"/version", "", "which build is running, and what the last check found"},
	{"/help", "", "keybindings and commands"},
}

// appCommands are the ":" set: each acts on the current view.
var appCommands = []SlashCommand{
	{":search", "<term>", "filter the resource list (left pane)"},
	{":scale", "<n>", "scale the selected workload to n replicas"},
	{":filter", "<term>", "filter rows of the current table"},
	{":mouse", "", "toggle mouse capture — off lets you select & copy"},
}

// SlashCommands is every command, both prefixes — for docs and tests.
var SlashCommands = append(append([]SlashCommand{}, clusterCommands...), appCommands...)

// commandsFor returns the set belonging to a prefix character.
func commandsFor(prefix byte) []SlashCommand {
	if prefix == ':' {
		return appCommands
	}
	return clusterCommands
}

// hasArgument reports whether the prompt already carries an argument after
// the command word.
func hasArgument(v string) bool {
	i := strings.IndexByte(v, ' ')
	return i >= 0 && strings.TrimSpace(v[i:]) != ""
}

// isCommandPrefix reports whether c starts a prompt command.
func isCommandPrefix(c byte) bool { return c == '/' || c == ':' }

// Help returns the /help text view content.
func Help() string {
	return `k10s — keybindings

  NAVIGATION
    tab / shift+tab     cycle Resources → Main → command box.
                        The Actions pane stays out of it: every action has a
                        hotkey and a clickable row.
    ↑↓                  move within the table (j/k are not bound —
                        k opens the command box with "k" already typed)
    pgup pgdn g G       page / first / last
    z                   zoom main pane · esc restores
    mouse               click anything: rows, actions, buttons, the header's
                        ns and theme buttons. Actions light up on hover and
                        flash when clicked.
    click               blank space in the centre pane selects it
    wheel               scrolls the centre pane; while a popup is open it
                        scrolls the popup, never what is behind it

  RESOURCE LIST (left pane)
    tab                 focus it
    type to search      any letters filter the list instantly; the active
                        filter shows in the panel title
    ↑↓                  move · enter / → back to the table · esc clears
    click               pick a kind (does not steal focus)
    the wheel does not scroll this pane — use ctrl+p or :search instead

  TABLE (main pane, table mode)
    f                   focus the table's own row-search box (find)
    (while searching)   type to filter · ↑↓ move · enter/esc close
                        pane switching and ctrl-shortcuts still work
    rows are numbered down the left gutter and sorted A→Z by default

  ACTIONS (main pane focused)
    enter / double-click  open the item: logs if it has them, else describe
                          on the Namespaces table, switches to that namespace
                          and shows its pods
    d describe · y yaml · l logs (follow, when live)
    s shell — a real interactive shell rendered inside the main panel;
      ctrl+] detaches. The rest of k10s stays visible while you are in it.
    p port-forward (real) · r rollout restart · c scale (prompts for n)
    e edit (opens $EDITOR on live YAML) · m top (metrics-server)
    o cordon/uncordon · u drain (nodes only)
    the pane lists only the actions that apply to the selected kind
    D delete (confirm dialog)

  COPY / SELECT
    ctrl+s              toggle mouse capture. With it OFF the terminal does
                        its own selection, so you can drag-select and copy;
                        clicking rows/buttons resumes when you turn it back on.

  SEARCH EVERYTHING
    ctrl+p              one box that finds resource kinds and objects.
                        Kinds you have opened are searched by object name
                        too; kinds not yet loaded match by name only (opening
                        them all would mean watching the whole cluster).
                        macOS note: Cmd+K cannot reach a terminal program —
                        the terminal keeps it. In iTerm2/WezTerm/Ghostty you
                        can bind Cmd+K to send \x10 (ctrl+p) to get it.

  PROMPT
    /                   choosers (namespace, context, theme, settings)
    :                   act on this view (search, filter, scale, mouse)
    k                   open it with "k " ready to type a command
    ctrl+z              grow the box to half the screen; typing a plain
                        command or AI prompt grows it automatically
    ctrl+a              toggle AI mode (✦) — plain text goes to the model
    ↑↓                  move through the command suggestions
    enter               run the highlighted command right away
    tab                 complete it instead (for ones taking an argument)
    esc                 close

  COMMANDS — two prefixes, each with its own popup
    "/" opens a chooser · ":" acts on what is on screen.
    Enter in the popup runs the highlighted command straight away.

    /ns                 choose a namespace (list opens in the main panel)
    /context            choose a kube context — reconnects
    /theme              theme picker, previews live (tab → Save, esc cancels)
    /settings           CLI name + AI provider + update check, in one dialog
    /update             check GitHub for a newer k10s and install it over
                        this binary. Confirms first, verifies the download
                        against the release checksums, then offers to restart
                        into it. The check also runs once a day at startup
                        and only speaks up when there is something newer —
                        turn it off in /settings.
    /update skip        stop mentioning the release the last check found,
                        without turning the check off
    /version            which build this is, where updates come from, and
                        what the last check found
    /help               this screen

    :search <term>      filter the resource list (left pane)
    :scale <n>          scale the selected deployment/statefulset
    :filter <term>      filter rows of the current table
    :mouse              same as ctrl+s

  MISC
    T / ctrl+t          next / previous theme (or click the theme button /
                        run /theme for the picker)
    q (outside search)  quit · ctrl+c always quits`
}
