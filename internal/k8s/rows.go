package k8s

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/duration"

	"k10s/internal/domain"
)

// nsRow pairs a namespace with a rendered row, mirroring how mock.NSRow used
// to bucket fake data — here the namespace comes straight off the live
// object instead of a static table.
type nsRow struct {
	ns  string
	row []string
}

func age(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return duration.ShortHumanDuration(time.Since(t))
}

// applyNamespace turns a flat set of namespaced rows into what the UI shows
// for ns: "" / "default" → only namespace "default"; AllNamespaces → every
// row with a NAMESPACE column prepended; anything else → only that ns.
func applyNamespace(cols []string, rows []nsRow, ns string) ([]string, [][]string) {
	return applyNamespaceOpt(cols, rows, ns, true)
}

// applyNamespaceOpt is applyNamespace with control over alphabetical
// ordering. Events pass sorted=false: they arrive newest-first from
// eventRows and that ordering is the point, so re-sorting them by name
// would bury the most recent ones.
func applyNamespaceOpt(cols []string, rows []nsRow, ns string, sorted bool) ([]string, [][]string) {
	eff := ns
	if eff == "" {
		eff = "default"
	}
	if eff == domain.AllNamespaces {
		out := make([][]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, append([]string{r.ns}, r.row...))
		}
		if sorted {
			// Under /ns all, group by namespace then name so related objects
			// stay together instead of interleaving. NAMESPACE was just
			// prepended, so it is column 0 and the name is column 1.
			sortRows(out, 0, 1)
		}
		return append([]string{"NAMESPACE"}, cols...), out
	}
	var out [][]string
	for _, r := range rows {
		if r.ns == eff {
			out = append(out, r.row)
		}
	}
	if sorted {
		sortRows(out, 0)
	}
	return cols, out
}

// sortRows orders rows alphabetically by the given column indices, in
// priority order. Informer caches come back in arbitrary (hash) order, so
// without this the table reshuffles on every refresh.
func sortRows(rows [][]string, cols ...int) {
	sort.SliceStable(rows, func(i, j int) bool {
		for _, c := range cols {
			a, b := "", ""
			if c < len(rows[i]) {
				a = rows[i][c]
			}
			if c < len(rows[j]) {
				b = rows[j][c]
			}
			if a != b {
				return domain.NaturalLess(a, b)
			}
		}
		return false
	})
}

func (s *Store) Rows(kind, ns string) ([]string, [][]string) {
	k := findKind(kind)
	if k == nil {
		return s.crRows(kind, ns)
	}
	switch kind {
	case "pods":
		return applyNamespace(k.Cols, s.podRows(), ns)
	case "deployments":
		return applyNamespace(k.Cols, s.deployRows(), ns)
	case "statefulsets":
		return applyNamespace(k.Cols, s.stsRows(), ns)
	case "daemonsets":
		return applyNamespace(k.Cols, s.dsRows(), ns)
	case "jobs":
		return applyNamespace(k.Cols, s.jobRows(), ns)
	case "cronjobs":
		return applyNamespace(k.Cols, s.cronRows(), ns)
	case "services":
		return applyNamespace(k.Cols, s.svcRows(), ns)
	case "ingresses":
		return applyNamespace(k.Cols, s.ingRows(), ns)
	case "configmaps":
		return applyNamespace(k.Cols, s.cmRows(), ns)
	case "secrets":
		return applyNamespace(k.Cols, s.secretRows(), ns)
	case "pvcs":
		return applyNamespace(k.Cols, s.pvcRows(), ns)
	case "events":
		// Newest-first, not alphabetical — see applyNamespaceOpt.
		return applyNamespaceOpt(k.Cols, s.eventRows(), ns, false)
	case "nodes":
		return k.Cols, s.nodeTableRows()
	case "namespaces":
		return k.Cols, s.namespaceRows()
	case "crds":
		return k.Cols, s.crdRows()
	case "customresources":
		return applyNamespace(k.Cols, s.customResourceRows(), ns)
	}
	return k.Cols, nil
}

// countNS counts items by namespace without building any formatted rows —
// RowCount backs the Resources-pane badge for every kind, redrawn on every
// keypress and repaint tick, so it must stay cheap even on a cluster with
// many pods/events/secrets (unlike Rows, which only runs for the kind
// currently on screen).
func countNS[T metav1.Object](items []T, ns string) int {
	eff := ns
	if eff == "" {
		eff = "default"
	}
	if eff == domain.AllNamespaces {
		return len(items)
	}
	n := 0
	for _, it := range items {
		if it.GetNamespace() == eff {
			n++
		}
	}
	return n
}

