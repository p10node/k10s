// Package domain defines the contract between the UI (internal/ui) and a
// cluster backend — either the real one (internal/k8s) or the offline demo
// (internal/mock). Neither backend package depends on the other; both depend
// only on this package, and the UI depends only on this package too.
package domain

import (
	"errors"
	"io"
	"slices"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// AllNamespaces is the :ns sentinel that shows every namespace at once.
const AllNamespaces = "all"

// ShellSession is a live exec stream: write keystrokes to it, read the
// program's output off Output, and tell it when the panel resizes.
type ShellSession interface {
	io.Writer
	// Output carries raw terminal bytes, escape sequences included — the
	// caller feeds them to a terminal emulator.
	Output() <-chan []byte
	Resize(cols, rows int)
	Close() error
}

// ErrNoShell means the backend cannot open an interactive shell here.
var ErrNoShell = errors.New("no interactive shell available")

// ErrNoLogs means the kind simply has no logs to show — a Secret, a
// Namespace, a CRD. It is not a failure: the UI shows describe instead.
// Anything else returned from a logs call is a real error worth reporting.
var ErrNoLogs = errors.New("this kind has no logs")

// CountUnknown is RowCount's answer for a kind whose data isn't loaded yet.
// A backend that watches lazily (internal/k8s) only knows counts for kinds
// the user has actually opened; the sidebar renders no badge rather than a
// misleading "0". Backends with everything in hand (internal/mock) never
// return it.
const CountUnknown = -1

// Action ids, shared between a Kind's Allowed list and Source method
// dispatch in the UI.
const (
	ADescribe = "describe"
	AYAML     = "yaml"
	ALogs     = "logs"
	AShell    = "shell"
	APortFwd  = "portfwd"
	ARestart  = "restart"
	AEdit     = "edit"
	AScale    = "scale"
	ATop      = "top"
	ACordon   = "cordon"
	ADrain    = "drain"
	ADelete   = "delete"
)

// Kind is one entry of the Resources pane: a resource kind plus which
// columns and actions apply to it.
type Kind struct {
	Key        string
	Name       string
	Short      string
	Group      string
	Namespaced bool
	Cols       []string
	Allowed    []string
}

func (k Kind) Can(id string) bool {
	return slices.Contains(k.Allowed, id)
}

// ClusterInfo is the header's identity line.
type ClusterInfo struct {
	Context    string
	Cluster    string
	User       string
	Groups     string
	Kubeconfig string
	Server     string
	Version    string
}

// NodeInfo is one row of the header's cluster-total gauges.
type NodeInfo struct {
	Name, Status, Role, Ver string
	CPU, Mem                int // percent
	Age                     string
}

// Source is everything the UI needs from a cluster backend. Rows/RowCount
// take ns == "" (meaning "default") or AllNamespaces or a specific namespace
// name, and ignore ns entirely for cluster-scoped kinds.
type Source interface {
	Kinds() []Kind
	Rows(kind, ns string) (cols []string, rows [][]string)
	RowCount(kind, ns string) int

	ClusterInfo() ClusterInfo
	Nodes() []NodeInfo
	DefaultNamespace() string

	Contexts() []string
	Namespaces() []string // does not include AllNamespaces
	SwitchContext(name string) (Source, error)

	Describe(kind, ns, name string) (string, error)
	YAML(kind, ns, name string) (string, error)
	Logs(kind, ns, name string) (string, error)
	// LogsTail returns the last n lines. Asking for more than exist returns
	// what there is with more=false, which is how the viewer knows it has
	// reached the beginning of the log.
	LogsTail(kind, ns, name string, n int) (lines []string, more bool, err error)
	// LogsFollow streams new log lines as they arrive (nil channel, nil
	// error means "not supported here" — the UI falls back to Logs).
	LogsFollow(kind, ns, name string) (lines <-chan string, stop func(), err error)
	TopPod(ns, name string) (string, error)
	TopNode(name string) (string, error)

	Delete(kind, ns, name string) error
	Restart(kind, ns, name string) error
	Scale(kind, ns, name string, replicas int) (int, error)
	Cordon(name string, disabled bool) error
	Drain(name string) error
	Apply(kind, ns, name, yaml string) error

	// Shell returns a process the UI can hand the terminal to via
	// tea.ExecProcess (nil, nil if the backend has no real shell — the UI
	// then falls back to a toast).
	Shell(kind, ns, name string) (tea.ExecCommand, error)
	// ShellSession opens an interactive shell the UI can render *inside* a
	// panel rather than by handing over the whole terminal. Returns
	// ErrNoShell when the backend can't provide one.
	ShellSession(kind, ns, name string, cols, rows int) (ShellSession, error)
	// PortForward starts forwarding in the background and returns the local
	// address plus a func to stop it.
	PortForward(kind, ns, name string) (localAddr string, stop func(), err error)

	Close()
}

// NaturalLess compares case-insensitively and treats digit runs as numbers,
// so pod-2 sorts before pod-10 the way people expect.
//
// It lives here because ordering is part of what both backends and the UI
// promise: a list that reshuffles between frames makes arrow keys jump
// around, so anything the user scrolls through must have a stable order.
func NaturalLess(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	i, j := 0, 0
	for i < len(la) && j < len(lb) {
		ca, cb := la[i], lb[j]
		if isDigit(ca) && isDigit(cb) {
			si, sj := i, j
			for i < len(la) && isDigit(la[i]) {
				i++
			}
			for j < len(lb) && isDigit(lb[j]) {
				j++
			}
			na := strings.TrimLeft(la[si:i], "0")
			nb := strings.TrimLeft(lb[sj:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return len(la)-i < len(lb)-j
}

// SortNames orders a list of object names for display.
func SortNames(names []string) {
	sort.SliceStable(names, func(i, j int) bool { return NaturalLess(names[i], names[j]) })
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
