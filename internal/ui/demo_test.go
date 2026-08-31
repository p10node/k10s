package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

// demoConnect is main.go's routing rule: a demo context name is served by the
// demo backend, anything else by the (here, fake) live one.
func demoConnect(t *testing.T) (func(string) (domain.Source, string), *[]string) {
	t.Helper()
	var asked []string
	return func(name string) (domain.Source, string) {
		asked = append(asked, name)
		if domain.IsDemoContext(name) {
			return mock.New(name), "demo mode — sample data, not a real cluster"
		}
		return &fakeLive{ctx: name}, ""
	}, &asked
}

// fakeLive stands in for k8s.Store: enough of a Source to be switched to and
// away from, serving kubeconfig's contexts the way the real one does.
type fakeLive struct {
	pendingSource
	ctx string
}

func (f *fakeLive) ClusterInfo() domain.ClusterInfo {
	return domain.ClusterInfo{Context: f.ctx, Version: "v1.31.0"}
}
func (f *fakeLive) Contexts() []string { return []string{"admin@tp3", "kind-kind"} }

func demoModel(t *testing.T) (*Model, *[]string) {
	t.Helper()
	t.Setenv("K10S_CONFIG", t.TempDir()+"/config.yaml")
	connect, asked := demoConnect(t)
	m := NewStartup(Startup{
		Kinds:    mock.New("").Kinds(),
		Contexts: []string{"admin@tp3", "kind-kind"},
		Context:  "admin@tp3",
		Connect:  connect,
	})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	dismissOnboarding(m)
	m.Update(m.connectCmd("")()) // land on the live cluster first
	return m, asked
}

// /demo enters the demo, and it goes through Connect — the UI must never
// build a demo backend itself, and asking the live backend to "switch
// context" to it would hand back a real cluster wearing the demo's name.
func TestDemoCommandEntersDemoThroughConnect(t *testing.T) {
	m, asked := demoModel(t)
	if m.demoMode() {
		t.Fatal("precondition: should have started on the live cluster")
	}

	cmd := m.runSlash("/demo")
	if cmd == nil {
		t.Fatal("/demo did nothing")
	}
	m.Update(cmd())

	if !m.demoMode() {
		t.Fatalf("/demo did not enter the demo, context = %q", m.src.ClusterInfo().Context)
	}
	if last := (*asked)[len(*asked)-1]; last != domain.DemoContext {
		t.Errorf("Connect was asked for %q, want %q", last, domain.DemoContext)
	}
	if !strings.Contains(m.toast, "demo") {
		t.Errorf("toast = %q, want it to say this is the demo", m.toast)
	}
}

// The frame has to keep saying it, not say it once: a toast scrolls away, and
// every number on screen is sample data for as long as the demo is up.
func TestDemoIsMarkedOnEveryFrame(t *testing.T) {
	m, _ := demoModel(t)
	m.Update(m.runSlash("/demo")())

	v := zone.Scan(m.View())
	if !strings.Contains(v, "DEMO") {
		t.Errorf("the header does not carry a DEMO marker:\n%s", v)
	}
	if !strings.Contains(v, domain.DemoContext) {
		t.Errorf("the header does not name the demo context:\n%s", v)
	}
}

// Leaving the demo is picking any other context — the requirement that makes
// the demo a context rather than a mode.
func TestPickingARealContextLeavesTheDemo(t *testing.T) {
	m, _ := demoModel(t)
	m.Update(m.runSlash("/demo")())
	if !m.demoMode() {
		t.Fatal("precondition: not in the demo")
	}

	m.showContextChooser()
	choices := m.ctxChoices()
	idx := -1
	for i, c := range choices {
		if c == "kind-kind" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("kubeconfig's contexts are unreachable from the demo: %v", choices)
	}
	m.ctxIdx = idx
	cmd := m.chooseContext()
	if cmd == nil {
		t.Fatal("picking a real context from the demo did nothing")
	}
	m.Update(cmd())

	if m.demoMode() {
		t.Error("still in the demo after picking a real context")
	}
	if got := m.src.ClusterInfo().Context; got != "kind-kind" {
		t.Errorf("landed on %q, want kind-kind", got)
	}
}