// RowCount backs the Resources-pane badge for every kind, on every repaint.
// It must therefore be cheap AND side-effect free: it reads only caches that
// are already running, and returns domain.CountUnknown for a kind whose
// informer hasn't started. Starting one here would mean that merely drawing
// the sidebar sets up a cluster-wide watch for all 15 kinds — including
// every secret and event — which is exactly what made the UI crawl.
func (s *Store) RowCount(kind, ns string) int {
	if kind == "customresources" {
		// Custom resources have no informer; they're refreshed off-thread.
		// Report whatever the last background sweep found, never block.
		s.crMu.Lock()
		cached, ok := s.crCache, !s.crAt.IsZero()
		s.crMu.Unlock()
		if !ok {
			return domain.CountUnknown
		}
		_, rows := applyNamespace(nil, cached, ns)
		return len(rows)
	}

	// Two cases fall back to the background sweep's cheap count:
	//   - the kind was never opened, so there is no informer at all;
	//   - it was just opened and its cache is still filling, where the
	//     lister would truthfully report 0 and the sidebar badge would drop
	//     to "0" for a second before snapping back to the real number.
	// Showing the last known count through the load is far less jarring.
	if !s.isStarted(kind) || !s.Synced(kind) {
		s.noteInterest(ns)
		if n, ok := s.cachedCount(kind, ns); ok {
			return n
		}
		return domain.CountUnknown
	}

	switch kind {
	case kPods:
		items, _ := s.podLister().List(labels.Everything())
		return countNS(items, ns)
	case kDeployments:
		items, _ := s.deployLister().List(labels.Everything())
		return countNS(items, ns)
	case kStatefulSet:
		items, _ := s.stsLister().List(labels.Everything())
		return countNS(items, ns)
	case kDaemonSets:
		items, _ := s.dsLister().List(labels.Everything())
		return countNS(items, ns)
	case kJobs:
		items, _ := s.jobLister().List(labels.Everything())
		return countNS(items, ns)
	case kCronJobs:
		items, _ := s.cronLister().List(labels.Everything())
		return countNS(items, ns)
	case kServices:
		items, _ := s.svcLister().List(labels.Everything())
		return countNS(items, ns)
	case kIngresses:
		items, _ := s.ingLister().List(labels.Everything())
		return countNS(items, ns)
	case kConfigMaps:
		items, _ := s.cmLister().List(labels.Everything())
		return countNS(items, ns)
	case kSecrets:
		items, _ := s.secretLister().List(labels.Everything())
		return countNS(items, ns)
	case kPVCs:
		items, _ := s.pvcLister().List(labels.Everything())
		return countNS(items, ns)
	case kEvents:
		items, _ := s.eventLister().List(labels.Everything())
		return countNS(items, ns)
	case kNodes:
		items, _ := s.nodeLister().List(labels.Everything())
		return len(items)
	case kNamespaces:
		items, _ := s.nsLister().List(labels.Everything())
		return len(items)
	case kCRDs:
		items, _ := s.crdLister().List(labels.Everything())
		return len(items)
	}
	return domain.CountUnknown
}

// ---- pods -------------------------------------------------------------

func (s *Store) podRows() []nsRow {
	pods, _ := s.podLister().List(labels.Everything())
	out := make([]nsRow, 0, len(pods))
	for _, p := range pods {
		ready, total := 0, len(p.Spec.Containers)
		var restarts int32
		for _, cs := range p.Status.ContainerStatuses {
			if cs.Ready {
				ready++
			}
			restarts += cs.RestartCount
		}
		cpu, mem := "-", "-"
		if m, ok := s.podMetric(p.Namespace, p.Name); ok {
			cpu = fmt.Sprintf("%dm", m.cpuMilli)
			mem = fmt.Sprintf("%dMi", m.memBytes/(1024*1024))
		}
		node := p.Spec.NodeName
		if node == "" {
			node = "<none>"
		}
		row := []string{
			p.Name, fmt.Sprintf("%d/%d", ready, total), podStatus(p), strconv.Itoa(int(restarts)),
			cpu, mem, node, age(p.CreationTimestamp.Time),
		}
		out = append(out, nsRow{p.Namespace, row})
	}
	return out
}

