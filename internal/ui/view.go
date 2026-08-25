package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"k10s/internal/mock"
	"k10s/internal/theme"
)

func (m *Model) View() string {
	if m.w < 10 || m.h < 8 {
		return ""
	}
	th := m.th()
	if m.w < 72 || m.h < 22 {
		msg := fmt.Sprintf("k10s needs a terminal ≥ 72x22 (currently %dx%d)", m.w, m.h)
		b := NewBlock(m.w, m.h, th.Bg)
		return zone.Scan(b.Overlay(BlockOf(len(msg), 1, []string{
			lipgloss.NewStyle().Background(th.Bg).Foreground(th.Warn).Render(msg),
		}, th.Bg), (m.w-len(msg))/2, m.h/2).String())
	}

	l := m.layout()

	header := m.viewHeader(l)
	var mid Block
	if m.zoomed {
		mid = m.viewMain(l.mainW, l.midH)
	} else {
		mid = HJoin(
			m.viewList(l.leftW, l.midH),
			m.viewMain(l.mainW, l.midH),
			m.viewActions(l.rightW, l.midH),
		)
	}
	root := VJoin(header, mid, m.viewPrompt(l), m.viewStatus())

	if sug := m.suggestions(); len(sug) > 0 && m.confirm == nil && !m.cfgOpen {
		root = m.overlaySuggestions(root, l, sug)
	}
	if m.cfgOpen {
		root = m.overlayConfig(root)
	}
	if m.confirm != nil {
		root = m.overlayConfirm(root)
	}
	return zone.Scan(root.String())
}

// ---- header (borderless): identity + cluster totals -----------------------

func gauge(th theme.Theme, pct, width int) string {
	col := th.Ok
	switch {
	case pct >= 85:
		col = th.Err
	case pct >= 60:
		col = th.Warn
	}
	filled := pct * width / 100
	if filled > width {
		filled = width
	}
	on := lipgloss.NewStyle().Background(th.Bg).Foreground(col).Render(strings.Repeat("▰", filled))
	off := lipgloss.NewStyle().Background(th.Bg).Foreground(th.Border).Render(strings.Repeat("▱", width-filled))
	num := lipgloss.NewStyle().Background(th.Bg).Foreground(col).Render(fmt.Sprintf("%3d%%", pct))
	return on + off + " " + num
}

// nodeCap is the mocked per-node capacity used for the cluster totals.
const nodeCores, nodeGiB = 16, 64

func (m *Model) viewHeader(l layout) Block {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	inner := m.w - 2

	ready := 0
	cpuSum, memSum := 0, 0
	for _, n := range mock.Cluster.Nodes {
		if n.Status == "Ready" {
			ready++
		}
		cpuSum += n.CPU
		memSum += n.Mem
	}
	nn := len(mock.Cluster.Nodes)
	cpuPct, memPct := cpuSum/nn, memSum/nn
	usedCores := float64(cpuSum) * nodeCores / 100
	usedGiB := float64(memSum) * nodeGiB / 100

	brand := s(th.Accent).Bold(true).Render(" ⎈ k10s")
	sep := s(th.Border).Render("  │  ")
	nodeCol := th.Ok
	if ready < nn {
		nodeCol = th.Warn
	}
	line0 := brand + sep + s(th.Accent2).Render(mock.Cluster.Context) +
		sep + s(th.Subtle).Render("ver ") + s(th.Fg).Render(mock.Cluster.Version) +
		sep + s(th.Subtle).Render("ns ") + s(th.Fg).Render(mock.Cluster.Namespace) +
		sep + s(th.Subtle).Render("nodes ") + s(nodeCol).Render(fmt.Sprintf("%d/%d ready", ready, nn))

	themeTag := m.mark("theme", s(th.Subtle).Render("theme ")+s(th.Accent).Render(m.th().Name)+s(th.Subtle).Render(" ⟳"))
	gapw := inner - lipgloss.Width(line0) - lipgloss.Width("theme "+m.th().Name+" ⟳")
	if gapw < 1 {
		gapw = 1
	}
	line0 += s(th.Bg).Render(spaces(gapw)) + themeTag

	totals := s(th.Subtle).Bold(true).Render(" CPU  ") + gauge(th, cpuPct, 16) +
		s(th.Subtle).Render(fmt.Sprintf("  %.1f/%d cores", usedCores, nn*nodeCores)) +
		s(th.Bg).Render("    ") +
		s(th.Subtle).Bold(true).Render("MEM  ") + gauge(th, memPct, 16) +
		s(th.Subtle).Render(fmt.Sprintf("  %.0f/%d GiB", usedGiB, nn*nodeGiB)) +
		s(th.Bg).Render("    ") +
		s(th.Subtle).Render("per-node view → Resources ▸ Nodes")

	lines := []string{
		line0,
		"",
		totals,
		s(th.Border).Render(spaces(1) + strings.Repeat("╌", maxi(1, inner))),
	}
	return BlockOf(m.w, l.headerH, lines, th.Bg)
}

