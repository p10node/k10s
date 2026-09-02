package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServicePortForwardResolvesNamedTargetPort(t *testing.T) {
	p := pod("default", "named-port-pod", "node-a", true)
	p.Labels = map[string]string{"app": "named-port"}
	p.Spec.Containers[0].Ports = []corev1.ContainerPort{{Name: "http", ContainerPort: 80, Protocol: corev1.ProtocolTCP}}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "named-port", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "named-port"},
			Ports:    []corev1.ServicePort{{Name: "web", Port: 8080, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP}},
		},
	}

	s := newTestStore(t, p, svc)
	syncKinds(t, s, kPods, kServices)

	podName, port, err := s.portForwardTarget(kServices, "default", "named-port")
	if err != nil {
		t.Fatalf("portForwardTarget: %v", err)
	}
	if podName != p.Name || port != 80 {
		t.Fatalf("target = %s:%d, want %s:80", podName, port, p.Name)
	}
}

func TestServicePortForwardRejectsUnknownNamedTargetPort(t *testing.T) {
	p := pod("default", "named-port-pod", "node-a", true)
	p.Labels = map[string]string{"app": "named-port"}
	p.Spec.Containers[0].Ports = []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "named-port", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "named-port"},
			Ports:    []corev1.ServicePort{{Port: 8080, TargetPort: intstr.FromString("http")}},
		},
	}

	s := newTestStore(t, p, svc)
	syncKinds(t, s, kPods, kServices)
	if _, _, err := s.portForwardTarget(kServices, "default", "named-port"); err == nil {
		t.Fatal("unknown named targetPort unexpectedly resolved")
	}
}

func TestServicePortForwardRejectsSelectorlessService(t *testing.T) {
	p := pod("default", "unrelated", "node-a", true)
	p.Spec.Containers[0].Ports = []corev1.ContainerPort{{ContainerPort: 80}}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(80)}},
		},
	}

	s := newTestStore(t, p, svc)
	syncKinds(t, s, kPods, kServices)
	if _, _, err := s.portForwardTarget(kServices, "default", "external"); err == nil {
		t.Fatal("selectorless service unexpectedly forwarded to an unrelated pod")
	}
}
