# Connecting k10s to a cluster

k10s does not create clusters and does not manage credentials. It reads the
same kubeconfig `kubectl` reads — `$KUBECONFIG` if set, otherwise
`~/.kube/config` — and shows whatever the current context can reach.

No kubeconfig, or a context whose API server does not answer, and k10s says
so:

```
╭─ No cluster ──────────────────────────────────────────────────╮
│   ⎈  No cluster                                               │
│                                                               │
│   invalid configuration: no configuration has been provided   │
│                                                               │
│   1  install kubectl       https://kubernetes.io/docs/…       │
│   2  get a kubeconfig      cloud CLI, or your cluster admin    │
│   3  or run one locally    kind · minikube · k3d              │
╰───────────────────────────────────────────────────────────────╯
```

It shows **no rows** in that state. Earlier versions fell back to the bundled
sample cluster here, which meant a laptop with no kubeconfig opened onto pods
that did not exist. The sample cluster is still there, but you have to ask
for it by name: `k10s demo` from a shell, `/demo` from inside, or the
`k10s-demo` entry in `:ctx`. Picking any other context leaves it, and the
header says `DEMO` for as long as it is up.

The same guide is inside the TUI as `/setup`, which is readable with nothing
connected.

This page links to the official documentation rather than restating it.
Installing `kubectl` and issuing credentials belong to the people who own
those tools, and a copy here would be the version that goes stale.

## 1 · Install kubectl

| Platform | Source |
|----------|--------------------------------------------------------------------|
| All      | <https://kubernetes.io/docs/tasks/tools/>                          |
| macOS    | <https://kubernetes.io/docs/tasks/tools/install-kubectl-macos/>    |
| Linux    | <https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/>    |
| Windows  | <https://kubernetes.io/docs/tasks/tools/install-kubectl-windows/>  |

Package managers are fine too — `brew install kubectl`, `winget install
Kubernetes.kubectl`, your distro's own package.

```bash
kubectl version --client     # proves the binary works
```

k10s does not shell out to `kubectl` for anything it displays (it speaks to
the API server directly through client-go), so this step is really about
having a working kubeconfig and a second way to check the connection.

## 2 · Get a kubeconfig

### A managed cluster writes the file for you

| Provider | Command | Docs |
|----------|---------|------|
| EKS | `aws eks update-kubeconfig --region <region> --name <cluster>` | [AWS](https://docs.aws.amazon.com/eks/latest/userguide/create-kubeconfig.html) |
| GKE | `gcloud container clusters get-credentials <cluster> --region <region>` | [Google](https://cloud.google.com/kubernetes-engine/docs/how-to/cluster-access-for-kubectl) |
| AKS | `az aks get-credentials --resource-group <rg> --name <cluster>` | [Microsoft](https://learn.microsoft.com/azure/aks/control-kubeconfig) |

Each of these merges a context into `~/.kube/config` and makes it current.
k10s picks it up on the next launch, or immediately via `:ctx`.

### A self-managed cluster

Ask whoever runs it for the file, then put it at `~/.kube/config` with mode
`600` — it holds credentials.

```bash
mkdir -p ~/.kube
install -m 600 /path/to/received-config ~/.kube/config
```

### Reference

- What the file is, and how one file holds several clusters:
  <https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/>
- Working with more than one cluster, including `$KUBECONFIG` with several
  paths:
  <https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/>

## 3 · Or run a cluster on this machine

Any of these gives you a real cluster to point k10s at, and all of them write
their context into `~/.kube/config` themselves.

| Tool | Install / quick start |
|------|-----------------------|
| kind | <https://kind.sigs.k8s.io/docs/user/quick-start/> |
| minikube | <https://minikube.sigs.k8s.io/docs/start/> |
| k3d | <https://k3d.io/stable/#installation> |
| Docker Desktop | enable Kubernetes in Settings → Kubernetes — <https://docs.docker.com/desktop/features/kubernetes/> |

```bash
kind create cluster            # or: minikube start
kubectl cluster-info
k10s
```

## 4 · Check it, then come back

```bash
kubectl config get-contexts      # what k10s lists under :ctx
kubectl config current-context   # what k10s opens on
kubectl cluster-info             # proves the API server actually answers
```

If `cluster-info` fails, k10s cannot connect either, and the fix is in the
cluster or the kubeconfig — not in k10s.

k10s always opens on kubeconfig's **current-context**, so
`kubectl config use-context <name>` is what changes where it starts next
time. `:ctx` inside k10s switches for the session only, deliberately: the
TUI and the `kubectl` in your other terminal should not disagree about which
cluster you are on without you saying so.

## Inside k10s

| | |
|---|---|
| `r` | retry the connection from the No cluster panel |
| `:ctx` | pick another context from kubeconfig |
| `/setup` | this guide, in a text view |
| `/demo` | the built-in sample cluster — fake data, clearly labelled |
| `k10s demo` | the same thing, opened straight from a shell |
| `:ctx` → any other context | leaves the demo — there is no separate exit |

## What counts as "no cluster"

Two different failures land on the same panel, both of them honest:

1. **kubeconfig will not load**, or names no context. client-go fails to
   build a client at all.
2. **The context's API server does not answer** — deleted cluster, VPN down,
   `minikube` not started. This one builds a perfectly healthy client: every
   client-go handle is lazy, and nothing dials until something is asked for.
   k10s catches it with a single version request at startup
   (`k8s.Client.Reachable`), which is also the request that fills in the
   `ver` field in the header.

A `401`/`403` is **not** treated as "no cluster": that is a live cluster
refusing your credentials, a different problem with a different fix, and
k10s connects anyway so you can see whatever you are allowed to see.

## Related

- [backends.md](backends.md) — the live backend, the demo backend, and what
  the UI does while a connection is in flight
- [config.md](config.md) — what k10s remembers between sessions
- [commands.md](commands.md) — `:ctx`, `/setup` and the rest of the prompt