// ---- left: resource list + search box --------------------------------------

func (m *Model) viewList(w, h int) Block {
	th := m.th()
	if w == 0 {
		return Block{W: 0, H: h, Lines: make([]string, h)}
	}
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	inner := w - 2
	focused := m.focus == focusList
	f := m.filtered()

	var lines []string
	group := ""
	for _, i := range f {
		r := mock.Resources[i]
		if r.Group != group {
			group = r.Group
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, s(th.Subtle).Bold(true).Render(" "+strings.ToUpper(group)))
		}
		count := fmt.Sprintf("%d", mock.VisibleCount(r, mock.Cluster.Namespace))
		label := trunc(r.Name, inner-4-len(count))
		gap := inner - 3 - lipgloss.Width(label) - len(count)
		if gap < 1 {
			gap = 1
		}
		var row string
		if i == m.resIdx {
			sel := lipgloss.NewStyle().Background(th.SelBg)
			row = sel.Foreground(th.Accent).Render(" ▸ ") +
				sel.Foreground(th.SelFg).Bold(true).Render(label) +
				sel.Render(spaces(gap)) +
				sel.Foreground(th.Accent2).Render(count)
		} else {
			row = s(th.Bg).Render("   ") + s(th.Fg).Render(label) +
				s(th.Bg).Render(spaces(gap)) + s(th.Subtle).Render(count)
		}
		lines = append(lines, m.mark(fmt.Sprintf("res:%d", i), padBG(row, inner, colorOf(i == m.resIdx, th.SelBg, th.Bg))))
	}
	if len(f) == 0 {
		lines = append(lines, s(th.Subtle).Render(" no match"))
	}

	// reserve two lines for the search box at the bottom
	avail := h - 2 - 2
	if len(lines) > avail {
		selLine := 0
		for idx, ln := range lines {
			if strings.Contains(ln, "▸") {
				selLine = idx
				break
			}
		}
		top := clamp(selLine-avail/2, 0, len(lines)-avail)
		lines = lines[top : top+avail]
	}
	for len(lines) < avail {
		lines = append(lines, "")
	}

	// search box
	lines = append(lines, s(th.Border).Render(strings.Repeat("╌", inner)))
	qCol, curCol := th.Subtle, th.Border
	if focused {
		qCol, curCol = th.Fg, th.Accent
	}
	q := m.search
	cnt := ""
	if focused || q != "" {
		cnt = fmt.Sprintf("%d/%d", len(f), len(mock.Resources))
	}
	srow := s(th.Accent).Render(" / ") + s(qCol).Render(trunc(q, inner-6-len(cnt)))
	if focused {
		srow += s(curCol).Render("█")
	} else if q == "" {
		srow += s(th.Subtle).Render(trunc("type to search", inner-5))
	}
	if cnt != "" {
		gap := inner - lipgloss.Width(srow) - len(cnt) - 1
		if gap < 1 {
			gap = 1
		}
		srow += s(th.Bg).Render(spaces(gap)) + s(th.Subtle).Render(cnt)
	}
	lines = append(lines, m.mark("searchbox", padBG(srow, inner, th.Bg)))

	return Panel(th, PanelOpts{Title: "Resources", Focused: focused, W: w, H: h}, lines)
}