func podStatus(p *corev1.Pod) string {
	if p.DeletionTimestamp != nil {
		return "Terminating"
	}
	switch p.Status.Phase {
	case corev1.PodSucceeded:
		return "Completed"
	case corev1.PodFailed:
		if p.Status.Reason != "" {
			return p.Status.Reason
		}
		return "Failed"
	}
	for _, cs := range p.Status.InitContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" && cs.State.Waiting.Reason != "PodInitializing" {
			return "Init:" + cs.State.Waiting.Reason
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return "Init:Error"
		}
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return "Error"
		}
	}
	return string(p.Status.Phase)
}

// ---- deployments / statefulsets / daemonsets ---------------------------

func firstImage(spec corev1.PodSpec) string {
	if len(spec.Containers) > 0 {
		return spec.Containers[0].Image
	}
	return "-"
}

func (s *Store) deployRows() []nsRow {
	items, _ := s.deployLister().List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, d := range items {
		desired := int32(1)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		row := []string{
			d.Name, fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, desired),
			strconv.Itoa(int(d.Status.UpdatedReplicas)), strconv.Itoa(int(d.Status.AvailableReplicas)),
			firstImage(d.Spec.Template.Spec), age(d.CreationTimestamp.Time),
		}
		out = append(out, nsRow{d.Namespace, row})
	}
	return out
}

func (s *Store) stsRows() []nsRow {
	items, _ := s.stsLister().List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, d := range items {
		desired := int32(1)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		row := []string{
			d.Name, fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, desired),
			firstImage(d.Spec.Template.Spec), age(d.CreationTimestamp.Time),
		}
		out = append(out, nsRow{d.Namespace, row})
	}
	return out
}

func (s *Store) dsRows() []nsRow {
	items, _ := s.dsLister().List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, d := range items {
		row := []string{
			d.Name, strconv.Itoa(int(d.Status.DesiredNumberScheduled)),
			strconv.Itoa(int(d.Status.NumberReady)), age(d.CreationTimestamp.Time),
		}
		out = append(out, nsRow{d.Namespace, row})
	}
	return out
}

// ---- jobs / cronjobs -----------------------------------------------------

func (s *Store) jobRows() []nsRow {
	items, _ := s.jobLister().List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, j := range items {
		completions := "1"
		if j.Spec.Completions != nil {
			completions = strconv.Itoa(int(*j.Spec.Completions))
		}
		dur := "-"
		if j.Status.StartTime != nil {
			end := time.Now()
			if j.Status.CompletionTime != nil {
				end = j.Status.CompletionTime.Time
			}
			dur = duration.ShortHumanDuration(end.Sub(j.Status.StartTime.Time))
		}
		row := []string{
			j.Name, fmt.Sprintf("%d/%s", j.Status.Succeeded, completions), dur, age(j.CreationTimestamp.Time),
		}
		out = append(out, nsRow{j.Namespace, row})
	}
	return out
}

func (s *Store) cronRows() []nsRow {
	items, _ := s.cronLister().List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, c := range items {
		suspend := c.Spec.Suspend != nil && *c.Spec.Suspend
		last := "<none>"
		if c.Status.LastScheduleTime != nil {
			last = age(c.Status.LastScheduleTime.Time)
		}
		row := []string{
			c.Name, c.Spec.Schedule, strconv.FormatBool(suspend), last, age(c.CreationTimestamp.Time),
		}
		out = append(out, nsRow{c.Namespace, row})
	}
	return out
}

// ---- services / ingresses -------------------------------------------------

func (s *Store) svcRows() []nsRow {
	items, _ := s.svcLister().List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, svc := range items {
		ports := make([]string, 0, len(svc.Spec.Ports))
		for _, p := range svc.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
		}
		portStr := strings.Join(ports, ",")
		if portStr == "" {
			portStr = "<none>"
		}
		clusterIP := svc.Spec.ClusterIP
		if clusterIP == "" {
			clusterIP = "<none>"
		}
		row := []string{svc.Name, string(svc.Spec.Type), clusterIP, portStr, age(svc.CreationTimestamp.Time)}
		out = append(out, nsRow{svc.Namespace, row})
	}
	return out
}

