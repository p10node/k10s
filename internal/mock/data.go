package mock

import "slices"

// Node is one row of the top (borderless) cluster panel.
type Node struct {
	Name   string
	Status string
	Role   string
	Ver    string
	CPU    int // percent
	Mem    int // percent
	Age    string
}

// Cluster is the header context.
var Cluster = struct {
	Context   string
	Server    string
	Version   string
	Namespace string
	Nodes     []Node
}{
	Context:   "teleport.internal.s3.p10node.onl-S3",
	Server:    "https://k8s.p10node.onl:6443",
	Version:   "v1.31.4",
	Namespace: "default",
	Nodes: []Node{
		{"ip-10-0-1-14.ap-southeast-1", "Ready", "control-plane", "v1.31.4", 34, 61, "128d"},
		{"ip-10-0-2-88.ap-southeast-1", "Ready", "worker", "v1.31.4", 72, 55, "128d"},
		{"ip-10-0-3-51.ap-southeast-1", "NotReady", "worker", "v1.31.4", 9, 12, "17d"},
	},
}

// Action ids available in the right panel.
const (
	ADescribe = "describe"
	AYAML     = "yaml"
	ALogs     = "logs"
	AShell    = "shell"
	APortFwd  = "portfwd"
	ARestart  = "restart"
	AEdit     = "edit"
	AScale    = "scale"
	ATop      = "top"
	ACordon   = "cordon"
	ADrain    = "drain"
	ADelete   = "delete"
)

type Action struct {
	ID    string
	Key   string
	Label string
	Icon  string
	Risky bool
}

var Actions = []Action{
	{ADescribe, "d", "Describe", "󰈙", false},
	{AYAML, "y", "YAML", "󰈮", false},
	{ALogs, "l", "Logs", "󰑍", false},
	{AShell, "s", "Shell", "", false},
	{APortFwd, "f", "Port Forward", "󰛳", false},
	{ARestart, "r", "Rollout Restart", "󰑐", false},
	{AScale, "c", "Scale", "󰡎", false},
	{AEdit, "e", "Edit", "", false},
	{ATop, "m", "Top (metrics)", "󰓅", false},
	{ACordon, "o", "Cordon", "󰇙", false},
	{ADrain, "u", "Drain", "󰗇", true},
	{ADelete, "D", "Delete", "󰆴", true},
}

// AllNamespaces is the /ns sentinel that shows every namespace at once.
const AllNamespaces = "all"

// NSRow is a table row that only exists in a namespace other than "default"
// — used to populate the /ns all and /ns <other> views. Rows in a Resource's
// base Rows field are implicitly namespace "default".
type NSRow struct {
	NS  string
	Row []string
}

// Resource is one entry of the left panel plus its table payload.
type Resource struct {
	Key        string
	Name       string
	Short      string
	Group      string
	Namespaced bool
	Cols       []string
	Rows       [][]string
	Extra      []NSRow // rows living in a non-default namespace
	Allowed    []string
}

func (r Resource) Can(id string) bool {
	return slices.Contains(r.Allowed, id)
}