func colorOf(cond bool, a, b lipgloss.Color) lipgloss.Color {
	if cond {
		return a
	}
	return b
}

// ---- center: table / text -------------------------------------------------

// fitCols sizes the visible columns for avail cells. Columns are dropped from
// the right (never the first one) before the name column gets crushed.
func fitCols(cols []string, rows [][]string, avail, gap int) ([]int, []int) {
	keep := make([]int, len(cols))
	for i := range keep {
		keep[i] = i
	}
	for {
		w, ok := tryFit(cols, rows, keep, avail, gap)
		if ok || len(keep) <= 2 {
			return w, keep
		}
		keep = keep[:len(keep)-1]
	}
}

func tryFit(cols []string, rows [][]string, keep []int, avail, gap int) ([]int, bool) {
	n := len(keep)
	nat := make([]int, n)
	min := make([]int, n)
	for k, ci := range keep {
		nat[k] = len(cols[ci])
		for _, r := range rows {
			if ci < len(r) && len(r[ci]) > nat[k] {
				nat[k] = len(r[ci])
			}
		}
		m := 7
		if ci == 0 {
			m = 18
		}
		if cols[ci] == "NAMESPACE" {
			m = 9 // short values (kube-system, cert-manager…); leave room for NAME
		}
		if m > nat[k] {
			m = nat[k]
		}
		min[k] = m
	}
	total := gap * (n - 1)
	for _, x := range nat {
		total += x
	}
	for total > avail {
		bi, bv := -1, 0
		for i := range nat {
			if nat[i] > min[i] && nat[i] > bv {
				bi, bv = i, nat[i]
			}
		}
		if bi < 0 {
			return nat, false
		}
		nat[bi]--
		total--
	}
	return nat, true
}

var statusColors = map[string]string{
	"Running": "ok", "Ready": "ok", "Active": "ok", "Bound": "ok", "True": "ok", "Normal": "ok",
	"Completed": "subtle", "False": "subtle", "<none>": "subtle", "-": "subtle",
	"Pending": "warn", "Terminating": "warn", "ContainerCreating": "warn", "Warning": "warn", "NotReady": "err",
	"CrashLoopBackOff": "err", "Error": "err", "ImagePullBackOff": "err", "Failed": "err", "Evicted": "err",
}

func cellColor(th theme.Theme, v string, def lipgloss.Color) lipgloss.Color {
	if strings.Contains(v, "SchedulingDisabled") {
		return th.Warn
	}
	switch statusColors[v] {
	case "ok":
		return th.Ok
	case "warn":
		return th.Warn
	case "err":
		return th.Err
	case "subtle":
		return th.Subtle
	}
	if strings.Contains(v, "/") && len(v) <= 7 {
		parts := strings.SplitN(v, "/", 2)
		if len(parts) == 2 && parts[0] != parts[1] {
			return th.Warn
		}
	}
	return def
}

func (m *Model) viewMain(w, h int) Block {
	th := m.th()
	focused := m.focus == focusMain || m.focus == focusMainSearch
	inner := w - 2

	zoomPlain := "[ zoom ]"
	zoomLbl := "zoom"
	if m.zoomed {
		zoomPlain, zoomLbl = "[ restore ]", "restore"
	}
	tagStyle := lipgloss.NewStyle().Background(th.Bg).Foreground(th.Accent2)
	brk := lipgloss.NewStyle().Background(th.Bg).Foreground(th.Border)
	zoomTag := m.mark("zoom", brk.Render("[ ")+tagStyle.Render(zoomLbl)+brk.Render(" ]"))

	if m.mode == modeText {
		closeTag := m.mark("close", brk.Render("[ ")+lipgloss.NewStyle().Background(th.Bg).Foreground(th.Err).Render("close")+brk.Render(" ]"))
		return Panel(th, PanelOpts{
			Title: m.textTitle, Tag: closeTag + brk.Render(" ") + zoomTag,
			TagPlain: "[ close ] " + zoomPlain, Focused: focused, W: w, H: h,
		}, m.textBody(inner, h-2))
	}

	nsLabel := mock.Cluster.Namespace
	if nsLabel == mock.AllNamespaces {
		nsLabel = "all namespaces"
	}

	bodyH := h - 2 - 2 // reserve 2 rows at the bottom for the row-search box
	if bodyH < 1 {
		bodyH = 1
	}
	body := m.tableBody(inner, bodyH)
	for len(body) < bodyH {
		body = append(body, "")
	}
	body = append(body, lipgloss.NewStyle().Background(th.Bg).Foreground(th.Border).Render(strings.Repeat("╌", inner)))
	body = append(body, m.tableSearchBox(inner))

	return Panel(th, PanelOpts{
		Title: m.res().Name + " · " + nsLabel,
		Tag:   zoomTag, TagPlain: zoomPlain, Focused: focused, W: w, H: h,
	}, body)
}

