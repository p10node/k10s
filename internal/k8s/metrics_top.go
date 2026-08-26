package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func fmtColW(s string, w int) string {
	if len(s) >= w {
		return s + " "
	}
	return s + strings.Repeat(" ", w-len(s))
}

// TopPod renders `kubectl top pod --containers`-equivalent output from the
// metrics-server snapshot.
func (s *Store) TopPod(ns, name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pm, err := s.c.Metrics.MetricsV1beta1().PodMetricses(effectiveNS(ns)).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("metrics-server: %w (is it installed?)", err)
	}

	var totalCPU, totalMem int64
	var b strings.Builder
	fmt.Fprintf(&b, "NAME                                    CPU(cores)   MEMORY(bytes)\n")
	for _, c := range pm.Containers {
		cpu := c.Usage[corev1.ResourceCPU]
		mem := c.Usage[corev1.ResourceMemory]
		totalCPU += cpu.MilliValue()
		totalMem += mem.Value()
	}
	fmt.Fprintf(&b, "%s%dm         %dMi\n\n", fmtColW(name, 40), totalCPU, totalMem/(1024*1024))
	b.WriteString("  CONTAINER   CPU(cores)   MEMORY(bytes)\n")
	for _, c := range pm.Containers {
		cpu := c.Usage[corev1.ResourceCPU]
		mem := c.Usage[corev1.ResourceMemory]
		fmt.Fprintf(&b, "  %s%dm          %dMi\n", fmtColW(c.Name, 12), cpu.MilliValue(), mem.Value()/(1024*1024))
	}
	return b.String(), nil
}

// TopNode renders `kubectl top node`-equivalent output.
func (s *Store) TopNode(name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	nm, err := s.c.Metrics.MetricsV1beta1().NodeMetricses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("metrics-server: %w (is it installed?)", err)
	}
	n, err := s.nodeLister().Get(name)
	if err != nil {
		return "", err
	}

	cpu := nm.Usage[corev1.ResourceCPU]
	mem := nm.Usage[corev1.ResourceMemory]
	capCPU := n.Status.Capacity.Cpu().MilliValue()
	capMem := n.Status.Capacity.Memory().Value()
	allocCPU := n.Status.Allocatable.Cpu()
	allocMem := n.Status.Allocatable.Memory()
	cpuPct, memPct := 0, 0
	if capCPU > 0 {
		cpuPct = int(cpu.MilliValue() * 100 / capCPU)
	}
	if capMem > 0 {
		memPct = int(mem.Value() * 100 / capMem)
	}
	sched := "schedulable"
	if n.Spec.Unschedulable {
		sched = "SchedulingDisabled"
	}

	var b strings.Builder
	b.WriteString("NAME                            CPU(cores)   CPU%   MEMORY(bytes)   MEMORY%\n")
	fmt.Fprintf(&b, "%s%s%s%s%d%%\n\n",
		fmtColW(name, 32), fmtColW(fmt.Sprintf("%dm", cpu.MilliValue()), 13),
		fmtColW(fmt.Sprintf("%d%%", cpuPct), 7), fmtColW(fmt.Sprintf("%dMi", mem.Value()/(1024*1024)), 16), memPct)
	fmt.Fprintf(&b, "  capacity     cpu: %s   memory: %s   pods: %s\n",
		n.Status.Capacity.Cpu().String(), n.Status.Capacity.Memory().String(), n.Status.Capacity.Pods().String())
	fmt.Fprintf(&b, "  allocatable  cpu: %s memory: %s   pods: %s\n", allocCPU.String(), allocMem.String(), n.Status.Allocatable.Pods().String())
	fmt.Fprintf(&b, "  scheduling   %s\n", sched)
	return b.String(), nil
}
