package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/hinshun/vt10x"

	"github.com/p10node/k10s/internal/ai"
	"github.com/p10node/k10s/internal/config"
	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/plugin"
	"github.com/p10node/k10s/internal/theme"
	"github.com/p10node/k10s/internal/update"
)

type focusPane int

// Tab cycles Resources → Main → Command box, in layout order. The Actions
// pane stays out of it: every action already has a hotkey and a clickable
// row, so a stop there would be a keystroke that leads nowhere.
const (
	focusMain focusPane = iota
	focusPrompt
	focusMainSearch // typing into the table's own row-search box
	focusList

	// Never focused; retained so the mouse code can name the pane.
	focusActions
)

// tabOrder is the cycle tab walks. Keeping it as data means forward and
// backward can't drift apart.
var tabOrder = []focusPane{focusList, focusMain, focusPrompt}

// nextFocus returns the pane tab (dir=+1) or shift+tab (dir=-1) moves to.
func nextFocus(cur focusPane, dir int) focusPane {
	at := 0
	for i, f := range tabOrder {
		if f == cur {
			at = i
		}
	}
	return tabOrder[(at+dir+len(tabOrder))%len(tabOrder)]
}

type mainMode int

const (
	modeTable mainMode = iota
	modeText
	modeLogs
	modeContexts
	modeShell
)

type promptMode int

const (
	promptCmd promptMode = iota
	promptAI
)

type confirmState struct {
	title   string
	message []string
	danger  bool
	// notice means there is nothing to decide — the modal is only telling
	// you something, so it gets one button and dismissing it isn't a
	// "cancelled" anything.
	notice bool
	onOK   func(*Model) tea.Cmd
}

type aiConfig struct {
	provider int // index into ai.Providers
	url      string
	model    string
	key      string
}

type Model struct {
	w, h int

	src       domain.Source
	namespace string

	themes   []theme.Theme
	themeIdx int
	focus    focusPane

	resIdx int
	search string
	// collapsed folds a Resources-pane group away. Groups you rarely touch
	// start folded (defaultCollapsedGroups): thirty kinds do not fit on a
	// laptop-sized sidebar, and a folded group also costs no badge requests
	// — the backend only counts kinds that are actually drawn.
	collapsed map[string]bool
	// listScroll is the Resources pane's own scroll offset, in rendered
	// lines. Scrolling it never moves the selection — the wheel is for
	// looking around, and a kind list that reselected as it slid past would
	// change the whole main panel by accident.
	listScroll int
	rowIdx     int
	rowScroll  int
	rowSearch  string // filters rows of the currently displayed table

	mode      mainMode
	textTitle string
	textLines []string
	textTop   int

	zoomed  bool
	confirm *confirmState

	// cpuTrend/memTrend follow the header gauges; trends follows the CPU and
	// MEM cells of whichever table is showing. See trend.go.
	cpuTrend, memTrend trend
	trends             map[string]*trend

	pmode promptMode
	input textinput.Model
	toast string
	// themeWarn survives startup connection toasts so malformed custom
	// theme files are actually visible on the production NewStartup path.
	themeWarn string

	cfg aiConfig

	// cli is the command name echoed in hints ("kubectl" / "k" / …), and
	// clis is every name recognised when you type a command at the prompt.
	// All three presets are enabled by default because people alias them
	// interchangeably.
	cli  string
	clis []string

	// k9s-compatible command plugins loaded at startup.
	plugins    []plugin.Named
	pluginWarn string

	// One settings modal: CLI name + AI provider + the update check.
	setOpen    bool
	setRow     int
	setEditing bool
	// onboarded records that k10s has been run before; firstRun is that
	// same fact for this session, so the status bar can point at /settings
	// once instead of a dialog doing it every time.
	onboarded bool
	firstRun  bool

	// themePicker: live-previewing theme chooser opened by /theme.
	themeOpen bool
	themeRow  int
	themeOrig int  // restored if the picker is cancelled
	themeSave bool // focus is on the Save button

	// sugIdx is the highlighted entry in the slash-command popup.
	sugIdx int

	// palette: the global "search everything" box (ctrl+p).
	palOpen bool
	palIdx  int

	// context picker, opened by :ctx.
	ctxOpen   bool
	ctxIdx    int
	ctxFilter string

	// Action-pane feedback: which action the pointer is over, and which one
	// was just clicked (briefly lit so the click is visible).
	hoverAct string
	flashAct string
	flashGen int

	// Which kind to return to after the namespace chooser — you usually
	// want the view you left, not pods.
	nsReturnKind string

	// Double-click detection: bubbletea reports individual presses, so the
	// pair has to be recognised here.
	lastClickRow int
	lastClickAt  time.Time

	// An interactive shell rendered inside the main panel: the live exec
	// stream plus the terminal emulator its output is fed into.
	shellSess domain.ShellSession
	shellTerm vt10x.Terminal
	shellGen  int
	shellName string

	// busy marks an action whose result hasn't arrived yet, so the main
	// panel can say so instead of looking like nothing happened.
	busy      bool
	busyLabel string

	// anim advances on every repaint tick and drives the loading spinner.
	anim int

	// promptZoom grows the command box to half the screen so a long
	// command or AI prompt is readable while typing it.
	promptZoom bool

	// mouseOff disables mouse capture so the terminal's own selection (and
	// therefore copy) works.
	mouseOff bool

	// nsPinned marks a namespace restored from config, so the first
	// connection doesn't overwrite it with the context's own default.
	nsPinned bool

	// connect is set when the backend is built in the background (see
	// connect.go): the UI opens before the cluster answers, and connecting
	// is what the main panel shows a spinner for. connGen discards a stale
	// attempt that lands after a newer one was started.
	connect    func(context string) (domain.Source, string)
	connecting bool
	connGen    int
	connName   string

	// startTarget is the context the first connection asks for: "" (let
	// kubeconfig choose) for a normal launch, the demo context for
	// `k10s demo`.
	startTarget string

	// kubeCtxs is kubeconfig's own context list, read once at startup. The
	// live backend serves the same list, but the demo backend serves its
	// own, so this is what keeps the real contexts reachable from inside
	// the demo — see ctxChoices.
	kubeCtxs []string

	// offline is the settled version of the same story: the attempt is over
	// and there is no cluster. The main panel says so and shows no rows —
	// k10s does not stand in for a cluster it could not reach. offlineWhy is
	// the backend's own reason, shown verbatim. See nocluster.go.
	offline    bool
	offlineWhy string

	portFwds map[string]func()
	logStop  func()
	logCh    <-chan string
	logGen   int

	// Log viewer state. logScroll counts display rows hidden *below* the
	// view, so 0 means pinned to the newest line.
	logScroll  int
	logFollow  bool
	logMore    bool // older entries remain on the server
	logLoading bool // an older-page request is in flight
	logTail    int  // how many lines have been requested so far
	logKind    string
	logNS      string
	logName    string

	rowMem map[string]int

	// Self-update (see update.go). updRel is the newest release once a check
	// has come back — nil until then, which is what /update uses to decide
	// whether it needs to look first.
	updRel      *update.Release
	updDisabled bool
	updRepo     string
	updSkip     string
	updLast     time.Time
	updBusy     bool
	relaunch    bool
}

// New builds the UI model against src (either a real k8s.Store or the
// offline mock.Source — the UI has no idea which).
func New(src domain.Source) *Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 512
	themes, themeErr := theme.Load()

	m := &Model{
		src:       src,
		namespace: src.DefaultNamespace(),
		themes:    themes,
		themeIdx:  0,
		focus:     focusMain,
		input:     ti,
		rowMem:    map[string]int{},
		collapsed: map[string]bool{},
		portFwds:  map[string]func(){},
		cli:       config.DefaultCLI,
		clis:      append([]string(nil), config.CLIPresets...),
		toast:     "connected to " + src.ClusterInfo().Context,
		cfg: aiConfig{
			provider: 1,
			url:      ai.Providers[1].URL,
			model:    ai.Providers[1].Model,
			key:      "",
		},
	}
	plugins, pluginErr := plugin.Load()
	m.plugins = plugins
	if pluginErr != nil {
		m.pluginWarn = "plugins: " + strings.ReplaceAll(pluginErr.Error(), "\n", "; ")
		m.toast = m.pluginWarn
	}
	if themeErr != nil {
		m.themeWarn = "custom theme: " + strings.ReplaceAll(themeErr.Error(), "\n", "; ")
		m.toast = m.themeWarn
	}
	m.loadConfig()
	return m
}

// loadConfig applies ~/.k10s/config.yaml (or $K10S_CONFIG) on startup.
func (m *Model) loadConfig() {
	c, err := config.Load()
	if err != nil {
		m.toast = "config: " + err.Error()
		return
	}
	if c.Theme != "" {
		for i, t := range m.themes {
			if t.Name == c.Theme {
				m.themeIdx = i
			}
		}
	}
	// k10s always opens on kubeconfig's current-context — the cluster the
	// kubectl in the next terminal is talking to. The saved context is not
	// a "connect here" pin; it records which context the saved namespace
	// belongs to, so that namespace comes back only when you are on that
	// same cluster and never leaks onto another one.
	if c.Namespace != "" && c.Context == m.src.ClusterInfo().Context {
		m.namespace = c.Namespace
		m.nsPinned = true
	}
	if c.AI.Provider == "openai" {
		m.cfg.provider = 0
	}
	if c.AI.BaseURL != "" {
		m.cfg.url = c.AI.BaseURL
	}
	if c.AI.Model != "" {
		m.cfg.model = c.AI.Model
	}
	if c.AI.APIKey != "" {
		m.cfg.key = c.AI.APIKey
	}
	if c.CLI != "" {
		m.cli = c.CLI
	}
	if len(c.CLIs) > 0 {
		m.clis = c.CLIs
	}
	if c.CollapsedSet {
		m.collapsed = map[string]bool{}
		for _, g := range c.Collapsed {
			m.collapsed[g] = true
		}
	} else {
		m.collapsed = defaultCollapsed()
	}
	m.zoomed = c.Zoomed
	m.applyUpdateConfig(c.Update)
	m.onboarded = c.Onboarded
	// First run opens straight into the cluster. A settings dialog in front
	// of the thing you launched is a form to dismiss before you can look at
	// anything, and every field in it has a working default — the CLI name
	// is a label, the AI block is optional, the update check is on. The
	// status bar says where to find it instead.
	if !c.Onboarded {
		m.onboarded = true
		m.firstRun = true
		m.saveConfig()
	}
}