// tableSearchBox renders the main panel's own row-search box, mirroring the
// Resources pane's search box but scoped to the table currently on screen.
func (m *Model) tableSearchBox(inner int) string {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	focused := m.focus == focusMainSearch
	qCol, curCol := th.Subtle, th.Border
	if focused {
		qCol, curCol = th.Fg, th.Accent
	}
	total := m.tableTotal()
	cnt := ""
	if focused || m.rowSearch != "" {
		_, rows := m.tableData()
		cnt = fmt.Sprintf("%d/%d", len(rows), total)
	}
	row := s(th.Accent).Render(" / ") + s(qCol).Render(trunc(m.rowSearch, inner-6-len(cnt)))
	if focused {
		row += s(curCol).Render("█")
	} else if m.rowSearch == "" {
		row += s(th.Subtle).Render(trunc("search rows…", inner-5))
	}
	if cnt != "" {
		gap := inner - lipgloss.Width(row) - len(cnt) - 1
		if gap < 1 {
			gap = 1
		}
		row += s(th.Bg).Render(spaces(gap)) + s(th.Subtle).Render(cnt)
	}
	return m.mark("tablesearch", padBG(row, inner, th.Bg))
}

func (m *Model) textBody(inner, rows int) []string {
	th := m.th()
	out := make([]string, 0, rows)
	end := clamp(m.textTop+rows, 0, len(m.textLines))
	for i := m.textTop; i < end; i++ {
		ln := m.textLines[i]
		col := th.Fg
		switch {
		case strings.Contains(ln, "ERROR"), strings.Contains(ln, "Warning"):
			col = th.Err
		case strings.Contains(ln, "WARN"):
			col = th.Warn
		}
		if strings.HasPrefix(ln, "  ") && strings.Contains(ln, ":") {
			col = th.Subtle
		}
		out = append(out, lipgloss.NewStyle().Background(th.Bg).Foreground(col).Render(" "+trunc(ln, inner-1)))
	}
	if len(m.textLines) > rows {
		pctScrolled := (m.textTop + rows) * 100 / len(m.textLines)
		bar := lipgloss.NewStyle().Background(th.Bg).Foreground(th.Subtle).
			Render(fmt.Sprintf(" %d/%d  %d%%  ↑↓ scroll · esc close", end, len(m.textLines), pctScrolled))
		if len(out) == rows {
			out[rows-1] = bar
		}
	}
	return out
}