// The demo is always offered, from anywhere: it needs no kubeconfig and
// cannot fail, so there is no state in which it should be missing.
func TestContextPickerAlwaysOffersTheDemo(t *testing.T) {
	m, _ := demoModel(t)
	offersDemo := func(when string) {
		t.Helper()
		for _, c := range m.ctxChoices() {
			if c == domain.DemoContext {
				return
			}
		}
		t.Errorf("%s: %q is not in the picker: %v", when, domain.DemoContext, m.ctxChoices())
	}

	offersDemo("on a live cluster")
	m.Update(m.runSlash("/demo")())
	offersDemo("inside the demo")

	// And with nothing connected at all: the demo is the one thing that
	// still works there, so it must still be listed.
	m.connect = noCluster("no configuration has been provided")
	m.Update(m.connectCmd("kind-kind")())
	m.Update(m.connectCmd("kind-kind")())
	offersDemo("with no cluster")
}

// The one entry that is not a cluster has to say so where the choice is made.
func TestContextPickerLabelsTheDemo(t *testing.T) {
	m, _ := demoModel(t)
	m.showContextChooser()

	body := zone.Scan(strings.Join(m.contextBody(110, 30), "\n"))
	if !strings.Contains(body, "sample data") {
		t.Errorf("the demo row carries no label:\n%s", body)
	}
	if !strings.Contains(body, "built-in demo") && !strings.Contains(body, "own demo cluster") {
		t.Errorf("the list has no legend explaining the demo entry:\n%s", body)
	}
	if !strings.Contains(body, "leave it") {
		t.Errorf("the legend does not say how to leave the demo:\n%s", body)
	}
}

// Demo context names must be unmistakable on their own — they end up in
// screenshots, bug reports and the header of a session someone else is
// looking at.
func TestDemoContextsAreNamedAsDemos(t *testing.T) {
	for _, c := range mock.New("").Contexts() {
		if !domain.IsDemoContext(c) {
			t.Errorf("demo context %q does not read as a demo (and would not route to the demo backend)", c)
		}
	}
}

// `k10s demo` has to ask Connect for the demo *by name*. Kubeconfig cannot
// hand back a context it does not have, so a startup that just said "give me
// kubeconfig's current-context" would launch `k10s demo` into whatever the
// machine has — or, with no kubeconfig at all, into the No cluster panel.
func TestKubectlDemoArgumentConnectsToTheDemoContext(t *testing.T) {
	t.Setenv("K10S_CONFIG", t.TempDir()+"/config.yaml")
	connect, asked := demoConnect(t)
	m := NewStartup(Startup{
		Kinds:    mock.New("").Kinds(),
		Contexts: []string{"admin@tp3"},
		Context:  domain.DemoContext, // what main.go does for `k10s demo`
		Connect:  connect,
	})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m.Init()

	m.Update(m.connectCmd(m.startTarget)())
	if len(*asked) == 0 || (*asked)[len(*asked)-1] != domain.DemoContext {
		t.Fatalf("Connect was asked for %v, want %q", *asked, domain.DemoContext)
	}
	if !m.demoMode() {
		t.Errorf("`k10s demo` did not land in the demo, context = %q", m.src.ClusterInfo().Context)
	}
}

// A normal launch still follows kubeconfig — the demo must not have turned
// startup into "connect to whatever was displayed".
func TestNormalStartupStillLetsKubeconfigChoose(t *testing.T) {
	t.Setenv("K10S_CONFIG", t.TempDir()+"/config.yaml")
	connect, asked := demoConnect(t)
	m := NewStartup(Startup{
		Kinds:    mock.New("").Kinds(),
		Contexts: []string{"admin@tp3"},
		Context:  "admin@tp3",
		Connect:  connect,
	})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m.Update(m.connectCmd(m.startTarget)())

	if len(*asked) != 1 || (*asked)[0] != "" {
		t.Errorf("Connect calls = %v, want one for \"\" (kubeconfig's choice)", *asked)
	}
}

// A demo connection that never happened is not the demo: the header must not
// claim DEMO over a No cluster panel.
func TestFailedDemoConnectIsNotMarkedAsDemo(t *testing.T) {
	m := newStartupModel(t, noCluster("no configuration has been provided"))
	m.connName = domain.DemoContext
	m.Update(m.connectCmd(domain.DemoContext)())

	if !m.offline {
		t.Fatal("precondition: the attempt should have failed")
	}
	if m.demoMode() {
		t.Error("a failed connection is being reported as the demo")
	}
	if v := zone.Scan(m.View()); strings.Contains(v, "DEMO") {
		t.Errorf("the frame claims DEMO while showing No cluster:\n%s", v)
	}
}