// saveConfig persists the current settings; failures surface as a toast but
// never interrupt the UI.
func (m *Model) saveConfig() error {
	providers := []string{"openai", "anthropic"}
	// The context is saved as the namespace's address — which cluster it was
	// chosen on — so while a switch is in flight the one being switched *to*
	// is what the namespace will belong to.
	ctx := m.src.ClusterInfo().Context
	if m.connecting && m.connName != "" {
		ctx = m.connName
	}
	err := config.Save(config.Config{
		Theme:     m.th().Name,
		Context:   ctx,
		Namespace: m.namespace,
		CLI:       m.cli,
		CLIs:      m.clis,
		Onboarded: m.onboarded,
		// Always written, even when nothing is folded — see config.Config.
		Collapsed:    m.collapsedGroups(),
		CollapsedSet: true,
		Zoomed:       m.zoomed,
		AI: config.AI{
			Provider: providers[m.cfg.provider],
			BaseURL:  m.cfg.url,
			Model:    m.cfg.model,
			APIKey:   m.cfg.key,
		},
		Update: m.updateConfig(),
	})
	if err != nil {
		m.toast = "config save failed: " + err.Error()
	}
	return err
}

func (m *Model) th() theme.Theme { return m.themes[m.themeIdx] }

func (m *Model) withThemeWarning(status string) string {
	warnings := make([]string, 0, 2)
	if m.themeWarn != "" {
		warnings = append(warnings, m.themeWarn)
	}
	if m.pluginWarn != "" {
		warnings = append(warnings, m.pluginWarn)
	}
	if len(warnings) == 0 {
		return status
	}
	return strings.Join(warnings, "   ·   ") + "   ·   " + status
}

func (m *Model) kinds() []domain.Kind { return m.src.Kinds() }

// curKind (aliased as res for brevity at call sites) returns the currently
// selected kind, by resIdx into the full, unfiltered kind list.
func (m *Model) curKind() domain.Kind {
	ks := m.kinds()
	if m.resIdx < 0 || m.resIdx >= len(ks) {
		return domain.Kind{}
	}
	return ks[m.resIdx]
}

func (m *Model) res() domain.Kind { return m.curKind() }

// filtered returns kind indices matching the search box.
// defaultCollapsedGroups are folded on first run. Workloads, Network and
// Cluster are what people open k10s for; the rest are looked up occasionally,
// and unfolding one is a click.
var defaultCollapsedGroups = []string{"Config", "Storage", "RBAC"}

func defaultCollapsed() map[string]bool {
	out := make(map[string]bool, len(defaultCollapsedGroups))
	for _, g := range defaultCollapsedGroups {
		out[g] = true
	}
	return out
}

// collapsedGroups is the folded set, in kind-list order so the config file
// doesn't churn between saves.
func (m *Model) collapsedGroups() []string {
	var out []string
	seen := map[string]bool{}
	for _, k := range m.kinds() {
		if !seen[k.Group] && m.collapsed[k.Group] {
			seen[k.Group] = true
			out = append(out, k.Group)
		}
	}
	return out
}

// groupOrder lists the sidebar's groups once each, in the order the kinds
// declare them.
func (m *Model) groupOrder() []string {
	var out []string
	seen := map[string]bool{}
	for _, k := range m.kinds() {
		if !seen[k.Group] {
			seen[k.Group] = true
			out = append(out, k.Group)
		}
	}
	return out
}

// groupKinds returns the indices of every kind in a group, folded or not.
func (m *Model) groupKinds(group string) []int {
	var out []int
	for i, k := range m.kinds() {
		if k.Group == group {
			out = append(out, i)
		}
	}
	return out
}

// toggleGroup folds or unfolds one group and remembers the choice.
func (m *Model) toggleGroup(group string) {
	if m.collapsed == nil {
		m.collapsed = map[string]bool{}
	}
	m.collapsed[group] = !m.collapsed[group]
	if m.collapsed[group] {
		m.toast = "▸ " + group + " folded"
	} else {
		m.toast = "▾ " + group
	}
	m.syncListScroll()
	m.saveConfig()
}

// setZoomed shows or hides the side panes and remembers the choice, so the
// layout comes back the same way after a restart.
func (m *Model) setZoomed(on bool) {
	if m.zoomed == on {
		return
	}
	m.zoomed = on
	m.saveConfig()
}

// listEntry is one rendered line of the Resources pane: a group header, a
// blank separator, or a kind row. The pane is built from this skeleton
// twice — once to draw it, once to work out where the selection and the
// scroll window sit — so the two can never disagree about which line is
// which.
type listEntry struct {
	group  string // non-empty on a group header
	folded bool   // that header's state
	kind   int    // index into kinds(); -1 on headers and blanks
}

func (m *Model) listEntries() []listEntry {
	searching := m.search != ""
	ks := m.kinds()

	var out []listEntry
	group := ""
	prevFolded := false
	for _, i := range m.filtered() {
		r := ks[i]
		if r.Group != group {
			group = r.Group
			folded := !searching && m.collapsed[group]
			// One blank line separates groups, except between two folded
			// ones: three headers with a gap each is more air than the two
			// words they separate deserve.
			if len(out) > 0 && !(prevFolded && folded) {
				out = append(out, listEntry{kind: -1})
			}
			prevFolded = folded
			out = append(out, listEntry{group: group, folded: folded, kind: -1})
		}
		if !searching && m.collapsed[r.Group] {
			continue
		}
		out = append(out, listEntry{kind: i})
	}
	return out
}

// listRows is how many lines of the Resources pane are on screen.
func (m *Model) listRows() int {
	return maxi(1, m.layout().midH-2)
}

// listTop is the first visible line, clamped to what there is to show. The
// view reads it rather than storing its own, so a resize can't leave the
// pane scrolled past its end.
func (m *Model) listTop(total int) int {
	return clamp(m.listScroll, 0, maxi(0, total-m.listRows()))
}

// scrollListPane moves the Resources pane by delta lines and nothing else.
func (m *Model) scrollListPane(delta int) {
	total := len(m.listEntries())
	// From where the pane actually is, not from a stale offset a resize may
	// have left behind.
	m.listScroll = m.listTop(total) + delta
	m.listScroll = m.listTop(total)
}

// syncListScroll nudges the pane just far enough to show the selected kind.
// Selection moves the scroll; the scroll never moves the selection. A
// selection folded out of sight moves nothing — its group header carries
// the marker instead.
func (m *Model) syncListScroll() {
	entries := m.listEntries()
	avail := m.listRows()

	line := -1
	for i, e := range entries {
		if e.kind == m.resIdx {
			line = i
			break
		}
	}
	if line < 0 {
		m.listScroll = m.listTop(len(entries))
		return
	}
	top := m.listTop(len(entries))
	switch {
	case line < top:
		top = line
		// Bring the group header along: a kind scrolled to the very top
		// with no header over it reads as belonging to whichever group is
		// above the window.
		if line > 0 && entries[line-1].group != "" {
			top = line - 1
		}
	case line >= top+avail:
		top = line - avail + 1
	}
	m.listScroll = clamp(top, 0, maxi(0, len(entries)-avail))
}

// visible returns the kind indices the Resources pane is showing: the search
// matches while a search is on (folding must never hide what you searched
// for), otherwise everything in an unfolded group.
func (m *Model) visible() []int {
	f := m.filtered()
	if m.search != "" {
		return f
	}
	ks := m.kinds()
	out := make([]int, 0, len(f))
	for _, i := range f {
		if !m.collapsed[ks[i].Group] {
			out = append(out, i)
		}
	}
	return out
}

func (m *Model) filtered() []int {
	q := strings.ToLower(m.search)
	var out []int
	for i, r := range m.kinds() {
		if q == "" || strings.Contains(strings.ToLower(r.Name), q) ||
			strings.Contains(strings.ToLower(r.Short), q) ||
			strings.Contains(strings.ToLower(r.Group), q) {
			out = append(out, i)
		}
	}
	return out
}

// tableData returns the columns and rows for the currently selected
// resource, after namespace filtering and the main-panel row search
// (m.rowSearch). Every place that reads the table — selection bounds,
// rendering, click targets — goes through this so they never disagree
// about what's currently showing.
func (m *Model) tableData() ([]string, [][]string) {
	cols, rows := m.src.Rows(m.curKind().Key, m.namespace)

	// The Namespaces table doubles as the namespace switcher, so "all" leads
	// it as a first-class choice. It is synthesized here rather than in a
	// backend because it is not a namespace object — it is a view over all
	// of them. Doing it in tableData keeps rendering, selection and click
	// targets working from one definition.
	if m.curKind().Key == "namespaces" {
		rows = append([][]string{allNamespacesRow(cols)}, rows...)
	}

	if m.rowSearch != "" {
		rows = filterRows(rows, m.rowSearch)
	}
	return cols, rows
}

// showNamespaceChooser opens the Namespaces table in the main panel — the
// one route for changing namespace, whether you click the header button or
// type :ns. Enter on a row switches to it (see openSelected).
func (m *Model) showNamespaceChooser() {
	// Remember what the user was looking at: after picking a namespace they
	// almost always want the same view, not pods.
	if k := m.curKind().Key; k != "namespaces" {
		m.nsReturnKind = k
	}
	m.jumpToResource("namespaces")
	m.toast = "pick a namespace · enter switches and returns to " + m.nsReturnLabel()
}

// nsReturnLabel names the kind the chooser will go back to.
func (m *Model) nsReturnLabel() string {
	for _, k := range m.kinds() {
		if k.Key == m.nsReturnKind {
			return strings.ToLower(k.Name)
		}
	}
	return "pods"
}