func (m *Model) tableBody(inner, rows int) []string {
	th := m.th()
	nameCol := "NAME"
	if m.res().Key == "events" {
		nameCol = "OBJECT"
	}
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	const gap = 2
	cols, allRows := m.tableData()
	widths, keep := fitCols(cols, allRows, inner-3, gap)

	var hdr strings.Builder
	hdr.WriteString(s(th.Bg).Render("   "))
	for k, ci := range keep {
		hdr.WriteString(s(th.Subtle).Bold(true).Render(fmt.Sprintf("%-*s", widths[k], trunc(cols[ci], widths[k]))))
		if k < len(keep)-1 {
			hdr.WriteString(s(th.Bg).Render(spaces(gap)))
		}
	}
	out := []string{hdr.String(), s(th.Border).Render(strings.Repeat("╌", inner))}

	visible := rows - 2
	if visible < 1 {
		visible = 1
	}
	m.rowScroll = clamp(m.rowScroll, 0, maxi(0, len(allRows)-visible))
	end := clamp(m.rowScroll+visible, 0, len(allRows))

	for i := m.rowScroll; i < end; i++ {
		row := allRows[i]
		sel := i == m.rowIdx
		bg := th.Bg
		base := th.Fg
		if sel {
			bg, base = th.SelBg, th.SelFg
		}
		st := func(c lipgloss.Color) lipgloss.Style {
			return lipgloss.NewStyle().Background(bg).Foreground(c)
		}
		var b strings.Builder
		if sel {
			b.WriteString(st(th.Accent).Render(" ▌ "))
		} else {
			b.WriteString(st(bg).Render("   "))
		}
		for k, ci := range keep {
			v := ""
			if ci < len(row) {
				v = row[ci]
			}
			col := cellColor(th, v, base)
			if ci < len(cols) && cols[ci] == "NAMESPACE" {
				col = th.Accent2
			}
			cell := fmt.Sprintf("%-*s", widths[k], trunc(v, widths[k]))
			if ci < len(cols) && cols[ci] == nameCol && sel {
				b.WriteString(st(col).Bold(true).Render(cell))
			} else {
				b.WriteString(st(col).Render(cell))
			}
			if k < len(keep)-1 {
				b.WriteString(st(bg).Render(spaces(gap)))
			}
		}
		out = append(out, m.mark(fmt.Sprintf("row:%d", i), padBG(b.String(), inner, bg)))
	}
	if len(allRows) == 0 {
		msg := "no resources found"
		if m.rowSearch != "" {
			msg = fmt.Sprintf("no rows match %q", m.rowSearch)
		}
		out = append(out, s(th.Subtle).Render("   "+msg))
	}
	return out
}

// ---- right: quick actions -------------------------------------------------

func (m *Model) viewActions(w, h int) Block {
	th := m.th()
	if w == 0 {
		return Block{W: 0, H: h, Lines: make([]string, h)}
	}
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	inner := w - 2
	r := m.res()

	lines := []string{
		s(th.Subtle).Render(" " + trunc(r.Short+"/"+m.curName(), inner-1)),
		s(th.Border).Render(strings.Repeat("╌", inner)),
	}
	for _, a := range mock.Actions {
		enabled := r.Can(a.ID)
		if a.Risky {
			lines = append(lines, s(th.Border).Render(strings.Repeat("╌", inner)))
		}
		keyCol, labCol := th.Accent, th.Fg
		if a.Risky {
			keyCol, labCol = th.Err, th.Err
		}
		if !enabled {
			keyCol, labCol = th.Border, th.Subtle
		}
		label := a.Label
		if a.ID == mock.ACordon && enabled && mock.NodeCordoned(m.curName()) {
			label = "Uncordon"
		}
		row := s(th.Bg).Render(" ") + s(th.Border).Render("[") + s(keyCol).Render(a.Key) + s(th.Border).Render("] ") +
			s(labCol).Render(trunc(label, inner-6))
		lines = append(lines, m.mark("act:"+a.ID, padBG(row, inner, th.Bg)))
	}
	return Panel(th, PanelOpts{Title: "Actions", Focused: m.focus == focusActions, W: w, H: h}, lines)
}

// ---- bottom: prompt + status ---------------------------------------------

