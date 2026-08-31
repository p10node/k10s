package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/p10node/k10s/internal/domain"
)

// SlashCommand describes one prompt command for the suggestion popup.
//
// Two prefixes, split by what the command acts on:
//
//	"/"  pickers and settings — choose a namespace, context, theme, or open
//	     the settings dialog. These show you something to pick from.
//	":"  k9s-style: jump to a resource view (":po", ":deploy", ":ns"), or
//	     act on what is already on screen (":search", ":filter", ":scale").
//
// Both open the same popup, filtered to their own set, so typing either
// prefix shows exactly what it can do.
type SlashCommand struct {
	Name string
	Args string
	Desc string
	// Alt are the other spellings that reach the same command — ":pods" and
	// ":pod" for ":po". They match while typing but never take a row of
	// their own in the popup, which would otherwise list four rows per kind.
	Alt []string
	// Full is the spelled-out name shown next to the short one, so a row
	// says ":po :pods" rather than making you guess that they are the same
	// command. Empty when there is nothing longer to show.
	Full string
	// OptArgs marks an argument the command runs perfectly well without, so
	// enter runs it instead of stopping to have the argument typed.
	OptArgs bool
}

// matches reports whether a fully typed command word is this command.
func (c SlashCommand) matches(head string) bool {
	return c.Name == head || slices.Contains(c.Alt, head)
}

// prefixed reports whether head is the start of any of its spellings.
func (c SlashCommand) prefixed(head string) bool {
	if strings.HasPrefix(c.Name, head) {
		return true
	}
	for _, a := range c.Alt {
		if strings.HasPrefix(a, head) {
			return true
		}
	}
	return false
}

// clusterCommands are the "/" set: each opens something to choose from.
//
// Namespace and context are not here: they name things the cluster has, so
// they belong with the rest of the ":" vocabulary (":ns", ":ctx"), where a
// k9s user reaches for them anyway. What is left under "/" is k10s's own
// settings — nothing about the cluster you are looking at.
var clusterCommands = []SlashCommand{
	{Name: "/theme", Desc: "theme picker with live preview"},
	{Name: "/settings", Desc: "CLI name, update check"},
	{Name: "/update", Args: "[skip]", Desc: "check for a newer k10s and install it", OptArgs: true},
	{Name: "/version", Desc: "which build is running, and what the last check found"},
	{Name: "/help", Desc: "keybindings and commands"},
}

// appCommands are the ":" set that acts on the current view, rather than
// naming a resource to open. The resource half is generated from the live
// kind list — see resourceCommands.
var appCommands = []SlashCommand{
	{Name: ":search", Args: "<term>", Desc: "filter the resource list (left pane)"},
	{Name: ":scale", Args: "<n>", Desc: "scale the selected workload to n replicas"},
	{Name: ":filter", Args: "<term>", Desc: "filter rows of the current table"},
	{Name: ":mouse", Desc: "toggle mouse capture — off lets you select & copy"},
	{Name: ":ctx", Full: ":context", Args: "[name]", Desc: "kube contexts — a name switches straight to it",
		Alt: []string{":context", ":contexts"}, OptArgs: true},
	{Name: ":aliases", Desc: "every :name that opens a resource view", Alt: []string{":alias"}},
	{Name: ":q", Full: ":quit", Desc: "quit k10s", Alt: []string{":quit", ":qa", ":q!"}},
}

// SlashCommands is every fixed command, both prefixes — for docs and tests.
// It does not include the per-kind commands, which depend on what the
// connected cluster serves.
var SlashCommands = append(append([]SlashCommand{}, clusterCommands...), appCommands...)

