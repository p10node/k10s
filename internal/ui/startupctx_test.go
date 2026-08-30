package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/config"
	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

// k10s opens on kubeconfig's current-context, full stop. Anything else means
// the TUI and the kubectl in the next terminal are pointed at different
// clusters without saying so — which is a good way to run the right command
// on the wrong cluster.
func TestStartupConnectsToKubeconfigCurrentContext(t *testing.T) {
	t.Setenv("K10S_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	// A config left over from a session on another cluster.
	if err := config.Save(config.Config{Context: "gke-prod-asia", Namespace: "argocd"}); err != nil {
		t.Fatalf("config.Save: %v", err)
	}

	var asked []string
	m := NewStartup(Startup{
		Kinds:    mock.New("").Kinds(),
		Contexts: []string{"eks-staging-apse1", "gke-prod-asia"},
		Context:  "eks-staging-apse1", // kubeconfig's current-context
		Connect: func(name string) (domain.Source, string) {
			asked = append(asked, name)
			return mock.New(name), ""
		},
	})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})

	// Init's batch is not run here — timers would block — but connectCmd's
	// own bookkeeping happens as it is built, and that says who is targeted.
	m.Init()
	if !m.connecting {
		t.Fatal("startup did not begin connecting")
	}
	if m.connName != "eks-staging-apse1" {
		t.Errorf("startup targeted %q, want kubeconfig's current-context", m.connName)
	}

	// Landing on that context must not then bounce to the saved one, which
	// is what the old "context pinned in config" behaviour did.
	cmd := m.connectCmd("")
	msg := cmd()
	if len(asked) != 1 || asked[0] != "" {
		t.Fatalf("connect calls = %v, want one for \"\" (kubeconfig's choice)", asked)
	}
	connected, ok := msg.(srcConnectedMsg)
	if !ok {
		t.Fatalf("connect returned %T", msg)
	}
	if follow := m.handleConnected(connected); follow != nil {
		t.Error("connecting was followed by another switch — the saved context was applied on top")
	}
}

// The saved namespace is addressed to a cluster. Back on that cluster it
// comes back; on any other it would be a namespace that may not even exist
// there, so the context's own default wins instead.
func TestSavedNamespaceOnlyAppliesToItsOwnContext(t *testing.T) {
	t.Setenv("K10S_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	current := mock.New("").ClusterInfo().Context

	if err := config.Save(config.Config{Context: current, Namespace: "monitoring"}); err != nil {
		t.Fatalf("config.Save: %v", err)
	}
	same := New(mock.New(""))
	if same.namespace != "monitoring" {
		t.Errorf("namespace = %q, want the one saved for this context", same.namespace)
	}

	if err := config.Save(config.Config{Context: "some-other-cluster", Namespace: "monitoring"}); err != nil {
		t.Fatalf("config.Save: %v", err)
	}
	other := New(mock.New(""))
	if other.namespace == "monitoring" {
		t.Error("a namespace saved on another cluster was applied here")
	}
	if other.namespace != mock.New("").DefaultNamespace() {
		t.Errorf("namespace = %q, want the context's own default", other.namespace)
	}
	if other.nsPinned {
		t.Error("a namespace from another context should not count as pinned")
	}
}

// Switching context in-app re-addresses the saved namespace, so quitting
// right after does not leave the pair pointing at two different clusters.
func TestSwitchingContextReaddressesTheSavedNamespace(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)

	m.applyNamespace("kube-system")
	c, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if c.Namespace != "kube-system" || c.Context != m.src.ClusterInfo().Context {
		t.Fatalf("saved %q/%q, want kube-system on %q", c.Context, c.Namespace, m.src.ClusterInfo().Context)
	}

	// Mid-switch, the namespace belongs to the context being switched to.
	m.connecting, m.connName = true, "gke-prod-asia"
	m.applyNamespace("argocd")
	c, err = config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if c.Context != "gke-prod-asia" || c.Namespace != "argocd" {
		t.Errorf("saved %q/%q, want argocd on gke-prod-asia", c.Context, c.Namespace)
	}
}

// The one thing the old first-run dialog was for — telling you /settings
// exists — is said once in the status bar, when the cluster lands.
func TestFirstRunHintIsSaidOnceOnConnect(t *testing.T) {
	t.Setenv("K10S_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))

	m := NewStartup(Startup{
		Kinds:    mock.New("").Kinds(),
		Contexts: []string{"alpha"},
		Context:  "alpha",
		Connect:  func(string) (domain.Source, string) { return mock.New(""), "" },
	})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	if m.setOpen {
		t.Fatal("first run opened a dialog")
	}

	cmd := m.connectCmd("")
	m.Update(cmd())
	if !strings.Contains(m.toast, "/settings") {
		t.Errorf("toast = %q, want the one-off pointer at /settings", m.toast)
	}

	// Connecting again (a context switch, say) does not repeat it.
	cmd = m.connectCmd("")
	m.Update(cmd())
	if strings.Contains(m.toast, "/settings") {
		t.Errorf("toast = %q, want the hint said only once", m.toast)
	}
}