// applyNamespace switches namespace and resets the table position, since the
// row set changes completely.
func (m *Model) applyNamespace(ns string) {
	m.namespace = ns
	m.rowIdx, m.rowScroll = 0, 0
	m.saveConfig()

	label := ns
	if ns == domain.AllNamespaces {
		label = "all namespaces"
	}
	m.toast = "namespace → " + label
}

// allNamespacesRow builds the synthetic leading row for the Namespaces
// table, sized to whatever columns the backend reports.
func allNamespacesRow(cols []string) []string {
	row := make([]string, len(cols))
	for i := range row {
		row[i] = "—"
	}
	if len(row) > 0 {
		row[0] = domain.AllNamespaces
	}
	return row
}

// tableTotal is the row count before the row search is applied, for the
// "matches/total" counter in the search box. The visible kind is always
// loaded by the time this runs, but fall back to the rendered rows rather
// than ever printing an unknown-count sentinel.
func (m *Model) tableTotal() int {
	if n := m.src.RowCount(m.curKind().Key, m.namespace); n != domain.CountUnknown {
		return n
	}
	_, rows := m.src.Rows(m.curKind().Key, m.namespace)
	return len(rows)
}

func filterRows(rows [][]string, q string) [][]string {
	q = strings.ToLower(q)
	var out [][]string
	for _, row := range rows {
		if strings.Contains(strings.ToLower(strings.Join(row, " ")), q) {
			out = append(out, row)
		}
	}
	return out
}

func (m *Model) curRow() []string {
	_, rows := m.tableData()
	if m.rowIdx < 0 || m.rowIdx >= len(rows) {
		return nil
	}
	return rows[m.rowIdx]
}

// curName reads the row's identity column: NAME for everything, OBJECT for
// events. Looked up by header name (not a fixed index) since :ns all
// prepends a NAMESPACE column that shifts every other column right.
func (m *Model) curName() string {
	row := m.curRow()
	if len(row) == 0 {
		return "-"
	}
	key := "NAME"
	if m.curKind().Key == "events" {
		key = "OBJECT"
	}
	cols, _ := m.tableData()
	for i, c := range cols {
		if c == key && i < len(row) {
			return row[i]
		}
	}
	return row[0]
}

// curNamespace is the namespace of the currently selected row: the active
// filter when it names one namespace, or — under :ns all — whatever that
// row's own NAMESPACE cell says, since rows there span many namespaces.
func (m *Model) curNamespace() string {
	if m.namespace != domain.AllNamespaces {
		return m.namespace
	}
	row := m.curRow()
	cols, _ := m.tableData()
	for i, c := range cols {
		if c == "NAMESPACE" && i < len(row) {
			return row[i]
		}
	}
	return m.namespace
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink, m.repaintTick()}
	if m.connect != nil {
		// The cluster is reached off the event loop, so the first frame —
		// spinner and all — is on screen before anything can block. The
		// empty name means kubeconfig's current-context, which is the only
		// cluster k10s ever opens on. startTarget is non-empty only when the
		// caller asked for a context that is not kubeconfig's to give —
		// `k10s demo`, which has to be requested by name or Connect would
		// never know to serve the demo.
		cmds = append(cmds, m.connectCmd(m.startTarget))
	}
	// Nil unless the check is on and a day has passed, so most launches make
	// no network call at all.
	if c := m.autoCheckCmd(); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

func tick() tea.Cmd { return tickEvery(2 * time.Second) }

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
}

// repaintTick polls faster while the visible kind is still loading, so a
// freshly-opened resource fills in promptly, then backs off once its cache
// has synced. Backends that don't report sync state (the offline demo) just
// get the steady rate.
func (m *Model) repaintTick() tea.Cmd {
	if m.connecting {
		return tickEvery(150 * time.Millisecond)
	}
	if supported, synced := sourceSynced(m.src, m.curKind().Key, m.namespace); supported && !synced {
		return tickEvery(150 * time.Millisecond)
	}
	return tick()
}

// ---- geometry -------------------------------------------------------------

type layout struct {
	headerH          int
	midY, midH       int
	leftW, rightW    int
	mainW            int
	promptY, promptH int
	statusY          int
}

func (m *Model) layout() layout {
	l := layout{headerH: 4, promptH: 3}
	if m.promptZoom {
		// Half the screen: enough to read a long command or an AI prompt
		// while composing it, without hiding the table entirely.
		l.promptH = clamp(m.h/2, 3, maxi(3, m.h-l.headerH-4))
	}
	l.midY = l.headerH
	l.statusY = m.h - 1
	l.promptY = m.h - 1 - l.promptH
	l.midH = l.promptY - l.midY
	if l.midH < 3 {
		l.midH = 3
	}
	l.leftW, l.rightW = 22, 24
	if m.w < 96 {
		l.leftW, l.rightW = 18, 20
	}
	if m.zoomed {
		l.leftW, l.rightW = 0, 0
	}
	l.mainW = m.w - l.leftW - l.rightW
	return l
}

func (m *Model) visibleRows() int {
	l := m.layout()
	n := l.midH - 4
	if n < 1 {
		n = 1
	}
	return n
}

// ---- update ---------------------------------------------------------------

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		if m.mode == modeShell && m.shellSess != nil {
			cols, rows := m.shellSize()
			m.shellTerm.Resize(cols, rows)
			m.shellSess.Resize(cols, rows)
		}
		return m, nil

	case tea.MouseMsg:
		return m, m.handleMouse(msg)

	case tea.KeyMsg:
		return m, m.handleKey(msg)

	case tickMsg:
		m.anim++
		return m, m.repaintTick()

	case flashDoneMsg:
		if msg.gen == m.flashGen {
			m.flashAct = ""
		}
		return m, nil

	case textResultMsg:
		m.busy = false
		if msg.err != nil {
			m.toast = "✗ " + msg.err.Error()
			return m, nil
		}
		m.showText(msg.title, msg.body)
		return m, nil

	case actionResultMsg:
		m.busy = false
		var resumeMouse tea.Cmd
		if msg.resumeMouse {
			resumeMouse = m.resumeMouseAfterExec()
		}
		if msg.err != nil {
			m.toast = "✗ " + msg.err.Error()
		} else {
			m.toast = msg.toast
		}
		return m, resumeMouse

	case updateCheckMsg:
		return m, m.handleUpdateCheck(msg)

	case updateAppliedMsg:
		return m, m.handleUpdateApplied(msg)

	case srcConnectedMsg:
		return m, m.handleConnected(msg)

	case ctxSwitchMsg:
		m.busy = false
		if msg.err != nil {
			m.toast = "✗ context " + msg.name + ": " + msg.err.Error()
			return m, nil
		}
		m.src.Close()
		m.src = msg.src
		m.resetTrends()
		m.namespace = m.src.DefaultNamespace()
		m.resIdx, m.search = 0, ""
		m.rowIdx, m.rowScroll = 0, 0
		m.rowMem = map[string]int{}
		m.saveConfig()
		m.toast = "context → " + m.src.ClusterInfo().Context
		return m, nil

	case logStartMsg:
		m.busy = false
		if errors.Is(msg.err, domain.ErrNoLogs) {
			// Not a failure — this kind simply has no logs. Show what it
			// does have rather than an error the user can do nothing about.
			m.toast = "no logs for " + msg.name + " — showing describe"
			return m, m.runFetch("describe "+msg.name, func() (string, error) {
				return m.src.Describe(msg.kind, msg.ns, msg.name)
			})
		}
		if msg.err != nil {
			m.toast = "✗ " + msg.err.Error()
			return m, nil
		}
		if m.logStop != nil {
			m.logStop()
			m.logStop = nil
		}
		m.logGen++
		m.mode = modeLogs
		m.focus = focusMain
		m.textTitle = msg.title
		m.textLines = msg.lines
		m.logKind, m.logNS, m.logName = msg.kind, msg.ns, msg.name
		m.logMore = msg.more
		m.logTail = logInitial
		m.logLoading = false
		m.logScroll = 0
		m.logFollow = true
		m.toast = msg.title
		if msg.ch == nil {
			return m, nil // history only; nothing to follow
		}
		m.logStop = msg.stop
		m.logCh = msg.ch
		return m, waitLogLine(m.logGen, m.logCh)

	case shellStartMsg:
		m.busy = false
		if errors.Is(msg.err, domain.ErrNoShell) {
			m.toast = "no interactive shell here (offline demo, or not a pod)"
			return m, nil
		}
		if msg.err != nil {
			m.toast = "✗ " + msg.err.Error()
			return m, nil
		}
		m.closeShell("")
		m.shellGen++
		m.shellSess = msg.sess
		m.shellTerm = vt10x.New(vt10x.WithSize(msg.cols, msg.rows))
		m.shellName = msg.name
		m.mode = modeShell
		m.focus = focusMain
		m.toast = "shell into " + msg.name + " · " + detachKey + " to detach"
		return m, waitShellOut(m.shellGen, msg.sess.Output())

	case shellOutMsg:
		if msg.gen != m.shellGen || m.shellTerm == nil {
			return m, nil // a stale session's output
		}
		if !msg.ok {
			m.closeShell("shell session ended")
			return m, nil
		}
		m.shellTerm.Write(msg.data)
		return m, waitShellOut(m.shellGen, m.shellSess.Output())

	case logOlderMsg:
		m.logLoading = false
		if msg.err != nil || msg.kind != m.logKind || msg.name != m.logName {
			if msg.err != nil {
				m.toast = "✗ " + msg.err.Error()
			}
			return m, nil
		}
		// Keep the newest lines we already have (the stream may have added
		// some) and prepend whatever is older than our current first line.
		if added := len(msg.lines) - len(m.textLines); added > 0 {
			m.textLines = append(msg.lines[:added:added], m.textLines...)
		}
		m.logMore = msg.more
		return m, nil

	case logLineMsg:
		if msg.gen != m.logGen {
			return m, nil // stale stream, user moved on
		}
		if !msg.ok {
			m.textLines = append(m.textLines, "── stream closed")
			return m, nil
		}
		m.textLines = append(m.textLines, msg.line)
		// While paused, a new line arriving at the bottom would shift the
		// view; keep the same content in place by growing the offset. The
		// offset counts display rows, so a line that wraps has to push by
		// every row it takes — pushing by one per line was what made a
		// paused view still creep upwards.
		if !m.logFollow {
			m.logScroll += m.logRows(msg.line)
		}
		return m, waitLogLine(m.logGen, m.logCh)

	case editFetchedMsg:
		m.busy = false
		if msg.err != nil {
			m.toast = "✗ " + msg.err.Error()
			return m, nil
		}
		c, err := editorCommand(os.Getenv("EDITOR"), msg.path)
		if err != nil {
			return m, func() tea.Msg {
				return editExitMsg{kind: msg.kind, ns: msg.ns, name: msg.name, path: msg.path, err: err}
			}
		}
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return editExitMsg{kind: msg.kind, ns: msg.ns, name: msg.name, path: msg.path, err: err}
		})

	case editExitMsg:
		// Bubble Tea releases the terminal while $EDITOR is running. Its
		// RestoreTerminal path restores raw input and the alternate screen,
		// but not mouse tracking, so explicitly put k10s's mouse mode back
		// before handling the editor result. Respect copy mode: when the user
		// deliberately disabled capture with ctrl+s, it must stay disabled.
		resumeMouse := m.resumeMouseAfterExec()
		defer os.Remove(msg.path)
		if msg.err != nil {
			m.toast = "✗ editor: " + msg.err.Error()
			return m, resumeMouse
		}
		data, err := os.ReadFile(msg.path)
		if err != nil {
			m.toast = "✗ " + err.Error()
			return m, resumeMouse
		}
		kind, ns, name := msg.kind, msg.ns, msg.name
		apply := m.runAction("✓ "+name+" updated", func() error {
			return m.src.Apply(kind, ns, name, string(data))
		})
		return m, tea.Batch(resumeMouse, apply)

	case portForwardMsg:
		m.busy = false
		if msg.err != nil {
			m.toast = "✗ " + msg.err.Error()
			return m, nil
		}
		if stop, ok := m.portFwds[msg.key]; ok && stop != nil {
			stop()
		}
		m.portFwds[msg.key] = msg.stop
		m.toast = "⟩ port-forward " + msg.key + " → " + msg.addr
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func waitLogLine(gen int, ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		return logLineMsg{gen: gen, line: line, ok: ok}
	}
}

