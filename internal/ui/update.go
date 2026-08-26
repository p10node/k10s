package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"k10s/internal/config"
	"k10s/internal/update"
	"k10s/internal/version"
)

// Self-update, in three steps that are deliberately separate:
//
//	1. a check — one HTTP GET, run on startup at most once a day, whose only
//	   effect is a toast. Nothing is downloaded and nothing is asked of the
//	   user, because most launches have no update to report.
//	2. /update — confirms first. Replacing the binary someone is running is
//	   not something to do off a mistyped key, so it goes through the same
//	   modal as delete and drain.
//	3. the install, off the render path like every other network call here,
//	   ending in an offer to relaunch.
//
// The version comparison and the download live in internal/update; this file
// is only the plumbing between them and the UI.

// updateClient builds the client from the persisted settings.
func (m *Model) updateClient() *update.Client {
	return &update.Client{Repo: m.updRepo}
}

// checkUpdateCmd asks GitHub for the newest release. auto marks the startup
// check, which stays silent unless there is something to say.
func (m *Model) checkUpdateCmd(auto bool) tea.Cmd {
	c := m.updateClient()
	cur := version.Current()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rel, newer, err := c.Check(ctx, cur)
		return updateCheckMsg{rel: rel, newer: newer, auto: auto, err: err}
	}
}

// autoCheckCmd is the startup check: nil unless it is switched on and the
// last successful check has aged out.
func (m *Model) autoCheckCmd() tea.Cmd {
	if m.updDisabled || !update.Due(m.updLast) {
		return nil
	}
	return m.checkUpdateCmd(true)
}

// startUpdate runs /update. With no check behind it, it checks first and
// chains straight into the confirmation.
//
// The one argument is "skip": stop mentioning the release the last check
// found, without turning the whole check off. That is the middle ground
// between installing now and never hearing about updates again.
func (m *Model) startUpdate(arg string) tea.Cmd {
	if arg == "skip" {
		return m.skipRelease()
	}
	if m.updBusy {
		m.toast = "… an update is already running"
		return nil
	}
	if m.updRel == nil {
		m.startBusy("checking for a newer k10s")
		return m.checkUpdateCmd(false)
	}
	return m.offerUpdate(m.updRel)
}

// skipRelease silences one version. It needs a version to silence, so it
// asks for a check first when none has run.
func (m *Model) skipRelease() tea.Cmd {
	if m.updRel == nil {
		m.toast = "nothing to skip yet — /update checks first"
		return nil
	}
	m.updSkip = m.updRel.Version
	m.saveConfig()
	m.toast = "skipping k10s " + m.updSkip + " — /update still installs it on demand"
	return nil
}

// offerUpdate is what both paths end at: either "you are on the latest" or
// the confirmation modal.
func (m *Model) offerUpdate(rel *update.Release) tea.Cmd {
	cur := version.Current()
	if !update.Newer(cur, rel.Version) {
		m.toast = fmt.Sprintf("✓ k10s %s is the latest release", cur)
		return nil
	}
	if !rel.HasAsset() {
		m.toast = fmt.Sprintf("✗ %s has no build for this platform — %s", rel.Tag, rel.AssetHint())
		return nil
	}

	msg := []string{
		"Install k10s " + rel.Version + " over " + cur + " ?",
		"",
		"from   " + m.updateClient().Repository(),
		"file   " + rel.Asset.Name + "  " + humanBytes(rel.Asset.Size),
	}
	if rel.Sums.Name == "" {
		// Worth saying out loud: without a manifest there is nothing to
		// check the download against.
		msg = append(msg, "sums   none published — installed unverified")
	} else {
		msg = append(msg, "sums   verified against "+rel.Sums.Name)
	}
	if notes := releaseNotes(rel.Notes, 6); len(notes) > 0 {
		msg = append(msg, "")
		msg = append(msg, notes...)
	}

	m.confirm = &confirmState{
		title:   "Update k10s",
		message: msg,
		onOK:    func(mm *Model) tea.Cmd { return mm.applyUpdateCmd(rel) },
	}
	return nil
}

// applyUpdateCmd downloads and swaps in the new binary off the UI thread.
func (m *Model) applyUpdateCmd(rel *update.Release) tea.Cmd {
	m.updBusy = true
	m.startBusy("installing k10s " + rel.Version)
	c := m.updateClient()
	return func() tea.Msg {
		// Generous: a release archive is tens of megabytes and someone may
		// be on hotel wifi.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		res, err := update.Apply(ctx, c, rel, nil)
		return updateAppliedMsg{version: res.Version, path: res.Path, err: err}
	}
}

// handleUpdateCheck folds a finished check back into the model.
func (m *Model) handleUpdateCheck(msg updateCheckMsg) tea.Cmd {
	// Only a manual check set the busy flag, so only a manual check may
	// clear it — otherwise the startup check finishing would wipe the
	// spinner off a describe that is still in flight.
	if !msg.auto {
		m.busy = false
	}
	if msg.err != nil {
		// A failed automatic check says nothing: it is not something the
		// user asked for, and an offline laptop is not an error worth a line
		// of screen.
		if !msg.auto {
			m.toast = "✗ update check: " + msg.err.Error()
		}
		return nil
	}
	m.updRel = msg.rel
	m.updLast = time.Now()
	m.saveConfig()

	if !msg.auto {
		return m.offerUpdate(msg.rel)
	}
	if !msg.newer || msg.rel.Version == m.updSkip {
		return nil
	}
	m.toast = fmt.Sprintf("⇧ k10s %s is out (you have %s) — /update to install", msg.rel.Version, version.Current())
	return nil
}

