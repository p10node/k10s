package mock

import (
	"strconv"
	"strings"
)

const nodeCores, nodeGiB = 16, 64

// describeTpl returns fake `kubectl describe` output.
func describeTpl(name string) string {
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

// logsTpl returns fake container logs.
func logsTpl(name string) string {
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

// yamlTpl returns fake manifest output.
func yamlTpl(kind, name string) string {
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

// topPodTpl returns canned `kubectl top pod --containers` output.
func topPodTpl(name string) string {
	return `NAME                                    CPU(cores)   MEMORY(bytes)
` + fmtCol(name, 40) + `142m         310Mi

  CONTAINER   CPU(cores)   MEMORY(bytes)
  app         138m         298Mi
  istio-proxy 4m           12Mi

  requests    cpu: 100m (0.6%)   memory: 256Mi (0.4%)
  limits      cpu: 500m (3.1%)   memory: 512Mi (0.8%)`
}

// topNodeTpl returns canned `kubectl top node` + capacity/allocatable output.
func topNodeTpl(n *node, cordoned bool) string {
	cpu, mem := 38, 42
	if n != nil {
		cpu, mem = n.CPU, n.Mem
	}
	cores, gib := nodeCores*cpu/100, nodeGiB*mem/100
	sched := "schedulable"
	if cordoned {
		sched = "SchedulingDisabled"
	}
	name := "?"
	if n != nil {
		name = n.Name
	}
	return `NAME                            CPU(cores)   CPU%   MEMORY(bytes)   MEMORY%
` + fmtCol(name, 32) + fmtCol(strconv.Itoa(cores)+"m", 13) + fmtCol(strconv.Itoa(cpu)+"%", 7) + fmtCol(strconv.Itoa(gib)+"Mi", 16) + strconv.Itoa(mem) + `%

  capacity     cpu: ` + strconv.Itoa(nodeCores) + `   memory: ` + strconv.Itoa(nodeGiB) + `Gi   pods: 110
  allocatable  cpu: ` + strconv.Itoa(nodeCores-1) + `900m memory: ` + strconv.Itoa(nodeGiB-2) + `Gi   pods: 110
  scheduling   ` + sched
}

func fmtCol(s string, w int) string {
	if len(s) >= w {
		return s + " "
	}
	return s + strings.Repeat(" ", w-len(s))
}
