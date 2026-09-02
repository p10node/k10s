package k8s

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/p10node/k10s/internal/domain"
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
			// Under :ns all, group by namespace then name so related objects
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
		return applyNamespace(k.Cols, s.podRows(ns), ns)
	case "deployments":
		return applyNamespace(k.Cols, s.deployRows(ns), ns)
	case "statefulsets":
		return applyNamespace(k.Cols, s.stsRows(ns), ns)
	case "daemonsets":
		return applyNamespace(k.Cols, s.dsRows(ns), ns)
	case "jobs":
		return applyNamespace(k.Cols, s.jobRows(ns), ns)
	case "cronjobs":
		return applyNamespace(k.Cols, s.cronRows(ns), ns)
	case "services":
		return applyNamespace(k.Cols, s.svcRows(ns), ns)
	case "ingresses":
		return applyNamespace(k.Cols, s.ingRows(ns), ns)
	case "configmaps":
		return applyNamespace(k.Cols, s.cmRows(ns), ns)
	case "secrets":
		return applyNamespace(k.Cols, s.secretRows(ns), ns)
	case "pvcs":
		return applyNamespace(k.Cols, s.pvcRows(ns), ns)
	case "events":
		// Newest-first, not alphabetical — see applyNamespaceOpt.
		return applyNamespaceOpt(k.Cols, s.eventRows(ns), ns, false)
	case "nodes":
		return k.Cols, s.nodeTableRows()
	case "namespaces":
		return k.Cols, s.namespaceRows()
	case "crds":
		return k.Cols, s.crdRows()
	case "customresources":
		return applyNamespace(k.Cols, s.customResourceRows(), ns)
	case "replicasets":
		return applyNamespace(k.Cols, s.rsRows(ns), ns)
	case "hpas":
		return applyNamespace(k.Cols, s.hpaRows(ns), ns)
	case "endpoints":
		return applyNamespace(k.Cols, s.endpointRows(ns), ns)
	case "networkpolicies":
		return applyNamespace(k.Cols, s.netPolRows(ns), ns)
	case "resourcequotas":
		return applyNamespace(k.Cols, s.quotaRows(ns), ns)
	case "limitranges":
		return applyNamespace(k.Cols, s.limitRangeRows(ns), ns)
	case "pdbs":
		return applyNamespace(k.Cols, s.pdbRows(ns), ns)
	case "serviceaccounts":
		return applyNamespace(k.Cols, s.saRows(ns), ns)
	case "roles":
		return applyNamespace(k.Cols, s.roleRows(ns), ns)
	case "rolebindings":
		return applyNamespace(k.Cols, s.roleBindingRows(ns), ns)
	case "pvs":
		return k.Cols, s.pvRows()
	case "storageclasses":
		return k.Cols, s.storageClassRows()
	case "clusterroles":
		return k.Cols, s.clusterRoleRows()
	case "clusterrolebindings":
		return k.Cols, s.clusterRoleBindingRows()
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
		// Saying so here is also what starts that sweep — it costs a LIST
		// per CRD, so it waits until this badge is actually on screen.
		s.noteInterest(kind, ns)
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
	if !s.isStarted(kind, ns) || !s.SyncedFor(kind, ns) {
		s.noteInterest(kind, ns)
		if n, ok := s.cachedCount(kind, ns); ok {
			return n
		}
		return domain.CountUnknown
	}

	switch kind {
	case kPods:
		items, _ := s.podLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kDeployments:
		items, _ := s.deployLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kStatefulSet:
		items, _ := s.stsLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kDaemonSets:
		items, _ := s.dsLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kJobs:
		items, _ := s.jobLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kCronJobs:
		items, _ := s.cronLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kServices:
		items, _ := s.svcLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kIngresses:
		items, _ := s.ingLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kConfigMaps:
		items, _ := s.cmLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kSecrets:
		items, _ := s.secretLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kPVCs:
		items, _ := s.pvcLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kEvents:
		items, _ := s.eventLister(ns).List(labels.Everything())
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
	case kReplicaSets:
		items, _ := s.rsLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kHPAs:
		items, _ := s.hpaLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kEndpoints:
		items, _ := s.endpointsLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kNetPols:
		items, _ := s.netPolLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kQuotas:
		items, _ := s.quotaLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kLimitRanges:
		items, _ := s.limitRangeLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kPDBs:
		items, _ := s.pdbLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kSAs:
		items, _ := s.saLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kRoles:
		items, _ := s.roleLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kRoleBinds:
		items, _ := s.roleBindingLister(ns).List(labels.Everything())
		return countNS(items, ns)
	case kPVs:
		items, _ := s.pvLister().List(labels.Everything())
		return len(items)
	case kStorageCls:
		items, _ := s.storageClassLister().List(labels.Everything())
		return len(items)
	case kClusterRole:
		items, _ := s.clusterRoleLister().List(labels.Everything())
		return len(items)
	case kClusterBind:
		items, _ := s.clusterRoleBindingLister().List(labels.Everything())
		return len(items)
	}
	return domain.CountUnknown
}

