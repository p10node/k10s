package k8s

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/p10node/k10s/internal/domain"
)

// fixtures is one object of every kind added for k9s parity, so each one can
// be listed, counted and rendered the same way the older kinds are.
func fixtures() []runtime.Object {
	two := int32(2)
	min := int32(2)
	util := int32(70)
	cur := int32(41)
	retain := corev1.PersistentVolumeReclaimRetain
	scReclaim := corev1.PersistentVolumeReclaimDelete
	binding := storagev1.VolumeBindingWaitForFirstConsumer
	minAvail := intstr.FromInt32(1)

	return []runtime.Object{
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "web-7d9f", Namespace: "default"},
			Spec:       appsv1.ReplicaSetSpec{Replicas: &two, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}}},
			Status:     appsv1.ReplicaSetStatus{Replicas: 2, ReadyReplicas: 2},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "web"},
				MinReplicas:    &min, MaxReplicas: 10,
				Metrics: []autoscalingv2.MetricSpec{{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name:   corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{AverageUtilization: &util},
					},
				}},
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{
				CurrentReplicas: 3,
				CurrentMetrics: []autoscalingv2.MetricStatus{{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricStatus{
						Name:    corev1.ResourceCPU,
						Current: autoscalingv2.MetricValueStatus{AverageUtilization: &cur},
					},
				}},
			},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "10.244.1.5"}},
				Ports:     []corev1.EndpointPort{{Port: 8080}},
			}},
		},
		&networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "deny-all", Namespace: "default"},
		},
		&corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "compute", Namespace: "default"},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{"requests.cpu": apiresource.MustParse("4")},
				Used: corev1.ResourceList{"requests.cpu": apiresource.MustParse("1")},
			},
		},
		&corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: "defaults", Namespace: "default"},
			Spec:       corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{Type: corev1.LimitTypeContainer}}},
		},
		&policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec:       policyv1.PodDisruptionBudgetSpec{MinAvailable: &minAvail},
			Status:     policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: 1},
		},
		&corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-123"},
			Spec: corev1.PersistentVolumeSpec{
				Capacity:                      corev1.ResourceList{corev1.ResourceStorage: apiresource.MustParse("10Gi")},
				AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				PersistentVolumeReclaimPolicy: retain,
				StorageClassName:              "gp3",
				ClaimRef:                      &corev1.ObjectReference{Namespace: "default", Name: "data-0"},
			},
			Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
		},
		&storagev1.StorageClass{
			ObjectMeta:        metav1.ObjectMeta{Name: "gp3", Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}},
			Provisioner:       "ebs.csi.aws.com",
			ReclaimPolicy:     &scReclaim,
			VolumeBindingMode: &binding,
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Secrets:    []corev1.ObjectReference{{Name: "web-token"}},
		},
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "config-reader", Namespace: "default"},
			Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}, Resources: []string{"configmaps"}}},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "config-reader", Namespace: "default"},
			RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "config-reader"},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "web"}},
		},
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "view"},
			Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}, Resources: []string{"pods"}}},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "view"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "view"},
			Subjects:   []rbacv1.Subject{{Kind: "Group", Name: "developers"}},
		},
	}
}

var newKinds = []string{
	kReplicaSets, kHPAs, kEndpoints, kNetPols, kQuotas, kLimitRanges, kPDBs,
	kPVs, kStorageCls, kSAs, kRoles, kRoleBinds, kClusterRole, kClusterBind,
}

// Every kind added for k9s parity must actually render: a kind in the list
// whose Rows come back empty is a menu entry that opens onto nothing.
func TestNewKindsRenderRowsAndCounts(t *testing.T) {
	s := newTestStore(t, fixtures()...)
	syncKinds(t, s, newKinds...)

	for _, key := range newKinds {
		k := findKind(key)
		if k == nil {
			t.Fatalf("%q is not in builtinKinds", key)
		}
		cols, rows := s.Rows(key, domain.AllNamespaces)
		if len(rows) != 1 {
			t.Errorf("Rows(%q) returned %d rows, want the 1 fixture object", key, len(rows))
			continue
		}
		want := len(k.Cols)
		if k.Namespaced {
			want++ // applyNamespace prepends NAMESPACE under "all"
		}
		if len(cols) != want || len(rows[0]) != want {
			t.Errorf("Rows(%q): %d cols, row has %d cells, want %d", key, len(cols), len(rows[0]), want)
		}
		for i, cell := range rows[0] {
			if cell == "" {
				t.Errorf("Rows(%q) row cell %d (%s) is empty", key, i, cols[i])
			}
		}
		if n := s.RowCount(key, domain.AllNamespaces); n != 1 {
			t.Errorf("RowCount(%q) = %d, want 1", key, n)
		}
	}
}

// The generic paths — YAML, edit, delete, badge counts for unopened kinds —
// all resolve through gvrFor, so a kind missing there is broken in four
// places at once.
func TestEveryKindResolvesAGVR(t *testing.T) {
	s := newTestStore(t)
	for _, k := range Kinds() {
		if k.Key == "customresources" {
			continue // resolved per-instance against the discovered CRD
		}
		gvr, namespaced, err := s.gvrFor(k.Key)
		if err != nil {
			t.Errorf("gvrFor(%q): %v", k.Key, err)
			continue
		}
		if gvr.Resource == "" {
			t.Errorf("gvrFor(%q) returned an empty resource", k.Key)
		}
		if namespaced != k.Namespaced {
			t.Errorf("gvrFor(%q) says namespaced=%v, the kind says %v", k.Key, namespaced, k.Namespaced)
		}
	}
}

// Describe is the one action every kind allows, so every kind must have a
// describer — the built-in one where kubectl has it, generic otherwise.
func TestEveryKindHasADescriber(t *testing.T) {
	for _, k := range Kinds() {
		if k.Key == "customresources" || k.Key == "crds" {
			continue // both go through genericDescribe by design
		}
		if _, ok := kindToGK[k.Key]; !ok {
			t.Errorf("%q has no GroupKind, so describe falls back to the generic path", k.Key)
		}
	}
}

// Opening one of the new kinds must not start watches for the others.
func TestNewKindsStayLazy(t *testing.T) {
	s := newTestStore(t, fixtures()...)
	s.Rows("pvs", domain.AllNamespaces)

	if !s.isStarted(kPVs) {
		t.Fatal("viewing PVs did not start the PV informer")
	}
	for _, k := range newKinds {
		if k != kPVs && s.isStarted(k) {
			t.Errorf("opening PVs also started %q", k)
		}
	}
}