// kindAliases is the k9s vocabulary: a kind answers to its short form, its
// plural and its singular alike, so ":ns", ":namespaces" and ":namespace"
// all land on the same view.
//
// The first entry is what the popup shows, and it is the **short form** —
// the thing people actually type, the same name the sidebar and toasts use
// (po/api-gateway). Leading with ":namespaces" hid ":ns" behind an alias
// nobody could see, and the long names crowd the description off the row.
var kindAliases = map[string][]string{
	"pods":            {"po", "pods", "pod"},
	"deployments":     {"deploy", "deployments", "deployment", "dp"},
	"statefulsets":    {"sts", "statefulsets", "statefulset"},
	"daemonsets":      {"ds", "daemonsets", "daemonset"},
	"jobs":            {"job", "jobs"},
	"cronjobs":        {"cj", "cronjobs", "cronjob"},
	"services":        {"svc", "services", "service"},
	"ingresses":       {"ing", "ingresses", "ingress"},
	"configmaps":      {"cm", "configmaps", "configmap"},
	"secrets":         {"sec", "secrets", "secret"},
	"pvcs":            {"pvc", "pvcs", "persistentvolumeclaims", "persistentvolumeclaim"},
	"nodes":           {"no", "nodes", "node"},
	"namespaces":      {"ns", "namespaces", "namespace"},
	"events":          {"ev", "events", "event"},
	"crds":            {"crd", "crds", "customresourcedefinitions", "customresourcedefinition"},
	"customresources": {"cr", "customresources", "crs", "customresource"},

	"replicasets":         {"rs", "replicasets", "replicaset"},
	"hpas":                {"hpa", "hpas", "horizontalpodautoscalers", "horizontalpodautoscaler"},
	"endpoints":           {"ep", "endpoints", "endpoint"},
	"networkpolicies":     {"netpol", "netpols", "networkpolicies", "networkpolicy"},
	"resourcequotas":      {"quota", "quotas", "resourcequotas", "resourcequota"},
	"limitranges":         {"limits", "limitranges", "limitrange"},
	"pdbs":                {"pdb", "pdbs", "poddisruptionbudgets", "poddisruptionbudget"},
	"pvs":                 {"pv", "pvs", "persistentvolumes", "persistentvolume"},
	"storageclasses":      {"sc", "storageclasses", "storageclass"},
	"serviceaccounts":     {"sa", "serviceaccounts", "serviceaccount"},
	"roles":               {"role", "roles"},
	"rolebindings":        {"rb", "rolebindings", "rolebinding"},
	"clusterroles":        {"crole", "clusterroles", "croles", "clusterrole"},
	"clusterrolebindings": {"crb", "clusterrolebindings", "crbs", "clusterrolebinding"},
}