// startBusy marks the panel as waiting on something. Every async action
// goes through here so pressing a key always produces visible feedback,
// even for actions whose only result is a toast.
func (m *Model) startBusy(label string) {
	m.busy = true
	m.busyLabel = label
}

// runFetch wraps a blocking read (describe/YAML/logs/top/AI) as an async
// tea.Cmd so the UI never blocks on network I/O.
func (m *Model) runFetch(title string, fn func() (string, error)) tea.Cmd {
	m.toast = "… " + title
	m.startBusy(title)
	return func() tea.Msg {
		body, err := fn()
		return textResultMsg{title: title, body: body, err: err}
	}
}

// runAction wraps a blocking mutating call (delete/restart/scale/cordon/
// drain/apply) as an async tea.Cmd.
func (m *Model) runAction(okToast string, fn func() error) tea.Cmd {
	m.startBusy(strings.TrimPrefix(okToast, "✓ "))
	return func() tea.Msg {
		err := fn()
		return actionResultMsg{toast: okToast, err: err}
	}
}

func (m *Model) switchContextCmd(name string) tea.Cmd {
	// No backend to switch *from* yet — retarget the connection instead, so
	// picking a context is a way out of a first connection that is hanging.
	if _, pending := m.src.(*pendingSource); pending {
		return m.connectCmd(name)
	}
	// Entering or leaving the demo means a different backend entirely, and
	// only Connect can build one. Asking the current backend to switch would
	// have the demo hand back a demo cluster wearing a real context's name,
	// which is the exact confusion the demo is not allowed to cause.
	if m.connect != nil && domain.IsDemoContext(name) != m.demoMode() {
		return m.connectCmd(name)
	}
	m.toast = "… switching context"
	m.startBusy("connecting to " + name)
	src := m.src
	return func() tea.Msg {
		ns, err := src.SwitchContext(name)
		return ctxSwitchMsg{name: name, src: ns, err: err}
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()

	// A running shell owns the keyboard: everything except the detach key
	// belongs to the program in the pod — ctrl+c included, since interrupting
	// a command in there is the whole reason you reach for it.
	if m.mode == modeShell {
		return m.handleShellKey(msg)
	}

	if key == "ctrl+c" {
		return tea.Quit
	}

	// confirm modal captures everything
	if m.confirm != nil {
		notice := m.confirm.notice
		switch key {
		case "enter", "y", "Y":
			cb := m.confirm.onOK
			m.confirm = nil
			if cb != nil {
				return cb(m)
			}
		case "esc", "n", "N", "q":
			m.confirm = nil
			if !notice {
				m.toast = "cancelled"
			}
		}
		return nil
	}

	if m.palOpen {
		return m.handlePaletteKey(msg)
	}

	// the settings modal takes priority over everything but quit
	if m.setOpen {
		return m.handleSettingsKey(msg)
	}

	// theme picker
	if m.themeOpen {
		return m.handleThemeKey(msg)
	}

	// prompt captures printable input
	if m.focus == focusPrompt {
		sug := m.suggestions()
		switch key {
		case "esc":
			// esc backs out one step at a time: shrink first, leave second.
			if m.promptZoom {
				m.promptZoom = false
				return nil
			}
			m.focus = focusMain
			m.sugIdx = 0
			m.input.Blur()
			return nil
		case "ctrl+z":
			m.promptZoom = !m.promptZoom
			return nil
		case "ctrl+a":
			m.togglePromptMode()
			return nil
		case "up":
			if len(sug) > 0 {
				m.sugIdx = (m.sugIdx - 1 + len(sug)) % len(sug)
				return nil
			}
		case "down":
			if len(sug) > 0 {
				m.sugIdx = (m.sugIdx + 1) % len(sug)
				return nil
			}
		case "tab":
			// With the popup open, tab completes the highlighted suggestion
			// so the list is usable without the mouse. Otherwise it toggles
			// back to the table, the other half of the tab pairing.
			if len(sug) > 0 {
				m.acceptSuggestion(sug[clamp(m.sugIdx, 0, len(sug)-1)])
				return nil
			}
			return m.focusNext(1)
		case "shift+tab":
			return m.focusNext(-1)
		case "enter":
			// Enter runs the highlighted suggestion outright — having to
			// pick it and then press enter again was pure friction. The one
			// exception is a command that still needs an argument: fill it
			// in so the argument can be typed.
			if len(sug) > 0 {
				c := sug[clamp(m.sugIdx, 0, len(sug)-1)]
				if c.Args != "" && !c.OptArgs && !hasArgument(m.input.Value()) {
					m.acceptSuggestion(c)
					return nil
				}
				if !hasArgument(m.input.Value()) {
					m.input.SetValue(c.Name)
				}
			}
			cmd := m.runCommand(strings.TrimSpace(m.input.Value()))
			m.input.SetValue("")
			m.sugIdx = 0
			return cmd
		}
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if v := m.input.Value(); v != before {
			m.sugIdx = 0 // the candidate list just changed underneath us
			// A command that isn't a "/" or ":" command is free text — a
			// kubectl line or an AI question — and those get long, so the
			// box grows to fit instead of scrolling a 1-row field.
			if v != "" && !isCommandPrefix(v[0]) {
				m.promptZoom = true
			} else if v == "" {
				m.promptZoom = false
			}
		}
		return cmd
	}

	// Override plugins replace pane-local and global bindings consistently.
	// Prompt, shell and modals retain ownership because typing or confirming
	// must never launch a plugin accidentally.
	if p, ok := m.pluginForKey(key, true); ok {
		return m.firePlugin(p)
	}

	// resource list: type-to-filter
	if m.focus == focusList {
		switch key {
		case "up":
			m.moveList(-1)
			return nil
		case "down":
			m.moveList(1)
			return nil
		case "esc":
			m.search = ""
			return nil
		case "backspace":
			if len(m.search) > 0 {
				rs := []rune(m.search)
				m.search = string(rs[:len(rs)-1])
				m.applySearch()
			}
			return nil
		case "tab":
			return m.focusNext(1)
		case "shift+tab":
			return m.focusNext(-1)
		case "right", "enter":
			m.focus = focusMain
			return nil
		case " ", "space":
			// Space folds the group you are standing in. While a search is
			// running it stays a search character — "custom resources" has
			// one in it — and folding is off anyway.
			if m.search == "" {
				m.toggleGroup(m.curKind().Group)
				return nil
			}
		case "left":
			// The tree-shaped half of the pairing with "right": fold this
			// group away. Space is the toggle that also opens it again.
			if m.search == "" && !m.collapsed[m.curKind().Group] {
				m.toggleGroup(m.curKind().Group)
			}
			return nil
		case ":", "/":
			return m.openPrompt(key)
		}
		if cmd, handled := m.globalShortcut(msg); handled {
			return cmd
		}
		if !isTypedText(msg) {
			if p, ok := m.pluginForKey(key, false); ok {
				return m.firePlugin(p)
			}
		}
		// Bubbletea reports a lone space as KeySpace, not KeyRunes, so it
		// has to be admitted by hand — without it "custom resources" is
		// unsearchable.
		if isTypedText(msg) && len(key) < 24 {
			m.search += string(msg.Runes)
			m.applySearch()
		}
		return nil
	}

	// main panel's own row-search box: type-to-filter, like the resource
	// list, but scoped to the table currently on screen
	if m.focus == focusMainSearch {
		switch key {
		case "up":
			m.move(-1)
			return nil
		case "down":
			m.move(1)
			return nil
		case "esc":
			m.rowSearch = ""
			m.resetRowSelection()
			m.focus = focusMain
			return nil
		case "enter":
			m.focus = focusMain
			return nil
		case "backspace":
			if len(m.rowSearch) > 0 {
				rs := []rune(m.rowSearch)
				m.rowSearch = string(rs[:len(rs)-1])
				m.resetRowSelection()
			}
			return nil
		case "tab":
			m.focus = focusMain
			return m.focusNext(1)
		case "shift+tab":
			m.focus = focusMain
			return m.focusNext(-1)
		}
		// Navigation and global shortcuts keep working while typing a
		// filter, so you never have to leave the box first. Only bare
		// printable runes are treated as search text.
		if cmd, handled := m.globalShortcut(msg); handled {
			return cmd
		}
		if !isTypedText(msg) {
			if p, ok := m.pluginForKey(key, false); ok {
				return m.firePlugin(p)
			}
		}
		if isTypedText(msg) && len(key) < 24 {
			m.rowSearch += string(msg.Runes)
			m.resetRowSelection()
		}
		return nil
	}

	// With no cluster there is no table to search or act on, so the No
	// cluster panel's own two keys take precedence over the action hotkeys
	// they collide with ("r" is Rollout Restart, which has nothing to
	// restart here).
	if m.offline && m.mode == modeTable {
		if cmd, handled := m.offlineKey(key); handled {
			return cmd
		}
	}

	// `/` while browsing a table searches its rows (like less/vim); anywhere
	// else it opens the global command prompt pre-filled with "/".
	// "f" (find) opens the focused pane's own search box; "/" is reserved
	// for the command prompt everywhere, so there is one consistent answer
	// to "where does slash go".
	// (The resource list is already type-to-filter and returns earlier, so
	// this only ever concerns the main table.)
	if key == "f" && m.focus == focusMain && m.mode == modeTable {
		m.focus = focusMainSearch
		return nil
	}

	switch key {
	case "q":
		return tea.Quit
	case ":", "/":
		return m.openPrompt(key)
	case "enter":
		// Opening an item goes to the most useful view it has: logs for
		// anything that produces them, otherwise describe.
		return m.openSelected()
	case "ctrl+p":
		return m.openPalette()
	case "ctrl+s":
		return m.toggleMouse()
	case "ctrl+a":
		if aiDisabled {
			m.noticeAI()
			return nil
		}
		m.togglePromptMode()
		m.focus = focusPrompt
		return m.input.Focus()
	case "tab":
		return m.focusNext(1)
	case "shift+tab":
		return m.focusNext(-1)
	case "T":
		m.themeIdx = (m.themeIdx + 1) % len(m.themes)
		m.toast = "theme → " + m.th().Name
		m.saveConfig()
	case "ctrl+t":
		m.themeIdx = (m.themeIdx - 1 + len(m.themes)) % len(m.themes)
		m.toast = "theme → " + m.th().Name
		m.saveConfig()
	case "z":
		m.setZoomed(!m.zoomed)
		m.toast = map[bool]string{true: "zoomed", false: "restored"}[m.zoomed]
	case "esc":
		switch {
		case m.mode == modeText || m.mode == modeLogs || m.mode == modeContexts:
			if m.logStop != nil {
				m.logStop()
				m.logStop = nil
			}
			m.mode = modeTable
		case m.zoomed:
			m.setZoomed(false)
		}
	case "up":
		m.move(-1)
		return m.maybeLoadOlder()
	case "down":
		m.move(1)
		return m.maybeLoadOlder()
	case "k":
		// "k" is a command name, not a direction: open the prompt with it
		// already typed so "k get pods" flows straight from the keystroke.
		// This is why j/k are no longer bound to movement.
		return m.openPrompt("k")
	case "pgup", "ctrl+b":
		m.move(-m.visibleRows())
		return m.maybeLoadOlder()
	case "pgdown", "ctrl+f":
		m.move(m.visibleRows())
		return m.maybeLoadOlder()
	case "home", "g":
		m.move(-1 << 20)
	case "end", "G":
		if m.mode == modeLogs {
			// End is the canonical "resume following" gesture.
			m.logScroll = 0
			m.logFollow = true
			return nil
		}
		m.move(1 << 20)

	default:
		for _, a := range Actions {
			if a.Key == key {
				return m.fireAction(a)
			}
		}
		if p, ok := m.pluginForKey(key, false); ok {
			return m.firePlugin(p)
		}
	}
	return nil
}

// openPrompt focuses the command box, optionally seeding it. The two
// command prefixes pre-fill so their popup appears immediately; a CLI name
// pre-fills so typing it starts the command it looks like.
func (m *Model) openPrompt(key string) tea.Cmd {
	m.focus = focusPrompt
	if key != "" {
		m.input.SetValue(key)
		if !isCommandPrefix(key[0]) {
			// Free text: the box grows, same as typing it would.
			m.promptZoom = true
		}
	}
	m.input.CursorEnd()
	return m.input.Focus()
}

// paneAt returns which of the three middle panes contains (x, y), and
// whether the point is inside the middle band at all.
func (m *Model) paneAt(x, y int) (focusPane, bool) {
	l := m.layout()
	if y < l.midY || y >= l.midY+l.midH {
		return 0, false // header, prompt or status bar
	}
	switch {
	case !m.zoomed && x < l.leftW:
		return focusList, true
	case !m.zoomed && x >= l.leftW+l.mainW:
		return focusActions, true
	default:
		return focusMain, true
	}
}

// focusPaneAt gives a pane keyboard focus because it was clicked or
// scrolled — anywhere inside its bounds counts, including empty space
// below the last row.
func (m *Model) focusPaneAt(x, y int) {
	pane, ok := m.paneAt(x, y)
	// Only the centre pane is focusable; clicking or scrolling the side
	// panes acts on them without moving keyboard focus there.
	if !ok || pane != focusMain {
		return
	}
	// Don't yank focus out of the row-search box just because the table it
	// filters was clicked; they're the same pane from the user's side.
	if m.focus == focusMainSearch {
		return
	}
	m.focus = focusMain
}

// scrollModal moves the selection inside whichever popup is open, so the
// wheel does something sensible there instead of leaking to the background.
func (m *Model) scrollModal(delta int) {
	switch {
	case m.themeOpen:
		if m.themeSave && delta < 0 {
			m.themeSave = false
			m.themeRow = len(m.themes) - 1
		} else if !m.themeSave {
			next := m.themeRow + delta
			if next >= len(m.themes) {
				m.themeSave = true
			} else {
				m.themeRow = clamp(next, 0, len(m.themes)-1)
			}
		}
		m.previewTheme()

	case m.palOpen:
		if hits := m.paletteHits(); len(hits) > 0 {
			m.palIdx = clamp(m.palIdx+delta, 0, len(hits)-1)
		}

	case m.setOpen:
		if m.setRow == setSaveRow {
			if delta < 0 {
				m.setRow = setRows() - 1
			}
		} else {
			next := m.setRow + delta
			if next >= setRows() {
				m.setRow = setSaveRow
			} else {
				m.setRow = clamp(next, 0, setRows()-1)
			}
		}
	}
}

// scrollPaneAt scrolls whichever pane the point (x, y) falls in, and focuses
// it — scrolling a pane is a statement of intent about where you're working.
func (m *Model) scrollPaneAt(x, y, delta int) {
	pane, ok := m.paneAt(x, y)
	if !ok {
		return
	}
	m.focusPaneAt(x, y)

	switch pane {
	case focusMain:
		m.scrollMain(delta)
	case focusList:
		// The Resources pane scrolls, but scrolling is only looking: the
		// selected kind stays selected and the main panel stays put. It
		// used to move the selection, which meant brushing the wheel on the
		// way to the table swapped out the whole view.
		m.scrollListPane(delta)
	}
}

// scrollMain scrolls the centre pane, honouring whether it's showing a
// table or a text view.
func (m *Model) scrollMain(delta int) {
	if m.mode == modeLogs {
		m.logScrollBy(-delta)
		return
	}
	if m.mode == modeText {
		m.textTop = clamp(m.textTop+delta, 0, maxi(0, len(m.textLines)-(m.layout().midH-2)))
		return
	}
	_, rows := m.tableData()
	m.rowIdx = clamp(m.rowIdx+delta, 0, maxi(0, len(rows)-1))
	m.rowMem[m.curKind().Key] = m.rowIdx
	m.syncScroll()
}

// doubleClickWindow is how close two clicks on the same row must be to count
// as one double-click. Bubbletea reports presses individually, so the pair
// has to be recognised here.
const doubleClickWindow = 400 * time.Millisecond

// flashLen is how long a clicked action stays lit. Long enough to register,
// short enough not to feel like a stuck state.
const flashLen = 160 * time.Millisecond

// flashAction lights an action row briefly so a click is visibly acknowledged
// even when the action itself only produces a toast.
func (m *Model) flashAction(id string) tea.Cmd {
	m.flashAct = id
	m.flashGen++
	gen := m.flashGen
	return tea.Tick(flashLen, func(time.Time) tea.Msg {
		return flashDoneMsg{gen: gen}
	})
}

// openSelected is what Enter does on a row: show logs when the kind has
// them, and fall back to describe when it doesn't — every kind has a
// describe, so this always lands somewhere useful.
func (m *Model) openSelected() tea.Cmd {
	if m.mode == modeContexts {
		return m.chooseContext()
	}

	r := m.curKind()

	// On the Namespaces table, "open" means "work in this namespace" — that
	// is what you came to the list for. Describe is still on `d`.
	if r.Key == "namespaces" {
		if name := m.curName(); name != "" && name != "-" {
			m.applyNamespace(name)
			back := m.nsReturnKind
			if back == "" {
				back = "pods"
			}
			m.jumpToResource(back)
			label := name
			if name == domain.AllNamespaces {
				label = "all namespaces"
			}
			m.toast = "namespace → " + label + "   ·   " + m.nsReturnLabel()
		}
		return nil
	}

	want := domain.ALogs
	if !r.Can(want) {
		want = domain.ADescribe
	}
	for _, a := range Actions {
		if a.ID == want {
			return m.fireAction(a)
		}
	}
	return nil
}

// focusNext moves one step around the tab cycle, focusing or blurring the
// prompt's text input as needed.
func (m *Model) focusNext(dir int) tea.Cmd {
	next := nextFocus(m.focus, dir)
	if next == focusPrompt {
		return m.openPrompt("")
	}
	m.focus = next
	m.input.Blur()
	return nil
}

// globalShortcut handles the keys that must work even while a search box has
// focus — pane switching, the command prompt, zoom, copy-mode. Returns
// handled=false for anything that should be treated as search text instead.
//
// Only non-printable or modifier combinations qualify: a bare letter always
// belongs to whatever the user is typing.
func (m *Model) globalShortcut(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+p":
		return m.openPalette(), true
	case "ctrl+s":
		return m.toggleMouse(), true

	case "ctrl+z":
		m.setZoomed(!m.zoomed)
		return nil, true
	case "pgup", "ctrl+b":
		m.move(-m.visibleRows())
		return nil, true
	case "pgdown", "ctrl+f":
		m.move(m.visibleRows())
		return nil, true
	}
	return nil, false
}

