// Package mock is the offline demo backend: it implements domain.Source with
// static, in-memory fake cluster data, no network calls. It backs the
// headless dev renderer (cmd/shot) and is the fallback when k10s can't reach
// a real cluster.
package mock

import (
	"k10s/internal/domain"
)

// node is one row of the fake cluster's node list.
type node struct {
	Name   string
	Status string
	Role   string
	Ver    string
	CPU    int // percent
	Mem    int // percent
	Age    string
}

// nsRow is a table row that only exists in a namespace other than "default"
// — rows in a resourceDef's base Rows field are implicitly namespace
// "default".
type nsRow struct {
	NS  string
	Row []string
}

// resourceDef is one entry of the left panel plus its table payload.
type resourceDef struct {
	domain.Kind
	Rows  [][]string
	Extra []nsRow // rows living in a non-default namespace
}

var podActions = []string{domain.ADescribe, domain.AYAML, domain.ALogs, domain.AShell, domain.APortFwd, domain.ATop, domain.AEdit, domain.ADelete}
var wlActions = []string{domain.ADescribe, domain.AYAML, domain.ALogs, domain.ARestart, domain.AScale, domain.AEdit, domain.ADelete}
var basicActions = []string{domain.ADescribe, domain.AYAML, domain.AEdit, domain.ADelete}
var nodeActions = []string{domain.ADescribe, domain.AYAML, domain.ATop, domain.ACordon, domain.ADrain, domain.AEdit}

// clusterNodes are the fake cluster's nodes (header gauges + "nodes" kind).
var clusterNodes = []node{
	{"ip-10-0-1-14.ap-southeast-1", "Ready", "control-plane", "v1.31.4", 34, 61, "128d"},
	{"ip-10-0-2-88.ap-southeast-1", "Ready", "worker", "v1.31.4", 72, 55, "128d"},
	{"ip-10-0-3-51.ap-southeast-1", "NotReady", "worker", "v1.31.4", 9, 12, "17d"},
}

const clusterVersion = "v1.31.4"

// contexts available for /context switching.
var contexts = []string{
	"teleport.internal.s3.p10node.onl-S3",
	"eks-staging-apse1",
	"gke-prod-asia",
}

