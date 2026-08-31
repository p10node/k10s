package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

// noCluster is what main.go's Connect does when there is nothing to connect
// to: no backend, and the reason.
func noCluster(reason string) func(string) (domain.Source, string) {
	return func(string) (domain.Source, string) { return nil, reason }
}

// The regression this whole state exists for: k10s used to answer "no
// cluster here" with the bundled demo cluster, so a laptop with no
// kubeconfig showed forty pods that do not exist. It must now show none.
func TestNoClusterShowsNoRows(t *testing.T) {
	m := newStartupModel(t, noCluster("invalid configuration: no server found"))
	m.Update(m.connectCmd("")())

	if !m.offline {
		t.Fatal("a failed connection did not put the UI in its no-cluster state")
	}
	if _, rows := m.src.Rows("pods", "default"); len(rows) > 0 {
		t.Fatalf("no cluster, but the backend served %d pod rows", len(rows))
	}
	if n := m.src.RowCount("pods", "default"); n != domain.CountUnknown {
		t.Errorf("RowCount = %d, want CountUnknown (no badge, not a count of nothing)", n)
	}

	v := zone.Scan(m.View())
	if !strings.Contains(v, "No cluster") {
		t.Fatalf("the frame never says there is no cluster:\n%s", v)
	}
	if !strings.Contains(v, "invalid configuration") {
		t.Errorf("the reason from the backend is not on screen:\n%s", v)
	}
	// A row from the demo data. Its absence is the point of the test.
	if strings.Contains(v, "billing-worker") {
		t.Errorf("demo data leaked into the no-cluster screen:\n%s", v)
	}
	// The way out has to be visible, not remembered.
	for _, want := range []string{"/setup", ":ctx", "retry"} {
		if !strings.Contains(v, want) {
			t.Errorf("the no-cluster panel does not mention %q", want)
		}
	}
}

// Every row of every frame is exactly the terminal width — the invariant all
// joining and overlaying depends on. A new panel is a new chance to break it.
func TestNoClusterFrameKeepsEveryRowAtTerminalWidth(t *testing.T) {
	for _, w := range []int{80, 100, 140} {
		m := newStartupModel(t, noCluster("Get \"https://127.0.0.1:6443/version\": dial tcp 127.0.0.1:6443: connect: connection refused"))
		m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		m.Update(m.connectCmd("")())

		for i, ln := range strings.Split(m.View(), "\n") {
			if got := lipgloss.Width(zone.Scan(ln)); got != m.w {
				t.Fatalf("w=%d: row %d is %d cells, want %d: %q", w, i, got, m.w, zone.Scan(ln))
			}
		}
	}
}

// Actions must fail with "there is no cluster", not with the "still
// connecting" line — the wait is over, and the two have different fixes.
func TestNoClusterActionsSayThereIsNoCluster(t *testing.T) {
	m := newStartupModel(t, noCluster("no configuration has been provided"))
	m.Update(m.connectCmd("")())

	if _, err := m.src.Describe("pods", "default", "api"); err != errNoCluster {
		t.Errorf("Describe err = %v, want errNoCluster", err)
	}
	if err := m.src.Delete("pods", "default", "api"); err != errNoCluster {
		t.Errorf("Delete err = %v, want errNoCluster", err)
	}
}

// "r" retries from the panel. It collides with Rollout Restart, which is
// exactly the sort of action that has nothing to act on here.
func TestNoClusterRetryReconnects(t *testing.T) {
	tries := 0
	m := newStartupModel(t, func(string) (domain.Source, string) {
		tries++
		if tries == 1 {
			return nil, "connection refused"
		}
		return mock.New(""), ""
	})
	m.Update(m.connectCmd("")())
	if !m.offline {
		t.Fatal("precondition: the first attempt should have failed")
	}

	cmd := m.handleKey(key("r"))
	if cmd == nil {
		t.Fatal("r did nothing on the no-cluster panel")
	}
	if !m.connecting || m.offline {
		t.Errorf("after r: connecting=%v offline=%v, want a retry in flight", m.connecting, m.offline)
	}
	m.Update(cmd())
	if m.offline {
		t.Error("a successful retry left the UI in the no-cluster state")
	}
	if tries != 2 {
		t.Errorf("connect was called %d times, want 2", tries)
	}
}

// A context switch that fails is not the same as having no cluster: the one
// already on screen still works, and throwing it away would lose the session
// for no reason.
func TestFailedContextSwitchKeepsTheLiveCluster(t *testing.T) {
	m := newStartupModel(t, func(string) (domain.Source, string) { return mock.New(""), "" })
	m.Update(m.connectCmd("")())
	live := m.src

	m.connect = noCluster("dial tcp: i/o timeout")
	m.Update(m.connectCmd("gke-prod-asia")())

	if m.offline {
		t.Error("a failed switch dropped a working cluster into the no-cluster state")
	}
	if m.src != live {
		t.Error("the live backend was replaced by the failed switch")
	}
	if !strings.Contains(m.toast, "could not connect") {
		t.Errorf("toast = %q, want it to say the switch did not happen", m.toast)
	}
}

// /setup is the long form, and it has to be readable with nothing connected
// — that is the only time anyone opens it.
func TestSetupGuideIsReachableWithNoCluster(t *testing.T) {
	m := newStartupModel(t, noCluster("no configuration has been provided"))
	m.Update(m.connectCmd("")())

	m.runSlash("/setup")
	if m.mode != modeText {
		t.Fatalf("/setup did not open a text view, mode = %v", m.mode)
	}
	body := strings.Join(m.textLines, "\n")
	for _, want := range []string{
		"kubernetes.io/docs/tasks/tools/",          // kubectl install
		"organize-cluster-access-kubeconfig",       // what ~/.kube/config is
		"kind.sigs.k8s.io", "minikube.sigs.k8s.io", // a cluster on this machine
		"kubectl cluster-info", // how to check it outside k10s
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/setup does not link %q", want)
		}
	}
	// It links rather than transcribes: a page that pasted the install steps
	// in full would be the copy that goes stale.
	if len(m.textLines) > 90 {
		t.Errorf("/setup is %d lines — link to the source instead of quoting it", len(m.textLines))
	}
}

// Picking the context you are already "on" is a retry while offline, not a
// no-op: that context is the one that just failed, and answering "already on
// X" from the No cluster panel would be both a dead end and untrue.
func TestContextPickerRetriesTheFailedContext(t *testing.T) {
	asked := 0
	m := newStartupModel(t, func(string) (domain.Source, string) {
		asked++
		return nil, "connection refused"
	})
	m.Update(m.connectCmd("")())
	if !m.offline || asked != 1 {
		t.Fatalf("precondition: offline=%v asked=%d", m.offline, asked)
	}

	m.showContextChooser()
	m.ctxIdx = 0 // "alpha" — the context that just failed
	cmd := m.chooseContext()
	if cmd == nil {
		t.Fatalf("picking the failed context did nothing, toast = %q", m.toast)
	}
	cmd()
	if asked != 2 {
		t.Errorf("connect called %d times, want a second attempt", asked)
	}
}