// toggleMouse turns mouse capture off and on. While k10s captures the mouse
// the terminal can't do its own click-drag selection, so there is no way to
// copy text out; switching capture off hands selection back to the terminal
// (clicking rows/buttons stops working until it's switched back).
func (m *Model) toggleMouse() tea.Cmd {
	m.mouseOff = !m.mouseOff
	if m.mouseOff {
		m.toast = "mouse off — drag to select & copy · ctrl+s to re-enable clicking"
		return tea.DisableMouse
	}
	m.toast = "mouse on — click rows, actions and buttons"
	return tea.EnableMouseCellMotion
}

// resumeMouseAfterExec restores the capture mode that Bubble Tea turns off
// while an external terminal process (such as $EDITOR) is running. Returning
// nil in copy mode preserves the user's deliberate ctrl+s choice.
func (m *Model) resumeMouseAfterExec() tea.Cmd {
	if m.mouseOff {
		return nil
	}
	return tea.EnableMouseCellMotion
}

// acceptSuggestion fills the prompt with a slash command. Commands that take
// no argument are left ready to run; the rest get a trailing space so the
// user can type the argument straight away.
func (m *Model) acceptSuggestion(c SlashCommand) {
	v := c.Name
	if c.Args != "" && !c.OptArgs {
		v += " "
	}
	m.input.SetValue(v)
	m.input.CursorEnd()
	m.sugIdx = 0
}