// aliasesFor returns every spelling a kind answers to. A kind a backend adds
// that the table does not know about — a custom resource, say — still gets
// its key, its short form and its name, so nothing is unreachable.
func aliasesFor(k domain.Kind) []string {
	if a, ok := kindAliases[k.Key]; ok {
		return a
	}
	var out []string
	for _, s := range []string{k.Short, k.Key, strings.ToLower(k.Name)} {
		s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// resourceCommands turns the live kind list into ":" commands. Generating
// them means a backend that serves an extra kind gets it in the popup for
// free, and the names can never drift from what the Resources pane lists.
func resourceCommands(kinds []domain.Kind) []SlashCommand {
	out := make([]SlashCommand, 0, len(kinds))
	for _, k := range kinds {
		al := aliasesFor(k)
		if len(al) == 0 {
			continue
		}
		c := SlashCommand{Name: ":" + al[0], Desc: "view " + k.Name, Args: "[filter]", OptArgs: true}
		if len(al) > 1 {
			c.Full = ":" + al[1]
		}
		if k.Namespaced {
			c.Args = "[ns|filter]"
		}
		// Namespaces is the switcher as well as a table — ":ns kube-system"
		// changes where you are, so the popup should say that rather than
		// "view Namespaces".
		if k.Key == "namespaces" {
			c.Args = "[name]"
			c.Desc = "namespaces — a name switches straight to it"
		}
		for _, a := range al[1:] {
			c.Alt = append(c.Alt, ":"+a)
		}
		out = append(out, c)
	}
	return out
}

// kindForAlias resolves a typed word (":po", ":deployment") to a kind key.
func kindForAlias(word string, kinds []domain.Kind) (string, bool) {
	w := strings.ToLower(strings.TrimPrefix(word, ":"))
	if w == "" {
		return "", false
	}
	for _, k := range kinds {
		if slices.Contains(aliasesFor(k), w) {
			return k.Key, true
		}
	}
	return "", false
}

// aliasReport is the :aliases text view — the whole ":" vocabulary in one
// screen, since four spellings per kind is more than a popup should list.
func aliasReport(kinds []domain.Kind) string {
	var b strings.Builder
	b.WriteString("k10s — \":\" commands\n\n  RESOURCE VIEWS\n")
	for _, k := range kinds {
		names := ":" + strings.Join(aliasesFor(k), "  :")
		b.WriteString(fmt.Sprintf("    %-46s %s\n", names, k.Name))
	}
	b.WriteString(`
  ARGUMENT (optional, on any of the above)
    :<kind> <namespace>   switch to that namespace and show the kind there
                          ("all" for every namespace at once)
    :<kind> <text>        filter the rows of that view instead

  ON THE CURRENT VIEW
`)
	for _, c := range appCommands {
		names := c.Name
		if len(c.Alt) > 0 {
			names += "  " + strings.Join(c.Alt, "  ")
		}
		if c.Args != "" {
			names += " " + c.Args
		}
		b.WriteString(fmt.Sprintf("    %-46s %s\n", names, c.Desc))
	}
	return b.String()
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
    wheel               scrolls the pane under the pointer; while a popup
                        is open it scrolls the popup, never what is behind it

  RESOURCE LIST (left pane)
    tab                 focus it
    type to search      any letters filter the list instantly; the active
                        filter shows in the panel title. A search looks
                        inside folded groups too.
    ↑↓                  move · enter / → back to the table · esc clears
    click               pick a kind (does not steal focus)
    wheel               scrolls this pane without changing what is selected —
                        the main panel stays on the kind you picked. ↑ / ↓ in
                        the panel title say which way there is more.
    space               fold / unfold the group you are standing in
    left                fold it · click a ▾/▸ header does either
    Config, Storage and RBAC start folded — thirty kinds do not fit a
    laptop sidebar, and a folded group also asks the cluster for nothing.
    A folded header shows how many kinds it hides, and keeps the ▸ marker
    if the kind you are looking at is one of them.
    :po, :sec, ctrl+p and the search all reach a folded kind directly, and
    unfold its group on the way in.

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
    plugins             ~/.k10s/plugins.yaml shortcuts appear in the same pane;
                        built-in keys win unless override: true

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
    /                   k10s's own settings (theme, settings, update, help)
    :                   k9s-style: :po, :deploy, :ns … plus search, filter,
                        scale and mouse for the view you are on
    k                   open it with "k " ready to type a command
    ctrl+z              grow the box to half the screen; typing a long
                        command grows it automatically
    plain text          runs as a shell command — "date", "kubectl get pods
                        -o wide", "helm list" — and its output opens in the
                        main panel. 30s cap, and nothing gets a terminal, so
                        anything interactive (vim, top) will just time out.
    ctrl+a              AI mode — disabled in this build while it is
                        reworked; pressing it says so
    ↑↓ / wheel          move through the command suggestions. The popup
                        draws a screenful and scrolls: the tag counts where
                        you are (17/37) and ↑ / ↓ say which way there is more.
    enter               run the highlighted command right away
    tab                 complete it instead (for ones taking an argument)
    esc                 close

  COMMANDS — two prefixes, each with its own popup
    "/" is k10s itself · ":" is the cluster: a resource view, the namespace,
    the context, or something acting on what is on screen.
    Enter in the popup runs the highlighted command straight away.

    /theme              theme picker, previews live (tab → Save, esc cancels)
    /settings           CLI name + update check, in one dialog
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

    :po :deploy …       go to a resource view, k9s-style. Every kind
                        answers to its plural, its short form and its
                        singular alike (:po :pods :pod), and every kind in
                        the Resources pane has one:

      workloads   po  deploy  rs  sts  ds  job  cj  hpa
      network     svc  ep  ing  netpol
      config      cm  sec  quota  limits  pdb
      storage     pvc  pv  sc
      rbac        sa  role  rb  crole  crb
      cluster     no  ns  ev            custom   crd  cr

                        :aliases lists every spelling in one screen.

    :<kind> <namespace> switch to that namespace and show the kind there
                        (":po kube-system", ":po all")
    :<kind> <text>      anything that is not a namespace filters the rows
                        instead (":svc api")
    :ns                 the namespace switcher — the Namespaces table, the
                        same one the header's ns button opens
    :ns <name>          switch namespace without leaving this view
    :ctx                the context picker
    :ctx <name>         switch straight to that context — reconnects for
                        this session only. k10s always opens on kubeconfig's
                        current-context, so "kubectl config use-context" is
                        what changes where it starts next time.
    :aliases            every ":" name, in a text view
    :search <term>      filter the resource list (left pane)
    :scale <n>          scale the selected deployment/statefulset/replicaset
    :filter <term>      filter rows of the current table
    :mouse              same as ctrl+s
    :q                  quit (:quit, :qa too)

  MISC
    T / ctrl+t          next / previous theme (or click the theme button /
                        run /theme for the picker)
    q (outside search)  quit · ctrl+c always quits`
}
