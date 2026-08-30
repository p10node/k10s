package k8s

import "github.com/p10node/k10s/internal/domain"

var podActions = []string{domain.ADescribe, domain.AYAML, domain.ALogs, domain.AShell, domain.APortFwd, domain.ATop, domain.AEdit, domain.ADelete}
var wlActions = []string{domain.ADescribe, domain.AYAML, domain.ALogs, domain.ARestart, domain.AScale, domain.AEdit, domain.ADelete}
var basicActions = []string{domain.ADescribe, domain.AYAML, domain.AEdit, domain.ADelete}
var nodeActions = []string{domain.ADescribe, domain.AYAML, domain.ATop, domain.ACordon, domain.ADrain, domain.AEdit}

// ReplicaSets scale and have logs like the workloads above, but nothing
// patches a rollout restart onto them — that belongs to the Deployment.
var rsActions = []string{domain.ADescribe, domain.AYAML, domain.ALogs, domain.AScale, domain.AEdit, domain.ADelete}

// builtinKinds mirrors internal/mock's resource list so the UI behaves
// identically against either backend.
var builtinKinds = []domain.Kind{
	{Key: "pods", Name: "Pods", Short: "po", Group: "Workloads", Namespaced: true,
		Cols: []string{"NAME", "READY", "STATUS", "RESTARTS", "CPU", "MEM", "NODE", "AGE"}, Allowed: podActions},
	{Key: "deployments", Name: "Deployments", Short: "deploy", Group: "Workloads", Namespaced: true,
		Cols: []string{"NAME", "READY", "UP-TO-DATE", "AVAILABLE", "IMAGE", "AGE"}, Allowed: wlActions},
	{Key: "replicasets", Name: "ReplicaSets", Short: "rs", Group: "Workloads", Namespaced: true,
		Cols: []string{"NAME", "DESIRED", "CURRENT", "READY", "AGE"}, Allowed: rsActions},
	{Key: "statefulsets", Name: "StatefulSets", Short: "sts", Group: "Workloads", Namespaced: true,
		Cols: []string{"NAME", "READY", "IMAGE", "AGE"}, Allowed: wlActions},
	{Key: "daemonsets", Name: "DaemonSets", Short: "ds", Group: "Workloads", Namespaced: true,
		Cols: []string{"NAME", "DESIRED", "READY", "AGE"}, Allowed: wlActions},
	{Key: "jobs", Name: "Jobs", Short: "job", Group: "Workloads", Namespaced: true,
		Cols: []string{"NAME", "COMPLETIONS", "DURATION", "AGE"}, Allowed: []string{domain.ADescribe, domain.AYAML, domain.ALogs, domain.ADelete}},
	{Key: "cronjobs", Name: "CronJobs", Short: "cj", Group: "Workloads", Namespaced: true,
		Cols: []string{"NAME", "SCHEDULE", "SUSPEND", "LAST SCHEDULE", "AGE"}, Allowed: basicActions},
	{Key: "hpas", Name: "HPAs", Short: "hpa", Group: "Workloads", Namespaced: true,
		Cols: []string{"NAME", "REFERENCE", "TARGETS", "MIN", "MAX", "REPLICAS", "AGE"}, Allowed: basicActions},
	{Key: "services", Name: "Services", Short: "svc", Group: "Network", Namespaced: true,
		Cols: []string{"NAME", "TYPE", "CLUSTER-IP", "PORTS", "AGE"}, Allowed: []string{domain.ADescribe, domain.AYAML, domain.APortFwd, domain.AEdit, domain.ADelete}},
	{Key: "endpoints", Name: "Endpoints", Short: "ep", Group: "Network", Namespaced: true,
		Cols: []string{"NAME", "ENDPOINTS", "AGE"}, Allowed: basicActions},
	{Key: "ingresses", Name: "Ingresses", Short: "ing", Group: "Network", Namespaced: true,
		Cols: []string{"NAME", "CLASS", "HOSTS", "ADDRESS", "AGE"}, Allowed: basicActions},
	{Key: "networkpolicies", Name: "NetworkPolicies", Short: "netpol", Group: "Network", Namespaced: true,
		Cols: []string{"NAME", "POD-SELECTOR", "AGE"}, Allowed: basicActions},
	{Key: "configmaps", Name: "ConfigMaps", Short: "cm", Group: "Config", Namespaced: true,
		Cols: []string{"NAME", "DATA", "AGE"}, Allowed: basicActions},
	{Key: "secrets", Name: "Secrets", Short: "sec", Group: "Config", Namespaced: true,
		Cols: []string{"NAME", "TYPE", "DATA", "AGE"}, Allowed: basicActions},
	{Key: "resourcequotas", Name: "ResourceQuotas", Short: "quota", Group: "Config", Namespaced: true,
		Cols: []string{"NAME", "REQUEST", "LIMIT", "AGE"}, Allowed: basicActions},
	{Key: "limitranges", Name: "LimitRanges", Short: "limits", Group: "Config", Namespaced: true,
		Cols: []string{"NAME", "LIMITS", "AGE"}, Allowed: basicActions},
	{Key: "pdbs", Name: "PDBs", Short: "pdb", Group: "Config", Namespaced: true,
		Cols: []string{"NAME", "MIN AVAILABLE", "MAX UNAVAILABLE", "ALLOWED", "AGE"}, Allowed: basicActions},
	{Key: "pvcs", Name: "PVCs", Short: "pvc", Group: "Storage", Namespaced: true,
		Cols: []string{"NAME", "STATUS", "CAPACITY", "STORAGECLASS", "AGE"}, Allowed: basicActions},
	{Key: "pvs", Name: "PVs", Short: "pv", Group: "Storage", Namespaced: false,
		Cols: []string{"NAME", "CAPACITY", "ACCESS", "RECLAIM", "STATUS", "CLAIM", "STORAGECLASS", "AGE"}, Allowed: basicActions},
	{Key: "storageclasses", Name: "StorageClasses", Short: "sc", Group: "Storage", Namespaced: false,
		Cols: []string{"NAME", "PROVISIONER", "RECLAIM", "BINDING", "AGE"}, Allowed: basicActions},
	{Key: "serviceaccounts", Name: "ServiceAccounts", Short: "sa", Group: "RBAC", Namespaced: true,
		Cols: []string{"NAME", "SECRETS", "AGE"}, Allowed: basicActions},
	{Key: "roles", Name: "Roles", Short: "role", Group: "RBAC", Namespaced: true,
		Cols: []string{"NAME", "RULES", "AGE"}, Allowed: basicActions},
	{Key: "rolebindings", Name: "RoleBindings", Short: "rb", Group: "RBAC", Namespaced: true,
		Cols: []string{"NAME", "ROLE", "SUBJECTS", "AGE"}, Allowed: basicActions},
	{Key: "clusterroles", Name: "ClusterRoles", Short: "crole", Group: "RBAC", Namespaced: false,
		Cols: []string{"NAME", "RULES", "AGE"}, Allowed: basicActions},
	{Key: "clusterrolebindings", Name: "ClusterRoleBindings", Short: "crb", Group: "RBAC", Namespaced: false,
		Cols: []string{"NAME", "ROLE", "SUBJECTS", "AGE"}, Allowed: basicActions},
	{Key: "nodes", Name: "Nodes", Short: "no", Group: "Cluster", Namespaced: false,
		Cols: []string{"NAME", "STATUS", "ROLES", "VERSION", "CPU%", "MEM%", "AGE"}, Allowed: nodeActions},
	{Key: "namespaces", Name: "Namespaces", Short: "ns", Group: "Cluster", Namespaced: false,
		Cols: []string{"NAME", "STATUS", "PODS", "AGE"}, Allowed: []string{domain.ADescribe, domain.AYAML, domain.ADelete}},
	{Key: "events", Name: "Events", Short: "ev", Group: "Cluster", Namespaced: true,
		Cols: []string{"TYPE", "REASON", "OBJECT", "MESSAGE", "AGE"}, Allowed: []string{domain.ADescribe}},
	{Key: "crds", Name: "CRDs", Short: "crd", Group: "Custom Resources", Namespaced: false,
		Cols: []string{"NAME", "GROUP", "VERSION", "SCOPE", "KIND", "AGE"}, Allowed: basicActions},
	{Key: "customresources", Name: "Custom Resources", Short: "cr", Group: "Custom Resources", Namespaced: true,
		Cols: []string{"NAME", "KIND", "AGE"}, Allowed: []string{domain.ADescribe, domain.AYAML, domain.AEdit, domain.ADelete}},
}

func findKind(key string) *domain.Kind {
	for i := range builtinKinds {
		if builtinKinds[i].Key == key {
			return &builtinKinds[i]
		}
	}
	return nil
}

// Kinds is the resource list the real backend serves, available without a
// cluster connection. Startup uses it so the Resources pane is populated
// while the connection is still being made.
func Kinds() []domain.Kind {
	return append([]domain.Kind(nil), builtinKinds...)
}