// handleUpdateApplied reports the install and offers to relaunch into it.
func (m *Model) handleUpdateApplied(msg updateAppliedMsg) tea.Cmd {
	m.updBusy = false
	m.busy = false
	if msg.err != nil {
		m.toast = "✗ update failed: " + msg.err.Error()
		return nil
	}
	// The new binary is on disk but this process is still the old image, so
	// the update isn't real until it restarts. Offering it here is the
	// difference between an update and a homework assignment.
	m.updRel = nil
	if msg.version == m.updSkip {
		// It has been installed, so there is nothing left to silence.
		m.updSkip = ""
	}
	m.updLast = time.Time{} // the next launch re-checks against the new version
	m.saveConfig()
	m.toast = "✓ k10s " + msg.version + " installed"
	m.confirm = &confirmState{
		title: "Restart into k10s " + msg.version,
		message: []string{
			"k10s " + msg.version + " is installed at",
			msg.path,
			"",
			"This session is still running the old build.",
			"Restart now? Esc keeps working in this one.",
		},
		onOK: func(mm *Model) tea.Cmd {
			mm.relaunch = true
			return tea.Quit
		},
	}
	return nil
}

// Relaunch reports whether the user accepted the offer to restart into a
// freshly installed build. main.go execs the new binary when it is true.
func (m *Model) Relaunch() bool { return m.relaunch }

// setUpdateChecks turns the startup check on or off and persists it.
func (m *Model) setUpdateChecks(on bool) {
	m.updDisabled = !on
	m.saveConfig()
	if on {
		m.toast = "update check → on (once a day at startup)"
	} else {
		m.toast = "update check → off — /update still works on demand"
	}
}

// updateConfig is the slice of the config file this feature owns.
func (m *Model) updateConfig() config.Update {
	var last int64
	if !m.updLast.IsZero() {
		last = m.updLast.Unix()
	}
	return config.Update{
		Disabled:  m.updDisabled,
		Repo:      m.updRepo,
		LastCheck: last,
		Skip:      m.updSkip,
	}
}

// applyUpdateConfig is the other direction, on startup.
func (m *Model) applyUpdateConfig(u config.Update) {
	m.updDisabled = u.Disabled
	m.updRepo = u.Repo
	m.updSkip = u.Skip
	if u.LastCheck > 0 {
		m.updLast = time.Unix(u.LastCheck, 0)
	}
}

// updateBadge is the status-bar notice: empty unless a newer release is
// waiting and the user has not skipped it.
func (m *Model) updateBadge() string {
	if m.updRel == nil || m.updRel.Version == m.updSkip {
		return ""
	}
	if !update.Newer(version.Current(), m.updRel.Version) {
		return ""
	}
	return "⇧ " + m.updRel.Version
}

// releaseNotes trims a GitHub release body down to something that fits in a
// modal: the first max non-empty lines, markdown bullets and all.
func releaseNotes(body string, max int) []string {
	var out []string
	for _, ln := range strings.Split(body, "\n") {
		ln = strings.TrimRight(ln, " \t\r")
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if len(out) == max {
			return append(out, "…")
		}
		out = append(out, ln)
	}
	return out
}

// humanBytes renders an asset size the way a download would report it.
func humanBytes(n int64) string {
	if n <= 0 {
		return ""
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB"} {
		v /= unit
		if v < unit {
			return fmt.Sprintf("%.1f %s", v, suffix)
		}
	}
	return fmt.Sprintf("%.1f TiB", v/unit)
}

// versionReport is the /version panel: what is running, where updates come
// from, and what the last check found.
func versionReport(m *Model) string {
	var b strings.Builder
	b.WriteString(version.String() + "\n\n")
	fmt.Fprintf(&b, "  releases    %s\n", m.updateClient().Repository())

	state := "on — once a day at startup"
	if m.updDisabled {
		state = "off — /update still works on demand"
	}
	fmt.Fprintf(&b, "  auto check  %s\n", state)

	if m.updLast.IsZero() {
		b.WriteString("  last check  never\n")
	} else {
		fmt.Fprintf(&b, "  last check  %s\n", m.updLast.Format("2006-01-02 15:04"))
	}

	switch {
	case m.updRel == nil:
		b.WriteString("  latest      not checked yet — run /update\n")
	case update.Newer(version.Current(), m.updRel.Version):
		fmt.Fprintf(&b, "  latest      %s — run /update to install it\n", m.updRel.Version)
	default:
		fmt.Fprintf(&b, "  latest      %s — up to date\n", m.updRel.Version)
	}
	if m.updSkip != "" {
		fmt.Fprintf(&b, "  skipped     %s\n", m.updSkip)
	}
	fmt.Fprintf(&b, "  config      %s\n", config.Path())

	if version.IsDev() {
		b.WriteString("\nThis is an unstamped build (go run / go build), so it reports\n")
		b.WriteString("\"dev\" and every release counts as newer. Release builds carry\n")
		b.WriteString("their version through -ldflags — see `just build`.\n")
	}
	if notes := releaseNotes(relNotes(m.updRel), 20); len(notes) > 0 {
		b.WriteString("\nRelease notes — " + m.updRel.Tag + "\n\n")
		for _, ln := range notes {
			b.WriteString("  " + ln + "\n")
		}
	}
	return b.String()
}

func relNotes(rel *update.Release) string {
	if rel == nil {
		return ""
	}
	return rel.Notes
}