func (m *Model) togglePromptMode() {
	if aiDisabled {
		m.noticeAI()
		return
	}
	if m.pmode == promptCmd {
		m.pmode = promptAI
		m.toast = "AI mode — plain text goes to " + m.cfg.model
	} else {
		m.pmode = promptCmd
		m.toast = "command mode"
	}
}

func (m *Model) applySearch() {
	f := m.filtered()
	if len(f) == 0 {
		return
	}
	for _, i := range f {
		if i == m.resIdx {
			return
		}
	}
	m.selectResource(f[0])
}

// isTypedText reports whether a key press is plain text for a search box.
// Space arrives as KeySpace rather than KeyRunes, and object names have
// spaces in them, so both count.
func isTypedText(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace
}

func (m *Model) moveList(delta int) {
	f := m.visible()
	if len(f) == 0 {
		return
	}
	// The selection can sit in a group that was folded after it was made.
	// Stepping from there means stepping from where it *would* be, so ↓
	// lands on the next visible kind rather than jumping to the top.
	pos, found := 0, false
	for k, i := range f {
		if i == m.resIdx {
			pos, found = k, true
			break
		}
		if i < m.resIdx {
			pos = k
		}
	}
	if !found && delta > 0 {
		pos++
		delta--
	}
	m.selectResource(f[clamp(pos+delta, 0, len(f)-1)])
}

func (m *Model) move(delta int) {
	switch m.focus {
	case focusList:
		m.moveList(delta)
	case focusActions:
	default:
		if m.mode == modeContexts {
			m.ctxIdx = clamp(m.ctxIdx+delta, 0, maxi(0, len(m.ctxChoices())-1))
			return
		}
		if m.mode == modeLogs {
			m.logScrollBy(-delta) // up the screen = toward older entries
			return
		}
		if m.mode == modeText {
			m.textTop = clamp(m.textTop+delta, 0, maxi(0, len(m.textLines)-(m.layout().midH-2)))
			return
		}
		_, rows := m.tableData()
		m.rowIdx = clamp(m.rowIdx+delta, 0, maxi(0, len(rows)-1))
		m.rowMem[m.curKind().Key] = m.rowIdx
		m.syncScroll()
	}
}

// resetRowSelection snaps back to the top row after the row-search text
// changes, since the filtered set — and its size — just changed underneath
// the current selection.
func (m *Model) resetRowSelection() {
	m.rowIdx = 0
	m.rowScroll = 0
}

func (m *Model) syncScroll() {
	vis := m.visibleRows()
	if m.rowIdx < m.rowScroll {
		m.rowScroll = m.rowIdx
	}
	if m.rowIdx >= m.rowScroll+vis {
		m.rowScroll = m.rowIdx - vis + 1
	}
	if m.rowScroll < 0 {
		m.rowScroll = 0
	}
}

func (m *Model) selectResource(i int) {
	if i == m.resIdx {
		return
	}
	m.resIdx = i
	m.mode = modeTable
	m.rowSearch = ""
	m.rowIdx = m.rowMem[m.curKind().Key]
	_, rows := m.tableData()
	m.rowIdx = clamp(m.rowIdx, 0, maxi(0, len(rows)-1))
	m.rowScroll = 0
	m.syncScroll()
	m.syncListScroll()
	m.toast = "→ " + m.curKind().Name
}

func (m *Model) fireAction(a Action) tea.Cmd {
	// The context chooser is a virtual table, not a Kubernetes resource
	// view. curKind/curName still remember the table underneath it so the UI
	// can return there after reconnecting, but acting on that hidden selection
	// would be both surprising and dangerous (D could delete an unseen pod).
	if m.mode == modeContexts {
		return nil
	}

	r := m.curKind()
	name := m.curName()
	kind := r.Key
	ns := m.curNamespace()

	if !r.Can(a.ID) {
		m.toast = fmt.Sprintf("✗ %s is not available for %s", a.Label, r.Name)
		return nil
	}
	if _, rows := m.tableData(); len(rows) == 0 {
		m.toast = "✗ nothing selected"
		return nil
	}

	switch a.ID {
	case domain.ADescribe:
		return m.runFetch("describe "+r.Short+"/"+name, func() (string, error) {
			return m.src.Describe(kind, ns, name)
		})
	case domain.AYAML:
		return m.runFetch("yaml "+r.Short+"/"+name, func() (string, error) {
			return m.src.YAML(kind, ns, name)
		})
	case domain.ALogs:
		return m.startLogs(kind, ns, name)
	case domain.ARestart:
		m.confirm = &confirmState{
			title:   "Rollout restart",
			message: []string{"Restart every pod of", r.Short + "/" + name + " ?", "", "Rolling update — zero downtime with 2+ replicas."},
			onOK: func(mm *Model) tea.Cmd {
				return mm.runAction("✓ "+r.Short+"/"+name+" restarted", func() error {
					return mm.src.Restart(kind, ns, name)
				})
			},
		}
	case domain.ADelete:
		m.confirm = &confirmState{
			title:   "Delete " + r.Short,
			danger:  true,
			message: []string{"Permanently delete", r.Short + "/" + name, "namespace: " + ns, "", "This action CANNOT be undone."},
			onOK: func(mm *Model) tea.Cmd {
				return mm.runAction("✓ "+r.Short+"/"+name+" deleted", func() error {
					return mm.src.Delete(kind, ns, name)
				})
			},
		}
	case domain.AShell:
		return m.startShellSession(kind, ns, name)
	case domain.APortFwd:
		return m.startPortForward(kind, ns, name)
	case domain.AEdit:
		return m.startEdit(kind, ns, name)
	case domain.AScale:
		total := "1"
		if row := m.curRow(); len(row) > 0 {
			cols, _ := m.tableData()
			for i, c := range cols {
				if c == "READY" && i < len(row) {
					if _, t, ok := strings.Cut(row[i], "/"); ok {
						total = t
					}
				}
			}
		}
		m.focus = focusPrompt
		m.pmode = promptCmd
		m.input.SetValue(":scale " + total)
		m.input.CursorEnd()
		return m.input.Focus()
	case domain.ATop:
		if kind == "nodes" {
			return m.runFetch("top no/"+name, func() (string, error) { return m.src.TopNode(name) })
		}
		return m.runFetch("top po/"+name, func() (string, error) { return m.src.TopPod(ns, name) })
	case domain.ACordon:
		cordoned := strings.Contains(rowStatus(m), "SchedulingDisabled")
		disable := !cordoned
		verb := "cordoned"
		if !disable {
			verb = "uncordoned"
		}
		return m.runAction(fmt.Sprintf("✓ node/%s %s", name, verb), func() error {
			return m.src.Cordon(name, disable)
		})
	case domain.ADrain:
		m.confirm = &confirmState{
			title:  "Drain node",
			danger: true,
			message: []string{
				"Cordon and evict all pods from", "no/" + name, "",
				"Pods are rescheduled onto other nodes.",
				"DaemonSet-managed pods are not evicted.",
			},
			onOK: func(mm *Model) tea.Cmd {
				return mm.runAction("✓ node/"+name+" drained", func() error {
					return mm.src.Drain(name)
				})
			},
		}
	}
	return nil
}

// rowStatus reads the STATUS cell of the current row (nodes kind), if any.
func rowStatus(m *Model) string {
	row := m.curRow()
	cols, _ := m.tableData()
	for i, c := range cols {
		if c == "STATUS" && i < len(row) {
			return row[i]
		}
	}
	return ""
}

