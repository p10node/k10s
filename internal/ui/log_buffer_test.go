package ui

import (
	"github.com/p10node/k10s/internal/config"
	"github.com/p10node/k10s/internal/mock"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLogRetentionByteBudgetAndHistory(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.mode = modeLogs
	m.logLimits = config.Logs{MaxLines: 4, MaxBytes: 10}
	m.logKind, m.logNS, m.logName = "pods", "default", "pod"
	m.textLines = []string{"cccc", "dddd"}
	m.Update(logOlderMsg{kind: "pods", ns: "default", name: "pod", lines: []string{"aaaa", "bbbb", "cccc", "dddd"}, more: true})
	if strings.Join(m.textLines, ",") != "cccc,dddd" || m.logMore {
		t.Fatalf("history escaped retention: %v more=%v", m.textLines, m.logMore)
	}
	m.Update(logLineMsg{gen: m.logGen, line: "eeee", ok: true})
	if strings.Join(m.textLines, ",") != "dddd,eeee" {
		t.Fatalf("wrong retained tail: %v", m.textLines)
	}
}

func TestLogRetentionBoundsLiveLines(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.mode = modeLogs
	m.textLines = make([]string, 5000)
	for i := range m.textLines {
		m.textLines[i] = "old"
	}
	m.Update(logLineMsg{gen: m.logGen, line: "newest", ok: true})
	if len(m.textLines) != 5000 || m.textLines[len(m.textLines)-1] != "newest" {
		t.Fatalf("retention failed: %d lines", len(m.textLines))
	}
}

func TestLogRetentionBoundsOversizedUTF8Line(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.mode = modeLogs
	m.Update(logLineMsg{gen: m.logGen, line: strings.Repeat("ế", 100000), ok: true})
	if len(m.textLines) != 1 || len(m.textLines[0]) > 64*1024 || !utf8.ValidString(m.textLines[0]) {
		t.Fatal("oversized log line was not bounded safely")
	}
}
