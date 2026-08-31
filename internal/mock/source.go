package mock

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/p10node/k10s/internal/domain"
)

// Source is the offline demo backend: it implements domain.Source entirely
// from the in-memory data in this package, no network calls.
type Source struct {
	mu       sync.Mutex
	ctxIdx   int
	cordoned map[string]bool
}

// New returns a demo Source. ctx selects the starting context (by name or
// substring match); empty picks the first one.
func New(ctx string) *Source {
	s := &Source{cordoned: map[string]bool{}}
	if ctx != "" {
		for i, c := range contexts {
			if strings.Contains(c, ctx) {
				s.ctxIdx = i
				break
			}
		}
	}
	return s
}

func (s *Source) Kinds() []domain.Kind {
	out := make([]domain.Kind, len(resources))
	for i, r := range resources {
		out[i] = r.Kind
	}
	return out
}

func (s *Source) nodeRows() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make([][]string, len(clusterNodes))
	for i, n := range clusterNodes {
		status := n.Status
		if s.cordoned[n.Name] {
			status += ",SchedulingDisabled"
		}
		rows[i] = []string{n.Name, status, n.Role, n.Ver, strconv.Itoa(n.CPU) + "%", strconv.Itoa(n.Mem) + "%", n.Age}
	}
	return rows
}

func (s *Source) Rows(kind, ns string) (cols []string, rows [][]string) {
	if kind == "nodes" {
		return []string{"NAME", "STATUS", "ROLES", "VERSION", "CPU%", "MEM%", "AGE"}, s.nodeRows()
	}
	r := findResource(kind)
	if r == nil {
		return nil, nil
	}
	return visible(r, ns)
}

func (s *Source) RowCount(kind, ns string) int {
	_, rows := s.Rows(kind, ns)
	return len(rows)
}

func (s *Source) ClusterInfo() domain.ClusterInfo {
	return domain.ClusterInfo{
		Context:    contexts[s.ctxIdx],
		Cluster:    contexts[s.ctxIdx] + "-cluster",
		User:       "demo-user",
		Groups:     "demo-team",
		Kubeconfig: "(built-in demo — no kubeconfig)",
		Server:     "(built-in demo — no API server)",
		Version:    clusterVersion,
	}
}

func (s *Source) Nodes() []domain.NodeInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.NodeInfo, len(clusterNodes))
	for i, n := range clusterNodes {
		out[i] = domain.NodeInfo{Name: n.Name, Status: n.Status, Role: n.Role, Ver: n.Ver, CPU: n.CPU, Mem: n.Mem, Age: n.Age}
	}
	return out
}

func (s *Source) DefaultNamespace() string { return "default" }

func (s *Source) Contexts() []string { return append([]string(nil), contexts...) }

func (s *Source) Namespaces() []string {
	r := findResource("namespaces")
	if r == nil {
		return nil
	}
	out := make([]string, len(r.Rows))
	for i, row := range r.Rows {
		out[i] = row[0]
	}
	return out
}

func (s *Source) SwitchContext(name string) (domain.Source, error) {
	ns := New(name)
	if name == "" {
		ns.ctxIdx = (s.ctxIdx + 1) % len(contexts)
	}
	return ns, nil
}

func nameColIndex(r *resourceDef) int {
	for i, c := range r.Cols {
		if c == "NAME" || c == "OBJECT" {
			return i
		}
	}
	return 0
}

func findRow(r *resourceDef, ns, name string) []string {
	_, rows := visible(r, ns)
	idx := nameColIndex(r)
	for _, row := range rows {
		if idx < len(row) && row[idx] == name {
			return row
		}
	}
	return nil
}

func (s *Source) Describe(kind, ns, name string) (string, error) {
	return describeTpl(name), nil
}

func (s *Source) YAML(kind, ns, name string) (string, error) {
	r := findResource(kind)
	k := kind
	if r != nil {
		k = strings.TrimSuffix(r.Short, "s")
		if k == "" {
			k = kind
		}
	}
	return yamlTpl(k, name), nil
}

