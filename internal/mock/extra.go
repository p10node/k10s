package mock

import (
	"strconv"
	"strings"
)

// Contexts available for /context switching.
var Contexts = []string{
	"teleport.internal.s3.p10node.onl-S3",
	"eks-staging-apse1",
	"gke-prod-asia",
}

// SlashCommand describes one /command for the prompt suggestion popup.
type SlashCommand struct {
	Name string
	Args string
	Desc string
}

var SlashCommands = []SlashCommand{
	{"/context", "[name]", "switch kube context (no arg: cycle)"},
	{"/ns", "[name]", "switch namespace — try \"all\" (no arg: cycle)"},
	{"/theme", "[name]", "switch color theme (no arg: cycle)"},
	{"/config", "", "AI prompt settings (provider, url, model, key)"},
	{"/ai", "<prompt>", "ask AI once, regardless of prompt mode"},
	{"/search", "<term>", "filter the resource list (left pane)"},
	{"/filter", "<term>", "filter rows of the current table"},
	{"/crd", "", "jump to CustomResourceDefinitions"},
	{"/dr", "", "jump to Custom Resource instances"},
	{"/help", "", "keybindings and commands"},
}

// AIProvider presets for the settings modal.
type AIProvider struct {
	Label string
	URL   string
	Model string
}

var AIProviders = []AIProvider{
	{"OpenAI-compatible", "https://api.openai.com/v1", "gpt-5"},
	{"Anthropic", "https://api.anthropic.com/v1", "claude-sonnet-5"},
}

// AIAnswer returns a canned AI response for the mock.
func AIAnswer(q string) string {
	return `✦ ` + q + `

Looking at the current cluster state, two things stand out:

1. billing-worker is in CrashLoopBackOff (17 restarts, 3h)
   The container exits shortly after start. Typical causes here:
   • a failed DB migration — the migrate-db job ran 51m ago, the worker
     may be running against an incompatible schema
   • missing/rotated secret — it mounts 'db-credentials'
   Suggested checks:
     $ kubectl logs billing-worker-6f8d9c5b7-qq91x --previous
     $ kubectl describe pod billing-worker-6f8d9c5b7-qq91x

2. payment-api-9d7c8f6b5-wr3nc is Pending (4m)
   Scheduler reports: 0/3 nodes available, insufficient cpu.
   ip-10-0-2-88 is at 72% CPU and ip-10-0-3-51 is NotReady, so only
   the control-plane node has room but it may be tainted.
   Options:
   • lower the deployment's cpu request (currently likely 500m+)
   • fix ip-10-0-3-51 (NotReady for 17d — kubelet down?)
   • add a node

Run '/ai explain the NotReady node' to dig into item 2.

──
model: claude-sonnet-5 · mock response, no API call was made`
}

// nodeRow finds a row of the "nodes" resource by NAME (column 0), or nil.
func nodeRow(name string) []string {
	for i := range Resources {
		if Resources[i].Key != "nodes" {
			continue
		}
		for _, row := range Resources[i].Rows {
			if row[0] == name {
				return row
			}
		}
	}
	return nil
}

// NodeCordoned reports whether name currently shows SchedulingDisabled.
func NodeCordoned(name string) bool {
	row := nodeRow(name)
	return row != nil && strings.Contains(row[1], "SchedulingDisabled")
}

// SetCordon forces a node's STATUS to (not) carry SchedulingDisabled,
// mutating the "nodes" resource row in place (mock has no separate node
// object — the table row is the state).
func SetCordon(name string, disabled bool) {
	row := nodeRow(name)
	if row == nil {
		return
	}
	base := strings.TrimSuffix(row[1], ",SchedulingDisabled")
	if disabled {
		row[1] = base + ",SchedulingDisabled"
	} else {
		row[1] = base
	}
}

// ToggleCordon flips a node's schedulable state and returns the new
// "cordoned" value.
func ToggleCordon(name string) bool {
	disabled := !NodeCordoned(name)
	SetCordon(name, disabled)
	return disabled
}

