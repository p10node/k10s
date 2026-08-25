package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"k10s/internal/config"
	"k10s/internal/mock"
	"k10s/internal/theme"
)

type focusPane int

const (
	focusList focusPane = iota
	focusMain
	focusActions
	focusPrompt
	focusMainSearch // typing into the table's own row-search box
)

type mainMode int

const (
	modeTable mainMode = iota
	modeText
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
	onOK    func(*Model)
}

type aiConfig struct {
	provider int // index into mock.AIProviders
	url      string
	model    string
	key      string
}

type Model struct {
	w, h int

	themeIdx int
	focus    focusPane

	resIdx    int
	search    string
	rowIdx    int
	rowScroll int
	rowSearch string // filters rows of the currently displayed table

	mode      mainMode
	textTitle string
	textLines []string
	textTop   int

	zoomed  bool
	confirm *confirmState

	pmode promptMode
	input textinput.Model
	toast string

	cfg        aiConfig
	cfgOpen    bool
	cfgRow     int
	cfgEditing bool

	ctxIdx int

	rowMem map[string]int
}

func New() *Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 512

	m := &Model{
		themeIdx: 0,
		focus:    focusMain,
		input:    ti,
		rowMem:   map[string]int{},
		toast:    "mock mode — not connected to a real cluster",
		cfg: aiConfig{
			provider: 1,
			url:      mock.AIProviders[1].URL,
			model:    mock.AIProviders[1].Model,
			key:      "sk-ant-api03-••••••••••••7f2a",
		},
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
		for i, t := range theme.Themes {
			if t.Name == c.Theme {
				m.themeIdx = i
			}
		}
	}
	if c.Context != "" {
		for i, ctx := range mock.Contexts {
			if ctx == c.Context {
				m.ctxIdx = i
				mock.Cluster.Context = ctx
			}
		}
	}
	if c.Namespace != "" {
		mock.Cluster.Namespace = c.Namespace
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
}

// saveConfig persists the current settings; failures surface as a toast but
// never interrupt the UI.
func (m *Model) saveConfig() {
	providers := []string{"openai", "anthropic"}
	err := config.Save(config.Config{
		Theme:     m.th().Name,
		Context:   mock.Cluster.Context,
		Namespace: mock.Cluster.Namespace,
		AI: config.AI{
			Provider: providers[m.cfg.provider],
			BaseURL:  m.cfg.url,
			Model:    m.cfg.model,
			APIKey:   m.cfg.key,
		},
	})
	if err != nil {
		m.toast = "config save failed: " + err.Error()
	}
}

func (m *Model) th() theme.Theme { return theme.Themes[m.themeIdx] }

func (m *Model) res() mock.Resource { return mock.Resources[m.resIdx] }

// filtered returns resource indices matching the search box.
func (m *Model) filtered() []int {
	q := strings.ToLower(m.search)
	var out []int
	for i, r := range mock.Resources {
		if q == "" || strings.Contains(strings.ToLower(r.Name), q) ||
			strings.Contains(strings.ToLower(r.Short), q) ||
			strings.Contains(strings.ToLower(r.Group), q) {
			out = append(out, i)
		}
	}
	return out
}

// tableData returns the columns and rows for the currently selected
// resource, after namespace filtering (mock.Visible) and the main-panel row
// search (m.rowSearch). Every place that reads the table — selection bounds,
// rendering, click targets — goes through this so they never disagree about
// what's currently showing.
func (m *Model) tableData() ([]string, [][]string) {
	cols, rows := mock.Visible(m.res(), mock.Cluster.Namespace)
	if m.rowSearch != "" {
		rows = filterRows(rows, m.rowSearch)
	}
	return cols, rows
}

