package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/p10node/k10s/internal/config"
	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/k8s"
	"github.com/p10node/k10s/internal/mock"
	"github.com/p10node/k10s/internal/ui"
	"github.com/p10node/k10s/internal/update"
	"github.com/p10node/k10s/internal/version"
)

// newSource connects to the real cluster (ctx, or kubeconfig's
// current-context when empty). When there is none it returns **no backend**
// and the reason, and the UI shows its "No cluster" panel.
//
// It used to hand back the bundled demo cluster instead. That meant a
// machine with no kubeconfig opened onto forty pods, three nodes and a
// CrashLoopBackOff that existed nowhere — labelled only by a line in the
// status bar. Fake data is worth having (`k10s demo`, cmd/shot, the tests)
// but never as the answer to "what is on this machine".
//
// Two different failures land here and both count as "no cluster":
// kubeconfig that will not load or name a context, and a context whose API
// server does not answer (deleted cluster, VPN down, minikube not started).
// The second one builds a perfectly healthy client — only Ping catches it.
//
// This can block for a long time: an API server behind a downed VPN, or an
// exec credential plugin that stalls, both land here. So it is never called
// before the program starts — the UI runs it as a background command and
// shows a spinner meanwhile (see ui.Startup).
func newSource(ctx string) (domain.Source, string) {
	// The demo is a context like any other, so everything that can name a
	// context can reach it — `k10s demo`, `/demo`, and `:ctx` — and leaving
	// it is picking a different context. The warning rides along with it and
	// is repeated in the header for as long as it is on screen: sample data
	// has to keep saying so, not say so once.
	if domain.IsDemoContext(ctx) {
		return mock.New(ctx), "demo mode — sample data, not a real cluster · :ctx leaves"
	}

	store, err := k8s.NewStore("", ctx)
	if err != nil {
		return nil, noClusterReason(err)
	}
	if err := store.Ping(); err != nil {
		store.Close()
		return nil, noClusterReason(err)
	}
	return store, ""
}

// noClusterReason turns a client-go failure into the single line shown under
// "No cluster". Those errors run to several sentences and end in advice
// aimed at a different tool ("try setting KUBERNETES_MASTER…"), so only the
// first, useful clause survives.
func noClusterReason(err error) string {
	msg := strings.Join(strings.Fields(err.Error()), " ")
	for _, cut := range []string{", try setting", "; have you", " Try "} {
		if i := strings.Index(msg, cut); i > 0 {
			msg = msg[:i]
		}
	}
	return strings.TrimRight(msg, " .")
}

func main() {
	// Two things are worth doing without a TTY, so they are handled before
	// anything opens a terminal: saying which build this is, and installing
	// a new one. The second matters most when the TUI itself is what broke.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Println(version.String())
			return
		case "update", "--update", "upgrade", "self-update":
			if err := selfUpdate(); err != nil {
				fmt.Fprintln(os.Stderr, "k10s:", err)
				os.Exit(1)
			}
			return
		case "-h", "--help":
			usage()
			return
		}
	}

	k8s.SilenceLogging()

	zone.NewGlobal()
	defer zone.Close()

	// Everything the UI needs to draw its first frame comes from kubeconfig
	// alone — no request, so no hang. The connection itself happens once the
	// event loop is running.
	ctxNames, curCtx := k8s.KubeContexts("")

	// `k10s demo` opens on the demo context instead of kubeconfig's. That is
	// the whole implementation: the demo is not a mode the program is in,
	// only the context it started on. Without the argument k10s shows what
	// this machine can actually reach, and nothing else.
	if len(os.Args) > 1 && (os.Args[1] == "demo" || os.Args[1] == "--demo") {
		curCtx = domain.DemoContext
	}

	m := ui.NewStartup(ui.Startup{
		Kinds:    k8s.Kinds(),
		Contexts: ctxNames,
		Context:  curCtx,
		Connect:  newSource,
	})

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "k10s:", err)
		os.Exit(1)
	}

	// Set when the user accepted the restart offer after /update installed a
	// new binary. zone.Close has already run via defer order, and the alt
	// screen is torn down by Run, so the exec lands in a clean terminal.
	if m.Relaunch() {
		exe, err := update.TargetPath()
		if err == nil {
			err = update.Relaunch(exe)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "k10s: could not restart:", err)
			os.Exit(1)
		}
	}
}

// selfUpdate is `k10s update`: the same install the TUI's /update performs,
// with a progress line instead of a modal.
func selfUpdate() error {
	cfg, _ := config.Load()
	c := &update.Client{Repo: cfg.Update.Repo}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cur := version.Current()
	fmt.Printf("k10s %s — checking %s\n", cur, c.Repository())
	rel, newer, err := c.Check(ctx, cur)
	if err != nil {
		return err
	}
	if !newer {
		fmt.Printf("already on the latest release (%s)\n", rel.Version)
		return nil
	}
	if !rel.HasAsset() {
		return fmt.Errorf("release %s has no build for this platform — %s", rel.Tag, rel.AssetHint())
	}

	fmt.Printf("installing %s (%s)\n", rel.Version, rel.Asset.Name)
	last := -1
	res, err := update.Apply(ctx, c, rel, func(done, total int64) {
		if total <= 0 {
			return
		}
		// Whole percent only: a progress line that rewrites itself on every
		// 32 KiB chunk is just noise in a scrollback.
		if pct := int(done * 100 / total); pct != last {
			last = pct
			fmt.Printf("\r  %3d%%", pct)
		}
	})
	if err != nil {
		fmt.Println()
		return err
	}
	fmt.Printf("\rk10s %s installed at %s\n", res.Version, res.Path)
	if res.Backup != "" {
		fmt.Printf("the previous binary is still at %s — delete it when convenient\n", res.Backup)
	}
	return nil
}

func usage() {
	fmt.Println(`k10s — clickable Kubernetes TUI

usage:
  k10s              open the dashboard against the current kubeconfig context
  k10s demo         open the built-in sample cluster (fake data, no cluster
                    needed) — k10s never shows this unless you ask for it
  k10s update       install the newest release over this binary
  k10s --version    print the running build
  k10s --help       this text

No cluster? k10s reads $KUBECONFIG, else ~/.kube/config, exactly like
kubectl. /setup inside the TUI (or docs/cluster-setup.md) has the install
and kubeconfig links.

Everything else is inside the TUI: "/" for choosers and actions, ":" to act
on the current view, /help for the full keymap.`)
}
