package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

// ---- k9s-style resource commands ------------------------------------------

func TestColonResourceAliasesOpenTheirView(t *testing.T) {
	cases := map[string]string{
		":po": "pods", ":pod": "pods", ":pods": "pods",
		":deploy": "deployments", ":dp": "deployments", ":deployments": "deployments",
		":sts": "statefulsets", ":statefulset": "statefulsets",
		":ds": "daemonsets", ":job": "jobs", ":cj": "cronjobs",
		":svc": "services", ":service": "services", ":services": "services",
		":ing": "ingresses", ":cm": "configmaps", ":sec": "secrets",
		":pvc": "pvcs", ":no": "nodes", ":node": "nodes", ":nodes": "nodes",
		":ev": "events", ":crd": "crds", ":cr": "customresources",
	}
	for cmd, want := range cases {
		m := newTestModel(t, mock.New(""))
		dismissOnboarding(m)
		m.runSlash(cmd)
		if got := m.curKind().Key; got != want {
			t.Errorf("%s → %q, want %q", cmd, got, want)
		}
	}
}

func TestColonAliasesAreCaseInsensitiveAndTyped(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openPrompt(":")
	m.input.SetValue(":PO")
	m.handleKey(key("enter"))
	if got := m.curKind().Key; got != "pods" {
		t.Errorf("kind = %q, want pods — :PO should reach the same view", got)
	}
}

func TestColonNamespaceCommandSwitchesWithoutLeavingTheView(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.runSlash(":svc")

	m.runSlash(":ns kube-system")
	if m.namespace != "kube-system" {
		t.Errorf("namespace = %q, want kube-system", m.namespace)
	}
	if got := m.curKind().Key; got != "services" {
		t.Errorf("kind = %q — :ns <name> should leave the view alone", got)
	}

	m.runSlash(":ns all")
	if m.namespace != domain.AllNamespaces {
		t.Errorf("namespace = %q, want %q", m.namespace, domain.AllNamespaces)
	}
}

func TestBareColonNamespaceOpensTheChooser(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.runSlash(":pods")
	m.runSlash(":ns")
	if got := m.curKind().Key; got != "namespaces" {
		t.Errorf("kind = %q, want the Namespaces table", got)
	}
	if m.nsReturnKind != "pods" {
		t.Errorf("nsReturnKind = %q, want pods — the chooser should come back", m.nsReturnKind)
	}
}

func TestResourceArgumentIsNamespaceOrFilter(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.runSlash(":po kube-system")
	if m.namespace != "kube-system" || m.curKind().Key != "pods" {
		t.Errorf("ns = %q kind = %q, want kube-system/pods", m.namespace, m.curKind().Key)
	}
	if m.rowSearch != "" {
		t.Errorf("rowSearch = %q, a namespace argument is not a filter", m.rowSearch)
	}

	m.runSlash(":po coredns")
	if m.rowSearch != "coredns" {
		t.Errorf("rowSearch = %q, want coredns — a non-namespace argument filters", m.rowSearch)
	}
	if m.namespace != "kube-system" {
		t.Errorf("namespace = %q, a filter should not move you", m.namespace)
	}

	// Arriving with no argument clears the filter left behind.
	m.runSlash(":po")
	if m.rowSearch != "" {
		t.Errorf("rowSearch = %q, want it cleared", m.rowSearch)
	}
}

func TestClusterScopedKindIgnoresNamespaceArgument(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.runSlash(":no kube-system")
	if m.curKind().Key != "nodes" {
		t.Fatalf("kind = %q, want nodes", m.curKind().Key)
	}
	if m.namespace == "kube-system" {
		t.Error("a cluster-scoped kind should not switch namespace")
	}
	if m.rowSearch != "kube-system" {
		t.Errorf("rowSearch = %q, want the argument used as a filter", m.rowSearch)
	}
}

func TestResourceCommandLeavesTextMode(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.showText("help", Help())
	m.runSlash(":svc")
	if m.mode != modeTable {
		t.Errorf("mode = %v, want the table back", m.mode)
	}
}

// ---- suggestions ----------------------------------------------------------

func TestPopupOffersResourceCommandsUnderColon(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	// Each kind is offered under the short form — the name people type, and
	// the one the sidebar and toasts already use.
	m.openPrompt(":")
	m.input.SetValue(":de")
	sug := m.suggestions()
	found := false
	for _, c := range sug {
		if c.Name == ":deploy" {
			found = true
		}
		if !strings.HasPrefix(c.Name, ":") {
			t.Errorf("%q offered under the : prefix", c.Name)
		}
	}
	if !found {
		t.Errorf(":de offered %v, want :deploy among them", names(sug))
	}

	// A longer spelling matches too, and still shows that one row rather
	// than one per spelling.
	m.input.SetValue(":deployme")
	if sug := m.suggestions(); len(sug) != 1 || sug[0].Name != ":deploy" {
		t.Errorf(":deployme offered %v, want just :deploy", names(sug))
	}

	// A fully typed alias keeps its row: the popup vanishing on the last
	// letter reads as "no such command", which is the opposite of true.
	m.input.SetValue(":dp")
	if sug := m.suggestions(); len(sug) != 1 || sug[0].Name != ":deploy" {
		t.Errorf(":dp offered %v, want the :deploy row to stay", names(sug))
	}
}