// ---- pods -------------------------------------------------------------

func (s *Store) podRows(ns string) []nsRow {
	pods, _ := s.podLister(ns).List(labels.Everything())
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

func (s *Store) deployRows(ns string) []nsRow {
	items, _ := s.deployLister(ns).List(labels.Everything())
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

func (s *Store) stsRows(ns string) []nsRow {
	items, _ := s.stsLister(ns).List(labels.Everything())
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

func (s *Store) dsRows(ns string) []nsRow {
	items, _ := s.dsLister(ns).List(labels.Everything())
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

func (s *Store) jobRows(ns string) []nsRow {
	items, _ := s.jobLister(ns).List(labels.Everything())
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

func (s *Store) cronRows(ns string) []nsRow {
	items, _ := s.cronLister(ns).List(labels.Everything())
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

func (s *Store) svcRows(ns string) []nsRow {
	items, _ := s.svcLister(ns).List(labels.Everything())
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

func (s *Store) ingRows(ns string) []nsRow {
	items, _ := s.ingLister(ns).List(labels.Everything())
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

func (s *Store) cmRows(ns string) []nsRow {
	items, _ := s.cmLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, cm := range items {
		row := []string{cm.Name, strconv.Itoa(len(cm.Data) + len(cm.BinaryData)), age(cm.CreationTimestamp.Time)}
		out = append(out, nsRow{cm.Namespace, row})
	}
	return out
}

func (s *Store) secretRows(ns string) []nsRow {
	items, _ := s.secretLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, sec := range items {
		row := []string{sec.Name, string(sec.Type), strconv.Itoa(len(sec.Data)), age(sec.CreationTimestamp.Time)}
		out = append(out, nsRow{sec.Namespace, row})
	}
	return out
}

func (s *Store) pvcRows(ns string) []nsRow {
	items, _ := s.pvcLister(ns).List(labels.Everything())
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

func (s *Store) eventRows(ns string) []nsRow {
	items, _ := s.eventLister(ns).List(labels.Everything())
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
	pods, _ := s.podLister(domain.AllNamespaces).List(labels.Everything())
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// This is already a polling path. Read CRDs directly so an immediate
	// first sweep cannot mistake an informer that has not synced yet for a
	// genuinely empty CRD list and mark the view loaded too early.
	crdList, err := s.apiext.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		s.crMu.Lock()
		s.crErr = err
		s.crMu.Unlock()
		return
	}

	var out []nsRow
	var firstErr error
	for i := range crdList.Items {
		crd := &crdList.Items[i]
		ver := servedStorageVersion(crd)
		if ver == "" {
			continue
		}
		gvr := schema.GroupVersionResource{Group: crd.Spec.Group, Version: ver, Resource: crd.Spec.Names.Plural}
		list, err := s.listCustomResources(ctx, gvr, crd.Spec.Scope == apiextv1.NamespaceScoped)
		if err != nil || list == nil {
			if err != nil && firstErr == nil {
				firstErr = err
			}
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
	s.crCache, s.crAt, s.crErr = out, time.Now(), firstErr
	s.crMu.Unlock()
}

// listCustomResources isolates failures from runtime-discovered clients.
// Real dynamic clients return an error for an unusable GVR; some fake or
// third-party implementations panic when a List kind is missing. The sweep
// runs in a background goroutine, where such a panic must not kill the TUI.
func (s *Store) listCustomResources(ctx context.Context, gvr schema.GroupVersionResource, namespaced bool) (list *unstructured.UnstructuredList, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			list = nil
			err = fmt.Errorf("list %s: %v", gvr.String(), recovered)
		}
	}()
	if namespaced {
		return s.c.Dynamic.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	}
	return s.c.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
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

// ---- workloads and policy the k9s vocabulary reaches -----------------------
//
// Everything below mirrors what `kubectl get <kind>` prints for the same
// object, condensed to the columns declared in kinds.go. They are all shaped
// the same way: read the (lazily started) lister, format, hand back nsRow so
// applyNamespace can do the namespace filtering.

func (s *Store) rsRows(ns string) []nsRow {
	items, _ := s.rsLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, rs := range items {
		desired := int32(0)
		if rs.Spec.Replicas != nil {
			desired = *rs.Spec.Replicas
		}
		row := []string{
			rs.Name, strconv.Itoa(int(desired)), strconv.Itoa(int(rs.Status.Replicas)),
			strconv.Itoa(int(rs.Status.ReadyReplicas)), age(rs.CreationTimestamp.Time),
		}
		out = append(out, nsRow{rs.Namespace, row})
	}
	return out
}

func (s *Store) hpaRows(ns string) []nsRow {
	items, _ := s.hpaLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, h := range items {
		min := "-"
		if h.Spec.MinReplicas != nil {
			min = strconv.Itoa(int(*h.Spec.MinReplicas))
		}
		ref := h.Spec.ScaleTargetRef.Kind + "/" + h.Spec.ScaleTargetRef.Name
		row := []string{
			h.Name, ref, hpaTargets(h), min, strconv.Itoa(int(h.Spec.MaxReplicas)),
			strconv.Itoa(int(h.Status.CurrentReplicas)), age(h.CreationTimestamp.Time),
		}
		out = append(out, nsRow{h.Namespace, row})
	}
	return out
}

// hpaTargets renders "current/target" per metric the way kubectl's TARGETS
// column does, keeping only the first two so one autoscaler with six metrics
// can't push every other column off the table.
func hpaTargets(h *autoscalingv2.HorizontalPodAutoscaler) string {
	current := map[string]string{}
	for _, m := range h.Status.CurrentMetrics {
		name, val := metricNameValue(m.Type, m.Resource, m.Pods, m.Object, m.External)
		if name != "" {
			current[name] = val
		}
	}

	var parts []string
	for _, m := range h.Spec.Metrics {
		name, target := metricSpecNameTarget(m)
		if name == "" {
			continue
		}
		cur, ok := current[name]
		if !ok || cur == "" {
			cur = "<unknown>"
		}
		parts = append(parts, name+": "+cur+"/"+target)
	}
	if len(parts) == 0 {
		return "<none>"
	}
	if len(parts) > 2 {
		parts = append(parts[:2], fmt.Sprintf("+%d more", len(parts)-2))
	}
	return strings.Join(parts, ", ")
}

func metricNameValue(t autoscalingv2.MetricSourceType, res *autoscalingv2.ResourceMetricStatus,
	pods *autoscalingv2.PodsMetricStatus, obj *autoscalingv2.ObjectMetricStatus,
	ext *autoscalingv2.ExternalMetricStatus) (string, string) {
	switch t {
	case autoscalingv2.ResourceMetricSourceType:
		if res == nil {
			return "", ""
		}
		if res.Current.AverageUtilization != nil {
			return string(res.Name), strconv.Itoa(int(*res.Current.AverageUtilization)) + "%"
		}
		if res.Current.AverageValue != nil {
			return string(res.Name), res.Current.AverageValue.String()
		}
		return string(res.Name), "<unknown>"
	case autoscalingv2.PodsMetricSourceType:
		if pods == nil {
			return "", ""
		}
		return pods.Metric.Name, quantityOrUnknown(pods.Current.AverageValue)
	case autoscalingv2.ObjectMetricSourceType:
		if obj == nil {
			return "", ""
		}
		return obj.Metric.Name, quantityOrUnknown(obj.Current.Value)
	case autoscalingv2.ExternalMetricSourceType:
		if ext == nil {
			return "", ""
		}
		return ext.Metric.Name, quantityOrUnknown(ext.Current.Value)
	}
	return "", ""
}

func metricSpecNameTarget(m autoscalingv2.MetricSpec) (string, string) {
	target := func(t autoscalingv2.MetricTarget) string {
		if t.AverageUtilization != nil {
			return strconv.Itoa(int(*t.AverageUtilization)) + "%"
		}
		if t.AverageValue != nil {
			return t.AverageValue.String()
		}
		return quantityOrUnknown(t.Value)
	}
	switch m.Type {
	case autoscalingv2.ResourceMetricSourceType:
		if m.Resource == nil {
			return "", ""
		}
		return string(m.Resource.Name), target(m.Resource.Target)
	case autoscalingv2.PodsMetricSourceType:
		if m.Pods == nil {
			return "", ""
		}
		return m.Pods.Metric.Name, target(m.Pods.Target)
	case autoscalingv2.ObjectMetricSourceType:
		if m.Object == nil {
			return "", ""
		}
		return m.Object.Metric.Name, target(m.Object.Target)
	case autoscalingv2.ExternalMetricSourceType:
		if m.External == nil {
			return "", ""
		}
		return m.External.Metric.Name, target(m.External.Target)
	}
	return "", ""
}

func quantityOrUnknown(q *apiresource.Quantity) string {
	if q == nil {
		return "<unknown>"
	}
	return q.String()
}

func (s *Store) endpointRows(ns string) []nsRow {
	items, _ := s.endpointsLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, ep := range items {
		var addrs []string
		total := 0
		for _, sub := range ep.Subsets {
			for _, a := range sub.Addresses {
				total++
				if len(addrs) < 3 {
					if len(sub.Ports) > 0 {
						addrs = append(addrs, fmt.Sprintf("%s:%d", a.IP, sub.Ports[0].Port))
					} else {
						addrs = append(addrs, a.IP)
					}
				}
			}
		}
		list := strings.Join(addrs, ",")
		switch {
		case total == 0:
			list = "<none>"
		case total > len(addrs):
			list += fmt.Sprintf(" +%d more", total-len(addrs))
		}
		out = append(out, nsRow{ep.Namespace, []string{ep.Name, list, age(ep.CreationTimestamp.Time)}})
	}
	return out
}

func (s *Store) netPolRows(ns string) []nsRow {
	items, _ := s.netPolLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, np := range items {
		out = append(out, nsRow{np.Namespace, []string{
			np.Name, selectorString(&np.Spec.PodSelector), age(np.CreationTimestamp.Time),
		}})
	}
	return out
}

// selectorString renders a label selector the way kubectl does, with
// "<none>" for the empty selector that matches everything.
func selectorString(sel *metav1.LabelSelector) string {
	if sel == nil {
		return "<none>"
	}
	s, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil || s.Empty() {
		return "<none>"
	}
	return s.String()
}

func (s *Store) quotaRows(ns string) []nsRow {
	items, _ := s.quotaLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, q := range items {
		out = append(out, nsRow{q.Namespace, []string{
			q.Name, quotaUsage(q, "requests."), quotaUsage(q, "limits."), age(q.CreationTimestamp.Time),
		}})
	}
	return out
}

// quotaUsage summarises "used/hard" for the request or limit half of a quota,
// which is what kubectl's REQUEST and LIMIT columns show.
func quotaUsage(q *corev1.ResourceQuota, prefix string) string {
	names := make([]string, 0, len(q.Status.Hard))
	for name := range q.Status.Hard {
		if strings.HasPrefix(string(name), prefix) {
			names = append(names, string(name))
		}
	}
	if len(names) == 0 {
		return "-"
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		hard := q.Status.Hard[corev1.ResourceName(name)]
		used := "0"
		if u, ok := q.Status.Used[corev1.ResourceName(name)]; ok {
			used = u.String()
		}
		parts = append(parts, strings.TrimPrefix(name, prefix)+": "+used+"/"+hard.String())
	}
	if len(parts) > 2 {
		parts = append(parts[:2], fmt.Sprintf("+%d more", len(parts)-2))
	}
	return strings.Join(parts, ", ")
}

func (s *Store) limitRangeRows(ns string) []nsRow {
	items, _ := s.limitRangeLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, lr := range items {
		types := make([]string, 0, len(lr.Spec.Limits))
		for _, l := range lr.Spec.Limits {
			types = append(types, string(l.Type))
		}
		list := strings.Join(types, ",")
		if list == "" {
			list = "<none>"
		}
		out = append(out, nsRow{lr.Namespace, []string{lr.Name, list, age(lr.CreationTimestamp.Time)}})
	}
	return out
}

func (s *Store) pdbRows(ns string) []nsRow {
	items, _ := s.pdbLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, p := range items {
		minAvail, maxUnavail := "N/A", "N/A"
		if p.Spec.MinAvailable != nil {
			minAvail = p.Spec.MinAvailable.String()
		}
		if p.Spec.MaxUnavailable != nil {
			maxUnavail = p.Spec.MaxUnavailable.String()
		}
		out = append(out, nsRow{p.Namespace, []string{
			p.Name, minAvail, maxUnavail, strconv.Itoa(int(p.Status.DisruptionsAllowed)),
			age(p.CreationTimestamp.Time),
		}})
	}
	return out
}

// ---- RBAC ------------------------------------------------------------------

func (s *Store) saRows(ns string) []nsRow {
	items, _ := s.saLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, sa := range items {
		out = append(out, nsRow{sa.Namespace, []string{
			sa.Name, strconv.Itoa(len(sa.Secrets)), age(sa.CreationTimestamp.Time),
		}})
	}
	return out
}

func (s *Store) roleRows(ns string) []nsRow {
	items, _ := s.roleLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, r := range items {
		out = append(out, nsRow{r.Namespace, []string{
			r.Name, strconv.Itoa(len(r.Rules)), age(r.CreationTimestamp.Time),
		}})
	}
	return out
}

func (s *Store) roleBindingRows(ns string) []nsRow {
	items, _ := s.roleBindingLister(ns).List(labels.Everything())
	out := make([]nsRow, 0, len(items))
	for _, rb := range items {
		out = append(out, nsRow{rb.Namespace, []string{
			rb.Name, rb.RoleRef.Kind + "/" + rb.RoleRef.Name, subjectList(rb.Subjects),
			age(rb.CreationTimestamp.Time),
		}})
	}
	return out
}

func (s *Store) clusterRoleRows() [][]string {
	items, _ := s.clusterRoleLister().List(labels.Everything())
	out := make([][]string, 0, len(items))
	for _, r := range items {
		out = append(out, []string{r.Name, strconv.Itoa(len(r.Rules)), age(r.CreationTimestamp.Time)})
	}
	sortRows(out, 0)
	return out
}

func (s *Store) clusterRoleBindingRows() [][]string {
	items, _ := s.clusterRoleBindingLister().List(labels.Everything())
	out := make([][]string, 0, len(items))
	for _, rb := range items {
		out = append(out, []string{
			rb.Name, rb.RoleRef.Kind + "/" + rb.RoleRef.Name, subjectList(rb.Subjects),
			age(rb.CreationTimestamp.Time),
		})
	}
	sortRows(out, 0)
	return out
}

// subjectList names who a binding grants to, capped so a binding with fifty
// service accounts stays one readable row.
func subjectList(subjects []rbacv1.Subject) string {
	if len(subjects) == 0 {
		return "<none>"
	}
	names := make([]string, 0, len(subjects))
	for _, s := range subjects {
		names = append(names, s.Name)
	}
	if len(names) > 2 {
		return strings.Join(names[:2], ",") + fmt.Sprintf(" +%d more", len(names)-2)
	}
	return strings.Join(names, ",")
}

// ---- cluster-scoped storage ------------------------------------------------

func (s *Store) pvRows() [][]string {
	items, _ := s.pvLister().List(labels.Everything())
	out := make([][]string, 0, len(items))
	for _, pv := range items {
		capacity := "-"
		if q, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
			capacity = q.String()
		}
		claim := "<none>"
		if pv.Spec.ClaimRef != nil {
			claim = pv.Spec.ClaimRef.Namespace + "/" + pv.Spec.ClaimRef.Name
		}
		sc := pv.Spec.StorageClassName
		if sc == "" {
			sc = "<none>"
		}
		out = append(out, []string{
			pv.Name, capacity, accessModes(pv.Spec.AccessModes), string(pv.Spec.PersistentVolumeReclaimPolicy),
			string(pv.Status.Phase), claim, sc, age(pv.CreationTimestamp.Time),
		})
	}
	sortRows(out, 0)
	return out
}

// accessModes abbreviates the way kubectl does: RWO/ROX/RWX/RWOP.
func accessModes(modes []corev1.PersistentVolumeAccessMode) string {
	short := map[corev1.PersistentVolumeAccessMode]string{
		corev1.ReadWriteOnce:    "RWO",
		corev1.ReadOnlyMany:     "ROX",
		corev1.ReadWriteMany:    "RWX",
		corev1.ReadWriteOncePod: "RWOP",
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range modes {
		s, ok := short[m]
		if !ok {
			s = string(m)
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return "-"
	}
	return strings.Join(out, ",")
}

func (s *Store) storageClassRows() [][]string {
	items, _ := s.storageClassLister().List(labels.Everything())
	out := make([][]string, 0, len(items))
	for _, sc := range items {
		reclaim := "Delete"
		if sc.ReclaimPolicy != nil {
			reclaim = string(*sc.ReclaimPolicy)
		}
		binding := "Immediate"
		if sc.VolumeBindingMode != nil {
			binding = string(*sc.VolumeBindingMode)
		}
		name := sc.Name
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			name += " (default)"
		}
		out = append(out, []string{name, sc.Provisioner, reclaim, binding, age(sc.CreationTimestamp.Time)})
	}
	sortRows(out, 0)
	return out
}
