package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"k10s/internal/config"
	"k10s/internal/mock"
	"k10s/internal/update"
	"k10s/internal/version"
)

// newerRelease is what a check that found something returns.
func newerRelease() *update.Release {
	return &update.Release{
		Version: "9.9.9",
		Tag:     "v9.9.9",
		Notes:   "- much faster\n\n- fewer bugs",
		Asset:   update.Asset{Name: "k10s_9.9.9_linux_amd64.tar.gz", URL: "https://dl.test/a.tar.gz", Size: 31 << 20},
		Sums:    update.Asset{Name: "checksums.txt", URL: "https://dl.test/checksums.txt"},
	}
}

func updateModel(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	return m
}

func TestAutomaticCheckOnlyReportsSomethingNewer(t *testing.T) {
	m := updateModel(t)

	m.Update(updateCheckMsg{rel: newerRelease(), newer: true, auto: true})
	if !strings.Contains(m.toast, "9.9.9") {
		t.Errorf("toast = %q, want it to name the new release", m.toast)
	}
	// No modal: the startup check must never interrupt what someone opened
	// k10s to do.
	if m.confirm != nil {
		t.Error("the automatic check opened a dialog")
	}

	m.toast = "untouched"
	m.Update(updateCheckMsg{rel: &update.Release{Version: "0.0.1"}, newer: false, auto: true})
	if m.toast != "untouched" {
		t.Errorf("toast = %q, want silence when there is nothing newer", m.toast)
	}
}

func TestAutomaticCheckStaysQuietAboutItsOwnFailure(t *testing.T) {
	m := updateModel(t)
	m.toast = "untouched"

	// Being offline is not something the user asked about.
	m.Update(updateCheckMsg{auto: true, err: errString("dial tcp: no route to host")})
	if m.toast != "untouched" {
		t.Errorf("toast = %q, want the failed background check to stay silent", m.toast)
	}

	// An explicit /update says so, because it was asked for.
	m.Update(updateCheckMsg{auto: false, err: errString("dial tcp: no route to host")})
	if !strings.Contains(m.toast, "no route to host") {
		t.Errorf("toast = %q, want the error surfaced for a manual check", m.toast)
	}
}

func TestAutomaticCheckHonoursASkippedVersion(t *testing.T) {
	m := updateModel(t)
	m.updSkip = "9.9.9"
	m.toast = "untouched"

	m.Update(updateCheckMsg{rel: newerRelease(), newer: true, auto: true})
	if m.toast != "untouched" {
		t.Errorf("toast = %q, want silence about a skipped version", m.toast)
	}
	if m.updateBadge() != "" {
		t.Error("the status bar badged a version the user skipped")
	}
}

func TestManualUpdateConfirmsBeforeInstalling(t *testing.T) {
	m := updateModel(t)

	m.Update(updateCheckMsg{rel: newerRelease(), newer: true, auto: false})
	if m.confirm == nil {
		t.Fatal("/update installed without confirming")
	}
	body := strings.Join(m.confirm.message, "\n")
	for _, want := range []string{"9.9.9", "k10s_9.9.9_linux_amd64.tar.gz", "MiB", "much faster"} {
		if !strings.Contains(body, want) {
			t.Errorf("confirm body = %q, missing %q", body, want)
		}
	}
	// Replacing a binary is not a destructive act to undo, so it is not
	// styled as one — but it is still a confirmation.
	if m.confirm.danger {
		t.Error("the update dialog is styled as dangerous")
	}
}

func TestManualUpdateSaysWhenAlreadyCurrent(t *testing.T) {
	m := updateModel(t)

	rel := &update.Release{Version: version.Current(), Tag: "v" + version.Current()}
	m.Update(updateCheckMsg{rel: rel, newer: false, auto: false})
	if m.confirm != nil {
		t.Fatal("a dialog opened for a version we already run")
	}
	if !strings.Contains(m.toast, "latest") {
		t.Errorf("toast = %q, want it to say this is the latest", m.toast)
	}
}

func TestUpdateRefusesAReleaseWithNoBuildForThisPlatform(t *testing.T) {
	m := updateModel(t)

	rel := newerRelease()
	rel.Asset = update.Asset{}
	rel.Assets = []update.Asset{{Name: "k10s_9.9.9_plan9_mips.tar.gz"}}
	m.Update(updateCheckMsg{rel: rel, newer: true, auto: false})

	if m.confirm != nil {
		t.Fatal("a dialog opened for a release with nothing to install")
	}
	if !strings.Contains(m.toast, "plan9") {
		t.Errorf("toast = %q, want it to list what was published", m.toast)
	}
}