func (m *Model) startLogs(kind, ns, name string) tea.Cmd {
	// No busy spinner here: the log view opens already scrolled to the
	// newest line, and flashing a spinner first made entering logs look
	// like it jumped. The status line inside the viewer covers the wait.
	m.toast = "… loading logs"
	src := m.src
	return func() tea.Msg {
		// Load a page of history first so the view opens with content, then
		// attach the follow stream on top of it.
		lines, more, err := src.LogsTail(kind, ns, name, logInitial)
		if err != nil {
			return logStartMsg{kind: kind, ns: ns, name: name, err: err}
		}
		ch, stop, ferr := src.LogsFollow(kind, ns, name)
		if ferr != nil {
			// History is still worth showing even if following failed.
			return logStartMsg{
				kind: kind, ns: ns, name: name,
				title: "logs " + name, lines: lines, more: more,
			}
		}
		return logStartMsg{
			kind: kind, ns: ns, name: name,
			title: "logs -f " + name, lines: lines, more: more,
			ch: ch, stop: stop,
		}
	}
}

// maybeLoadOlder fetches more history when the view has scrolled near the
// oldest loaded line, so scrolling up keeps producing content.
func (m *Model) maybeLoadOlder() tea.Cmd {
	if m.mode != modeLogs || !m.logNeedsOlder() {
		return nil
	}
	return m.loadOlderLogs()
}

// loadOlderLogs asks for a larger tail and keeps only the part that is new
// at the top. The Kubernetes API has no backwards cursor, so re-reading with
// a bigger tail is the only way to reach older entries.
func (m *Model) loadOlderLogs() tea.Cmd {
	if m.logLoading || !m.logMore {
		return nil
	}
	m.logLoading = true
	src := m.src
	kind, ns, name := m.logKind, m.logNS, m.logName
	want := m.logTail + logChunk
	m.logTail = want

	return func() tea.Msg {
		lines, more, err := src.LogsTail(kind, ns, name, want)
		return logOlderMsg{kind: kind, ns: ns, name: name, lines: lines, more: more, err: err}
	}
}

func (m *Model) startShell(kind, ns, name string) tea.Cmd {
	cmd, err := m.src.Shell(kind, ns, name)
	if err != nil {
		m.toast = "✗ " + err.Error()
		return nil
	}
	if cmd == nil {
		m.toast = "⟩ " + m.cli + " exec -it " + name + " -- /bin/sh (not supported here)"
		return nil
	}
	return tea.Exec(cmd, func(err error) tea.Msg {
		return actionResultMsg{toast: "✓ shell session closed", err: err, resumeMouse: true}
	})
}

func (m *Model) startPortForward(kind, ns, name string) tea.Cmd {
	key := kind + "/" + ns + "/" + name
	if stop, ok := m.portFwds[key]; ok && stop != nil {
		stop()
		delete(m.portFwds, key)
		m.toast = "✓ port-forward " + key + " stopped"
		return nil
	}
	m.toast = "… starting port-forward"
	m.startBusy("port-forward " + name)
	src := m.src
	return func() tea.Msg {
		addr, stop, err := src.PortForward(kind, ns, name)
		if err == nil && addr == "" && stop == nil {
			err = fmt.Errorf("port-forward is not supported here")
		}
		return portForwardMsg{key: key, addr: addr, stop: stop, err: err}
	}
}

func (m *Model) startEdit(kind, ns, name string) tea.Cmd {
	m.toast = "… opening $EDITOR for " + name
	m.startBusy("opening $EDITOR")
	src := m.src
	return func() tea.Msg {
		body, err := src.YAML(kind, ns, name)
		if err != nil {
			return editFetchedMsg{err: err}
		}
		f, err := os.CreateTemp("", "k10s-edit-*.yaml")
		if err != nil {
			return editFetchedMsg{err: err}
		}
		defer f.Close()
		if _, err := f.WriteString(body); err != nil {
			return editFetchedMsg{err: err}
		}
		return editFetchedMsg{kind: kind, ns: ns, name: name, path: f.Name()}
	}
}

func (m *Model) showText(title, body string) {
	if m.logStop != nil {
		m.logStop()
		m.logStop = nil
	}
	m.mode = modeText
	m.textTitle = title
	m.textLines = strings.Split(body, "\n")
	m.textTop = 0
	m.focus = focusMain
	m.toast = title
}

func (m *Model) closePrompt() {
	m.focus = focusMain
	m.promptZoom = false
	m.input.Blur()
}

func (m *Model) runCommand(cmd string) tea.Cmd {
	if cmd == "" {
		m.closePrompt()
		return nil
	}

	if isCommandPrefix(cmd[0]) {
		return m.runSlash(cmd)
	}

	if m.pmode == promptAI {
		m.closePrompt()
		if aiDisabled {
			m.noticeAI()
			return nil
		}
		return m.askAI(cmd)
	}

	// Anything else is a shell command: it runs, and its output opens in the
	// main panel (see shellcmd.go). Resource navigation is ":po", ":svc" …,
	// so nothing here has to guess what a line of text meant.
	m.closePrompt()
	return m.runShellCmd(cmd)
}

func (m *Model) askAI(prompt string) tea.Cmd {
	m.startBusy("asking " + m.cfg.model)
	cfg := ai.Config{
		Provider: []string{"openai", "anthropic"}[m.cfg.provider],
		BaseURL:  m.cfg.url,
		Model:    m.cfg.model,
		APIKey:   m.cfg.key,
	}
	cc := ai.Context{
		ClusterContext: m.src.ClusterInfo().Context,
		Namespace:      m.namespace,
		ResourceKind:   m.curKind().Name,
		SelectedName:   m.curName(),
	}
	title := "ai ✦ " + trunc(prompt, 48)
	m.toast = "… asking " + m.cfg.model
	return func() tea.Msg {
		body, err := ai.Ask(context.Background(), cfg, cc, prompt)
		return textResultMsg{title: title, body: body, err: err}
	}
}

func (m *Model) runSlash(cmd string) tea.Cmd {
	name, arg, _ := strings.Cut(strings.TrimSpace(cmd), " ")
	arg = strings.TrimSpace(arg)

	switch name {
	// Namespace and context moved to ":" — they name things the cluster
	// has, like every other ":" command, and that is where a k9s user
	// reaches for them. What is left under "/" is k10s's own settings.
	case "/theme":
		m.openThemePicker()
		m.closePrompt()
		return nil
	case "/settings":
		m.openSettings()
		m.closePrompt()
		return nil
	case "/mouse":
		m.closePrompt()
		return m.toggleMouse()
	case "/update":
		m.closePrompt()
		return m.startUpdate(arg)
	case "/version":
		m.showText("version", versionReport(m))
		m.closePrompt()
		return nil
	case ":ctx", ":context", ":contexts":
		m.closePrompt()
		if arg == "" {
			m.showContextChooser()
			return nil
		}
		// Validate against the same merged set the chooser displays. In Demo
		// mode the current backend only knows demo contexts; kubeconfig's real
		// contexts live in m.kubeCtxs and are how the user leaves the demo.
		if !slices.Contains(m.allContextChoices(), arg) {
			m.toast = "✗ no context named " + arg
			return nil
		}
		return m.switchContextCmd(arg)
	case ":aliases", ":alias":
		m.showText("aliases", aliasReport(m.kinds()))
		m.closePrompt()
		return nil
	case ":q", ":quit", ":qa", ":q!":
		return tea.Quit

	case ":scale":
		n, err := strconv.Atoi(arg)
		if err != nil || n < 0 {
			m.toast = "usage: /scale <replicas>"
			m.closePrompt()
			return nil
		}
		kind, ns, rname := m.curKind().Key, m.curNamespace(), m.curName()
		m.closePrompt()
		return m.runAction(fmt.Sprintf("✓ %s scaled to %d", rname, n), func() error {
			_, err := m.src.Scale(kind, ns, rname, n)
			return err
		})
	case ":search":
		m.search = arg
		m.applySearch()
		m.focus = focusMain
		m.input.Blur()
		m.toast = "filter: " + arg
		return nil
	case ":filter":
		m.rowSearch = arg
		m.resetRowSelection()
		m.focus = focusMain
		m.input.Blur()
		if arg == "" {
			m.toast = "row filter cleared"
		} else {
			m.toast = "row filter: " + arg
		}
		return nil
	case "/demo":
		m.closePrompt()
		if m.demoMode() {
			m.toast = "already in the demo — :ctx picks a real context to leave it"
			return nil
		}
		return m.switchContextCmd(domain.DemoContext)
	case "/setup":
		m.showText("setup", SetupGuide())
	case "/help":
		m.showText("help", Help())
	default:
		// Anything else under ":" is a resource name, k9s-style — ":po",
		// ":deploy", ":ns" — optionally followed by a namespace or a filter.
		if name != "" && name[0] == ':' {
			if key, ok := kindForAlias(name, m.kinds()); ok {
				m.closePrompt()
				m.gotoKind(key, arg)
				return nil
			}
		}
		m.toast = "unknown command " + name + " — /help lists everything"
	}
	m.closePrompt()
	return nil
}

// gotoKind is what ":po", ":deploy kube-system" and ":svc api" do: open that
// kind's table, and read the optional argument as a namespace when it names
// one, otherwise as a row filter — the two things you'd want to narrow by.
func (m *Model) gotoKind(key, arg string) {
	// ":ns" alone is the namespace switcher, the same one the header button
	// opens; ":ns <name>" skips the picking and switches outright,
	// leaving you on whatever view you were already reading.
	if key == "namespaces" {
		if arg == "" {
			m.showNamespaceChooser()
			return
		}
		if m.isNamespace(arg) {
			m.applyNamespace(arg)
			return
		}
	}

	m.jumpToResource(key)
	m.mode = modeTable
	m.rowSearch = ""
	label := "→ " + m.curKind().Name

	switch {
	case arg == "":
	case m.curKind().Namespaced && m.isNamespace(arg):
		m.applyNamespace(arg)
		label += " · " + arg
	default:
		m.rowSearch = arg
		label += " · filter: " + arg
	}
	m.resetRowSelection()
	m.toast = label
}