func (s *Store) ingRows() []nsRow {
	items, _ := s.ingLister().List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, ing := range items {
		class := "<none>"
		if ing.Spec.IngressClassName != nil {
			class = *ing.Spec.IngressClassName
		}
		hosts := make([]string, 0, len(ing.Spec.Rules))
		for _, r := range ing.Spec.Rules {
			if r.Host != "" {
				hosts = append(hosts, r.Host)
			}
		}
		hostStr := strings.Join(hosts, ",")
		if hostStr == "" {
			hostStr = "*"
		}
		addrs := make([]string, 0, len(ing.Status.LoadBalancer.Ingress))
		for _, lb := range ing.Status.LoadBalancer.Ingress {
			if lb.IP != "" {
				addrs = append(addrs, lb.IP)
			} else if lb.Hostname != "" {
				addrs = append(addrs, lb.Hostname)
			}
		}
		addrStr := strings.Join(addrs, ",")
		if addrStr == "" {
			addrStr = "-"
		}
		row := []string{ing.Name, class, hostStr, addrStr, age(ing.CreationTimestamp.Time)}
		out = append(out, nsRow{ing.Namespace, row})
	}
	return out
}

// ---- configmaps / secrets / pvcs ------------------------------------------

func (s *Store) cmRows() []nsRow {
	items, _ := s.cmLister().List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, cm := range items {
		row := []string{cm.Name, strconv.Itoa(len(cm.Data) + len(cm.BinaryData)), age(cm.CreationTimestamp.Time)}
		out = append(out, nsRow{cm.Namespace, row})
	}
	return out
}

func (s *Store) secretRows() []nsRow {
	items, _ := s.secretLister().List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, sec := range items {
		row := []string{sec.Name, string(sec.Type), strconv.Itoa(len(sec.Data)), age(sec.CreationTimestamp.Time)}
		out = append(out, nsRow{sec.Namespace, row})
	}
	return out
}

func (s *Store) pvcRows() []nsRow {
	items, _ := s.pvcLister().List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, pvc := range items {
		cap := "-"
		if q, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
			cap = q.String()
		}
		sc := "<none>"
		if pvc.Spec.StorageClassName != nil {
			sc = *pvc.Spec.StorageClassName
		}
		row := []string{pvc.Name, string(pvc.Status.Phase), cap, sc, age(pvc.CreationTimestamp.Time)}
		out = append(out, nsRow{pvc.Namespace, row})
	}
	return out
}

// ---- events ----------------------------------------------------------------

func (s *Store) eventRows() []nsRow {
	items, _ := s.eventLister().List(labels.Everything())
	sort.Slice(items, func(i, j int) bool {
		return eventTime(items[i]).After(eventTime(items[j]))
	})
	if len(items) > 300 {
		items = items[:300]
	}
	out := make([]nsRow, 0, len(items))
	for _, e := range items {
		obj := strings.ToLower(e.InvolvedObject.Kind) + "/" + e.InvolvedObject.Name
		row := []string{e.Type, e.Reason, obj, e.Message, age(eventTime(e))}
		out = append(out, nsRow{e.Namespace, row})
	}
	return out
}

func eventTime(e *corev1.Event) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	return e.CreationTimestamp.Time
}

// ---- cluster-scoped: nodes / namespaces / crds -----------------------------

func nodeRoles(n *corev1.Node) string {
	var roles []string
	for label := range n.Labels {
		if strings.HasPrefix(label, "node-role.kubernetes.io/") {
			roles = append(roles, strings.TrimPrefix(label, "node-role.kubernetes.io/"))
		}
	}
	if len(roles) == 0 {
		return "<none>"
	}
	sort.Strings(roles)
	return strings.Join(roles, ",")
}

func nodeStatus(n *corev1.Node) string {
	status := "Unknown"
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			if c.Status == corev1.ConditionTrue {
				status = "Ready"
			} else {
				status = "NotReady"
			}
		}
	}
	if n.Spec.Unschedulable {
		status += ",SchedulingDisabled"
	}
	return status
}

func nodePercents(n *corev1.Node, s *Store) (cpuPct, memPct int) {
	m, ok := s.nodeMetric(n.Name)
	if !ok {
		return 0, 0
	}
	capCPU := n.Status.Capacity.Cpu().MilliValue()
	capMem := n.Status.Capacity.Memory().Value()
	if capCPU > 0 {
		cpuPct = int(m.cpuMilli * 100 / capCPU)
	}
	if capMem > 0 {
		memPct = int(m.memBytes * 100 / capMem)
	}
	return
}

func nodeInfo(n *corev1.Node, s *Store) domain.NodeInfo {
	cpuPct, memPct := nodePercents(n, s)
	role := "worker"
	if strings.Contains(nodeRoles(n), "control-plane") || strings.Contains(nodeRoles(n), "master") {
		role = "control-plane"
	}
	status := "Ready"
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status != corev1.ConditionTrue {
			status = "NotReady"
		}
	}
	return domain.NodeInfo{
		Name: n.Name, Status: status, Role: role, Ver: n.Status.NodeInfo.KubeletVersion,
		CPU: cpuPct, Mem: memPct, Age: age(n.CreationTimestamp.Time),
	}
}