func TestUpdateWarnsWhenThereIsNoChecksumToVerifyAgainst(t *testing.T) {
	m := updateModel(t)

	rel := newerRelease()
	rel.Sums = update.Asset{}
	m.Update(updateCheckMsg{rel: rel, newer: true, auto: false})

	if m.confirm == nil {
		t.Fatal("no confirmation dialog")
	}
	body := strings.Join(m.confirm.message, "\n")
	if !strings.Contains(body, "unverified") {
		t.Errorf("confirm body = %q, want it to say the download is unverified", body)
	}
}

func TestSuccessfulInstallOffersToRestart(t *testing.T) {
	m := updateModel(t)
	m.updBusy = true

	m.Update(updateAppliedMsg{version: "9.9.9", path: "/usr/local/bin/k10s"})
	if m.updBusy {
		t.Error("updBusy stayed set after the install finished")
	}
	if m.confirm == nil {
		t.Fatal("no restart offer after installing")
	}
	if !strings.Contains(strings.Join(m.confirm.message, "\n"), "/usr/local/bin/k10s") {
		t.Error("the restart dialog does not say where the new binary went")
	}
	if m.Relaunch() {
		t.Error("Relaunch() = true before the offer was accepted")
	}

	cb := m.confirm.onOK
	cmd := cb(m)
	if !m.Relaunch() {
		t.Error("Relaunch() = false after accepting the restart")
	}
	if cmd == nil {
		t.Fatal("accepting the restart returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("accepting the restart did not quit")
	}
}

func TestFailedInstallReportsTheErrorAndClearsBusy(t *testing.T) {
	m := updateModel(t)
	m.updBusy = true

	m.Update(updateAppliedMsg{err: errString("checksum mismatch")})
	if m.updBusy || m.busy {
		t.Error("a failed install left the UI busy")
	}
	if !strings.Contains(m.toast, "checksum mismatch") {
		t.Errorf("toast = %q, want the failure surfaced", m.toast)
	}
	if m.confirm != nil {
		t.Error("a failed install offered to restart")
	}
}

func TestCheckResultIsPersistedSoTheNextLaunchIsQuiet(t *testing.T) {
	m := updateModel(t)

	m.Update(updateCheckMsg{rel: newerRelease(), newer: true, auto: true})

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Update.LastCheck == 0 {
		t.Error("update.last_check was not written; every launch would re-check")
	}
	// A check that just happened must not be due again.
	if update.Due(time.Unix(c.Update.LastCheck, 0)) {
		t.Error("a fresh check is immediately due again")
	}
}

func TestAutoCheckCommandRespectsTheSettingAndTheThrottle(t *testing.T) {
	m := updateModel(t)

	m.updDisabled, m.updLast = false, time.Time{}
	if m.autoCheckCmd() == nil {
		t.Error("no check scheduled on a first run with the setting on")
	}

	m.updLast = time.Now()
	if m.autoCheckCmd() != nil {
		t.Error("a check was scheduled minutes after the last one")
	}

	m.updDisabled, m.updLast = true, time.Time{}
	if m.autoCheckCmd() != nil {
		t.Error("a check was scheduled with the setting off")
	}
}

func TestUpdateCheckToggleIsPersisted(t *testing.T) {
	m := updateModel(t)

	m.setRow = rowUpdate()
	m.handleSettingsKey(key("enter")) // enter toggles, like the provider row
	if !m.updDisabled {
		t.Fatal("enter on the update row did not turn the check off")
	}
	c, _ := config.Load()
	if !c.Update.Disabled {
		t.Error("the setting was not written to the config file")
	}

	m.handleSettingsKey(key("enter"))
	if m.updDisabled {
		t.Error("enter did not turn the check back on")
	}
}

func TestSettingsDialogShowsTheUpdateRow(t *testing.T) {
	m := updateModel(t)
	m.openSettings()

	out := m.View()
	if !strings.Contains(out, "UPDATES") {
		t.Error("the settings dialog has no updates section")
	}
	if !strings.Contains(out, "daily") {
		t.Error("the settings dialog does not offer the daily check")
	}
}

func TestStatusBarBadgesAWaitingRelease(t *testing.T) {
	m := updateModel(t)
	if strings.Contains(m.View(), "⇧") {
		t.Fatal("the badge shows before any check has run")
	}

	m.updRel = newerRelease()
	if got := m.updateBadge(); got != "⇧ 9.9.9" {
		t.Errorf("updateBadge() = %q, want the version", got)
	}
	if !strings.Contains(m.View(), "9.9.9") {
		t.Error("the status bar does not show the waiting release")
	}
}

func TestUpdateCommandIsListedAndRoutable(t *testing.T) {
	var found bool
	for _, c := range SlashCommands {
		if c.Name == "/update" {
			found = true
		}
	}
	if !found {
		t.Error("/update is not in the command list, so the popup cannot offer it")
	}
	if !strings.Contains(Help(), "/update") {
		t.Error("/update is missing from /help")
	}
}

func TestVersionPanelExplainsWhereUpdatesComeFrom(t *testing.T) {
	m := updateModel(t)
	m.updRel = newerRelease()

	m.runSlash("/version")
	body := strings.Join(m.textLines, "\n")
	for _, want := range []string{"k10s", m.updateClient().Repository(), "9.9.9", config.Path()} {
		if !strings.Contains(body, want) {
			t.Errorf("/version body missing %q:\n%s", want, body)
		}
	}
}

func TestStartUpdateChecksFirstWhenNothingIsKnown(t *testing.T) {
	m := updateModel(t)

	if cmd := m.startUpdate(""); cmd == nil {
		t.Fatal("/update with no prior check returned no command")
	}
	if !m.busy {
		t.Error("/update does not show that it is checking")
	}
}

func TestStartUpdateDoesNotStackInstalls(t *testing.T) {
	m := updateModel(t)
	m.updBusy = true

	if cmd := m.startUpdate(""); cmd != nil {
		t.Error("/update started a second install while one was running")
	}
	if !strings.Contains(m.toast, "already") {
		t.Errorf("toast = %q, want it to say an update is in flight", m.toast)
	}
}

func TestHumanBytes(t *testing.T) {
	for in, want := range map[int64]string{
		0:        "",
		512:      "512 B",
		2048:     "2.0 KiB",
		31 << 20: "31.0 MiB",
		3 << 30:  "3.0 GiB",
	} {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestReleaseNotesAreTrimmedToFitAModal(t *testing.T) {
	body := "- one\n\n- two\n\n- three\n- four"
	got := releaseNotes(body, 2)
	if len(got) != 3 || got[2] != "…" {
		t.Errorf("releaseNotes = %q, want two lines plus an ellipsis", got)
	}
	if releaseNotes("", 5) != nil {
		t.Error("releaseNotes of an empty body should be nil")
	}
}

// errString is a minimal error for table-driven message tests.
type errString string

func (e errString) Error() string { return string(e) }

func TestUpdateDialogRendersWithinTheFrame(t *testing.T) {
	m := updateModel(t)
	m.Update(updateCheckMsg{rel: newerRelease(), newer: true, auto: false})

	out := m.View()
	if !strings.Contains(out, "Update k10s") {
		t.Fatal("the update dialog is not on screen")
	}
	// Every Block is padded to exactly w display cells; a modal whose text
	// overflows would push rows wider and make the whole frame drift.
	for i, ln := range strings.Split(out, "\n") {
		if w := lipgloss.Width(zone.Scan(ln)); w != m.w {
			t.Fatalf("row %d is %d cells wide, want %d", i, w, m.w)
		}
	}
}

func TestSkipSilencesOneVersionWithoutTurningTheCheckOff(t *testing.T) {
	m := updateModel(t)
	m.Update(updateCheckMsg{rel: newerRelease(), newer: true, auto: true})
	if m.updateBadge() == "" {
		t.Fatal("no badge to skip")
	}

	m.runSlash("/update skip")
	if m.updSkip != "9.9.9" {
		t.Errorf("updSkip = %q, want the release the check found", m.updSkip)
	}
	if m.updateBadge() != "" {
		t.Error("the badge survived being skipped")
	}
	if m.updDisabled {
		t.Error("skipping one version turned the whole check off")
	}
	c, _ := config.Load()
	if c.Update.Skip != "9.9.9" {
		t.Errorf("config update.skip = %q, want it persisted", c.Update.Skip)
	}

	// It is silenced, not blocked: /update still installs it on request.
	if m.startUpdate(""); m.confirm == nil {
		t.Error("/update refused to install a skipped version")
	}
}

func TestSkipNeedsSomethingToSkip(t *testing.T) {
	m := updateModel(t)
	m.runSlash("/update skip")
	if m.updSkip != "" {
		t.Errorf("updSkip = %q with no check behind it", m.updSkip)
	}
	if !strings.Contains(m.toast, "nothing to skip") {
		t.Errorf("toast = %q, want it to say there is nothing to skip yet", m.toast)
	}
}

func TestInstallingAVersionUnskipsIt(t *testing.T) {
	m := updateModel(t)
	m.updSkip = "9.9.9"

	m.Update(updateAppliedMsg{version: "9.9.9", path: "/usr/local/bin/k10s"})
	if m.updSkip != "" {
		t.Errorf("updSkip = %q after installing that version", m.updSkip)
	}
	// The next launch must re-check: what it knew is now about the old build.
	if !m.updLast.IsZero() {
		t.Error("the throttle survived an install, so the next launch would not re-check")
	}
}

func TestTheStartupCheckDoesNotClearAnUnrelatedSpinner(t *testing.T) {
	m := updateModel(t)
	m.startBusy("describe po/api-gateway")

	m.Update(updateCheckMsg{rel: newerRelease(), newer: true, auto: true})
	if !m.busy {
		t.Error("the background check cleared the spinner off an action still in flight")
	}
}