// Every spelling stays on screen while you type it out — ":job", ":jobs",
// ":node", ":nodes" all keep confirming which command they are.
func TestPopupStaysOpenOnAFullyTypedCommand(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openPrompt(":")

	for _, c := range []struct{ typed, want string }{
		{":jo", ":job"}, {":job", ":job"}, {":jobs", ":job"},
		{":nod", ":no"}, {":node", ":no"}, {":nodes", ":no"},
		{":ns", ":ns"}, {":namespaces", ":ns"},
	} {
		m.input.SetValue(c.typed)
		sug := m.suggestions()
		if len(sug) == 0 {
			t.Errorf("%s offered nothing — the popup disappeared mid-word", c.typed)
			continue
		}
		if sug[0].Name != c.want {
			t.Errorf("%s highlights %q, want %q", c.typed, sug[0].Name, c.want)
		}
	}
}

// The row carries both names, so ":po" and ":pods" are visibly one command.
func TestPopupRowShowsShortAndFullName(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openPrompt(":")
	m.input.SetValue(":po")

	view := stripANSI(m.View())
	if !strings.Contains(view, ":po :pods") {
		t.Errorf("the Pods row does not show both spellings:\n%s", view)
	}
}

// The short form is what the popup offers, for every kind — that is what
// ":ns" going missing behind ":namespaces" looked like.
func TestPopupOffersTheShortForm(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openPrompt(":")
	m.input.SetValue(":")

	offered := map[string]bool{}
	for _, c := range m.suggestions() {
		offered[c.Name] = true
	}
	for _, k := range m.kinds() {
		if !offered[":"+k.Short] {
			t.Errorf("%s is offered as something other than :%s", k.Name, k.Short)
		}
	}
	for _, want := range []string{":ns", ":po", ":svc", ":pv", ":sa", ":crb"} {
		if !offered[want] {
			t.Errorf("%s is missing from the popup", want)
		}
	}
}

// The popup draws a screenful at a time — the matches themselves are not
// capped, only how many are visible at once (see the scrolling tests below).
func TestPopupNeverOutgrowsTheScreen(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.h = 14

	m.openPrompt(":")
	m.input.SetValue(":")
	if len(m.suggestions()) == 0 {
		t.Fatal(": offered nothing at all")
	}
	if rows := m.sugRows(); rows > m.h-8 {
		t.Errorf("popup draws %d rows on a %d-row screen", rows, m.h)
	}
}

func TestResourceCommandRunsOnEnterWithoutAnArgument(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.openPrompt(":")
	m.input.SetValue(":svc")
	m.handleKey(key("enter"))
	if got := m.curKind().Key; got != "services" {
		t.Errorf("kind = %q — enter should run a command whose argument is optional", got)
	}
}

// ---- the rest of the k9s vocabulary --------------------------------------

func TestColonContextCommand(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.runSlash(":ctx")
	if m.mode != modeContexts {
		t.Error(":ctx should open the context list")
	}

	m.mode = modeTable
	m.runSlash(":ctx no-such-context")
	if !strings.Contains(m.toast, "no context named") {
		t.Errorf("toast = %q, want it to say the context is unknown", m.toast)
	}
}

func TestColonQuitQuits(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	for _, c := range []string{":q", ":quit", ":qa"} {
		if cmd := m.runSlash(c); cmd == nil {
			t.Errorf("%s returned no command, want tea.Quit", c)
		}
	}
}

func TestAliasesReportListsEveryKind(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.runSlash(":aliases")
	if m.mode != modeText {
		t.Fatalf("mode = %v, want the text view", m.mode)
	}
	body := strings.Join(m.textLines, "\n")
	for _, k := range m.kinds() {
		if !strings.Contains(body, k.Name) {
			t.Errorf(":aliases does not mention %s", k.Name)
		}
	}
	for _, a := range []string{":po", ":deploy", ":ns", ":search", ":q"} {
		if !strings.Contains(body, a) {
			t.Errorf(":aliases does not mention %s", a)
		}
	}
}

func TestEveryKindIsReachableByCommand(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	for _, k := range m.kinds() {
		for _, a := range aliasesFor(k) {
			key, ok := kindForAlias(":"+a, m.kinds())
			if !ok || key != k.Key {
				t.Errorf(":%s → %q (%v), want %q", a, key, ok, k.Key)
			}
		}
	}
}