// tableTotal is the row count before the row search is applied, for the
// "matches/total" counter in the search box.
func (m *Model) tableTotal() int {
	return mock.VisibleCount(m.res(), mock.Cluster.Namespace)
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
// events. Looked up by header name (not a fixed index) since /ns all
// prepends a NAMESPACE column that shifts every other column right.
func (m *Model) curName() string {
	row := m.curRow()
	if len(row) == 0 {
		return "-"
	}
	key := "NAME"
	if m.res().Key == "events" {
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
// filter when it names one namespace, or — under /ns all — whatever that
// row's own NAMESPACE cell says, since rows there span many namespaces.
func (m *Model) curNamespace() string {
	if mock.Cluster.Namespace != mock.AllNamespaces {
		return mock.Cluster.Namespace
	}
	row := m.curRow()
	cols, _ := m.tableData()
	for i, c := range cols {
		if c == "NAMESPACE" && i < len(row) {
			return row[i]
		}
	}
	return mock.Cluster.Namespace
}

func (m *Model) Init() tea.Cmd { return textinput.Blink }

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
		return m, nil

	case tea.MouseMsg:
		return m, m.handleMouse(msg)

	case tea.KeyMsg:
		return m, m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()

	if key == "ctrl+c" {
		return tea.Quit
	}

	// confirm modal captures everything
	if m.confirm != nil {
		switch key {
		case "enter", "y", "Y":
			cb := m.confirm.onOK
			m.confirm = nil
			if cb != nil {
				cb(m)
			}
		case "esc", "n", "N", "q":
			m.confirm = nil
			m.toast = "cancelled"
		}
		return nil
	}

	// AI settings modal
	if m.cfgOpen {
		return m.handleCfgKey(msg)
	}

	// prompt captures printable input
	if m.focus == focusPrompt {
		switch key {
		case "esc":
			m.focus = focusMain
			m.input.Blur()
			return nil
		case "ctrl+a":
			m.togglePromptMode()
			return nil
		case "enter":
			m.runCommand(strings.TrimSpace(m.input.Value()))
			m.input.SetValue("")
			return nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return cmd
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
			m.focus = focusMain
			return nil
		case "shift+tab":
			m.focus = focusActions
			return nil
		case "right", "enter":
			m.focus = focusMain
			return nil
		case ":", "/":
			return m.openPrompt(key)
		}
		if msg.Type == tea.KeyRunes && len(key) < 24 {
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
			m.focus = focusActions
			return nil
		case "shift+tab":
			m.focus = focusList
			return nil
		}
		if msg.Type == tea.KeyRunes && len(key) < 24 {
			m.rowSearch += string(msg.Runes)
			m.resetRowSelection()
		}
		return nil
	}

	// `/` while browsing a table searches its rows (like less/vim); anywhere
	// else it opens the global command prompt pre-filled with "/".
	if key == "/" && m.focus == focusMain && m.mode == modeTable {
		m.focus = focusMainSearch
		return nil
	}

	switch key {
	case "q":
		return tea.Quit
	case ":", "/":
		return m.openPrompt(key)
	case "ctrl+a":
		m.togglePromptMode()
		m.focus = focusPrompt
		return m.input.Focus()
	case "tab":
		m.focus = (m.focus + 1) % 3
	case "shift+tab":
		m.focus = (m.focus + 2) % 3
	case "T":
		m.themeIdx = (m.themeIdx + 1) % len(theme.Themes)
		m.toast = "theme → " + m.th().Name
		m.saveConfig()
	case "ctrl+t":
		m.themeIdx = (m.themeIdx - 1 + len(theme.Themes)) % len(theme.Themes)
		m.toast = "theme → " + m.th().Name
		m.saveConfig()
	case "z":
		m.zoomed = !m.zoomed
		m.toast = map[bool]string{true: "zoomed", false: "restored"}[m.zoomed]
	case "esc":
		switch {
		case m.mode == modeText:
			m.mode = modeTable
		case m.zoomed:
			m.zoomed = false
		}
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup", "ctrl+b":
		m.move(-m.visibleRows())
	case "pgdown", "ctrl+f":
		m.move(m.visibleRows())
	case "home", "g":
		m.move(-1 << 20)
	case "end", "G":
		m.move(1 << 20)
	case "left", "h":
		m.focus = focusList
	case "right":
		m.focus = focusMain
	default:
		for _, a := range mock.Actions {
			if a.Key == key {
				m.fireAction(a)
				return nil
			}
		}
	}
	return nil
}

func (m *Model) openPrompt(key string) tea.Cmd {
	m.focus = focusPrompt
	if key == "/" {
		m.input.SetValue("/")
	}
	m.input.CursorEnd()
	return m.input.Focus()
}

func (m *Model) togglePromptMode() {
	if m.pmode == promptCmd {
		m.pmode = promptAI
		m.toast = "AI mode — plain text goes to " + m.cfg.model
	} else {
		m.pmode = promptCmd
		m.toast = "command mode"
	}
}

func (m *Model) handleCfgKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()

	if m.cfgEditing {
		switch key {
		case "enter":
			v := strings.TrimSpace(m.input.Value())
			switch m.cfgRow {
			case 1:
				m.cfg.url = v
			case 2:
				m.cfg.model = v
			case 3:
				m.cfg.key = v
			}
			m.cfgEditing = false
			m.input.SetValue("")
			m.input.Blur()
			m.saveConfig()
			m.toast = "saved → " + config.Path()
		case "esc":
			m.cfgEditing = false
			m.input.SetValue("")
			m.input.Blur()
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return cmd
		}
		return nil
	}

	switch key {
	case "esc", "q":
		m.cfgOpen = false
	case "up", "k":
		m.cfgRow = clamp(m.cfgRow-1, 0, 3)
	case "down", "j", "tab":
		m.cfgRow = clamp(m.cfgRow+1, 0, 3)
	case "left", "right", "h", "l":
		if m.cfgRow == 0 {
			m.setProvider(1 - m.cfg.provider)
		}
	case "enter":
		if m.cfgRow == 0 {
			m.setProvider(1 - m.cfg.provider)
			return nil
		}
		m.cfgEditing = true
		switch m.cfgRow {
		case 1:
			m.input.SetValue(m.cfg.url)
		case 2:
			m.input.SetValue(m.cfg.model)
		case 3:
			m.input.SetValue("")
		}
		m.input.CursorEnd()
		return m.input.Focus()
	}
	return nil
}

func (m *Model) setProvider(p int) {
	m.cfg.provider = p
	m.cfg.url = mock.AIProviders[p].URL
	m.cfg.model = mock.AIProviders[p].Model
	m.saveConfig()
	m.toast = "provider → " + mock.AIProviders[p].Label
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

func (m *Model) moveList(delta int) {
	f := m.filtered()
	if len(f) == 0 {
		return
	}
	pos := 0
	for k, i := range f {
		if i == m.resIdx {
			pos = k
			break
		}
	}
	m.selectResource(f[clamp(pos+delta, 0, len(f)-1)])
}

func (m *Model) move(delta int) {
	switch m.focus {
	case focusList:
		m.moveList(delta)
	case focusActions:
	default:
		if m.mode == modeText {
			m.textTop = clamp(m.textTop+delta, 0, maxi(0, len(m.textLines)-(m.layout().midH-2)))
			return
		}
		_, rows := m.tableData()
		m.rowIdx = clamp(m.rowIdx+delta, 0, maxi(0, len(rows)-1))
		m.rowMem[m.res().Key] = m.rowIdx
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
	m.rowIdx = m.rowMem[m.res().Key]
	_, rows := m.tableData()
	m.rowIdx = clamp(m.rowIdx, 0, maxi(0, len(rows)-1))
	m.rowScroll = 0
	m.syncScroll()
	m.toast = "→ " + m.res().Name
}

func (m *Model) fireAction(a mock.Action) {
	r := m.res()
	name := m.curName()

	if !r.Can(a.ID) {
		m.toast = fmt.Sprintf("✗ %s is not available for %s", a.Label, r.Name)
		return
	}
	if _, rows := m.tableData(); len(rows) == 0 {
		m.toast = "✗ nothing selected"
		return
	}

	switch a.ID {
	case mock.ADescribe:
		m.showText("describe "+r.Short+"/"+name, mock.Describe(r.Name, name))
	case mock.AYAML:
		m.showText("yaml "+r.Short+"/"+name, mock.YAML(strings.TrimSuffix(r.Name, "s"), name))
	case mock.ALogs:
		m.showText("logs -f "+name, mock.Logs(name))
	case mock.ARestart:
		m.confirm = &confirmState{
			title:   "Rollout restart",
			message: []string{"Restart every pod of", r.Short + "/" + name + " ?", "", "Rolling update — zero downtime with 2+ replicas."},
			onOK: func(mm *Model) {
				mm.toast = "✓ " + r.Short + "/" + name + " restarted (mock)"
			},
		}
	case mock.ADelete:
		m.confirm = &confirmState{
			title:   "Delete " + r.Short,
			danger:  true,
			message: []string{"Permanently delete", r.Short + "/" + name, "namespace: " + m.curNamespace(), "", "This action CANNOT be undone."},
			onOK: func(mm *Model) {
				mm.toast = "✓ " + r.Short + "/" + name + " deleted (mock)"
			},
		}
	case mock.AShell:
		m.toast = "⟩ kubectl exec -it " + name + " -- /bin/sh (mock)"
	case mock.APortFwd:
		m.toast = "⟩ port-forward " + name + " 8080:8080 → localhost:8080 (mock)"
	case mock.AEdit:
		m.toast = "⟩ opening $EDITOR for " + r.Short + "/" + name + " (mock)"
	case mock.AScale:
		m.toast = "⟩ scale " + r.Short + "/" + name + " --replicas=? (mock)"
	case mock.ATop:
		if r.Key == "nodes" {
			m.showText("top no/"+name, mock.TopNode(name))
		} else {
			m.showText("top po/"+name, mock.TopPod(name))
		}
	case mock.ACordon:
		if mock.ToggleCordon(name) {
			m.toast = "✓ node/" + name + " cordoned (mock) — unschedulable"
		} else {
			m.toast = "✓ node/" + name + " uncordoned (mock) — schedulable"
		}
	case mock.ADrain:
		m.confirm = &confirmState{
			title:  "Drain node",
			danger: true,
			message: []string{
				"Cordon and evict all pods from", "no/" + name, "",
				"Pods are rescheduled onto other nodes.",
				"DaemonSet-managed pods are not evicted.",
			},
			onOK: func(mm *Model) {
				mock.SetCordon(name, true)
				mm.toast = "✓ node/" + name + " drained (mock) — cordoned, pods evicted"
			},
		}
	}
}

func (m *Model) showText(title, body string) {
	m.mode = modeText
	m.textTitle = title
	m.textLines = strings.Split(body, "\n")
	m.textTop = 0
	m.focus = focusMain
	m.toast = title
}

func (m *Model) closePrompt() {
	m.focus = focusMain
	m.input.Blur()
}

func (m *Model) runCommand(cmd string) {
	if cmd == "" {
		m.closePrompt()
		return
	}

	if strings.HasPrefix(cmd, "/") {
		m.runSlash(cmd)
		return
	}

	if m.pmode == promptAI {
		m.showText("ai ✦ "+trunc(cmd, 48), mock.AIAnswer(cmd))
		m.closePrompt()
		return
	}

	for i, r := range mock.Resources {
		if strings.Contains(cmd, r.Key) || strings.Contains(cmd, " "+r.Short) {
			m.selectResource(i)
			m.toast = "$ kubectl " + cmd
			m.closePrompt()
			return
		}
	}
	m.toast = "$ kubectl " + cmd + "  (mock)"
	m.closePrompt()
}

func (m *Model) runSlash(cmd string) {
	name, arg, _ := strings.Cut(strings.TrimSpace(cmd), " ")
	arg = strings.TrimSpace(arg)

	switch name {
	case "/context", "/ctx":
		if arg == "" {
			m.ctxIdx = (m.ctxIdx + 1) % len(mock.Contexts)
		} else {
			for i, c := range mock.Contexts {
				if strings.Contains(c, arg) {
					m.ctxIdx = i
					break
				}
			}
		}
		mock.Cluster.Context = mock.Contexts[m.ctxIdx]
		m.saveConfig()
		m.toast = "context → " + mock.Cluster.Context
	case "/ns", "/namespace":
		cyc := mock.NamespaceCycle()
		switch {
		case arg == "":
			cur := 0
			for i, n := range cyc {
				if n == mock.Cluster.Namespace {
					cur = i
				}
			}
			mock.Cluster.Namespace = cyc[(cur+1)%len(cyc)]
		case strings.EqualFold(arg, mock.AllNamespaces):
			mock.Cluster.Namespace = mock.AllNamespaces
		default:
			mock.Cluster.Namespace = arg
		}
		m.rowIdx, m.rowScroll = 0, 0
		m.saveConfig()
		label := mock.Cluster.Namespace
		if label == mock.AllNamespaces {
			label = "all namespaces"
		}
		m.toast = "namespace → " + label
	case "/theme":
		if arg == "" {
			m.themeIdx = (m.themeIdx + 1) % len(theme.Themes)
		} else {
			for i, t := range theme.Themes {
				if strings.Contains(t.Name, arg) {
					m.themeIdx = i
					break
				}
			}
		}
		m.saveConfig()
		m.toast = "theme → " + m.th().Name
	case "/config":
		m.cfgOpen = true
		m.cfgRow = 0
		m.closePrompt()
		return
	case "/ai":
		if arg == "" {
			m.toast = "usage: /ai <prompt>"
		} else {
			m.showText("ai ✦ "+trunc(arg, 48), mock.AIAnswer(arg))
		}
	case "/search":
		m.search = arg
		m.applySearch()
		m.focus = focusList
		m.input.Blur()
		m.toast = "filter: " + arg
		return
	case "/filter":
		m.rowSearch = arg
		m.resetRowSelection()
		m.focus = focusMain
		m.input.Blur()
		if arg == "" {
			m.toast = "row filter cleared"
		} else {
			m.toast = "row filter: " + arg
		}
		return
	case "/crd":
		m.jumpToResource("crds")
	case "/dr":
		m.jumpToResource("customresources")
	case "/help":
		m.showText("help", mock.Help())
	default:
		m.toast = "unknown command " + name + " — /help lists everything"
	}
	m.closePrompt()
}

// jumpToResource selects a resource kind by its Key, used by /crd and /dr.
func (m *Model) jumpToResource(key string) {
	for i, r := range mock.Resources {
		if r.Key == key {
			m.selectResource(i)
			m.focus = focusMain
			return
		}
	}
}

// suggestions returns slash commands matching the current input.
func (m *Model) suggestions() []mock.SlashCommand {
	v := m.input.Value()
	if m.focus != focusPrompt || !strings.HasPrefix(v, "/") {
		return nil
	}
	head, _, _ := strings.Cut(v, " ")
	var out []mock.SlashCommand
	for _, c := range mock.SlashCommands {
		if strings.HasPrefix(c.Name, head) {
			out = append(out, c)
		}
	}
	if len(out) == 1 && out[0].Name == head {
		return nil
	}
	return out
}

// ---- mouse ----------------------------------------------------------------

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Button == tea.MouseButtonWheelUp {
		m.move(-2)
		return nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		m.move(2)
		return nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}

	if m.confirm != nil {
		if zone.Get("cf:ok").InBounds(msg) {
			cb := m.confirm.onOK
			m.confirm = nil
			if cb != nil {
				cb(m)
			}
		} else if zone.Get("cf:no").InBounds(msg) {
			m.confirm = nil
			m.toast = "cancelled"
		}
		return nil
	}

	if m.cfgOpen {
		if zone.Get("cfg:openai").InBounds(msg) {
			m.setProvider(0)
		} else if zone.Get("cfg:anthropic").InBounds(msg) {
			m.setProvider(1)
		} else if zone.Get("cfg:close").InBounds(msg) {
			m.cfgOpen = false
		} else {
			for i := 0; i < 4; i++ {
				if zone.Get(fmt.Sprintf("cfg:row:%d", i)).InBounds(msg) {
					m.cfgRow = i
				}
			}
		}
		return nil
	}

	for i, c := range m.suggestions() {
		if zone.Get(fmt.Sprintf("sug:%d", i)).InBounds(msg) {
			m.input.SetValue(c.Name + " ")
			m.input.CursorEnd()
			return nil
		}
	}

	if zone.Get("zoom").InBounds(msg) {
		m.zoomed = !m.zoomed
		return nil
	}
	if zone.Get("close").InBounds(msg) {
		m.mode = modeTable
		return nil
	}
	if zone.Get("theme").InBounds(msg) {
		m.themeIdx = (m.themeIdx + 1) % len(theme.Themes)
		m.toast = "theme → " + m.th().Name
		m.saveConfig()
		return nil
	}
	if zone.Get("aimode").InBounds(msg) {
		m.togglePromptMode()
		return nil
	}
	if zone.Get("prompt").InBounds(msg) {
		m.focus = focusPrompt
		return m.input.Focus()
	}
	if zone.Get("searchbox").InBounds(msg) {
		m.focus = focusList
		return nil
	}
	if zone.Get("tablesearch").InBounds(msg) {
		if m.mode == modeTable {
			m.focus = focusMainSearch
		}
		return nil
	}

	for i := range mock.Resources {
		if zone.Get(fmt.Sprintf("res:%d", i)).InBounds(msg) {
			m.focus = focusList
			m.selectResource(i)
			return nil
		}
	}
	_, curRows := m.tableData()
	for i := range curRows {
		if zone.Get(fmt.Sprintf("row:%d", i)).InBounds(msg) {
			m.focus = focusMain
			m.rowIdx = i
			m.rowMem[m.res().Key] = i
			return nil
		}
	}
	for _, a := range mock.Actions {
		if zone.Get("act:" + a.ID).InBounds(msg) {
			m.focus = focusActions
			m.fireAction(a)
			return nil
		}
	}
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
	if m.confirm != nil || m.cfgOpen {
		return s
	}
	return zone.Mark(id, s)
}
