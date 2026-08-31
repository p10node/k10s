package ui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/domain"
)

// Connecting to a cluster is the one startup step that can take arbitrarily
// long: the API server may be unreachable, behind a VPN that is down, or
// fronted by an exec credential plugin that stalls. Doing it before the
// program starts meant the terminal sat there with nothing on it, looking
// hung. So the UI opens first against pendingSource — a backend that answers
// "nothing yet" to everything — and the real one is swapped in when it
// arrives.

// errConnecting is what every action on pendingSource returns: the answer is
// never "no", only "not yet".
var errConnecting = errors.New("still connecting to the cluster")

// Startup is what the UI can show before a connection exists, plus the
// connect call itself. Connect takes a context name ("" = kubeconfig's
// current-context) and returns the backend to use plus an optional warning
// to show as a toast — main.go falls back to the offline demo that way.
type Startup struct {
	Kinds    []domain.Kind
	Contexts []string
	Context  string
	Connect  func(context string) (domain.Source, string)
}

// NewStartup builds a model that is already renderable and connects in the
// background. The main panel shows a spinner until Connect comes back.
func NewStartup(s Startup) *Model {
	m := New(&pendingSource{kinds: s.Kinds, contexts: s.Contexts, ctx: s.Context})
	m.connect = s.Connect
	m.connecting = true
	m.connName = s.Context
	m.toast = m.withThemeWarning("connecting…")
	return m
}

// connectCmd runs Connect off the event loop. Each attempt carries a
// generation so a slow first attempt landing after the user has picked
// another context can be discarded instead of overwriting it.
func (m *Model) connectCmd(name string) tea.Cmd {
	if m.connect == nil {
		return nil
	}
	m.connecting = true
	m.connGen++
	gen, fn := m.connGen, m.connect
	m.connName = name
	if name == "" {
		m.connName = m.src.ClusterInfo().Context
	}
	m.toast = m.withThemeWarning("… connecting")
	return func() tea.Msg {
		src, warn := fn(name)
		return srcConnectedMsg{gen: gen, src: src, warn: warn}
	}
}

// handleConnected swaps in the real backend once it is up.
func (m *Model) handleConnected(msg srcConnectedMsg) tea.Cmd {
	if msg.gen != m.connGen {
		// Superseded by a later attempt: close what we won't use, keep the
		// one already in flight.
		if msg.src != nil {
			msg.src.Close()
		}
		return nil
	}
	m.connecting = false
	if msg.src == nil {
		m.toast = m.withThemeWarning("✗ could not connect")
		return nil
	}
	old := m.src
	m.src = msg.src
	if old != nil {
		old.Close()
	}
	// A namespace restored from config for *this* context wins; otherwise
	// take the context's own.
	if !m.nsPinned || m.namespace == "" {
		m.namespace = m.src.DefaultNamespace()
	}
	m.resIdx, m.search = 0, ""
	m.rowIdx, m.rowScroll = 0, 0
	m.rowMem = map[string]int{}
	switch {
	case msg.warn != "":
		m.toast = m.withThemeWarning(msg.warn)
	case m.firstRun:
		// Said once, in the status bar, instead of a first-run dialog
		// standing between you and the cluster.
		m.firstRun = false
		m.toast = m.withThemeWarning("connected to " + m.src.ClusterInfo().Context +
			"   ·   /help for keys · /settings for the CLI name and updates")
	default:
		m.toast = m.withThemeWarning("connected to " + m.src.ClusterInfo().Context)
	}
	return nil
}

// pendingSource is a domain.Source that has no cluster behind it yet.
// Everything it can answer locally (the kind list, kubeconfig contexts) it
// answers; everything else is empty or errConnecting.
type pendingSource struct {
	kinds    []domain.Kind
	contexts []string
	ctx      string
}

func (p *pendingSource) Kinds() []domain.Kind { return append([]domain.Kind(nil), p.kinds...) }

func (p *pendingSource) Rows(kind, ns string) ([]string, [][]string) {
	for _, k := range p.kinds {
		if k.Key == kind {
			return k.Cols, nil
		}
	}
	return nil, nil
}

// CountUnknown, not 0: the sidebar draws no badge rather than claiming the
// cluster is empty.
func (p *pendingSource) RowCount(kind, ns string) int { return domain.CountUnknown }

func (p *pendingSource) ClusterInfo() domain.ClusterInfo {
	return domain.ClusterInfo{Context: p.ctx, Version: "…"}
}

func (p *pendingSource) Nodes() []domain.NodeInfo { return nil }
func (p *pendingSource) DefaultNamespace() string { return "default" }
func (p *pendingSource) Contexts() []string       { return append([]string(nil), p.contexts...) }
func (p *pendingSource) Namespaces() []string     { return nil }

// SwitchContext is never reached: the model routes context switches back
// through connectCmd while a pendingSource is in place.
func (p *pendingSource) SwitchContext(name string) (domain.Source, error) {
	return nil, errConnecting
}

func (p *pendingSource) Describe(kind, ns, name string) (string, error) { return "", errConnecting }
func (p *pendingSource) YAML(kind, ns, name string) (string, error)     { return "", errConnecting }
func (p *pendingSource) Logs(kind, ns, name string) (string, error)     { return "", errConnecting }

func (p *pendingSource) LogsTail(kind, ns, name string, n int) ([]string, bool, error) {
	return nil, false, errConnecting
}

func (p *pendingSource) LogsFollow(kind, ns, name string) (<-chan string, func(), error) {
	return nil, nil, errConnecting
}

func (p *pendingSource) TopPod(ns, name string) (string, error) { return "", errConnecting }
func (p *pendingSource) TopNode(name string) (string, error)    { return "", errConnecting }

func (p *pendingSource) Delete(kind, ns, name string) error  { return errConnecting }
func (p *pendingSource) Restart(kind, ns, name string) error { return errConnecting }

func (p *pendingSource) Scale(kind, ns, name string, replicas int) (int, error) {
	return 0, errConnecting
}

func (p *pendingSource) Cordon(name string, disabled bool) error { return errConnecting }
func (p *pendingSource) Drain(name string) error                 { return errConnecting }
func (p *pendingSource) Apply(kind, ns, name, yaml string) error { return errConnecting }

func (p *pendingSource) Shell(kind, ns, name string) (tea.ExecCommand, error) {
	return nil, errConnecting
}

func (p *pendingSource) ShellSession(kind, ns, name string, cols, rows int) (domain.ShellSession, error) {
	return nil, errConnecting
}

func (p *pendingSource) PortForward(kind, ns, name string) (string, func(), error) {
	return "", nil, errConnecting
}

func (p *pendingSource) Close() {}