// TopPod returns canned `kubectl top pod --containers` output.
func TopPod(name string) string {
	return `NAME                                    CPU(cores)   MEMORY(bytes)
` + fmtCol(name, 40) + `142m         310Mi

  CONTAINER   CPU(cores)   MEMORY(bytes)
  app         138m         298Mi
  istio-proxy 4m           12Mi

  requests    cpu: 100m (0.6%)   memory: 256Mi (0.4%)
  limits      cpu: 500m (3.1%)   memory: 512Mi (0.8%)`
}

// TopNode returns canned `kubectl top node` + capacity/allocatable output.
func TopNode(name string) string {
	var n *Node
	for i := range Cluster.Nodes {
		if Cluster.Nodes[i].Name == name {
			n = &Cluster.Nodes[i]
		}
	}
	cpu, mem := 38, 42
	cores, gib := nodeCores*cpu/100, nodeGiB*mem/100
	if n != nil {
		cpu, mem = n.CPU, n.Mem
		cores, gib = nodeCores*cpu/100, nodeGiB*mem/100
	}
	cordoned := "schedulable"
	if NodeCordoned(name) {
		cordoned = "SchedulingDisabled"
	}
	return `NAME                            CPU(cores)   CPU%   MEMORY(bytes)   MEMORY%
` + fmtCol(name, 32) + fmtCol(strconv.Itoa(cores)+"m", 13) + fmtCol(strconv.Itoa(cpu)+"%", 7) + fmtCol(strconv.Itoa(gib)+"Mi", 16) + strconv.Itoa(mem) + `%

  capacity     cpu: ` + strconv.Itoa(nodeCores) + `   memory: ` + strconv.Itoa(nodeGiB) + `Gi   pods: 110
  allocatable  cpu: ` + strconv.Itoa(nodeCores-1) + `900m memory: ` + strconv.Itoa(nodeGiB-2) + `Gi   pods: 110
  scheduling   ` + cordoned
}

const nodeCores, nodeGiB = 16, 64

func fmtCol(s string, w int) string {
	if len(s) >= w {
		return s + " "
	}
	return s + strings.Repeat(" ", w-len(s))
}

// Help returns the /help text view content.
func Help() string {
	return `k10s — keybindings

  NAVIGATION
    tab / shift+tab     cycle panes (resources → main → actions)
    ↑↓ / j k            move (j/k in main pane only)
    ← / →               jump to resource list / main pane
    pgup pgdn g G       page / first / last
    z                   zoom main pane · esc restores
    mouse               click anything: rows, actions, buttons, search

  RESOURCE LIST (left pane)
    type to search      any letters filter the list instantly
    esc                 clear search

  TABLE (main pane, table mode)
    /                   focus the table's own row-search box
    (while searching)   type to filter · ↑↓ move · enter/esc close

  ACTIONS (main pane focused)
    d describe · y yaml · l logs · s shell · f port-forward
    r rollout restart · c scale · e edit · m top (metrics)
    o cordon/uncordon · u drain (nodes only, confirm dialog)
    D delete (confirm dialog)

  PROMPT
    :                   open command prompt
    /                   open prompt with a slash command (or table search,
                        see above, when the main pane is focused)
    ctrl+a              toggle AI mode (✦) — plain text goes to the model
    enter / esc         run / close

  SLASH COMMANDS
    /context [name]     switch kube context
    /ns [name]          switch namespace — "all" shows every namespace
                        with a NAMESPACE column; no arg cycles
    /theme [name]       switch theme
    /config             AI settings: provider, base url, model, api key
    /ai <prompt>        one-shot AI question
    /search <term>      filter the resource list (left pane)
    /filter <term>      filter rows of the current table
    /crd                jump to CustomResourceDefinitions
    /dr                 jump to Custom Resource instances
    /help               this screen

  MISC
    T / ctrl+t          next / previous theme
    q (outside search)  quit · ctrl+c always quits`
}
