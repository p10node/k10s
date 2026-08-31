package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/p10node/k10s/internal/domain"
)

// What k10s shows when there is no cluster.
//
// It used to show the bundled demo cluster (internal/mock) instead: a laptop
// with no kubeconfig at all opened onto three nodes, a CrashLoopBackOff and
// forty pods that do not exist. The status bar said "mock mode", which is not
// enough — every colour, count and gauge on screen still read as this
// machine's cluster, and the first honest signal came only when an action
// failed.
//
// So the demo is now something you ask for by name — `k10s demo`, `/demo`,
// or the k10s-demo entry in `:ctx` — and this is the default: the same UI, no
// rows, and the reason plus the way in. Nothing here is invented.

// setupDocs is the canonical guide behind the panel's links. Kept as one
// constant because it is printed in three places (the panel, /setup, the
// docs page) and they must not drift.
const setupDocsURL = "https://github.com/p10node/k10s/blob/main/docs/cluster-setup.md"

// demoMode reports whether what is on screen is the built-in demo rather
// than a cluster. It is read off the backend's own context name, so nothing
// has to be kept in sync: whatever Connect handed back for a demo context
// says so itself, and there is no separate flag to get stale.
func (m *Model) demoMode() bool {
	// Not while offline: the stub backend still carries the name of the
	// context that was being attempted, and a failed attempt at the demo is
	// not the demo. Claiming DEMO over a No cluster panel would be the same
	// class of lie this whole state exists to remove.
	return !m.offline && domain.IsDemoContext(m.src.ClusterInfo().Context)
}

// goOffline is entered when Connect comes back with no backend.
//
// A connection that fails while a working one is on screen — a context
// switch to a cluster that is down — keeps what is already there: dropping a
// live cluster because another one is unreachable loses work for no reason.
// Only a startup with nothing behind it becomes the No cluster panel.
func (m *Model) goOffline(why string) tea.Cmd {
	if _, cluserless := m.src.(*pendingSource); !cluserless {
		m.toast = m.withThemeWarning("✗ could not connect — staying on " +
			m.src.ClusterInfo().Context)
		return nil
	}
	m.offline, m.offlineWhy = true, strings.TrimSpace(why)
	m.src = &pendingSource{
		kinds:    m.src.Kinds(),
		contexts: m.src.Contexts(),
		ctx:      m.connName,
		err:      errNoCluster,
	}
	m.mode = modeTable
	m.rowIdx, m.rowScroll = 0, 0
	m.toast = m.withThemeWarning("no cluster — r retries · /setup shows how to connect one")
	return nil
}

// offlineKey handles the two keys the No cluster panel offers. Everything
// else keeps working (the sidebar, :ctx, /setup), because none of it needs a
// cluster to be useful.
func (m *Model) offlineKey(key string) (tea.Cmd, bool) {
	switch key {
	case "r", "R", "enter":
		return m.connectCmd(m.connName), true
	}
	return nil, false
}

// noClusterLines is the main panel's body while offline: what happened, then
// the shortest path to a cluster. The long form is /setup.
func (m *Model) noClusterLines(inner int) []string {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	pad := "   "

	line := func(col lipgloss.Color, text string) string {
		return s(th.Bg).Render(pad) + s(col).Render(trunc(text, inner-len(pad)))
	}
	// A two-column row: label, then the link or command it points at.
	step := func(n, label, link string) string {
		return s(th.Bg).Render(pad) + s(th.Accent).Bold(true).Render(n) +
			s(th.Bg).Render("  ") + s(th.Fg).Render(pad2(label, 22)) +
			s(th.Subtle).Render(trunc(link, maxi(0, inner-len(pad)-24)))
	}

	out := []string{
		"",
		s(th.Bg).Render(pad) + s(th.Warn).Bold(true).Render("⎈  No cluster"),
		"",
	}
	if m.offlineWhy != "" {
		out = append(out, line(th.Err, m.offlineWhy), "")
	}
	out = append(out,
		line(th.Subtle, "k10s reads the same kubeconfig kubectl does — $KUBECONFIG,"),
		line(th.Subtle, "else ~/.kube/config. Nothing there answered, so there is"),
		line(th.Subtle, "nothing to show. No demo data is being displayed."),
		"",
		step("1", "install kubectl", "https://kubernetes.io/docs/tasks/tools/"),
		step("2", "get a kubeconfig", "cloud CLI, or your cluster admin"),
		step("3", "or run one locally", "kind · minikube · k3d · Docker Desktop"),
		"",
		line(th.Subtle, "check it outside k10s:"),
		line(th.Fg, "  kubectl config current-context"),
		line(th.Fg, "  kubectl cluster-info"),
		"",
		s(th.Bg).Render(pad)+s(th.Accent2).Render("r")+s(th.Subtle).Render(" retry   ")+
			s(th.Accent2).Render(":ctx")+s(th.Subtle).Render(" another context   ")+
			s(th.Accent2).Render("/setup")+s(th.Subtle).Render(" the full guide"),
		"",
		// Offered here because this is where someone with no cluster is
		// standing, and clicking around a sample one is a fair way to
		// decide whether k10s is worth setting up. It is labelled on every
		// frame it produces, so offering it costs no honesty.
		s(th.Bg).Render(pad)+s(th.Accent2).Render("/demo")+
			s(th.Subtle).Render(" try the UI on k10s's sample cluster — clearly marked, not your machine"),
	)
	return out
}

