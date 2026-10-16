package ui

import (
	"github.com/p10node/k10s/internal/mock"
	"reflect"
	"testing"
)

func TestLogHistoryOverlapKeepsRepeatedMessages(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		history, current, want []string
		ok                     bool
	}{
		{"repeated lines", []string{"old", "same", "same", "end"}, []string{"same", "same", "end", "live"}, []string{"old", "same", "same", "end", "live"}, true},
		{"history ahead of stream", []string{"old", "first", "pending"}, []string{"first"}, []string{"old", "first"}, true},
		{"ambiguous", []string{"old", "same", "same"}, []string{"same"}, []string{"same"}, false},
		{"no overlap", []string{"old"}, []string{"new"}, []string{"new"}, false},
		{"nothing delivered", []string{"old"}, nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := prependLogHistory(tc.history, tc.current)
			if ok != tc.ok || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v %v, want %v %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestLogHistoryMergesOverlapNotLengthDifference(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	m.mode = modeLogs
	m.logKind, m.logNS, m.logName = "pods", "default", "pod"
	m.textLines = []string{"t3 c", "t4 d", "t5 e", "t6 live", "t7 live"}
	m.Update(logOlderMsg{kind: "pods", ns: "default", name: "pod", lines: []string{"t1 a", "t2 b", "t3 c", "t4 d", "t5 e"}, more: true})
	want := []string{"t1 a", "t2 b", "t3 c", "t4 d", "t5 e", "t6 live", "t7 live"}
	if !reflect.DeepEqual(m.textLines, want) {
		t.Fatalf("history/live continuity: got %v want %v", m.textLines, want)
	}
}