func (m *Model) viewPrompt(l layout) Block {
	th := m.th()
	focused := m.focus == focusPrompt
	inner := m.w - 2

	var caret, modeTag, modePlain, title, placeholder string
	sBg := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	if m.pmode == promptAI {
		caret = sBg(th.Accent2).Bold(true).Render(" ✦ ")
		modePlain = "[ AI · " + m.cfg.model + " ]"
		modeTag = m.mark("aimode", sBg(th.Border).Render("[ ")+sBg(th.Accent2).Render("AI · "+m.cfg.model)+sBg(th.Border).Render(" ]"))
		title = "Prompt"
		if focused {
			title = "Prompt · plain text → AI · /commands still work · esc close"
		}
		placeholder = "ask about your cluster…   ·   /config to change provider/model"
	} else {
		caret = sBg(th.Accent).Bold(true).Render(" ❯ ")
		modePlain = "[ CMD ]"
		modeTag = m.mark("aimode", sBg(th.Border).Render("[ ")+sBg(th.Accent).Render("CMD")+sBg(th.Border).Render(" ]"))
		title = "Command"
		if focused {
			title = "Command · enter run · esc close"
		}
		placeholder = "kubectl get pods -A   ·   /context prod   ·   /help   ·   ctrl+a for AI"
	}

	m.input.Placeholder = placeholder
	m.input.Width = inner - 5
	m.input.TextStyle = lipgloss.NewStyle().Background(th.Bg).Foreground(th.Fg)
	m.input.PlaceholderStyle = lipgloss.NewStyle().Background(th.Bg).Foreground(th.Subtle)
	m.input.Cursor.Style = lipgloss.NewStyle().Background(th.Accent).Foreground(th.Bg)

	body := m.mark("prompt", padBG(caret+m.input.View(), inner, th.Bg))
	return Panel(th, PanelOpts{
		Title: title, Tag: modeTag, TagPlain: modePlain,
		Focused: focused, W: m.w, H: l.promptH,
	}, []string{body})
}

func (m *Model) viewStatus() Block {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	left := s(th.Accent2).Render(" ● ") + s(th.Fg).Render(trunc(m.toast, m.w/2))
	hints := "tab panes · ↑↓ move · z zoom · T theme · : cmd · ctrl+a ai · D delete · q quit"
	right := s(th.Subtle).Render(trunc(hints, m.w/2-2)) + s(th.Bg).Render(" ")
	gapw := m.w - lipgloss.Width(" ● ") - lipgloss.Width(trunc(m.toast, m.w/2)) - lipgloss.Width(trunc(hints, m.w/2-2)) - 1
	if gapw < 1 {
		gapw = 1
	}
	return BlockOf(m.w, 1, []string{left + s(th.Bg).Render(spaces(gapw)) + right}, th.Bg)
}

// ---- overlays --------------------------------------------------------------

func (m *Model) overlaySuggestions(root Block, l layout, sug []mock.SlashCommand) Block {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	w := 62
	if w > m.w-6 {
		w = m.w - 6
	}
	inner := w - 2

	var body []string
	for i, c := range sug {
		row := s(th.Bg).Render(" ") + s(th.Accent).Bold(true).Render(c.Name)
		if c.Args != "" {
			row += s(th.Bg).Render(" ") + s(th.Accent2).Render(c.Args)
		}
		desc := trunc(c.Desc, inner-lipgloss.Width(row)-3)
		gap := inner - lipgloss.Width(row) - lipgloss.Width(desc) - 1
		if gap < 1 {
			gap = 1
		}
		row += s(th.Bg).Render(spaces(gap)) + s(th.Subtle).Render(desc) + s(th.Bg).Render(" ")
		body = append(body, zone.Mark(fmt.Sprintf("sug:%d", i), padBG(row, inner, th.Bg)))
	}

	h := len(body) + 2
	box := Panel(th, PanelOpts{Title: "slash commands", W: w, H: h, Focused: true}, body)
	y := l.promptY - h + 1
	if y < 0 {
		y = 0
	}
	return root.Overlay(box, 1, y)
}