// Visible returns the columns and rows for r as seen under namespace ns.
// Cluster-scoped kinds ignore ns. For namespaced kinds: "" / "default" shows
// only the base Rows; AllNamespaces shows every row with a NAMESPACE column
// prepended; any other namespace shows only the matching Extra rows.
func Visible(r Resource, ns string) (cols []string, rows [][]string) {
	if !r.Namespaced {
		return r.Cols, r.Rows
	}
	switch ns {
	case "", "default":
		return r.Cols, r.Rows
	case AllNamespaces:
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

// VisibleCount is a cheap row count for the Resources-pane sidebar badges.
func VisibleCount(r Resource, ns string) int {
	_, rows := Visible(r, ns)
	return len(rows)
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

// NamespaceCycle lists every namespace known to the mock cluster, in the
// order /ns (no argument) cycles through, ending on AllNamespaces.
func NamespaceCycle() []string {
	var out []string
	for _, r := range Resources {
		if r.Key == "namespaces" {
			for _, row := range r.Rows {
				out = append(out, row[0])
			}
		}
	}
	return append(out, AllNamespaces)
}

var podActions = []string{ADescribe, AYAML, ALogs, AShell, APortFwd, ATop, AEdit, ADelete}
var wlActions = []string{ADescribe, AYAML, ALogs, ARestart, AScale, AEdit, ADelete}
var basicActions = []string{ADescribe, AYAML, AEdit, ADelete}
var nodeActions = []string{ADescribe, AYAML, ATop, ACordon, ADrain, AEdit}

var Resources = []Resource{
	{
		Key: "pods", Name: "Pods", Short: "po", Group: "Workloads", Namespaced: true,
		Cols:    []string{"NAME", "READY", "STATUS", "RESTARTS", "CPU", "MEM", "NODE", "AGE"},
		Allowed: podActions,
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
		Extra: []NSRow{
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
		Key: "deployments", Name: "Deployments", Short: "deploy", Group: "Workloads", Namespaced: true,
		Cols:    []string{"NAME", "READY", "UP-TO-DATE", "AVAILABLE", "IMAGE", "AGE"},
		Allowed: wlActions,
		Rows: [][]string{
			{"api-gateway", "2/2", "2", "2", "ghcr.io/p10/api-gateway:1.24.0", "48d"},
			{"auth-service", "1/1", "1", "1", "ghcr.io/p10/auth:3.8.1", "121d"},
			{"billing-worker", "0/1", "1", "0", "ghcr.io/p10/billing:0.9.4", "12d"},
			{"notify-consumer", "1/1", "1", "1", "ghcr.io/p10/notify:2.2.0", "34d"},
			{"payment-api", "1/2", "2", "1", "ghcr.io/p10/payment:5.1.7", "77d"},
			{"search-indexer", "1/1", "1", "1", "ghcr.io/p10/indexer:1.0.9", "21d"},
			{"web-frontend", "3/3", "3", "3", "ghcr.io/p10/web:2026.8.3", "9d"},
		},
		Extra: []NSRow{
			{"kube-system", []string{"coredns", "2/2", "2", "2", "registry.k8s.io/coredns/coredns:v1.11.3", "128d"}},
			{"monitoring", []string{"grafana", "1/1", "1", "1", "grafana/grafana:11.2.0", "62d"}},
			{"staging", []string{"checkout-api", "1/2", "2", "1", "ghcr.io/p10/checkout-api:2.3.0", "5d"}},
		},
	},
	{
		Key: "statefulsets", Name: "StatefulSets", Short: "sts", Group: "Workloads", Namespaced: true,
		Cols:    []string{"NAME", "READY", "IMAGE", "AGE"},
		Allowed: wlActions,
		Rows: [][]string{
			{"cache-redis", "2/2", "redis:7.4-alpine", "62d"},
			{"kafka", "3/3", "bitnami/kafka:3.9.0", "180d"},
			{"postgres", "1/1", "postgres:16.4", "180d"},
		},
	},
	{
		Key: "daemonsets", Name: "DaemonSets", Short: "ds", Group: "Workloads", Namespaced: true,
		Cols:    []string{"NAME", "DESIRED", "READY", "AGE"},
		Allowed: wlActions,
		Rows: [][]string{
			{"fluent-bit", "3", "3", "128d"},
			{"node-exporter", "3", "3", "128d"},
		},
	},
	{
		Key: "jobs", Name: "Jobs", Short: "job", Group: "Workloads", Namespaced: true,
		Cols:    []string{"NAME", "COMPLETIONS", "DURATION", "AGE"},
		Allowed: []string{ADescribe, AYAML, ALogs, ADelete},
		Rows: [][]string{
			{"migrate-db-29341", "1/1", "42s", "51m"},
			{"report-nightly-29338", "1/1", "6m12s", "1d"},
		},
	},
	{
		Key: "cronjobs", Name: "CronJobs", Short: "cj", Group: "Workloads", Namespaced: true,
		Cols:    []string{"NAME", "SCHEDULE", "SUSPEND", "LAST SCHEDULE", "AGE"},
		Allowed: basicActions,
		Rows: [][]string{
			{"report-nightly", "0 18 * * *", "False", "7h", "90d"},
			{"cleanup-tmp", "*/30 * * * *", "False", "12m", "90d"},
		},
	},
	{
		Key: "services", Name: "Services", Short: "svc", Group: "Network", Namespaced: true,
		Cols:    []string{"NAME", "TYPE", "CLUSTER-IP", "PORTS", "AGE"},
		Allowed: []string{ADescribe, AYAML, APortFwd, AEdit, ADelete},
		Rows: [][]string{
			{"api-gateway", "ClusterIP", "10.96.31.10", "80/TCP", "48d"},
			{"cache-redis", "Headless", "None", "6379/TCP", "62d"},
			{"payment-api", "ClusterIP", "10.96.14.77", "8080/TCP", "77d"},
			{"web-frontend", "LoadBalancer", "10.96.4.201", "80:31380/TCP", "9d"},
		},
		Extra: []NSRow{
			{"kube-system", []string{"kube-dns", "ClusterIP", "10.96.0.10", "53/UDP,53/TCP", "128d"}},
			{"monitoring", []string{"grafana", "ClusterIP", "10.96.55.2", "80/TCP", "62d"}},
		},
	},
	{
		Key: "ingresses", Name: "Ingresses", Short: "ing", Group: "Network", Namespaced: true,
		Cols:    []string{"NAME", "CLASS", "HOSTS", "ADDRESS", "AGE"},
		Allowed: basicActions,
		Rows: [][]string{
			{"web", "nginx", "app.p10node.onl", "10.0.9.4", "9d"},
			{"api", "nginx", "api.p10node.onl", "10.0.9.4", "48d"},
		},
	},
	{
		Key: "configmaps", Name: "ConfigMaps", Short: "cm", Group: "Config", Namespaced: true,
		Cols:    []string{"NAME", "DATA", "AGE"},
		Allowed: basicActions,
		Rows: [][]string{
			{"api-gateway-config", "6", "48d"},
			{"feature-flags", "23", "12d"},
			{"kube-root-ca.crt", "1", "180d"},
		},
		Extra: []NSRow{
			{"kube-system", []string{"coredns", "1", "128d"}},
			{"kube-system", []string{"extension-apiserver-authentication", "6", "128d"}},
		},
	},
	{
		Key: "secrets", Name: "Secrets", Short: "sec", Group: "Config", Namespaced: true,
		Cols:    []string{"NAME", "TYPE", "DATA", "AGE"},
		Allowed: basicActions,
		Rows: [][]string{
			{"db-credentials", "Opaque", "3", "180d"},
			{"ghcr-pull", "kubernetes.io/dockerconfigjson", "1", "180d"},
			{"tls-p10node", "kubernetes.io/tls", "2", "44d"},
		},
		Extra: []NSRow{
			{"kube-system", []string{"bootstrap-token-abc123", "bootstrap.kubernetes.io/token", "5", "128d"}},
		},
	},
	{
		Key: "pvcs", Name: "PVCs", Short: "pvc", Group: "Storage", Namespaced: true,
		Cols:    []string{"NAME", "STATUS", "CAPACITY", "STORAGECLASS", "AGE"},
		Allowed: basicActions,
		Rows: [][]string{
			{"data-cache-redis-0", "Bound", "10Gi", "gp3", "62d"},
			{"data-cache-redis-1", "Bound", "10Gi", "gp3", "62d"},
			{"data-postgres-0", "Bound", "100Gi", "gp3", "180d"},
		},
	},
	{
		Key: "nodes", Name: "Nodes", Short: "no", Group: "Cluster", Namespaced: false,
		Cols:    []string{"NAME", "STATUS", "ROLES", "VERSION", "CPU%", "MEM%", "AGE"},
		Allowed: nodeActions,
		Rows: [][]string{
			{"ip-10-0-1-14.ap-southeast-1", "Ready", "control-plane", "v1.31.4", "34%", "61%", "128d"},
			{"ip-10-0-2-88.ap-southeast-1", "Ready", "worker", "v1.31.4", "72%", "55%", "128d"},
			{"ip-10-0-3-51.ap-southeast-1", "NotReady", "worker", "v1.31.4", "9%", "12%", "17d"},
		},
	},
	{
		Key: "namespaces", Name: "Namespaces", Short: "ns", Group: "Cluster", Namespaced: false,
		Cols:    []string{"NAME", "STATUS", "PODS", "AGE"},
		Allowed: []string{ADescribe, AYAML, ADelete},
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
		Key: "events", Name: "Events", Short: "ev", Group: "Cluster", Namespaced: true,
		Cols:    []string{"TYPE", "REASON", "OBJECT", "MESSAGE", "AGE"},
		Allowed: []string{ADescribe},
		Rows: [][]string{
			{"Warning", "BackOff", "pod/billing-worker-6f8d9c5b7-qq91x", "Back-off restarting failed container", "31s"},
			{"Warning", "FailedScheduling", "pod/payment-api-9d7c8f6b5-wr3nc", "0/3 nodes available: insufficient cpu", "4m"},
			{"Normal", "Pulled", "pod/web-frontend-6b8c7d9f5-c8trq", "Container image already present on machine", "1d"},
			{"Normal", "Scheduled", "pod/migrate-db-29341-8kdlp", "Successfully assigned default/migrate-db", "51m"},
		},
	},
	{
		Key: "crds", Name: "CRDs", Short: "crd", Group: "Custom Resources", Namespaced: false,
		Cols:    []string{"NAME", "GROUP", "VERSION", "SCOPE", "KIND", "AGE"},
		Allowed: basicActions,
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
		Key: "customresources", Name: "Custom Resources", Short: "cr", Group: "Custom Resources", Namespaced: true,
		Cols:    []string{"NAME", "KIND", "AGE"},
		Allowed: []string{ADescribe, AYAML, AEdit, ADelete},
		Extra: []NSRow{
			{"argocd", []string{"my-app", "Application", "40d"}},
			{"argocd", []string{"platform-svc", "Application", "120d"}},
			{"cert-manager", []string{"web-tls", "Certificate", "44d"}},
			{"cert-manager", []string{"api-tls", "Certificate", "12d"}},
			{"monitoring", []string{"k8s-prometheus", "Prometheus", "128d"}},
			{"-", []string{"letsencrypt-prod", "ClusterIssuer", "180d"}},
		},
	},
}

// Describe returns fake `kubectl describe` output.
func Describe(kind, name string) string {
	return `Name:             ` + name + `
Namespace:        default
Priority:         0
Service Account:  default
Node:             ip-10-0-2-88.ap-southeast-1/10.0.2.88
Start Time:       Mon, 18 Aug 2026 09:14:22 +0700
Labels:           app=` + name + `
                  pod-template-hash=7d9f4c8b6d
Annotations:      kubectl.kubernetes.io/restartedAt: 2026-08-18T09:14:22+07:00
Status:           Running
IP:               10.244.2.117
IPs:
  IP:           10.244.2.117
Controlled By:  ReplicaSet/` + name + `

Containers:
  app:
    Container ID:   containerd://8f3c1a9e7b2d4f6a8c0e2b4d6f8a0c2e
    Image:          ghcr.io/p10/api-gateway:1.24.0
    Image ID:       ghcr.io/p10/api-gateway@sha256:9f2c...c41e
    Port:           8080/TCP
    Host Port:      0/TCP
    State:          Running
      Started:      Mon, 18 Aug 2026 09:14:31 +0700
    Ready:          True
    Restart Count:  0
    Limits:
      cpu:     500m
      memory:  512Mi
    Requests:
      cpu:     100m
      memory:  256Mi
    Liveness:   http-get http://:8080/healthz delay=10s timeout=1s period=10s
    Readiness:  http-get http://:8080/ready delay=5s timeout=1s period=5s
    Environment:
      LOG_LEVEL:     info
      DB_HOST:       postgres.default.svc.cluster.local
      REDIS_URL:     <set to the key 'url' in secret 'db-credentials'>
    Mounts:
      /etc/config from config (ro)
      /var/run/secrets/kubernetes.io/serviceaccount from kube-api-access (ro)

Conditions:
  Type                        Status
  PodReadyToStartContainers   True
  Initialized                 True
  Ready                       True
  ContainersReady             True
  PodScheduled                True

Volumes:
  config:
    Type:      ConfigMap (a volume populated by a ConfigMap)
    Name:      api-gateway-config
    Optional:  false

QoS Class:                   Burstable
Node-Selectors:              <none>
Tolerations:                 node.kubernetes.io/not-ready:NoExecute op=Exists for 300s
Events:
  Type    Reason     Age   From               Message
  ----    ------     ----  ----               -------
  Normal  Scheduled  6d    default-scheduler  Successfully assigned default/` + name + `
  Normal  Pulled     6d    kubelet            Container image already present on machine
  Normal  Created    6d    kubelet            Created container app
  Normal  Started    6d    kubelet            Started container app`
}

// Logs returns fake container logs.
func Logs(name string) string {
	return `2026-08-25T08:12:01.442Z INFO  server        listening addr=:8080 tls=false
2026-08-25T08:12:01.443Z INFO  db            connected dsn=postgres://***@postgres:5432/app pool=20
2026-08-25T08:12:01.451Z INFO  cache         connected addr=cache-redis:6379
2026-08-25T08:12:03.108Z INFO  http          GET  /healthz             200 0.4ms
2026-08-25T08:12:08.911Z INFO  http          POST /v1/payments         201 84.2ms trace=9f2c41ae
2026-08-25T08:12:09.220Z WARN  ratelimit     bucket near capacity key=tenant:4417 used=94%
2026-08-25T08:12:10.004Z INFO  http          GET  /v1/orders?limit=50  200 12.9ms trace=1b77c0de
2026-08-25T08:12:12.775Z ERROR upstream      dial tcp 10.96.14.77:8080: i/o timeout attempt=1/3
2026-08-25T08:12:13.780Z WARN  upstream      retrying request backoff=1s attempt=2/3
2026-08-25T08:12:14.902Z INFO  upstream      recovered latency=1.12s
2026-08-25T08:12:18.331Z INFO  http          GET  /v1/users/me         200 3.1ms trace=44b1e2f9
2026-08-25T08:12:21.660Z INFO  worker        flushed batch size=250 dur=41ms
2026-08-25T08:12:25.019Z INFO  http          GET  /healthz             200 0.3ms
2026-08-25T08:12:31.402Z INFO  http          DELETE /v1/sessions/9a1   204 5.7ms trace=7c0d19ba
2026-08-25T08:12:33.881Z WARN  gc            pause=18ms heap=412Mi
2026-08-25T08:12:40.117Z INFO  http          GET  /v1/orders/88213     200 7.4ms trace=e21f8b05
2026-08-25T08:12:44.590Z INFO  metrics       scrape ok series=1842
2026-08-25T08:12:51.008Z INFO  http          GET  /healthz             200 0.3ms`
}

// YAML returns fake manifest output.
func YAML(kind, name string) string {
	return `apiVersion: v1
kind: ` + kind + `
metadata:
  name: ` + name + `
  namespace: default
  labels:
    app: ` + name + `
    app.kubernetes.io/managed-by: argocd
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","kind":"` + kind + `"}
spec:
  containers:
    - name: app
      image: ghcr.io/p10/api-gateway:1.24.0
      ports:
        - containerPort: 8080
          protocol: TCP
      resources:
        limits:
          cpu: 500m
          memory: 512Mi
        requests:
          cpu: 100m
          memory: 256Mi
      env:
        - name: LOG_LEVEL
          value: info
        - name: DB_HOST
          value: postgres.default.svc.cluster.local
      volumeMounts:
        - name: config
          mountPath: /etc/config
          readOnly: true
  volumes:
    - name: config
      configMap:
        name: api-gateway-config
  restartPolicy: Always
  serviceAccountName: default
status:
  phase: Running
  podIP: 10.244.2.117
  qosClass: Burstable`
}
