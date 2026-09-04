package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

func TestTrendFirstReadingIsBaseline(t *testing.T) {
	var tr trend
	tr.observe(40, 0)
	if tr.arrow(0) != 0 {
		t.Fatal("the first reading has nothing to compare against and must not draw an arrow")
	}
	tr.observe(55, 1)
	if tr.arrow(1) != 1 {
		t.Errorf("40 → 55 should point up, got %d", tr.arrow(1))
	}
	tr.observe(30, 2)
	if tr.arrow(2) != -1 {
		t.Errorf("55 → 30 should point down, got %d", tr.arrow(2))
	}
	// Unchanged readings keep the last direction until it expires.
	tr.observe(30, 3)
	if tr.arrow(3) != -1 {
		t.Error("an unchanged reading must not clear the arrow right away")
	}
	if tr.arrow(3+trendTicks+1) != 0 {
		t.Error("the arrow should expire after trendTicks quiet ticks")
	}
}

func TestMetricValue(t *testing.T) {
	cases := map[string]struct {
		n  int
		ok bool
	}{"142m": {142, true}, "310Mi": {310, true}, "38%": {38, true}, "1.2Gi": {1, true}, "-": {0, false}, "": {0, false}, "<none>": {0, false}}
	for in, want := range cases {
		n, ok := metricValue(in)
		if n != want.n || ok != want.ok {
			t.Errorf("metricValue(%q) = %d,%v want %d,%v", in, n, ok, want.n, want.ok)
		}
	}
}

// driftSource is the demo cluster with node usage under test control.
type driftSource struct {
	domain.Source
	cpu, mem int
	podCPU   string
}

func (d *driftSource) Nodes() []domain.NodeInfo {
	nodes := d.Source.Nodes()
	for i := range nodes {
		nodes[i].CPU, nodes[i].Mem = d.cpu, d.mem
	}
	return nodes
}

func (d *driftSource) Rows(kind, ns string) ([]string, [][]string) {
	cols, rows := d.Source.Rows(kind, ns)
	if kind == "pods" && d.podCPU != "" {
		for _, r := range rows {
			if r[0] == "api-gateway-7d9f4c8b6d-2xk4p" {
				r[4] = d.podCPU
			}
		}
	}
	return cols, rows
}

func headerLine(m *Model) string {
	lines := strings.Split(stripANSI(m.View()), "\n")
	for _, l := range lines {
		if strings.Contains(l, "CPU") && strings.Contains(l, "cores") {
			return l
		}
	}
	return ""
}

func TestHeaderGaugesShowTrendArrows(t *testing.T) {
	src := &driftSource{Source: mock.New(""), cpu: 40, mem: 50}
	m := newTestModel(t, src)
	dismissOnboarding(m)

	line := headerLine(m)
	if strings.ContainsAny(line, "▲▼") {
		t.Fatalf("first frame has no history to trend from: %q", line)
	}

	src.cpu, src.mem = 60, 30
	m.Update(tickMsg{})
	frame := m.View()
	line = headerLine(m)
	cpuPart, memPart, _ := strings.Cut(line, "MEM")
	if !strings.Contains(cpuPart, "▲") {
		t.Errorf("CPU 40%% → 60%% should draw ▲: %q", line)
	}
	if !strings.Contains(memPart, "▼") {
		t.Errorf("MEM 50%% → 30%% should draw ▼: %q", line)
	}
	th := m.th()
	if !strings.Contains(frame, lipgloss.NewStyle().Background(th.Bg).Foreground(th.Err).Render("▲")) {
		t.Error("a rising gauge should paint its arrow in the theme's error colour")
	}
	if !strings.Contains(frame, lipgloss.NewStyle().Background(th.Bg).Foreground(th.Ok).Render("▼")) {
		t.Error("a falling gauge should paint its arrow in the theme's ok colour")
	}

	// The arrow outlives a few unchanged frames, then goes away.
	m.Update(tickMsg{})
	if l := headerLine(m); !strings.Contains(l, "▲") {
		t.Errorf("one quiet tick must not clear the arrow: %q", l)
	}
	for i := 0; i <= trendTicks; i++ {
		m.Update(tickMsg{})
	}
	if l := headerLine(m); strings.ContainsAny(l, "▲▼") {
		t.Errorf("arrows should expire after %d quiet ticks: %q", trendTicks, l)
	}
}

func podRow(m *Model, name string) string {
	for _, l := range strings.Split(stripANSI(m.View()), "\n") {
		if strings.Contains(l, name) {
			return l
		}
	}
	return ""
}

func TestTableMetricCellsShowTrendArrows(t *testing.T) {
	src := &driftSource{Source: mock.New(""), cpu: 40, mem: 50, podCPU: "142m"}
	m := newTestModel(t, src)
	dismissOnboarding(m)
	m.jumpToResource("pods")

	// The table truncates long names, so rows are found by their number.
	const pod = " 1 api-gateway"
	if row := podRow(m, pod); strings.ContainsAny(row, "▲▼") {
		t.Fatalf("first frame has no history to trend from: %q", row)
	}
	src.podCPU = "190m"
	m.Update(tickMsg{})
	// Arrows sit in the last cell of their column, so they line up down
	// the table whatever the value's width.
	up := regexp.MustCompile(`190m +▲ `)
	if row := podRow(m, pod); !up.MatchString(row) {
		t.Errorf("142m → 190m should draw ▲ in the CPU column: %q", row)
	}
	src.podCPU = "90m"
	m.Update(tickMsg{})
	down := regexp.MustCompile(`90m +▼ `)
	if row := podRow(m, pod); !down.MatchString(row) {
		t.Errorf("190m → 90m should draw ▼ in the CPU column: %q", row)
	}
	// Other pods did not move and stay arrow-free.
	if row := podRow(m, " 5 cache-redis-0"); strings.ContainsAny(row, "▲▼") {
		t.Errorf("an unchanged pod should carry no arrow: %q", row)
	}
	// "-" is not a reading and never trends.
	if row := podRow(m, "10 payment-api"); strings.ContainsAny(row, "▲▼") {
		t.Errorf("a pod with no metrics should carry no arrow: %q", row)
	}
}