func (s *Source) Logs(kind, ns, name string) (string, error) {
	lines, _, err := s.LogsTail(kind, ns, name, 500)
	if err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

// LogsTail returns the last n lines of the canned log, synthesising older
// entries as the viewer scrolls back so the paging path is exercisable
// offline. It stops at demoLogDepth, which is where "more" turns false.
func (s *Source) LogsTail(kind, ns, name string, n int) ([]string, bool, error) {
	k := findResource(kind)
	if k == nil || !k.Can(domain.ALogs) {
		return nil, false, domain.ErrNoLogs
	}

	base := strings.Split(logsTpl(name), "\n")
	all := make([]string, 0, demoLogDepth)
	for i := demoLogDepth - len(base); i > 0; i-- {
		all = append(all, fmt.Sprintf("2026-08-25T07:%02d:%02d.000Z INFO  history      older entry #%d", i/60%60, i%60, i))
	}
	all = append(all, base...)

	if n >= len(all) {
		return all, false, nil
	}
	return all[len(all)-n:], true, nil
}

// demoLogDepth is how much history the demo pretends to have.
const demoLogDepth = 1200

func (s *Source) LogsFollow(kind, ns, name string) (<-chan string, func(), error) {
	return nil, nil, nil
}

func (s *Source) TopPod(ns, name string) (string, error) {
	return topPodTpl(name), nil
}

func (s *Source) TopNode(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n *node
	for i := range clusterNodes {
		if clusterNodes[i].Name == name {
			n = &clusterNodes[i]
		}
	}
	return topNodeTpl(n, s.cordoned[name]), nil
}

func (s *Source) Delete(kind, ns, name string) error {
	r := findResource(kind)
	if r == nil {
		return fmt.Errorf("unknown kind %q", kind)
	}
	idx := nameColIndex(r)
	eff := ns
	if eff == "" {
		eff = "default"
	}
	if eff == "default" {
		out := r.Rows[:0]
		for _, row := range r.Rows {
			if !(idx < len(row) && row[idx] == name) {
				out = append(out, row)
			}
		}
		r.Rows = out
		return nil
	}
	out := r.Extra[:0]
	for _, e := range r.Extra {
		if !(e.NS == eff && idx < len(e.Row) && e.Row[idx] == name) {
			out = append(out, e)
		}
	}
	r.Extra = out
	return nil
}

func (s *Source) Restart(kind, ns, name string) error { return nil }

func (s *Source) Scale(kind, ns, name string, replicas int) (int, error) {
	r := findResource(kind)
	if r == nil {
		return 0, fmt.Errorf("unknown kind %q", kind)
	}
	row := findRow(r, ns, name)
	if row == nil {
		return 0, fmt.Errorf("%s/%s not found", kind, name)
	}
	for i, c := range r.Cols {
		if c == "READY" && i < len(row) {
			// The demo has no reconciliation loop, so simulate instant
			// convergence: both sides of READY become the new target,
			// rather than leaving the old desired count stale.
			row[i] = fmt.Sprintf("%d/%d", replicas, replicas)
		}
	}
	return replicas, nil
}

func (s *Source) Cordon(name string, disabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cordoned[name] = disabled
	return nil
}

func (s *Source) Drain(name string) error {
	s.mu.Lock()
	s.cordoned[name] = true
	s.mu.Unlock()
	return nil
}

func (s *Source) Apply(kind, ns, name, yaml string) error { return nil }

func (s *Source) Shell(kind, ns, name string) (tea.ExecCommand, error) { return nil, nil }

// ShellSession is not simulated: a fake shell that accepts input and does
// nothing would be worse than saying plainly that there isn't one.
func (s *Source) ShellSession(kind, ns, name string, cols, rows int) (domain.ShellSession, error) {
	return nil, domain.ErrNoShell
}

func (s *Source) PortForward(kind, ns, name string) (string, func(), error) {
	return "", nil, nil
}

func (s *Source) Close() {}