var resources = []resourceDef{
	{
		Kind: domain.Kind{Key: "pods", Name: "Pods", Short: "po", Group: "Workloads", Namespaced: true,
			Cols: []string{"NAME", "READY", "STATUS", "RESTARTS", "CPU", "MEM", "NODE", "AGE"}, Allowed: podActions},
		Rows: [][]string{
			{"api-gateway-7d9f4c8b6d-2xk4p", "1/1", "Running", "0", "142m", "310Mi", "ip-10-0-2-88", "6d"},
			{"api-gateway-7d9f4c8b6d-hv8qz", "1/1", "Running", "0", "128m", "298Mi", "ip-10-0-1-14", "6d"},
			{"auth-service-5c7b9d4f8-lm2rt", "1/1", "Running", "2", "64m", "188Mi", "ip-10-0-2-88", "14d"},
			{"billing-worker-6f8d9c5b7-qq91x", "0/1", "CrashLoopBackOff", "17", "0m", "24Mi", "ip-10-0-2-88", "3h"},
			{"cache-redis-0", "1/1", "Running", "0", "22m", "512Mi", "ip-10-0-1-14", "62d"},
			{"cache-redis-1", "1/1", "Running", "0", "19m", "498Mi", "ip-10-0-2-88", "62d"},
			{"migrate-db-29341-8kdlp", "0/1", "Completed", "0", "0m", "0Mi", "ip-10-0-1-14", "51m"},
			{"notify-consumer-84fbb9c7c-5wnjd", "1/1", "Running", "1", "88m", "241Mi", "ip-10-0-3-51", "9d"},
			{"payment-api-9d7c8f6b5-t4z8v", "2/2", "Running", "0", "204m", "722Mi", "ip-10-0-2-88", "2d"},
			{"payment-api-9d7c8f6b5-wr3nc", "1/2", "Pending", "0", "-", "-", "<none>", "4m"},
			{"search-indexer-7c9b4d8f6-jx0pm", "1/1", "Running", "0", "310m", "1.2Gi", "ip-10-0-2-88", "21d"},
			{"web-frontend-6b8c7d9f5-c8trq", "1/1", "Running", "0", "76m", "180Mi", "ip-10-0-1-14", "1d"},
			{"web-frontend-6b8c7d9f5-nb4kd", "1/1", "Running", "0", "71m", "176Mi", "ip-10-0-3-51", "1d"},
			{"web-frontend-6b8c7d9f5-zp7gx", "1/1", "Terminating", "0", "18m", "92Mi", "ip-10-0-2-88", "1d"},
		},
		Extra: []nsRow{
			{"kube-system", []string{"coredns-6d4b75cb6d-4x9kp", "1/1", "Running", "0", "3m", "18Mi", "ip-10-0-1-14", "128d"}},
			{"kube-system", []string{"coredns-6d4b75cb6d-9zt2q", "1/1", "Running", "0", "3m", "17Mi", "ip-10-0-2-88", "128d"}},
			{"kube-system", []string{"kube-proxy-vwj4r", "1/1", "Running", "0", "2m", "24Mi", "ip-10-0-2-88", "128d"}},
			{"monitoring", []string{"prometheus-server-0", "2/2", "Running", "0", "180m", "890Mi", "ip-10-0-1-14", "62d"}},
			{"monitoring", []string{"grafana-7f9c8d6b5-k2m1p", "1/1", "Running", "0", "12m", "156Mi", "ip-10-0-2-88", "62d"}},
			{"staging", []string{"checkout-api-8f7d6c5b4-x1y2z", "1/1", "Running", "0", "45m", "210Mi", "ip-10-0-2-88", "5d"}},
			{"staging", []string{"checkout-api-8f7d6c5b4-a3b4c", "0/1", "CrashLoopBackOff", "9", "0m", "31Mi", "ip-10-0-2-88", "5d"}},
		},
	},
	{
		Kind: domain.Kind{Key: "deployments", Name: "Deployments", Short: "deploy", Group: "Workloads", Namespaced: true,
			Cols: []string{"NAME", "READY", "UP-TO-DATE", "AVAILABLE", "IMAGE", "AGE"}, Allowed: wlActions},
		Rows: [][]string{
			{"api-gateway", "2/2", "2", "2", "ghcr.io/p10/api-gateway:1.24.0", "48d"},
			{"auth-service", "1/1", "1", "1", "ghcr.io/p10/auth:3.8.1", "121d"},
			{"billing-worker", "0/1", "1", "0", "ghcr.io/p10/billing:0.9.4", "12d"},
			{"notify-consumer", "1/1", "1", "1", "ghcr.io/p10/notify:2.2.0", "34d"},
			{"payment-api", "1/2", "2", "1", "ghcr.io/p10/payment:5.1.7", "77d"},
			{"search-indexer", "1/1", "1", "1", "ghcr.io/p10/indexer:1.0.9", "21d"},
			{"web-frontend", "3/3", "3", "3", "ghcr.io/p10/web:2026.8.3", "9d"},
		},
		Extra: []nsRow{
			{"kube-system", []string{"coredns", "2/2", "2", "2", "registry.k8s.io/coredns/coredns:v1.11.3", "128d"}},
			{"monitoring", []string{"grafana", "1/1", "1", "1", "grafana/grafana:11.2.0", "62d"}},
			{"staging", []string{"checkout-api", "1/2", "2", "1", "ghcr.io/p10/checkout-api:2.3.0", "5d"}},
		},
	},
	{
		Kind: domain.Kind{Key: "statefulsets", Name: "StatefulSets", Short: "sts", Group: "Workloads", Namespaced: true,
			Cols: []string{"NAME", "READY", "IMAGE", "AGE"}, Allowed: wlActions},
		Rows: [][]string{
			{"cache-redis", "2/2", "redis:7.4-alpine", "62d"},
			{"kafka", "3/3", "bitnami/kafka:3.9.0", "180d"},
			{"postgres", "1/1", "postgres:16.4", "180d"},
		},
	},
	{
		Kind: domain.Kind{Key: "daemonsets", Name: "DaemonSets", Short: "ds", Group: "Workloads", Namespaced: true,
			Cols: []string{"NAME", "DESIRED", "READY", "AGE"}, Allowed: wlActions},
		Rows: [][]string{
			{"fluent-bit", "3", "3", "128d"},
			{"node-exporter", "3", "3", "128d"},
		},
	},
	{
		Kind: domain.Kind{Key: "jobs", Name: "Jobs", Short: "job", Group: "Workloads", Namespaced: true,
			Cols: []string{"NAME", "COMPLETIONS", "DURATION", "AGE"}, Allowed: []string{domain.ADescribe, domain.AYAML, domain.ALogs, domain.ADelete}},
		Rows: [][]string{
			{"migrate-db-29341", "1/1", "42s", "51m"},
			{"report-nightly-29338", "1/1", "6m12s", "1d"},
		},
	},
	{
		Kind: domain.Kind{Key: "cronjobs", Name: "CronJobs", Short: "cj", Group: "Workloads", Namespaced: true,
			Cols: []string{"NAME", "SCHEDULE", "SUSPEND", "LAST SCHEDULE", "AGE"}, Allowed: basicActions},
		Rows: [][]string{
			{"report-nightly", "0 18 * * *", "False", "7h", "90d"},
			{"cleanup-tmp", "*/30 * * * *", "False", "12m", "90d"},
		},
	},
	{
		Kind: domain.Kind{Key: "services", Name: "Services", Short: "svc", Group: "Network", Namespaced: true,
			Cols: []string{"NAME", "TYPE", "CLUSTER-IP", "PORTS", "AGE"}, Allowed: []string{domain.ADescribe, domain.AYAML, domain.APortFwd, domain.AEdit, domain.ADelete}},
		Rows: [][]string{
			{"api-gateway", "ClusterIP", "10.96.31.10", "80/TCP", "48d"},
			{"cache-redis", "Headless", "None", "6379/TCP", "62d"},
			{"payment-api", "ClusterIP", "10.96.14.77", "8080/TCP", "77d"},
			{"web-frontend", "LoadBalancer", "10.96.4.201", "80:31380/TCP", "9d"},
		},
		Extra: []nsRow{
			{"kube-system", []string{"kube-dns", "ClusterIP", "10.96.0.10", "53/UDP,53/TCP", "128d"}},
			{"monitoring", []string{"grafana", "ClusterIP", "10.96.55.2", "80/TCP", "62d"}},
		},
	},
	{
		Kind: domain.Kind{Key: "ingresses", Name: "Ingresses", Short: "ing", Group: "Network", Namespaced: true,
			Cols: []string{"NAME", "CLASS", "HOSTS", "ADDRESS", "AGE"}, Allowed: basicActions},
		Rows: [][]string{
			{"web", "nginx", "app.p10node.onl", "10.0.9.4", "9d"},
			{"api", "nginx", "api.p10node.onl", "10.0.9.4", "48d"},
		},
	},
	{
		Kind: domain.Kind{Key: "configmaps", Name: "ConfigMaps", Short: "cm", Group: "Config", Namespaced: true,
			Cols: []string{"NAME", "DATA", "AGE"}, Allowed: basicActions},
		Rows: [][]string{
			{"api-gateway-config", "6", "48d"},
			{"feature-flags", "23", "12d"},
			{"kube-root-ca.crt", "1", "180d"},
		},
		Extra: []nsRow{
			{"kube-system", []string{"coredns", "1", "128d"}},
			{"kube-system", []string{"extension-apiserver-authentication", "6", "128d"}},
		},
	},
	{
		Kind: domain.Kind{Key: "secrets", Name: "Secrets", Short: "sec", Group: "Config", Namespaced: true,
			Cols: []string{"NAME", "TYPE", "DATA", "AGE"}, Allowed: basicActions},
		Rows: [][]string{
			{"db-credentials", "Opaque", "3", "180d"},
			{"ghcr-pull", "kubernetes.io/dockerconfigjson", "1", "180d"},
			{"tls-p10node", "kubernetes.io/tls", "2", "44d"},
		},
		Extra: []nsRow{
			{"kube-system", []string{"bootstrap-token-abc123", "bootstrap.kubernetes.io/token", "5", "128d"}},
		},
	},
	{
		Kind: domain.Kind{Key: "pvcs", Name: "PVCs", Short: "pvc", Group: "Storage", Namespaced: true,
			Cols: []string{"NAME", "STATUS", "CAPACITY", "STORAGECLASS", "AGE"}, Allowed: basicActions},
		Rows: [][]string{
			{"data-cache-redis-0", "Bound", "10Gi", "gp3", "62d"},
			{"data-cache-redis-1", "Bound", "10Gi", "gp3", "62d"},
			{"data-postgres-0", "Bound", "100Gi", "gp3", "180d"},
		},
	},
	{
		// Rows are built dynamically from clusterNodes by Source.Rows so
		// cordon state (SchedulingDisabled) reflects live toggles.
		Kind: domain.Kind{Key: "nodes", Name: "Nodes", Short: "no", Group: "Cluster", Namespaced: false,
			Cols: []string{"NAME", "STATUS", "ROLES", "VERSION", "CPU%", "MEM%", "AGE"}, Allowed: nodeActions},
	},
	{
		Kind: domain.Kind{Key: "namespaces", Name: "Namespaces", Short: "ns", Group: "Cluster", Namespaced: false,
			Cols: []string{"NAME", "STATUS", "PODS", "AGE"}, Allowed: []string{domain.ADescribe, domain.AYAML, domain.ADelete}},
		Rows: [][]string{
			{"default", "Active", "14", "180d"},
			{"kube-system", "Active", "22", "180d"},
			{"monitoring", "Active", "9", "128d"},
			{"staging", "Active", "31", "77d"},
			{"argocd", "Active", "6", "90d"},
			{"cert-manager", "Active", "3", "180d"},
		},
	},
	{
		Kind: domain.Kind{Key: "events", Name: "Events", Short: "ev", Group: "Cluster", Namespaced: true,
			Cols: []string{"TYPE", "REASON", "OBJECT", "MESSAGE", "AGE"}, Allowed: []string{domain.ADescribe}},
		Rows: [][]string{
			{"Warning", "BackOff", "pod/billing-worker-6f8d9c5b7-qq91x", "Back-off restarting failed container", "31s"},
			{"Warning", "FailedScheduling", "pod/payment-api-9d7c8f6b5-wr3nc", "0/3 nodes available: insufficient cpu", "4m"},
			{"Normal", "Pulled", "pod/web-frontend-6b8c7d9f5-c8trq", "Container image already present on machine", "1d"},
			{"Normal", "Scheduled", "pod/migrate-db-29341-8kdlp", "Successfully assigned default/migrate-db", "51m"},
		},
	},
	{
		Kind: domain.Kind{Key: "crds", Name: "CRDs", Short: "crd", Group: "Custom Resources", Namespaced: false,
			Cols: []string{"NAME", "GROUP", "VERSION", "SCOPE", "KIND", "AGE"}, Allowed: basicActions},
		Rows: [][]string{
			{"certificates.cert-manager.io", "cert-manager.io", "v1", "Namespaced", "Certificate", "180d"},
			{"clusterissuers.cert-manager.io", "cert-manager.io", "v1", "Cluster", "ClusterIssuer", "180d"},
			{"applications.argoproj.io", "argoproj.io", "v1alpha1", "Namespaced", "Application", "90d"},
			{"appprojects.argoproj.io", "argoproj.io", "v1alpha1", "Namespaced", "AppProject", "90d"},
			{"prometheuses.monitoring.coreos.com", "monitoring.coreos.com", "v1", "Namespaced", "Prometheus", "128d"},
			{"servicemonitors.monitoring.coreos.com", "monitoring.coreos.com", "v1", "Namespaced", "ServiceMonitor", "128d"},
		},
	},
	{
		// Instances of the CRDs above. Nothing lives in "default" here (real
		// clusters rarely put operator-managed CRs there), so /ns default
		// shows zero rows — switch to /ns all or a specific namespace
		// (argocd, cert-manager, monitoring) to see them via Extra.
		Kind: domain.Kind{Key: "customresources", Name: "Custom Resources", Short: "cr", Group: "Custom Resources", Namespaced: true,
			Cols: []string{"NAME", "KIND", "AGE"}, Allowed: []string{domain.ADescribe, domain.AYAML, domain.AEdit, domain.ADelete}},
		Extra: []nsRow{
			{"argocd", []string{"my-app", "Application", "40d"}},
			{"argocd", []string{"platform-svc", "Application", "120d"}},
			{"cert-manager", []string{"web-tls", "Certificate", "44d"}},
			{"cert-manager", []string{"api-tls", "Certificate", "12d"}},
			{"monitoring", []string{"k8s-prometheus", "Prometheus", "128d"}},
			{"-", []string{"letsencrypt-prod", "ClusterIssuer", "180d"}},
		},
	},
}

