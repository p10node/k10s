package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"k10s/internal/domain"
	"k10s/internal/update"
)

// IsAsyncMsg reports whether msg is one of this package's own async-result
// messages (as opposed to an unrelated framework message like a cursor
// blink or a tick, which recur indefinitely and aren't meant to be chased
// to completion). Used by cmd/shot to resolve real async action chains
// (describe/YAML/logs/actions/AI) synchronously without looping forever on
// a self-perpetuating Cmd like textinput's cursor blink.
func IsAsyncMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case textResultMsg, actionResultMsg, ctxSwitchMsg, logStartMsg, logOlderMsg, shellStartMsg, editFetchedMsg, editExitMsg, portForwardMsg, updateCheckMsg, updateAppliedMsg:
		return true
	}
	return false
}

// flashDoneMsg clears the brief highlight on a clicked action. The
// generation number means a newer click's flash isn't cancelled by an older
// one's timer.
type flashDoneMsg struct{ gen int }

// tickMsg drives periodic repaint: informer caches update in the
// background, so the UI just needs to redraw on a cadence to show it.
type tickMsg struct{}

// textResultMsg lands after an async describe/YAML/logs/top/AI fetch.
type textResultMsg struct {
	title string
	body  string
	err   error
}

// actionResultMsg lands after an async mutating call (delete, restart,
// scale, cordon, drain, apply).
type actionResultMsg struct {
	toast string
	err   error
}

// ctxSwitchMsg lands after switching kube context, which for the real
// backend rebuilds the whole client + informer cache.
type ctxSwitchMsg struct {
	name string
	src  domain.Source
	err  error
}

// logStartMsg lands after asking the backend to start following a pod's
// logs; a nil ch (no error) means "not supported here" — fall back to a
// snapshot.
type logStartMsg struct {
	kind, ns, name string
	title          string
	lines          []string // the history page the view opens with
	more           bool     // older entries remain
	ch             <-chan string
	stop           func()
	err            error
}

// logLineMsg carries one streamed log line, tagged with the generation it
// belongs to so a stale stream can't append into a view the user has since
// left.
type logLineMsg struct {
	gen  int
	line string
	ok   bool
}

// shellStartMsg lands after opening (or failing to open) an in-panel shell.
type shellStartMsg struct {
	kind, ns, name string
	sess           domain.ShellSession
	cols, rows     int
	err            error
}

// logOlderMsg lands after fetching an older page of the log.
type logOlderMsg struct {
	kind, ns, name string
	lines          []string
	more           bool
	err            error
}

// editFetchedMsg lands after fetching YAML to seed the $EDITOR flow.
type editFetchedMsg struct {
	kind, ns, name, path string
	err                  error
}

// editExitMsg lands after $EDITOR exits.
type editExitMsg struct {
	kind, ns, name, path string
	err                  error
}

// portForwardMsg lands after a port-forward session is up (or failed).
type portForwardMsg struct {
	key, addr string
	stop      func()
	err       error
}

// updateCheckMsg lands after asking GitHub for the newest release. auto
// marks the once-a-day startup check, which reports nothing when there is
// nothing to report — including its own failure, since being offline is not
// an error the user asked about.
type updateCheckMsg struct {
	rel   *update.Release
	newer bool
	auto  bool
	err   error
}

// updateAppliedMsg lands after the new binary has been swapped in.
type updateAppliedMsg struct {
	version string
	path    string
	err     error
}