func (s *Store) nodeTableRows() [][]string {
	items, _ := s.nodeLister().List(labels.Everything())
	out := make([][]string, 0, len(items))
	for _, n := range items {
		cpuPct, memPct := nodePercents(n, s)
		out = append(out, []string{
			n.Name, nodeStatus(n), nodeRoles(n), n.Status.NodeInfo.KubeletVersion,
			strconv.Itoa(cpuPct) + "%", strconv.Itoa(memPct) + "%", age(n.CreationTimestamp.Time),
		})
	}
	sortRows(out, 0)
	return out
}

func (s *Store) namespaceRows() [][]string {
	items, _ := s.nsLister().List(labels.Everything())
	pods, _ := s.podLister().List(labels.Everything())
	out := make([][]string, 0, len(items))
	for _, n := range items {
		count := 0
		for _, p := range pods {
			if p.Namespace == n.Name {
				count++
			}
		}
		out = append(out, []string{n.Name, string(n.Status.Phase), strconv.Itoa(count), age(n.CreationTimestamp.Time)})
	}
	sortRows(out, 0)
	return out
}

func (s *Store) crdRows() [][]string {
	items, _ := s.crdLister().List(labels.Everything())
	out := make([][]string, 0, len(items))
	for _, crd := range items {
		out = append(out, []string{
			crd.Name, crd.Spec.Group, servedStorageVersion(crd), string(crd.Spec.Scope), crd.Spec.Names.Kind,
			age(crd.CreationTimestamp.Time),
		})
	}
	sortRows(out, 0)
	return out
}

// customResourceRows aggregates instances of every namespaced-or-cluster CRD
// into one table (NAME, KIND, AGE), matching mock's "Custom Resources" kind.
// This does a live API list per CRD (not informer-cached) since instances of
// arbitrary, runtime-discovered CRDs aren't worth a dynamic informer per GVR
// for a view that's opened occasionally.
// customResourceRows returns the last background sweep's result. It never
// touches the network: listing custom resources means one API call per CRD
// (dozens on a cluster running cert-manager/argo/prometheus-operator), which
// must never happen on the render path. refreshCRs does that work off-thread.
func (s *Store) customResourceRows() []nsRow {
	s.ensureCRRefresh()
	s.crMu.Lock()
	defer s.crMu.Unlock()
	return s.crCache
}

// ensureCRRefresh kicks off the background CR sweep the first time custom
// resources are displayed, and keeps it refreshing on an interval.
func (s *Store) ensureCRRefresh() {
	s.crMu.Lock()
	already := s.crRunning
	s.crRunning = true
	s.crMu.Unlock()
	if already {
		return
	}
	go func() {
		s.refreshCRs()
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				s.refreshCRs()
			}
		}
	}()
}

func (s *Store) refreshCRs() {
	crds, _ := s.crdLister().List(labels.Everything())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out []nsRow
	for _, crd := range crds {
		ver := servedStorageVersion(crd)
		if ver == "" {
			continue
		}
		gvr := schema.GroupVersionResource{Group: crd.Spec.Group, Version: ver, Resource: crd.Spec.Names.Plural}
		var list *unstructured.UnstructuredList
		var err error
		if crd.Spec.Scope == apiextv1.NamespaceScoped {
			list, err = s.c.Dynamic.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		} else {
			list, err = s.c.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
		}
		if err != nil || list == nil {
			continue
		}
		for _, item := range list.Items {
			ns := item.GetNamespace()
			if ns == "" {
				ns = "-"
			}
			out = append(out, nsRow{ns, []string{item.GetName(), crd.Spec.Names.Kind, age(item.GetCreationTimestamp().Time)}})
		}
	}

	s.crMu.Lock()
	s.crCache, s.crAt = out, time.Now()
	s.crMu.Unlock()
}

// crRows handles a directly-encoded "cr|group|version|resource|namespaced"
// kind key for a single CRD's instances (used when jumping to one CRD via
// describe/yaml, not from the table itself).
func (s *Store) crRows(kind, ns string) ([]string, [][]string) {
	return []string{"NAME", "KIND", "AGE"}, nil
}

var (
	_ = appsv1.Deployment{}
	_ = batchv1.Job{}
	_ = networkingv1.Ingress{}
)