func findResource(key string) *resourceDef {
	for i := range resources {
		if resources[i].Key == key {
			return &resources[i]
		}
	}
	return nil
}

// visible returns the columns and rows for r as seen under namespace ns.
func visible(r *resourceDef, ns string) (cols []string, rows [][]string) {
	if !r.Namespaced {
		return r.Cols, r.Rows
	}
	switch ns {
	case "", "default":
		return r.Cols, r.Rows
	case domain.AllNamespaces:
		cols = withNamespaceCol(r.Cols)
		for _, row := range r.Rows {
			rows = append(rows, prependCol("default", row))
		}
		for _, e := range r.Extra {
			rows = append(rows, prependCol(e.NS, e.Row))
		}
		return cols, rows
	default:
		for _, e := range r.Extra {
			if e.NS == ns {
				rows = append(rows, e.Row)
			}
		}
		return r.Cols, rows
	}
}

func withNamespaceCol(cols []string) []string {
	if len(cols) > 0 && cols[0] == "NAMESPACE" {
		return cols
	}
	out := make([]string, 0, len(cols)+1)
	out = append(out, "NAMESPACE")
	return append(out, cols...)
}

func prependCol(v string, row []string) []string {
	out := make([]string, 0, len(row)+1)
	out = append(out, v)
	return append(out, row...)
}