func (m *Model) overlayConfig(root Block) Block {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	w := 66
	if w > m.w-6 {
		w = m.w - 6
	}
	inner := w - 2

	label := func(i int, name string) string {
		if i == m.cfgRow {
			return lipgloss.NewStyle().Background(th.SelBg).Foreground(th.Accent).Bold(true).Render(" ▸ " + fmt.Sprintf("%-10s", name))
		}
		return s(th.Subtle).Render("   " + fmt.Sprintf("%-10s", name))
	}
	rowBG := func(i int) lipgloss.Color { return colorOf(i == m.cfgRow, th.SelBg, th.Bg) }

	// provider row: two clickable options
	radio := func(on bool, txt string, id string) string {
		mark := "○ "
		col := th.Subtle
		if on {
			mark = "● "
			col = th.Accent2
		}
		st := lipgloss.NewStyle().Background(rowBG(0)).Foreground(col)
		return zone.Mark(id, st.Render(mark+txt))
	}
	provRow := label(0, "Provider") +
		lipgloss.NewStyle().Background(rowBG(0)).Render("  ") +
		radio(m.cfg.provider == 0, mock.AIProviders[0].Label, "cfg:openai") +
		lipgloss.NewStyle().Background(rowBG(0)).Render("    ") +
		radio(m.cfg.provider == 1, mock.AIProviders[1].Label, "cfg:anthropic")

	valCol := func(i int) lipgloss.Color { return colorOf(i == m.cfgRow, th.SelFg, th.Fg) }
	valRow := func(i int, name, val string) string {
		st := lipgloss.NewStyle().Background(rowBG(i)).Foreground(valCol(i))
		body := label(i, name) + lipgloss.NewStyle().Background(rowBG(i)).Render("  ")
		if m.cfgEditing && i == m.cfgRow {
			m.input.Width = inner - 18
			body += m.input.View()
		} else {
			body += st.Render(trunc(val, inner-18))
		}
		return zone.Mark(fmt.Sprintf("cfg:row:%d", i), padBG(body, inner, rowBG(i)))
	}

	keyShown := m.cfg.key
	if len(keyShown) > 18 {
		keyShown = keyShown[:10] + "••••" + keyShown[len(keyShown)-4:]
	}

	body := []string{
		"",
		zone.Mark("cfg:row:0", padBG(provRow, inner, rowBG(0))),
		"",
		valRow(1, "Base URL", m.cfg.url),
		"",
		valRow(2, "Model", m.cfg.model),
		"",
		valRow(3, "API Key", keyShown),
		"",
		s(th.Border).Render(strings.Repeat("╌", inner)),
	}
	foot := " ↑↓ select · enter edit · ←→ provider · esc close"
	if m.cfgEditing {
		foot = " editing — enter save · esc cancel"
	}
	closeTag := zone.Mark("cfg:close", s(th.Border).Render("[ ")+s(th.Err).Render("close")+s(th.Border).Render(" ]"))
	gap := inner - lipgloss.Width(foot) - lipgloss.Width("[ close ]") - 1
	if gap < 1 {
		gap = 1
	}
	body = append(body, s(th.Subtle).Render(foot)+s(th.Bg).Render(spaces(gap))+closeTag+s(th.Bg).Render(" "))

	h := len(body) + 2
	box := Panel(th, PanelOpts{Title: "✦ AI Prompt Settings", Focused: true, W: w, H: h}, body)
	return root.Overlay(box, (m.w-w)/2, (m.h-h)/2)
}

func (m *Model) overlayConfirm(root Block) Block {
	th := m.th()
	c := m.confirm
	accent := th.Accent
	if c.danger {
		accent = th.Err
	}
	s := func(col lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(col)
	}

	w := 58
	if w > m.w-8 {
		w = m.w - 8
	}
	inner := w - 2

	body := []string{""}
	for _, ln := range c.message {
		body = append(body, s(th.Fg).Render("  "+trunc(ln, inner-3)))
	}
	body = append(body, "")

	okPlain, noPlain := "  Enter · Confirm  ", "  Esc · Cancel  "
	ok := zone.Mark("cf:ok", lipgloss.NewStyle().Background(accent).Foreground(th.Bg).Bold(true).Render(okPlain))
	no := zone.Mark("cf:no", lipgloss.NewStyle().Background(th.Border).Foreground(th.Fg).Render(noPlain))
	btnGap := 2
	pre := (inner - len(okPlain) - len(noPlain) - btnGap) / 2
	if pre < 1 {
		pre = 1
	}
	body = append(body, s(th.Bg).Render(spaces(pre))+ok+s(th.Bg).Render(spaces(btnGap))+no)
	body = append(body, "")

	h := len(body) + 2
	border := th.Accent
	if c.danger {
		border = th.Err
	}
	box := Panel(th, PanelOpts{
		Title: "⚠  " + c.title, Focused: true, W: w, H: h, BorderCol: border,
	}, body)

	return root.Overlay(box, (m.w-w)/2, (m.h-h)/2)
}
