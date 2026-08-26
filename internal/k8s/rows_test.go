package k8s

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k10s/internal/domain"
)

func TestPodStatus(t *testing.T) {
	cases := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{"running", &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}, "Running"},
		{"succeeded->completed", &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}, "Completed"},
		{"failed with reason", &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed, Reason: "Evicted"}}, "Evicted"},
		{"failed no reason", &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}}, "Failed"},
		{
			"waiting container reason wins",
			&corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
					},
				},
			},
			"CrashLoopBackOff",
		},
		{
			"terminated non-zero -> Error",
			&corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}},
					},
				},
			},
			"Error",
		},
		{
			"deletion timestamp -> Terminating",
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
			"Terminating",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := podStatus(c.pod); got != c.want {
				t.Errorf("podStatus() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestAge(t *testing.T) {
	if got := age(time.Time{}); got != "-" {
		t.Errorf("age(zero) = %q, want -", got)
	}
	got := age(time.Now().Add(-6 * 24 * time.Hour))
	if !strings.Contains(got, "d") {
		t.Errorf("age(6d ago) = %q, want something like \"6d\"", got)
	}
}

func TestNodeRolesAndStatus(t *testing.T) {
	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			"node-role.kubernetes.io/control-plane": "",
		}},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	}
	if got := nodeRoles(n); got != "control-plane" {
		t.Errorf("nodeRoles() = %q, want control-plane", got)
	}
	if got := nodeStatus(n); got != "Ready" {
		t.Errorf("nodeStatus() = %q, want Ready", got)
	}

	n.Spec.Unschedulable = true
	if got := nodeStatus(n); got != "Ready,SchedulingDisabled" {
		t.Errorf("nodeStatus() cordoned = %q, want Ready,SchedulingDisabled", got)
	}

	bare := &corev1.Node{}
	if got := nodeRoles(bare); got != "<none>" {
		t.Errorf("nodeRoles(no labels) = %q, want <none>", got)
	}
}

func TestApplyNamespace(t *testing.T) {
	cols := []string{"NAME", "AGE"}
	rows := []nsRow{
		{ns: "default", row: []string{"a", "1d"}},
		{ns: "kube-system", row: []string{"b", "2d"}},
	}

	gotCols, gotRows := applyNamespace(cols, rows, "")
	if len(gotRows) != 1 || gotRows[0][0] != "a" {
		t.Errorf("ns=\"\" (default): got %v", gotRows)
	}
	if gotCols[0] == "NAMESPACE" {
		t.Errorf("ns=default should not prepend NAMESPACE column")
	}

	gotCols, gotRows = applyNamespace(cols, rows, domain.AllNamespaces)
	if gotCols[0] != "NAMESPACE" {
		t.Errorf("ns=all should prepend NAMESPACE column, got %v", gotCols)
	}
	if len(gotRows) != 2 {
		t.Errorf("ns=all: got %d rows, want 2", len(gotRows))
	}

	_, gotRows = applyNamespace(cols, rows, "kube-system")
	if len(gotRows) != 1 || gotRows[0][0] != "b" {
		t.Errorf("ns=kube-system: got %v", gotRows)
	}

	_, gotRows = applyNamespace(cols, rows, "no-such-namespace")
	if len(gotRows) != 0 {
		t.Errorf("ns=no-such-namespace: got %v, want none", gotRows)
	}
}

func TestCountNS(t *testing.T) {
	pods := []*corev1.Pod{
		pod("default", "a", "n1", true),
		pod("default", "b", "n1", true),
		pod("kube-system", "c", "n1", true),
	}
	if got := countNS(pods, ""); got != 2 {
		t.Errorf("countNS(default) = %d, want 2", got)
	}
	if got := countNS(pods, domain.AllNamespaces); got != 3 {
		t.Errorf("countNS(all) = %d, want 3", got)
	}
	if got := countNS(pods, "kube-system"); got != 1 {
		t.Errorf("countNS(kube-system) = %d, want 1", got)
	}
}
