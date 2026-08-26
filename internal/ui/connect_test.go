package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

func newStartupModel(t *testing.T, connect func(string) (domain.Source, string)) *Model {
	t.Helper()
	t.Setenv("K10S_CONFIG", t.TempDir()+"/config.yaml")
	m := NewStartup(Startup{
		Kinds:    mock.New("").Kinds(),
		Contexts: []string{"alpha", "beta"},
		Context:  "alpha",
		Connect:  connect,
	})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	return m
}

// The whole point of the startup path: a connect that never returns must
// still leave a fully rendered UI on screen, saying what it is waiting for.
func TestStartupRendersWhileConnecting(t *testing.T) {
	blocked := make(chan struct{})
	t.Cleanup(func() { close(blocked) })
	m := newStartupModel(t, func(string) (domain.Source, string) {
		<-blocked
		return mock.New(""), ""
	})
	m.Init()

	v := m.View()
	if !strings.Contains(v, "connecting to alpha") {
		t.Fatalf("main panel does not say it is connecting:\n%s", v)
	}
	// The rest of the shell is up too, not just the spinner.
	for _, want := range []string{"k10s", "Pods", "Deployments"} {
		if !strings.Contains(v, want) {
			t.Errorf("startup frame is missing %q", want)
		}
	}
}

func TestStartupSwapsInSource(t *testing.T) {
	m := newStartupModel(t, func(string) (domain.Source, string) { return mock.New(""), "" })

	cmd := m.connectCmd("")
	m.Update(cmd())

	if m.connecting {
		t.Fatal("still marked connecting after the source landed")
	}
	if got := m.src.ClusterInfo().Context; got == "alpha" || got == "" {
		t.Fatalf("source was not swapped in, context = %q", got)
	}
	if v := m.View(); strings.Contains(v, "connecting to") {
		t.Errorf("spinner still on screen after connecting:\n%s", v)
	}
}

// A first attempt that finally answers after the user retargeted the
// connection must not clobber the newer one.
func TestStaleConnectIgnored(t *testing.T) {
	m := newStartupModel(t, func(string) (domain.Source, string) { return mock.New(""), "" })

	stale := m.connectCmd("")()
	m.connectCmd("beta") // supersedes it
	m.Update(stale)

	if !m.connecting {
		t.Fatal("a superseded attempt was allowed to finish the connection")
	}
	if m.connName != "beta" {
		t.Errorf("connName = %q, want beta", m.connName)
	}
}

// Picking a context while the first connection hangs retries against that
// context instead of erroring on a backend that isn't there yet.
func TestContextPickRetargetsConnect(t *testing.T) {
	var asked []string
	m := newStartupModel(t, func(name string) (domain.Source, string) {
		asked = append(asked, name)
		return mock.New(""), ""
	})
	m.showContextChooser()
	m.ctxIdx = 1 // beta
	cmd := m.chooseContext()
	if cmd == nil {
		t.Fatal("picking a context while connecting did nothing")
	}
	cmd()
	if len(asked) != 1 || asked[0] != "beta" {
		t.Fatalf("connect asked for %v, want [beta]", asked)
	}
}

func TestPendingSourceIsASource(t *testing.T) {
	var _ domain.Source = (*pendingSource)(nil)
	p := &pendingSource{kinds: mock.New("").Kinds(), ctx: "alpha"}
	if n := p.RowCount("pods", "default"); n != domain.CountUnknown {
		t.Errorf("RowCount = %d, want CountUnknown", n)
	}
	if cols, rows := p.Rows("pods", "default"); len(cols) == 0 || rows != nil {
		t.Errorf("Rows = %v/%v, want the kind's columns and no rows", cols, rows)
	}
}
