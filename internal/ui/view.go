package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"k10s/internal/domain"
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

	if sug := m.suggestions(); len(sug) > 0 && !m.modalOpen() {
		root = m.overlaySuggestions(root, l, sug)
	}
	if m.themeOpen {
		root = m.overlayThemePicker(root)
	}
	if m.setOpen {
		root = m.overlaySettings(root)
	}
	if m.palOpen {
		root = m.overlayPalette(root)
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

	nodes := m.src.Nodes()
	ci := m.src.ClusterInfo()

	ready := 0
	cpuSum, memSum := 0, 0
	for _, n := range nodes {
		if n.Status == "Ready" {
			ready++
		}
		cpuSum += n.CPU
		memSum += n.Mem
	}
	nn := maxi(1, len(nodes))
	cpuPct, memPct := cpuSum/nn, memSum/nn
	usedCores := float64(cpuSum) * nodeCores / 100
	usedGiB := float64(memSum) * nodeGiB / 100

	brand := s(th.Accent).Bold(true).Render(" ⎈ k10s")
	sep := s(th.Border).Render("  │  ")
	nodeCol := th.Ok
	if ready < nn {
		nodeCol = th.Warn
	}
	line0 := brand + sep + s(th.Accent2).Render(ci.Context) +
		sep + s(th.Subtle).Render("ver ") + s(th.Fg).Render(ci.Version) +
		sep + s(th.Subtle).Render("nodes ") + s(nodeCol).Render(fmt.Sprintf("%d/%d ready", ready, len(nodes)))

	// Right-hand buttons: namespace, then theme. Both are clickable and
	// both say what they currently are, so the header doubles as status.
	nsPlain := "ns " + m.namespace + " ▾"
	nsBtn := m.mark("nsbtn", s(th.Subtle).Render("ns ")+s(th.Accent2).Render(m.namespace)+s(th.Subtle).Render(" ▾"))

	themePlain := "theme " + m.th().Name + " ⟳"
	themeTag := m.mark("theme", s(th.Subtle).Render("theme ")+s(th.Accent).Render(m.th().Name)+s(th.Subtle).Render(" ⟳"))

	right := nsBtn + s(th.Border).Render("  │  ") + themeTag
	rightPlain := nsPlain + "  │  " + themePlain

	gapw := inner - lipgloss.Width(line0) - lipgloss.Width(rightPlain)
	if gapw < 1 {
		gapw = 1
	}
	line0 += s(th.Bg).Render(spaces(gapw)) + right

	totals := s(th.Subtle).Bold(true).Render(" CPU  ") + gauge(th, cpuPct, 16) +
		s(th.Subtle).Render(fmt.Sprintf("  %.1f/%d cores", usedCores, nn*nodeCores)) +
		s(th.Bg).Render("    ") +
		s(th.Subtle).Bold(true).Render("MEM  ") + gauge(th, memPct, 16) +
		s(th.Subtle).Render(fmt.Sprintf("  %.0f/%d GiB", usedGiB, nn*nodeGiB))
		// s(th.Bg).Render("    ") +
		// s(th.Subtle).Render("per-node view → Resources ▸ Nodes")

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
	ks := m.kinds()

	var lines []string
	group := ""
	for _, i := range f {
		r := ks[i]
		if r.Group != group {
			group = r.Group
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, s(th.Subtle).Bold(true).Render(" "+strings.ToUpper(group)))
		}
		// A lazily-watching backend only knows counts for kinds already
		// opened; show nothing rather than a misleading 0.
		count := ""
		if n := m.src.RowCount(r.Key, m.namespace); n != domain.CountUnknown {
			count = strconv.Itoa(n)
		}
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

	// No search box here: the list is type-to-filter, so a permanent box was
	// two wasted rows. The active filter shows in the panel title instead.
	avail := h - 2
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

	title := "Resources"
	if m.search != "" {
		title += " · " + m.search
	}
	tag, tagPlain := "", ""
	if focused || m.search != "" {
		plain := fmt.Sprintf("%d/%d", len(f), len(ks))
		tag = s(th.Subtle).Render(plain)
		tagPlain = plain
	}

	return Panel(th, PanelOpts{Title: title, Tag: tag, TagPlain: tagPlain, Focused: focused, W: w, H: h}, lines)
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

	// An action in flight takes over the panel: pressing a key must visibly
	// do something, even for actions whose only result is a toast.
	if m.busy {
		body := make([]string, 0, h-2)
		body = append(body, "")
		body = append(body, m.busyLines(inner)...)
		return Panel(th, PanelOpts{
			Title: m.busyLabel, Tag: zoomTag, TagPlain: zoomPlain,
			Focused: focused, W: w, H: h,
		}, body)
	}

	if m.mode == modeShell {
		detach := m.mark("close", brk.Render("[ ")+
			lipgloss.NewStyle().Background(th.Bg).Foreground(th.Err).Render("detach")+brk.Render(" ]"))
		return Panel(th, PanelOpts{
			Title: "shell · " + m.shellName,
			Tag:   detach + brk.Render(" ") + zoomTag, TagPlain: "[ detach ] " + zoomPlain,
			Focused: focused, W: w, H: h,
		}, m.shellBody(inner, h-2))
	}

	if m.mode == modeContexts {
		return Panel(th, PanelOpts{
			Title: "Kube context · enter reconnects",
			Tag:   zoomTag, TagPlain: zoomPlain, Focused: focused, W: w, H: h,
		}, m.contextBody(inner, h-2))
	}

	if m.mode == modeText || m.mode == modeLogs {
		closeTag := m.mark("close", brk.Render("[ ")+lipgloss.NewStyle().Background(th.Bg).Foreground(th.Err).Render("close")+brk.Render(" ]"))
		body := m.textBody(inner, h-2)
		if m.mode == modeLogs {
			body = m.logBody(inner, h-2)
		}
		return Panel(th, PanelOpts{
			Title: m.textTitle, Tag: closeTag + brk.Render(" ") + zoomTag,
			TagPlain: "[ close ] " + zoomPlain, Focused: focused, W: w, H: h,
		}, body)
	}

	nsLabel := m.namespace
	if nsLabel == domain.AllNamespaces {
		nsLabel = "all namespaces"
	}

	// The search box only takes space while it's actually in use. Reserving
	// two rows permanently cost two rows of data on every screen for a box
	// that is empty most of the time.
	searching := m.focus == focusMainSearch || m.rowSearch != ""

	bodyH := h - 2
	if searching {
		bodyH -= 2
	}
	if bodyH < 1 {
		bodyH = 1
	}
	body := m.tableBody(inner, bodyH)
	for len(body) < bodyH {
		body = append(body, "")
	}
	if searching {
		body = append(body, lipgloss.NewStyle().Background(th.Bg).Foreground(th.Border).Render(strings.Repeat("╌", inner)))
		body = append(body, m.tableSearchBox(inner))
	}

	title := m.res().Name + " · " + nsLabel
	if m.rowSearch != "" {
		title += " · find: " + m.rowSearch
	}

	// Advertise the find key next to zoom, since there is no visible search
	// box to hint at it any more.
	tag, tagPlain := zoomTag, zoomPlain
	if !searching {
		findHint := m.mark("tablesearch", brk.Render("[ ")+tagStyle.Render("f")+
			lipgloss.NewStyle().Background(th.Bg).Foreground(th.Subtle).Render(" to search")+brk.Render(" ]"))
		tag = findHint + brk.Render(" ") + zoomTag
		tagPlain = "[ f to search ] " + zoomPlain
	}

	return Panel(th, PanelOpts{
		Title: title,
		Tag:   tag, TagPlain: tagPlain, Focused: focused, W: w, H: h,
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
		row += s(th.Subtle).Render(trunc("press f to search rows…", inner-5))
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

	// Left gutter: selection marker + a dim 1-based row number, so rows can
	// be referred to ("the 12th one") and position is obvious while
	// scrolling. Sized to the widest number actually present.
	numW := len(strconv.Itoa(maxi(1, len(allRows))))
	numW = clamp(numW, 2, 5)
	gutter := numW + 3 // "▌" + space + digits + space

	widths, keep := fitCols(cols, allRows, inner-gutter, gap)

	var hdr strings.Builder
	hdr.WriteString(s(th.Bg).Render(spaces(gutter)))
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
		// Gutter: marker, then the dim row number.
		if sel {
			b.WriteString(st(th.Accent).Render("▌"))
		} else {
			b.WriteString(st(bg).Render(" "))
		}
		numCol := th.Border
		if sel {
			numCol = th.Accent2
		}
		b.WriteString(st(bg).Render(" "))
		b.WriteString(st(numCol).Render(fmt.Sprintf("%*d", numW, i+rowNumBase(m.res().Key))))
		b.WriteString(st(bg).Render(" "))

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
		switch {
		case m.kindLoading():
			// Distinguish "still fetching" from "genuinely empty" — showing
			// "no resources found" during the first list is a lie.
			out = append(out, "")
			out = append(out, m.loadingLines(inner)...)
		case m.rowSearch != "":
			out = append(out, s(th.Subtle).Render(fmt.Sprintf("   no rows match %q", m.rowSearch)))
		default:
			out = append(out, s(th.Subtle).Render("   no resources found"))
		}
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
	// Only actions that actually apply to the selected kind are listed —
	// a pane of greyed-out rows is noise, and the list is short enough that
	// its contents change legibly as you move between kinds.
	shown := make([]Action, 0, len(Actions))
	for _, a := range Actions {
		if r.Can(a.ID) {
			shown = append(shown, a)
		}
	}

	sepDone := false
	for _, a := range shown {
		if a.Risky && !sepDone {
			lines = append(lines, s(th.Border).Render(strings.Repeat("╌", inner)))
			sepDone = true
		}

		// Three visual states so the pane responds to the pointer: normal,
		// hovered (pointer is over it), and flashed (just clicked — lit
		// briefly so the click is acknowledged even when the action only
		// produces a toast).
		flashed := m.flashAct == a.ID
		hovered := m.hoverAct == a.ID && !flashed

		bg := th.Bg
		keyCol, labCol := th.Accent, th.Fg
		if a.Risky {
			keyCol, labCol = th.Err, th.Err
		}
		switch {
		case flashed:
			bg = th.Accent
			keyCol, labCol = th.Bg, th.Bg
			if a.Risky {
				bg = th.Err
			}
		case hovered:
			bg = th.SelBg
			labCol = th.SelFg
			if a.Risky {
				labCol = th.Err
			}
		}
		st := func(c lipgloss.Color) lipgloss.Style {
			return lipgloss.NewStyle().Background(bg).Foreground(c)
		}

		label := a.Label
		if a.ID == domain.ACordon && strings.Contains(rowStatus(m), "SchedulingDisabled") {
			label = "Uncordon"
		}
		marker := " "
		if hovered || flashed {
			marker = "▌"
		}
		row := st(keyCol).Render(marker) + st(th.Border).Render("[") + st(keyCol).Render(a.Key) + st(th.Border).Render("] ") +
			st(labCol).Bold(flashed).Render(trunc(label, inner-6))
		lines = append(lines, m.mark("act:"+a.ID, padBG(row, inner, bg)))
	}
	return Panel(th, PanelOpts{Title: "Actions", Focused: false, W: w, H: h}, lines)
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
		placeholder = "ask about your cluster…   ·   :config to change provider/model"
	} else {
		caret = sBg(th.Accent).Bold(true).Render(" ❯ ")
		modePlain = "[ CMD ]"
		modeTag = m.mark("aimode", sBg(th.Border).Render("[ ")+sBg(th.Accent).Render("CMD")+sBg(th.Border).Render(" ]"))
		title = "Command"
		if focused {
			title = "Command · enter run · esc close"
		}
		placeholder = "kubectl get pods -A · /ns · :help · ctrl+a for AI"
	}

	m.input.Placeholder = placeholder
	m.input.Width = inner - 5
	m.input.TextStyle = lipgloss.NewStyle().Background(th.Bg).Foreground(th.Fg)
	m.input.PlaceholderStyle = lipgloss.NewStyle().Background(th.Bg).Foreground(th.Subtle)
	m.input.Cursor.Style = lipgloss.NewStyle().Background(th.Accent).Foreground(th.Bg)

	zoomLbl, zoomPlainTag := "grow", "[ grow ]"
	if m.promptZoom {
		zoomLbl, zoomPlainTag = "shrink", "[ shrink ]"
	}
	zoomTag := m.mark("promptzoom",
		sBg(th.Border).Render("[ ")+sBg(th.Accent2).Render(zoomLbl)+sBg(th.Border).Render(" ]"))

	var body []string
	if m.promptZoom {
		// A tall box is only worth the space if it actually shows the whole
		// command, so the value wraps across the rows rather than scrolling
		// sideways in a one-line field.
		body = m.zoomedPromptBody(caret, inner, l.promptH-2)
	} else {
		body = []string{m.mark("prompt", padBG(caret+m.input.View(), inner, th.Bg))}
	}

	return Panel(th, PanelOpts{
		Title: title, Tag: zoomTag + sBg(th.Border).Render(" ") + modeTag,
		TagPlain: zoomPlainTag + " " + modePlain,
		Focused:  focused, W: m.w, H: l.promptH,
	}, body)
}

func (m *Model) viewStatus() Block {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}
	dot, dotCol := " ● ", th.Accent2
	if m.mouseOff {
		// Make copy-mode unmistakable: clicking is dead while it's on.
		dot, dotCol = " ✂ ", th.Warn
	}
	left := s(dotCol).Render(dot) + s(th.Fg).Render(trunc(m.toast, m.w/2))
	hints := "tab panes · enter open · ctrl+p search · f find · z zoom · ctrl+s copy · q quit"
	right := s(th.Subtle).Render(trunc(hints, m.w/2-2)) + s(th.Bg).Render(" ")
	gapw := m.w - lipgloss.Width(dot) - lipgloss.Width(trunc(m.toast, m.w/2)) - lipgloss.Width(trunc(hints, m.w/2-2)) - 1
	if gapw < 1 {
		gapw = 1
	}
	return BlockOf(m.w, 1, []string{left + s(th.Bg).Render(spaces(gapw)) + right}, th.Bg)
}