func TestAliasesAreUnambiguous(t *testing.T) {
	seen := map[string]string{}
	for _, k := range mock.New("").Kinds() {
		for _, a := range aliasesFor(k) {
			if prev, dup := seen[a]; dup {
				t.Errorf("alias %q is claimed by both %s and %s", a, prev, k.Key)
			}
			seen[a] = k.Key
		}
	}
	// A resource alias must not shadow a command that acts on the view.
	for _, c := range appCommands {
		if owner, clash := seen[strings.TrimPrefix(c.Name, ":")]; clash {
			t.Errorf("%s collides with the %s alias", c.Name, owner)
		}
	}
}

func names(cs []SlashCommand) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

// ---- the kinds k9s has that k10s grew to match ----------------------------

func TestK9sVocabularyReachesEveryNewKind(t *testing.T) {
	cases := map[string]string{
		":rs": "replicasets", ":replicaset": "replicasets", ":replicasets": "replicasets",
		":hpa": "hpas", ":horizontalpodautoscalers": "hpas",
		":ep": "endpoints", ":endpoints": "endpoints",
		":netpol": "networkpolicies", ":networkpolicies": "networkpolicies",
		":quota": "resourcequotas", ":resourcequotas": "resourcequotas",
		":limits": "limitranges", ":limitrange": "limitranges",
		":pdb": "pdbs", ":poddisruptionbudgets": "pdbs",
		":pv": "pvs", ":persistentvolumes": "pvs",
		":sc": "storageclasses", ":storageclass": "storageclasses",
		":sa": "serviceaccounts", ":serviceaccounts": "serviceaccounts",
		":role": "roles", ":roles": "roles",
		":rb": "rolebindings", ":rolebinding": "rolebindings",
		":crole": "clusterroles", ":clusterroles": "clusterroles",
		":crb": "clusterrolebindings", ":clusterrolebinding": "clusterrolebindings",
	}
	for cmd, want := range cases {
		m := newTestModel(t, mock.New(""))
		dismissOnboarding(m)
		m.runSlash(cmd)
		if got := m.curKind().Key; got != want {
			t.Errorf("%s → %q, want %q", cmd, got, want)
		}
	}
}

// :pv and :pvc are different resources in k9s, and confusing them would send
// you to the wrong table with no hint that it happened.
func TestPVAndPVCAreDistinct(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.runSlash(":pv")
	if got := m.curKind().Key; got != "pvs" {
		t.Errorf(":pv → %q, want pvs", got)
	}
	m.runSlash(":pvc")
	if got := m.curKind().Key; got != "pvcs" {
		t.Errorf(":pvc → %q, want pvcs", got)
	}
}

func TestNewKindsRenderRows(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	for _, key := range []string{
		"replicasets", "hpas", "endpoints", "networkpolicies", "resourcequotas",
		"limitranges", "pdbs", "pvs", "storageclasses", "serviceaccounts",
		"roles", "rolebindings", "clusterroles", "clusterrolebindings",
	} {
		m.jumpToResource(key)
		m.namespace = domain.AllNamespaces
		cols, rows := m.tableData()
		if len(cols) == 0 {
			t.Errorf("%s has no columns", key)
		}
		if len(rows) == 0 {
			t.Errorf("%s has no demo rows — the offline backend should mirror the real one", key)
			continue
		}
		for _, r := range rows {
			if len(r) != len(cols) {
				t.Errorf("%s row %v has %d cells, want %d", key, r, len(r), len(cols))
			}
		}
	}
}

// A name that is also the prefix of a longer one must still run itself when
// enter takes the highlighted suggestion — ":pv" is PersistentVolumes even
// though ":pvcs" also starts with "pv", and ":role" is Roles, not
// RoleBindings.
func TestExactNameWinsOverLongerOnes(t *testing.T) {
	cases := map[string]string{
		":pv":   "pvs",
		":role": "roles",
		":cr":   "customresources",
		":no":   "nodes",
		":ns":   "namespaces",
	}
	for typed, want := range cases {
		m := newTestModel(t, mock.New(""))
		dismissOnboarding(m)
		m.openPrompt(":")
		m.input.SetValue(typed)

		if sug := m.suggestions(); len(sug) > 0 && !sug[0].matches(typed) {
			t.Errorf("%s: popup highlights %q first, want the exact match", typed, sug[0].Name)
		}
		m.handleKey(key("enter"))
		if got := m.curKind().Key; got != want {
			t.Errorf("%s ran %q, want %q", typed, got, want)
		}
	}
}

// ---- the popup shows everything, one screen at a time ---------------------