// pad2 right-pads a plain (unstyled) label to w cells.
func pad2(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + spaces(w-len(s))
}

// SetupGuide is /setup: how to get from "no cluster" to a connected k10s.
//
// It links rather than transcribes. Installing kubectl and writing a
// kubeconfig are documented by the people who own those tools, they change,
// and a copy inside a TUI would be the version that goes stale. What is
// spelled out here is only the part that is k10s-specific: which file it
// reads, and how to check the connection before blaming the UI.
func SetupGuide() string {
	return `k10s — connecting to a cluster

k10s does not manage clusters or credentials. It reads the same kubeconfig
kubectl reads — $KUBECONFIG if set, otherwise ~/.kube/config — and shows
whatever that context can reach. No kubeconfig, no cluster, nothing to show.

  1 · INSTALL kubectl

    Official install page, all platforms:
      https://kubernetes.io/docs/tasks/tools/
      macOS    https://kubernetes.io/docs/tasks/tools/install-kubectl-macos/
      Linux    https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/
      Windows  https://kubernetes.io/docs/tasks/tools/install-kubectl-windows/

    Package managers work too: brew install kubectl · winget install
    Kubernetes.kubectl · your distro's own package.

    Check it:  kubectl version --client

  2 · GET A KUBECONFIG (~/.kube/config)

    A managed cluster writes the file for you, with its own CLI:
      EKS  aws eks update-kubeconfig --region <region> --name <cluster>
           https://docs.aws.amazon.com/eks/latest/userguide/create-kubeconfig.html
      GKE  gcloud container clusters get-credentials <cluster> --region <r>
           https://cloud.google.com/kubernetes-engine/docs/how-to/cluster-access-for-kubectl
      AKS  az aks get-credentials --resource-group <rg> --name <cluster>
           https://learn.microsoft.com/azure/aks/control-kubeconfig

    A self-managed one: ask whoever runs it for the file, and put it at
    ~/.kube/config (mode 600 — it holds credentials).

    What the file is, and how to hold several clusters at once:
      https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/
      https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/

  3 · OR RUN ONE ON THIS MACHINE

    kind             https://kind.sigs.k8s.io/docs/user/quick-start/
    minikube         https://minikube.sigs.k8s.io/docs/start/
    k3d              https://k3d.io/stable/#installation
    Docker Desktop   enable Kubernetes in Settings → Kubernetes
                     https://docs.docker.com/desktop/features/kubernetes/

    All four write the context into ~/.kube/config themselves, so k10s
    picks them up with no further setup.

  4 · CHECK, THEN COME BACK

    kubectl config get-contexts      what k10s lists under :ctx
    kubectl config current-context   what k10s opens on
    kubectl cluster-info             proves the API server actually answers

    If the last one fails, k10s cannot connect either — that is the thing
    to fix, and it is not a k10s problem.

  IN k10s

    r        retry the connection
    :ctx     pick another context from kubeconfig
    /setup   this page
    /demo    k10s's built-in demo cluster — sample data, for trying the UI
             with no cluster at all. It is a context (k10s-demo), labelled
             DEMO in the header for as long as it is up; picking any other
             context in :ctx leaves it. "k10s demo" opens straight into it.

  Full write-up, with the same links:
    ` + setupDocsURL + `
`
}
