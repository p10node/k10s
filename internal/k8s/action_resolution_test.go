package k8s

import (
	"errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"

	"github.com/p10node/k10s/internal/domain"
)

func TestColdActionResolution(t *testing.T) {
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-pod", Namespace: "a", Labels: map[string]string{"app": "web"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "a"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}}}}
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "a"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "web"}, Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromString("http")}}}}
	for _, action := range []string{"pod-log", "workload-log", "shell", "pod-forward", "service-forward"} {
		t.Run(action, func(t *testing.T) {
			s := newTestStore(t, p, d, svc)
			switch action {
			case "pod-log", "workload-log":
				kind, name := "pods", "web-pod"
				if action == "workload-log" {
					kind, name = "deployments", "web"
				}
				got, c, err := s.logTarget(kind, "a", name)
				if err != nil || got != "web-pod" || c != "app" {
					t.Fatalf("target=%s/%s err=%v", got, c, err)
				}
			case "shell":
				if _, err := s.Shell("pods", "a", "web-pod"); err != nil {
					t.Fatal(err)
				}
			default:
				kind, name := "pods", "web-pod"
				if action == "service-forward" {
					kind, name = "services", "web"
				}
				got, port, err := s.portForwardTarget(kind, "a", name)
				if err != nil || got != "web-pod" || port != 8080 {
					t.Fatalf("target=%s:%d err=%v", got, port, err)
				}
			}
			if s.isStarted(kPods, "a") {
				t.Fatal("action started a persistent pod watch")
			}
		})
	}
}

// S runs Shell in the real terminal, so it has to land on the pod and
// container the in-panel shell picks: a workload resolves to one of its pods.
func TestShellResolvesTheSamePodAsShellSession(t *testing.T) {
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-pod", Namespace: "a", Labels: map[string]string{"app": "web"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "a"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}}}}
	s := newTestStore(t, p, d)

	for _, tc := range []struct{ kind, name string }{{"pods", "web-pod"}, {"deployments", "web"}} {
		cmd, err := s.Shell(tc.kind, "a", tc.name)
		if err != nil {
			t.Fatalf("Shell(%s/%s): %v", tc.kind, tc.name, err)
		}
		e, ok := cmd.(*execCommand)
		if !ok {
			t.Fatalf("Shell(%s/%s) returned %T", tc.kind, tc.name, cmd)
		}
		if e.ns != "a" || e.pod != "web-pod" || e.container != "app" {
			t.Errorf("Shell(%s/%s) targets %s/%s container %s, want a/web-pod container app", tc.kind, tc.name, e.ns, e.pod, e.container)
		}
	}

	if _, err := s.Shell("configmaps", "a", "settings"); !errors.Is(err, domain.ErrNoShell) {
		t.Errorf("Shell on a configmap: err = %v, want ErrNoShell", err)
	}
}