// A bare ":" matches more commands than fit above the prompt. They must be
// windowed, never dropped: a command the popup refuses to reach might as
// well not exist.
func TestPopupScrollsInsteadOfTruncating(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openPrompt(":")
	m.input.SetValue(":")

	sug := m.suggestions()
	if len(sug) <= m.sugRows() {
		t.Fatalf("only %d suggestions — this test needs more than one screenful", len(sug))
	}
	want := len(appCommands) + len(m.kinds())
	if len(sug) != want {
		t.Errorf("\":\" offered %d commands, want all %d", len(sug), want)
	}

	// The last one is reachable by walking there.
	last := sug[len(sug)-1]
	for i := 0; i < len(sug)-1; i++ {
		m.handleKey(key("down"))
	}
	if got := sug[clamp(m.sugIdx, 0, len(sug)-1)]; got.Name != last.Name {
		t.Fatalf("walked to %q, want the last entry %q", got.Name, last.Name)
	}
	if top := m.sugTop(len(sug)); top+m.sugRows() <= len(sug)-1 {
		t.Errorf("window at %d does not include the highlighted last entry", top)
	}

	// And running it there does what it says.
	m.handleKey(key("enter"))
	if key, ok := kindForAlias(last.Name, m.kinds()); ok && m.curKind().Key != key {
		t.Errorf("enter on %q opened %q", last.Name, m.curKind().Key)
	}
}

// Whichever entry is highlighted has to be on screen, wherever it is.
func TestPopupWindowFollowsTheHighlight(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openPrompt(":")
	m.input.SetValue(":")
	n := len(m.suggestions())

	for _, idx := range []int{0, 1, n / 2, n - 2, n - 1} {
		m.sugIdx = idx
		top := m.sugTop(n)
		if idx < top || idx >= top+m.sugRows() {
			t.Errorf("entry %d is outside the window [%d,%d)", idx, top, top+m.sugRows())
		}
		if top < 0 || top > maxi(0, n-m.sugRows()) {
			t.Errorf("window top %d is out of range for %d entries", top, n)
		}
	}
}

// The popup must fit above the prompt on a short terminal.
func TestPopupNeverOutgrowsTheScreenWhenScrolling(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.h = 16
	m.openPrompt(":")
	m.input.SetValue(":")

	if rows := m.sugRows(); rows > m.h-8 || rows < 3 {
		t.Errorf("sugRows = %d on a %d-row screen", rows, m.h)
	}
	view := stripANSI(m.View())
	if strings.Count(view, "\n")+1 > m.h {
		t.Error("the frame grew past the terminal height")
	}
}

// The popup holds more than it can draw, so the wheel has to reach the rest
// — and must not scroll the table behind it while doing so.
func TestWheelMovesThePopupHighlight(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openPrompt(":")
	m.input.SetValue(":")
	rowBefore, kindBefore := m.rowIdx, m.curKind().Key

	for i := 0; i < 5; i++ {
		m.handleMouse(tea.MouseMsg{
			X: 4, Y: m.layout().promptY - 2,
			Button: tea.MouseButtonWheelDown,
			Action: tea.MouseActionPress,
		})
	}
	if m.sugIdx != 5 {
		t.Errorf("sugIdx = %d after five notches, want 5", m.sugIdx)
	}
	if m.rowIdx != rowBefore || m.curKind().Key != kindBefore {
		t.Error("the wheel leaked through the popup to the view behind it")
	}

	// It stops at the top rather than wrapping — a wheel that wraps around
	// makes a long list impossible to read through.
	for i := 0; i < 20; i++ {
		m.handleMouse(tea.MouseMsg{
			X: 4, Y: m.layout().promptY - 2,
			Button: tea.MouseButtonWheelUp,
			Action: tea.MouseActionPress,
		})
	}
	if m.sugIdx != 0 {
		t.Errorf("sugIdx = %d at the top, want 0", m.sugIdx)
	}
}

// The second name is a name, not chrome: th.Border is the colour of the box
// lines and left ":context" barely visible against the background.
func TestPopupAliasIsLegible(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.openPrompt(":")
	m.input.SetValue(":ctx")

	// ":ctx" is the exact match, so it is the highlighted row — selection
	// background and all.
	th := m.th()
	view := m.View()
	on := func(fg lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.SelBg).Foreground(fg)
	}

	if !strings.Contains(view, on(th.Accent).Bold(true).Render(":ctx")) {
		t.Error("the command name is not rendered in the accent colour")
	}
	if !strings.Contains(view, on(th.Accent).Render(":context")) {
		t.Error("the spelled-out name should share the accent colour, just without the bold")
	}
	if strings.Contains(view, on(th.Border).Render(":context")) {
		t.Error(":context is drawn in the border colour — that is for box lines, not text")
	}
}