// ---- overlays --------------------------------------------------------------

func (m *Model) overlaySuggestions(root Block, l layout, sug []SlashCommand) Block {
	th := m.th()
	w := 62
	if w > m.w-6 {
		w = m.w - 6
	}
	inner := w - 2

	var body []string
	cur := clamp(m.sugIdx, 0, len(sug)-1)
	for i, c := range sug {
		selected := i == cur
		bg := th.Bg
		if selected {
			bg = th.SelBg
		}
		st := func(col lipgloss.Color) lipgloss.Style {
			return lipgloss.NewStyle().Background(bg).Foreground(col)
		}
		lead := "  "
		if selected {
			lead = "▸ "
		}
		row := st(th.Accent).Render(lead) + st(th.Accent).Bold(true).Render(c.Name)
		if c.Args != "" {
			row += st(bg).Render(" ") + st(th.Accent2).Render(c.Args)
		}
		desc := trunc(c.Desc, inner-lipgloss.Width(row)-3)
		gap := inner - lipgloss.Width(row) - lipgloss.Width(desc) - 1
		if gap < 1 {
			gap = 1
		}
		row += st(bg).Render(spaces(gap)) + st(th.Subtle).Render(desc) + st(bg).Render(" ")
		body = append(body, zone.Mark(fmt.Sprintf("sug:%d", i), padBG(row, inner, bg)))
	}

	h := len(body) + 2
	title := "cluster commands  /"
	if v := m.input.Value(); v != "" && v[0] == ':' {
		title = "k10s commands  :"
	}
	box := Panel(th, PanelOpts{Title: title, W: w, H: h, Focused: true}, body)
	y := l.promptY - h + 1
	if y < 0 {
		y = 0
	}
	return root.Overlay(box, 1, y)
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

// rowNumBase is the number given to the first row of a table. Normally 1,
// but the Namespaces table leads with the synthetic "all" entry, which is
// numbered 0 so the real namespaces below it are 1..N — matching the count
// the sidebar shows for the kind.
func rowNumBase(kindKey string) int {
	if kindKey == "namespaces" {
		return 0
	}
	return 1
}

// zoomedPromptBody lays the command out over the tall box: the live input on
// the first row (so the caret still blinks where you type) and the rest of
// the value wrapped underneath, which is the point of growing the box.
func (m *Model) zoomedPromptBody(caret string, inner, rows int) []string {
	th := m.th()
	s := func(c lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Background(th.Bg).Foreground(c)
	}

	textW := maxi(8, inner-4)
	segs := wrapLine(m.input.Value(), textW)

	out := make([]string, 0, rows)
	// Row 1 stays the real textinput so editing and the caret behave
	// normally; the wrap below is a read-only continuation.
	out = append(out, m.mark("prompt", padBG(caret+m.input.View(), inner, th.Bg)))
	if len(segs) > 1 {
		for _, seg := range segs[1:] {
			if len(out) >= rows-1 {
				break
			}
			out = append(out, padBG(s(th.Bg).Render("   ")+s(th.Fg).Render(seg), inner, th.Bg))
		}
	}
	for len(out) < rows-1 {
		out = append(out, "")
	}

	hint := " ctrl+z shrink · esc back · enter run"
	if m.pmode == promptAI {
		hint = " ctrl+z shrink · esc back · enter ask " + m.cfg.model
	}
	out = append(out, padBG(s(th.Subtle).Render(trunc(hint, inner)), inner, th.Bg))
	return out
}