// isNamespace reports whether a word names a namespace of this cluster, or
// the "all" sentinel — what tells ":po kube-system" from ":po nginx".
func (m *Model) isNamespace(name string) bool {
	return name == domain.AllNamespaces || slices.Contains(m.src.Namespaces(), name)
}

// jumpToResource selects a resource kind by its Key, used by /crd and /dr.
func (m *Model) jumpToResource(key string) {
	for i, r := range m.kinds() {
		if r.Key == key {
			m.revealGroup(i)
			m.selectResource(i)
			m.focus = focusMain
			return
		}
	}
}

// revealGroup unfolds the group a kind lives in. Arriving somewhere by
// command or palette should not leave you on a table whose sidebar row is
// folded out of sight — but only arriving does this: walking the list with
// the arrow keys never opens a group you folded, because folded kinds are
// not in the walk to begin with.
func (m *Model) revealGroup(kindIdx int) {
	ks := m.kinds()
	if kindIdx < 0 || kindIdx >= len(ks) || !m.collapsed[ks[kindIdx].Group] {
		return
	}
	m.collapsed[ks[kindIdx].Group] = false
	m.saveConfig()
}

// commandSet is what a prefix offers. The ":" set is built per call because
// its resource half comes from the kinds the connected backend serves.
func (m *Model) commandSet(prefix byte) []SlashCommand {
	if prefix != ':' {
		return clusterCommands
	}
	return append(append([]SlashCommand{}, appCommands...), resourceCommands(m.kinds())...)
}

// suggestions returns slash commands matching the current input.
func (m *Model) suggestions() []SlashCommand {
	v := m.input.Value()
	if m.focus != focusPrompt || v == "" || !isCommandPrefix(v[0]) {
		return nil
	}
	head, _, _ := strings.Cut(v, " ")
	head = strings.ToLower(head)
	// A fully typed name leads, even when it is also the prefix of a longer
	// one: ":pv" is PersistentVolumes, not the first of ":pvs"/":pvcs" in
	// kind order, and enter runs whatever is highlighted.
	var exact, rest []SlashCommand
	for _, c := range m.commandSet(v[0]) {
		switch {
		case c.matches(head):
			exact = append(exact, c)
		case c.prefixed(head):
			rest = append(rest, c)
		}
	}
	// A fully typed command keeps its row. Hiding the popup the moment the
	// last letter lands — which is what ":job" and ":nodes" used to do —
	// reads as "that command does not exist", exactly when you most want
	// confirmation that it does.
	return append(exact, rest...)
}

// sugRows is how many suggestions the popup can draw at once. It floats
// above the prompt, so a bare ":" — every command plus one per kind — must
// not be taller than the screen it sits in. The rest are not dropped: the
// popup scrolls (see sugTop), because a command you cannot reach might as
// well not exist.
func (m *Model) sugRows() int {
	return clamp(m.h-8, 3, 16)
}

// sugTop is the first suggestion drawn, chosen so the highlighted one is
// always among them.
func (m *Model) sugTop(n int) int {
	rows := m.sugRows()
	if n <= rows {
		return 0
	}
	cur := clamp(m.sugIdx, 0, n-1)
	top := clamp(cur-rows/2, 0, n-rows)
	return top
}

// ---- mouse ----------------------------------------------------------------

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	// Track what the pointer is over so the Actions pane can show a hover
	// highlight. Motion events arrive constantly; this must stay trivial.
	if !m.modalOpen() {
		m.hoverAct = ""
		for _, a := range Actions {
			if zone.Get("act:" + a.ID).InBounds(msg) {
				m.hoverAct = a.ID
				break
			}
		}
	}

	wheel := 0
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		wheel = -1
	case tea.MouseButtonWheelDown:
		wheel = 1
	}

	// An open popup owns the wheel. Letting it fall through to the pane
	// underneath meant scrolling a picker quietly moved the table behind
	// it — the list you were looking at stayed put and the wrong thing
	// changed.
	if wheel != 0 && m.modalOpen() {
		m.scrollModal(wheel)
		return nil
	}

	// The command popup owns it too, for the same reason: it holds more
	// entries than it can draw, so the wheel is how you get to the rest —
	// not a way to quietly scroll the table behind it.
	if wheel != 0 {
		if sug := m.suggestions(); len(sug) > 0 {
			m.sugIdx = clamp(m.sugIdx+wheel, 0, len(sug)-1)
			return nil
		}
	}

	// Otherwise the wheel scrolls whatever the pointer is over, not whatever
	// has keyboard focus — hovering a pane to scroll it shouldn't first
	// require clicking into it.
	if wheel != 0 {
		m.scrollPaneAt(msg.X, msg.Y, wheel*2)
		return m.maybeLoadOlder()
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}

	if m.confirm != nil {
		notice := m.confirm.notice
		if zone.Get("cf:ok").InBounds(msg) {
			cb := m.confirm.onOK
			m.confirm = nil
			if cb != nil {
				return cb(m)
			}
		} else if !notice && zone.Get("cf:no").InBounds(msg) {
			m.confirm = nil
			m.toast = "cancelled"
		}
		return nil
	}

	if m.palOpen {
		for i := range m.paletteHits() {
			if zone.Get(fmt.Sprintf("pal:%d", i)).InBounds(msg) {
				m.palIdx = i
				m.gotoHit(m.paletteHits()[i])
				return nil
			}
		}
		return nil
	}

	if m.setOpen {
		switch {
		case zone.Get("set:save").InBounds(msg):
			return m.closeSettings()
		case zone.Get("set:updon").InBounds(msg):
			m.setUpdateChecks(true)
			return nil
		case zone.Get("set:updoff").InBounds(msg):
			m.setUpdateChecks(false)
			return nil
		}
		for i := 0; i < setRows(); i++ {
			if zone.Get(fmt.Sprintf("set:%d", i)).InBounds(msg) {
				m.setRow = i
				return m.activateSettingRow()
			}
		}
		return nil
	}

	if m.themeOpen {
		if zone.Get("thm:save").InBounds(msg) {
			return m.saveTheme()
		}
		for i := range m.themes {
			if zone.Get(fmt.Sprintf("thm:%d", i)).InBounds(msg) {
				m.themeRow = i
				m.themeSave = false
				m.previewTheme()
			}
		}
		return nil
	}

	for i, c := range m.suggestions() {
		if zone.Get(fmt.Sprintf("sug:%d", i)).InBounds(msg) {
			m.acceptSuggestion(c)
			return nil
		}
	}

	if zone.Get("zoom").InBounds(msg) {
		m.setZoomed(!m.zoomed)
		return nil
	}
	if zone.Get("close").InBounds(msg) {
		m.mode = modeTable
		return nil
	}
	if zone.Get("updbtn").InBounds(msg) {
		return m.startUpdate("")
	}
	if zone.Get("nsbtn").InBounds(msg) {
		m.showNamespaceChooser()
		return nil
	}
	if zone.Get("theme").InBounds(msg) {
		// The same live-preview picker /theme opens — cycling blind through
		// eight themes to find one was never the nice way to choose.
		m.openThemePicker()
		return nil
	}
	if zone.Get("promptzoom").InBounds(msg) {
		m.promptZoom = !m.promptZoom
		if m.focus != focusPrompt {
			return m.openPrompt("")
		}
		return nil
	}
	if zone.Get("aimode").InBounds(msg) {
		m.togglePromptMode() // says why, when AI is disabled
		return nil
	}
	if zone.Get("prompt").InBounds(msg) {
		m.focus = focusPrompt
		return m.input.Focus()
	}
	if zone.Get("tablesearch").InBounds(msg) {
		if m.mode == modeTable {
			m.focus = focusMainSearch
		}
		return nil
	}

	for gi, g := range m.groupOrder() {
		if zone.Get(fmt.Sprintf("grp:%d", gi)).InBounds(msg) {
			m.toggleGroup(g)
			return nil
		}
	}
	for i := range m.kinds() {
		if zone.Get(fmt.Sprintf("res:%d", i)).InBounds(msg) {
			// Selecting a kind, not the pane: focus stays where it was.
			m.selectResource(i)
			return nil
		}
	}
	if m.mode == modeContexts {
		for i := range m.ctxChoices() {
			if zone.Get(fmt.Sprintf("ctxp:%d", i)).InBounds(msg) {
				m.ctxIdx = i
				return m.chooseContext()
			}
		}
		return nil
	}

	_, curRows := m.tableData()
	for i := range curRows {
		if zone.Get(fmt.Sprintf("row:%d", i)).InBounds(msg) {
			m.focus = focusMain
			m.rowIdx = i
			m.rowMem[m.curKind().Key] = i

			// A second click on the same row within the double-click window
			// opens it, same as enter.
			now := time.Now()
			double := m.lastClickRow == i && now.Sub(m.lastClickAt) < doubleClickWindow
			m.lastClickRow, m.lastClickAt = i, now
			if double {
				m.lastClickAt = time.Time{} // don't let a triple click re-fire
				return m.openSelected()
			}
			return nil
		}
	}
	for _, a := range Actions {
		if zone.Get("act:" + a.ID).InBounds(msg) {
			return tea.Batch(m.flashAction(a.ID), m.fireAction(a))
		}
	}
	for _, p := range m.availablePlugins() {
		if zone.Get("plugin:" + p.Name).InBounds(msg) {
			return m.firePlugin(p)
		}
	}

	// Nothing specific was hit, so treat the click as "work here": clicking
	// blank space inside a pane still selects that pane.
	m.focusPaneAt(msg.X, msg.Y)
	return nil
}

// ---- helpers --------------------------------------------------------------

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// mark wraps content for mouse hit-testing, disabled while a modal is up so
// the overlay never slices a marker in half.
func (m *Model) mark(id, s string) string {
	if m.modalOpen() {
		return s
	}
	return zone.Mark(id, s)
}

// modalOpen reports whether anything is overlaid on the main frame. While one
// is, background zones are not marked so an overlay can never slice a
// bubblezone marker in half.
func (m *Model) modalOpen() bool {
	return m.confirm != nil || m.setOpen || m.themeOpen || m.palOpen
}
