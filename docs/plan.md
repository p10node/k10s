# Execution plan

Backlog k10s ở dạng **task card cho worker agent**. Mỗi card tự đủ context:
một agent đọc card + section [Worker contract](#worker-contract) là đủ để làm,
không cần hỏi lại.

Dispatcher: chọn card → dán [prompt template](#prompt-template) với `<ID>` →
review diff → tick `[x]` ở [Board](#board).

---

## Worker contract

Đọc phần này **trước** khi sửa bất kỳ file nào. Đây là các invariant của repo,
phá là CI đỏ hoặc UI vỡ âm thầm.

### Kiến trúc

- `internal/domain` là boundary duy nhất giữa UI và backend.
  `internal/ui` **không được** import `internal/k8s` hay `internal/mock`.
- **Không thêm method mới vào `domain.Source`.** Interface đã 25 method và có
  4 impl (`k8s.Store`, `mock.Source`, các stub trong `internal/ui/*_test.go`,
  `connect_test.go:pendingSource`). Capability mới đi bằng **optional
  interface**:

  ```go
  // internal/domain/domain.go
  //
  // Containers lists the containers of a pod-bearing object. Backends that
  // cannot answer simply do not implement it; the UI then hides the picker.
  type Containers interface {
      Containers(kind, ns, name string) ([]string, error)
  }
  ```

  UI dùng type-assert, degrade im lặng khi thiếu:

  ```go
  if c, ok := m.src.(domain.Containers); ok { … }
  ```

  Cả `internal/k8s` **và** `internal/mock` phải impl (nếu không thì demo backend
  và `just shot` không thấy feature → không review được UI).

### Performance (có test gác, hỏng = regression thật)

- Render path **không được I/O**. Guard: `TestViewDoesNotBuildRows`,
  `TestKeypressLatency` trong `internal/ui`.
- `RowCount` **không format row, không mở watch**. Guard:
  `TestRowCount`, `TestNewStoreReturnsFast`.
- Informer là lazy per-kind. Startup đăng ký **zero** watch. Không thêm gì vào
  đường khởi động làm nó gọi API server.
- Đọc `docs/performance.md` trước khi chạm `internal/k8s/store.go` hoặc
  `counts.go`.

### Render

- **Mọi dòng render ra phải đúng bằng width terminal.** Overlay/join dựa vào đó.
  Guard: `internal/ui/block_test.go`.
- Không nest style trong `Render` của style khác (reset bên trong nuốt
  background bên ngoài) → dùng `padBG` + sibling run.
- Verify UI không cần TTY: `just shot 140 44 "j,j,d"`.

### Config

`internal/config/config.go` dùng **parser YAML tự viết**, phẳng, chỉ 1 cấp
nesting, **không hỗ trợ list-of-map**. Thêm field = sửa **cả** `render()` **và**
`parse()` **và** thêm case test. Cần cấu trúc phức tạp hơn (ví dụ saved views)
→ file riêng trong `~/.k10s/`, đừng nhồi vào `config.yaml`.

### Thêm resource kind

Theo đúng 5 bước ở `docs/dev.md` § "Adding a resource kind". Bỏ bước 5
(mirror vào `internal/mock/data.go`) là demo backend lệch với live.

### Definition of done

Card chỉ xong khi **tất cả** đúng:

1. `just check` xanh (= `fmt-check` + `vet` + `test`).
2. Có test mới cover đúng hành vi card mô tả. Backend test dùng fake clientset
   (`newTestStore`), nhớ `syncKinds(t, s, kPods, …)` trước khi assert rows.
3. Feature nhìn thấy được trên demo backend → dán output `just shot` vào PR.
4. Docs cập nhật: `docs/keybindings.md` nếu thêm phím, `docs/commands.md` nếu
   thêm command, `docs/config.md` nếu thêm config key, README bảng feature nếu
   user thấy được.
5. Tick card này trong `docs/plan.md` § Board.
6. Không đụng file ngoài "Files" của card. Thấy cần → ghi vào PR, đừng tự mở
   rộng scope.
7. **Single-card mode** (một agent, một card): không commit, để diff ở working
   tree. **Lane mode** (5 agent song song): một commit một card trên branch của
   lane, không push — xem § Dispatch protocol.

### Không làm

- Không đổi `domain.Source` signature.
- Không thêm dependency mới nếu stdlib/dep sẵn có làm được. `go.mod` hiện chỉ
  có bubbletea/lipgloss/bubblezone/vt10x/client-go.
- Không refactor "tiện tay".
- Không commit, không push, không tag. Để diff ở working tree.

---

## Board

### P0 — trust & correctness

- [ ] **T01** kind-cluster e2e trong CI
- [ ] **T02** container picker cho logs / shell / top
- [ ] **T03** port-forward chọn container + port
- [ ] **T04** events của object đang chọn
- [ ] **T05** CLI flags
- [ ] **T06** read-only mode + prod context guard

### P1 — daily driver

- [ ] **T07** sort theo cột
- [ ] **T08** multi-select + bulk action
- [ ] **T09** log: grep / previous / timestamps / save
- [ ] **T10** port-forward manager
- [ ] **T11** secret decode
- [ ] **T12** saved views
- [ ] **T13** label / field selector

### P2 — differentiator

- [ ] **T14** owner tree (xray)
- [ ] **T15** AI v2 — stream + auto-context + redact
- [ ] **T16** pulse dashboard
- [ ] **T17** `can-i` / RBAC introspect
- [ ] **T18** diff trước khi apply
- [ ] **T19** helm releases
- [ ] **T20** ephemeral debug container + node shell

### P3 — security / polish / distribution

- [ ] **T21** cosign verify cho self-update
- [ ] **T22** API key vào OS keychain
- [ ] **T23** custom keybindings + custom columns
- [ ] **T24** export CSV/JSON + clipboard OSC 52
- [ ] **T25** packaging: brew / scoop / nix
- [ ] **T26** kinds còn thiếu

### Lanes

Năm lane, mỗi lane một agent, mỗi lane sở hữu một vùng file. Trong lane chạy
**tuần tự** (deps); giữa các lane chạy **song song**.

| Lane | Sở hữu | Thứ tự card |
|---|---|---|
| **A** cluster backend | `internal/k8s/`, `internal/mock/`, `.github/workflows/e2e.yml` | T01 → T13 → T26 → T03 → T10 → T17 → T19 |
| **B** table & render | đường vẽ table trong `internal/ui/view.go` | T07 → T08 → T18 → T24 |
| **C** streams | logs / exec / shell / port-forward stream | T02 → T09 → T20 |
| **D** shell, config, release | `main.go`, `internal/config/`, `internal/update/` | T05 → T06 → T22 → T12 → T23 → T21 → T25 |
| **E** view mới & AI | file mới (`pulse.go`, `treeview.go`), `internal/ai/` | T04 → T11 → T16 → T14 → T15 |

Deps cắt ngang lane — có ba, và cả ba đều nằm **cuối** lane cần chúng:

```
C:T02 (container picker)  →  A:T03, A:T10
B:T07 (sort state)        →  D:T12, D:T23
D:T06 (cơ chế ẩn action)  →  A:T17
```

Card bị chặn mà dep chưa merge → **bỏ qua, làm card kế tiếp, báo cáo là hoãn**.
Không tự implement dep của lane khác.

### Dispatch protocol

Chạy 5 agent song song trên cùng một repo sẽ đụng nhau ở `model.go`,
`domain.go`, `actions.go`. Luật để việc đó không thành hỗn loạn:

1. **Mỗi lane một branch**: `lane/a-backend`, `lane/b-table`, `lane/c-streams`,
   `lane/d-shell`, `lane/e-views`. Tạo từ `main`.
2. **Một commit một card**, message `T07: sort theo cột`. Không push, không tag.
   (Đây là chỗ lane mode khác single-card mode ở § Definition of done.)
3. **Rebase lên `main` trước mỗi card mới.** Card sau của lane phải đứng trên
   thứ các lane khác vừa merge.
4. **File dùng chung — chỉ được thêm, không được sắp xếp lại:**
   - `internal/domain/domain.go`: interface mới **append cuối file**, một block
     một card, kèm comment card ID. Không sửa `Source`.
   - `internal/ui/model.go`: field mới **append cuối struct `Model`**, một block
     một card. Không đổi thứ tự field cũ.
   - `internal/ui/actions.go`: append vào cuối slice `Actions`.
   - `internal/mock/`: append. Không sửa row đã có (test khác đang assert nó).
   - Không `go mod tidy`, không đổi `go.mod`, trừ khi card nói rõ.
5. **Thứ tự merge**: `C → B → D → A → E`. Đây là thứ tự tô-pô của ba dep cắt
   ngang ở trên — A merge sau D vì `A:T17` cần `D:T06`.
6. Lane nào đụng file ngoài vùng sở hữu của mình → dừng, báo dispatcher.

---

# P0

## T01 — kind-cluster e2e trong CI

**Effort** M · **Deps** none · **Lane** A

**Goal** — CI chạy k10s thật với `kind` cluster, không chỉ fake clientset.

**Why** — `docs/roadmap.md` § Known limits tự nhận: "Untested against a real
cluster by its author". Toàn bộ live path (`internal/k8s`) chỉ được test bằng
`k8s.io/client-go/kubernetes/fake`, thứ không bắt được RBAC, discovery,
API version drift, SPDY, hay exec. Đây là rủi ro số 1 của repo — mọi card sau
đều đứng trên giả định "live path chạy được".

**Files**

- `.github/workflows/e2e.yml` (mới)
- `internal/k8s/e2e_test.go` (mới, build tag `//go:build e2e`)
- `Justfile` — thêm recipe `test-e2e`
- `docs/dev.md` § Tests

**Design**

- `helm/kind-action@v1` dựng cluster, apply một manifest fixture
  (`internal/k8s/testdata/e2e.yaml`: 1 Deployment 2 replica, 1 Service,
  1 ConfigMap, 1 Secret, 1 Job, 1 CrashLoopBackOff pod).
- Test build tag `e2e` → `just test` thường **không** chạy nó, CI gọi
  `go test -tags e2e ./internal/k8s/...`.
- Cover, mỗi cái một test: `NewStore` connect thật · `Rows` cho ≥8 kind ·
  `RowCount` · `Describe` · `YAML` · `LogsTail` · `LogsFollow` (nhận ≥1 dòng
  rồi `stop()`) · `Scale` · `Restart` · `Delete` · `PortForward` (dial được
  localAddr rồi stop) · `ShellSession` (chạy `echo hi`, đọc lại được).
- Metrics-server không có trong kind mặc định → `TopPod`/`TopNode` phải fail
  **gracefully**, assert đúng error chứ không panic.

**Accept**

- [ ] `just test` (không tag) vẫn không cần cluster, thời gian không đổi.
- [ ] `go test -tags e2e ./internal/k8s/...` xanh trên kind.
- [ ] Workflow chạy trên push `main` + PR, matrix ≥2 k8s version
      (oldest supported + latest).
- [ ] Bất kỳ path nào panic với real API server → test đỏ, không phải TUI chết.
- [ ] `docs/roadmap.md` § Known limits xoá dòng "Untested against a real cluster".

**Not** — không đổi production code trừ khi e2e phát hiện bug thật; bug tìm
được thì ghi riêng thành card mới.

---

## T02 — container picker cho logs / shell / top

**Effort** M · **Deps** none · **Lane** C

**Goal** — pod nhiều container: người dùng chọn container, không bị ép
`Containers[0]`.

**Why** — `internal/k8s/logs.go:18 podContainer()` luôn trả
`p.Spec.Containers[0].Name`. Với istio-proxy / vault-agent / linkerd sidecar,
`Containers[0]` thường **không** phải app → user đọc log sai container mà không
hề biết. Đây là bug thầm lặng, tệ hơn crash.

**Files**

- `internal/domain/domain.go` — optional interface `Containers`
- `internal/k8s/logs.go`, `internal/k8s/shell.go`, `internal/k8s/exec.go`,
  `internal/k8s/metrics_top.go`
- `internal/mock/source.go`, `internal/mock/data.go`
- `internal/ui/pickers.go`, `internal/ui/pickers_view.go`, `internal/ui/model.go`
- `docs/keybindings.md`

**Design**

```go
type Containers interface {
    // Containers lists container names of the pod this object resolves to,
    // init and ephemeral containers included, app containers first.
    Containers(kind, ns, name string) ([]string, error)
}
type LogsInContainer interface {
    LogsTailIn(kind, ns, name, container string, n int) ([]string, bool, error)
    LogsFollowIn(kind, ns, name, container string) (<-chan string, func(), error)
}
```

- 1 container → **không** hiện picker, hành vi y như cũ. Đây là case đa số,
  đừng bắt họ thêm một cú enter.
- ≥2 container → popup picker dùng lại pattern của `ctxpicker.go` (đã có filter,
  đã clickable). Container đã chọn nhớ theo `(ns,pod)` trong `Model`, không
  persist ra config.
- Tiêu đề log panel hiện `logs -f <pod> · <container>`.
- `c` cycle container ngay trong log view, không cần đóng mở lại.

**Accept**

- [ ] Pod 3 container → `l` mở picker, chọn container thứ 2 → log đúng của nó.
- [ ] Pod 1 container → `l` mở thẳng log, không popup.
- [ ] `s` (shell) và `m` (top) dùng chung container đã chọn.
- [ ] Backend không impl `Containers` → không có picker, không crash.
- [ ] Mock backend có pod đa container để `just shot` demo được.

**Tests** — `internal/k8s/logs_test.go`: pod 3 container, assert thứ tự và
`LogsTailIn` gọi đúng container. `internal/ui`: assert picker chỉ mở khi ≥2.

**Not** — không làm multi-container tail gộp (`--all-containers`); card riêng.

---

## T03 — port-forward chọn container + port

**Effort** S · **Deps** T02 (dùng lại picker) · **Lane** A

**Goal** — forward được port bất kỳ, không chỉ port đầu tiên của container đầu.

**Why** — `internal/k8s/portforward.go:76` hardcode
`p.Spec.Containers[0].Ports[0].ContainerPort`, và bỏ qua pod nào
`Containers[0]` không khai báo port (dòng 73) — pod có sidecar mesh gần như
luôn rơi vào case này. Service multi-port cũng chỉ forward được port đầu.

**Files** — `internal/k8s/portforward.go`, `internal/domain/domain.go`,
`internal/mock/source.go`, `internal/ui/pickers.go`

**Design**

- Optional interface `PortSource { Ports(kind, ns, name string) ([]Port, error) }`,
  `Port{Container, Name string; Number int32; Protocol string}`.
- Gom port của **mọi** container, không chỉ `[0]`.
- 1 port → forward luôn. ≥2 → picker, hiện `container · name · number`.
- Cho phép chỉ định local port: `p` = auto (:0), `shift+p` = hỏi local port.
- Service: resolve qua endpoints → pod, giữ nguyên hành vi hiện tại nhưng
  liệt kê đủ port của service.

**Accept**

- [ ] Pod mà `Containers[0]` không có port, `Containers[1]` có → forward được.
- [ ] Pod 3 port → picker, chọn cái nào forward đúng cái đó.
- [ ] Local port cụ thể bị chiếm → error rõ ràng, không treo.

---

## T04 — events của object đang chọn

**Effort** S · **Deps** none · **Lane** E

**Goal** — `shift+e` trên bất kỳ row nào → chỉ events của object đó, mới nhất trên.

**Why** — debug k8s thật sự bắt đầu ở events ("FailedScheduling",
"ImagePullBackOff", "OOMKilled"). Hiện `:ev` chỉ list toàn namespace, user phải
tự dò cột OBJECT. Backend đã có sẵn: `internal/k8s/rows.go:605` đã đọc
`e.InvolvedObject`, chỉ thiếu filter + đường vào UI.

**Files** — `internal/k8s/rows.go`, `internal/domain/domain.go`,
`internal/ui/actions.go`, `internal/ui/model.go`, `internal/mock/extra.go`,
`docs/keybindings.md`, README

**Design**

- Optional interface `ObjectEvents { EventsFor(kind, ns, name string) (cols []string, rows [][]string, err error) }`.
- Match theo `InvolvedObject.Kind` + `Name` (+ `UID` nếu có, để không dính pod
  cũ trùng tên).
- Deployment → gộp cả events của RS và Pod nó sở hữu. Đây là điểm khác biệt
  thật so với `kubectl describe`: lỗi của Deployment gần như luôn nằm ở Pod.
- Thêm action `{domain.AEvents, "E", "Events", "󰀦", false}`, cho **mọi** kind.
- Warning events tô màu `theme.Danger`; sort mới nhất trước.
- Rỗng → "no events for this object in the last hour" (events có TTL), không
  phải bảng trống.

**Accept**

- [ ] `shift+e` trên pod → chỉ events của pod đó.
- [ ] `shift+e` trên deployment → events của deploy + RS + pods.
- [ ] Object không có event → thông điệp giải thích TTL.
- [ ] Action hiện trong pane cho mọi kind.

---

## T05 — CLI flags

**Effort** S · **Deps** none · **Lane** D

**Goal** — `k10s -n kube-system --context prod po` mở thẳng đúng chỗ.

**Why** — `main.go:40` chỉ nhận `update` / `version` / `help`. Không script
được, không alias được, không dùng trong tmux layout được. Mọi TUI k8s đều có,
và đây là thứ chặn người dùng ngay trước khi họ thấy feature nào khác.

**Files** — `main.go`, `internal/ui/model.go` (`Startup` struct),
`docs/install.md`, README § Install

**Design**

Dùng `flag` stdlib, **không** thêm cobra.

```
k10s [flags] [resource]

  -n, --namespace   namespace mở lúc đầu ("all" cũng được)
      --context     kube context, ghi đè current-context
      --kubeconfig  đường dẫn kubeconfig
      --theme       theme cho phiên này, không ghi vào config
      --readonly    ẩn mọi action phá huỷ (xem T06)
  -h, --help  -v, --version
```

- Positional arg = alias resource, đi qua đúng `kindForAlias`
  (`internal/ui/commands.go:182`) mà `:po` dùng — một nguồn sự thật.
- Precedence: flag > env (`KUBECONFIG`) > config.yaml > kubeconfig
  current-context. Ghi bảng này vào `docs/config.md`.
- Flag **không** ghi đè config file. Nó là override cho phiên chạy.
- Alias sai → in ra list alias hợp lệ rồi `exit 2`, đừng mở TUI rồi mới báo.

**Accept**

- [ ] `k10s -n kube-system po` mở Pods ở kube-system.
- [ ] `k10s --context nonexistent` báo lỗi rõ, exit 1, không mở TUI.
- [ ] `k10s --theme dracula` không sửa `~/.k10s/config.yaml`.
- [ ] `k10s` trần vẫn y hệt hành vi cũ.
- [ ] `--help` liệt kê đủ flag.

---

## T06 — read-only mode + prod context guard

**Effort** M · **Deps** T05 · **Lane** D

**Goal** — không xoá nhầm production bằng một cú click.

**Why** — grep `readonly` trong `internal/` → 0 hit. k10s bán điểm mạnh là
"click được", nhưng `D` delete và `u` drain cũng click được, cách row bạn định
chọn đúng một pixel. Một TUI mouse-first mà không có phanh thì tai nạn chỉ là
vấn đề thời gian. k9s có `readOnly` từ lâu; đây là bảng cân đối cho tính năng
chủ đạo của repo.

**Files** — `internal/config/config.go`, `internal/ui/actions.go`,
`internal/ui/model.go`, `internal/ui/view.go`, `internal/plugin/plugin.go`,
`main.go`, `docs/config.md`, README

**Design**

Config mới (nhớ sửa **cả** `render()` và `parse()`):

```yaml
readonly: false
danger_contexts: "prod,production,*-prod"   # glob, khớp tên context
```

- Read-only → action `Risky` (`ADelete`, `ADrain`) và cả `AEdit`, `AScale`,
  `ARestart`, `ACordon` biến mất khỏi Actions pane, phím tương ứng thành no-op
  kèm toast "read-only mode". **Ẩn hẳn**, không phải hiện rồi báo lỗi — pane này
  là bản hợp đồng "đây là những gì bạn làm được".
- Plugin có `dangerous: true` cũng bị chặn. Plugin bypass được thì read-only
  chỉ là trang trí.
- Context khớp `danger_contexts` → banner đỏ liên tục trên header, và modal
  confirm của action phá huỷ bắt **gõ tên object** để xác nhận, không phải chỉ
  enter.
- `--readonly` bật cho phiên; `:ro` toggle trong phiên (chỉ khi config không
  ép `readonly: true`).

**Accept**

- [ ] `--readonly` → `D` không xoá, Actions pane không hiện Delete.
- [ ] Context `prod` → banner đỏ, delete bắt gõ tên.
- [ ] Plugin `dangerous: true` bị chặn ở read-only.
- [ ] Config `readonly: true` thì `:ro` không tắt được.
- [ ] `just shot` chụp được cả hai trạng thái.

---

# P1

## T07 — sort theo cột

**Effort** M · **Deps** none · **Lane** B

**Goal** — click header hoặc `shift+<n>` để sort, `RESTARTS` giảm dần là một
cú click.

**Why** — hiện chỉ có natural A→Z (`internal/k8s/rows.go:87 sortRows`).
Câu hỏi hay gặp nhất — "pod nào restart nhiều nhất", "node nào CPU cao nhất",
"gì vừa thay đổi" — đều là sort theo cột và đều không làm được.

**Files** — `internal/ui/view.go`, `internal/ui/model.go`, `internal/domain/domain.go`

**Design**

- Sort ở **UI**, không ở backend. Backend giữ nguyên order ổn định; UI sort bản
  đã filter. Sort ở backend sẽ đụng cache và `RowCount`.
- State: `sortCol int` (-1 = mặc định), `sortDesc bool`, nhớ **theo kind**
  (`map[string]sortState`) — sort của Pods không nên áp cho Services.
- Click lần 1 = asc, lần 2 = desc, lần 3 = về mặc định.
- Comparator tự nhận kiểu theo giá trị cột: số (`RESTARTS`, `CPU`), duration
  (`AGE`: `5d2h`), quantity (`100Mi`, `2Gi`), còn lại `domain.NaturalLess`.
  Viết thành `domain.CompareCell(a, b string) int` để test riêng được.
- Header cột đang sort hiện `▲`/`▼`, zone bubblezone cho từng header.
- Events giữ newest-first làm mặc định như hiện tại.

**Accept**

- [ ] Click `RESTARTS` 2 lần → nhiều nhất lên đầu.
- [ ] `AGE` sort theo thời gian thật, không theo chuỗi (`10m` < `2h` < `3d`).
- [ ] `2Gi` > `900Mi`.
- [ ] Đổi kind rồi quay lại → giữ sort của kind đó.
- [ ] `TestViewDoesNotBuildRows` vẫn xanh.

**Tests** — bảng test cho `CompareCell` phủ: số, duration, quantity, chuỗi,
ô rỗng, `<none>`.

---

## T08 — multi-select + bulk action

**Effort** M · **Deps** T07 · **Lane** B

**Goal** — `space` mark nhiều row, một lần `D` xử cả set.

**Why** — dọn 12 pod Evicted hiện là 12 lần `D` + 12 lần confirm.

**Files** — `internal/ui/model.go`, `internal/ui/view.go`, `internal/ui/actions.go`

**Design**

- `marked map[string]bool` khoá `ns/name`, **xoá sạch khi đổi kind hoặc
  namespace** — mark tàng hình xuyên view là cách xoá nhầm.
- `space` toggle, `ctrl+a`… đã dùng cho AI → dùng `*` để mark-all-filtered,
  `esc` clear.
- Row đã mark: gutter đổi màu + `▌` bên trái. Status bar hiện `3 marked`.
- Bulk chạy **tuần tự**, có progress, gặp lỗi thì tiếp tục phần còn lại và
  cuối cùng báo tổng kết "9 deleted, 3 failed" + list lỗi.
- Confirm modal liệt kê **đủ** tên (scroll được), không phải "3 objects".
- Read-only (T06) chặn bulk như chặn single.

**Accept**

- [ ] Mark 3 pod → `D` → 1 confirm, list đủ 3 tên, xoá cả 3.
- [ ] 1 trong 3 fail → 2 kia vẫn xoá, báo cáo chính xác.
- [ ] Đổi namespace → mark biến mất.
- [ ] Không mark gì → `D` vẫn xử row đang chọn như cũ.

---

## T09 — log: grep / previous / timestamps / save

**Effort** M · **Deps** T02 · **Lane** C

**Goal** — log view làm được 4 việc `kubectl logs` làm được mà nó chưa.

**Why** — `internal/ui/logview.go` chỉ có follow + scroll. Thiếu nhất là
`--previous`: pod CrashLoopBackOff thì log **hiện tại** rỗng, cái bạn cần nằm ở
container đã chết. Đúng lúc cần nhất thì công cụ không có.

**Files** — `internal/ui/logview.go`, `internal/ui/model.go`,
`internal/k8s/logs.go`, `internal/domain/domain.go`, `internal/mock/source.go`,
`docs/keybindings.md`

**Design**

Phím trong log view:

| phím | việc |
|---|---|
| `/` | grep: chỉ hiện dòng khớp, highlight, `n`/`N` nhảy, hoạt động cả khi đang follow |
| `p` | toggle `--previous` — container đã chết |
| `t` | toggle timestamps |
| `w` | ghi buffer ra `~/k10s-logs/<ns>-<pod>-<ts>.log`, toast đường dẫn |
| `c` | đổi container (T02) |

- Grep là **filter hiển thị**, không cắt buffer — tắt grep phải thấy lại đủ.
  Hỗ trợ regex, regex sai thì báo inline chứ không rơi về substring âm thầm.
- `--previous` mà không có container trước → nói thẳng "no previous container
  (pod has not restarted)", không phải panel trống.
- Đường dẫn file phải sanitize `ns`/`pod`.

**Accept**

- [ ] Grep đang follow: dòng mới không khớp không hiện, tắt grep là thấy lại.
- [ ] Pod restart → `p` ra log của lần chết trước.
- [ ] Pod chưa restart → `p` báo rõ.
- [ ] `w` ghi đúng file, kể cả 5000 dòng.
- [ ] Grep regex sai → báo lỗi inline, không crash.

---

## T10 — port-forward manager

**Effort** S · **Deps** T03 · **Lane** A

**Goal** — `:pf` liệt kê mọi forward đang chạy, stop từng cái hoặc stop hết.

**Why** — forward hiện sống trong pane; đóng pane hoặc đổi view là mất dấu.
Goroutine còn chạy, port còn giữ, không có đường nào nhìn thấy hay tắt trừ
thoát app.

**Files** — `internal/ui/model.go`, `internal/ui/commands.go`, `internal/ui/view.go`

**Design**

- Registry trong `Model`: `[]activeForward{kind, ns, name, container, local, remote, since, stop func()}`.
- View `:pf` là table thường (dùng lại render table sẵn có), action `D` = stop.
- Header hiện `⇄ 2` khi có forward đang chạy — trạng thái vô hình là trạng thái
  bị quên.
- Thoát app → stop hết trong cleanup.
- Forward chết ngoài ý muốn (pod bị xoá) → tự bỏ khỏi registry + toast.

**Accept**

- [ ] 2 forward → `:pf` hiện đủ 2 với local addr.
- [ ] `D` stop đúng cái đó, cái kia vẫn chạy.
- [ ] Xoá pod đang forward → biến khỏi list, có toast.
- [ ] Quit → không còn port nào bị giữ.

---

## T11 — secret decode

**Effort** S · **Deps** none · **Lane** E

**Goal** — `x` trên Secret → xem plaintext, không phải copy đi `base64 -d`.

**Why** — thao tác này ai cũng làm hàng ngày và hiện tại nó khó chịu vô lý.

**Files** — `internal/k8s/yaml.go`, `internal/domain/domain.go`,
`internal/mock/source.go`, `internal/ui/actions.go`, `docs/keybindings.md`

**Design**

- Optional interface `Decoder { Decoded(kind, ns, name string) (string, error) }`.
- Mặc định **che**: `api-key: ••••••••` — `x` một lần nữa mới hiện thật. Ai đó
  share màn hình là chuyện thường; mở ra là hiện secret thì tính năng này thành
  cái bẫy.
- Giá trị binary → hiện `<binary, 2048 bytes>`, không đổ byte rác ra terminal.
- Read-only mode (T06) **không** chặn — đây là đọc. Nhưng ghi log toast
  "revealed" để hành động có dấu vết trên màn hình.
- Copy mode (`ctrl+s`) vẫn dùng được để copy giá trị ra.

**Accept**

- [ ] `x` trên Secret → key + giá trị đã che.
- [ ] `x` lần 2 → giá trị thật.
- [ ] Secret binary (`dockerconfigjson`) → không phá layout.
- [ ] Kind khác Secret → không có action `x`.

---

## T12 — saved views

**Effort** M · **Deps** T07 · **Lane** D

**Goal** — `kind + ns + filter + sort` lưu thành tên, `1`–`9` nhảy tới.

**Why** — mỗi người có 3–5 chỗ hay xem ("pod lỗi ở prod", "ingress ở staging").
Hiện mỗi lần phải dựng lại bằng tay.

**Files** — `internal/config/` (file mới `views.go`), `internal/ui/model.go`,
`internal/ui/commands.go`, `docs/config.md`

**Design**

- **File riêng** `~/.k10s/views.yaml`. Parser trong `config.go` là flat, không
  đọc được list-of-map — đừng cố nhồi vào.
- Vẫn không thêm dep YAML: format tự định nghĩa, một view một khối, parser
  nhỏ + test.

  ```yaml
  views:
    - key: "1"
      name: "prod failures"
      kind: pods
      namespace: all
      filter: "Error|CrashLoop"
      sort: "RESTARTS:desc"
      context: prod
  ```

- `:save <name>` lưu view hiện tại vào slot trống tiếp theo; `:views` mở picker;
  `1`–`9` nhảy trực tiếp (chỉ khi table đang focus, không phải đang gõ).
- `context` optional: có thì switch context luôn.
- Kind/context trong file không còn tồn tại → bỏ qua view đó + cảnh báo, không
  làm hỏng startup.

**Accept**

- [ ] `:save prod-fail` → `~/.k10s/views.yaml` có entry, restart vẫn còn.
- [ ] `1` khôi phục đủ kind + ns + filter + sort.
- [ ] File hỏng → app vẫn khởi động, có cảnh báo.
- [ ] `1` khi đang gõ trong prompt thì gõ ra "1", không nhảy view.

---

## T13 — label / field selector

**Effort** S · **Deps** none · **Lane** A

**Goal** — `:po -l app=api` và `:po --field-selector status.phase=Running`.

**Why** — label selector là cách người ta thật sự nghĩ về workload. Filter chuỗi
hiện tại chỉ khớp text đang hiển thị, không đụng được tới label.

**Files** — `internal/ui/commands.go`, `internal/k8s/rows.go`,
`internal/domain/domain.go`, `internal/mock/source.go`

**Design**

- Optional interface `Selector { RowsSelected(kind, ns string, sel Selection) (cols []string, rows [][]string) }`,
  `Selection{Labels, Fields string}`.
- Label filter chạy **trên lister cache** bằng `labels.Parse` — không thêm API
  call, không phá guard performance.
- Field selector: chỉ hỗ trợ tập k8s thật sự hỗ trợ (`status.phase`,
  `spec.nodeName`, `metadata.name`). Field không hỗ trợ → báo rõ chứ đừng im
  lặng trả sai.
- Selector đang bật → hiện chip trong header, `esc` xoá.

**Accept**

- [ ] `:po -l app=api` chỉ ra pod có label đó.
- [ ] `-l 'env in (prod,staging)'` chạy đúng (`labels.Parse` lo).
- [ ] Selector sai cú pháp → lỗi rõ, không bảng trống bí ẩn.
- [ ] Không thêm request nào tới API server.

---

# P2

## T14 — owner tree (xray)

**Effort** L · **Deps** none · **Lane** E

**Goal** — `ctrl+r` mở cây `Deployment → ReplicaSet → Pod → Container`, click
được từng nút.

**Why** — đây là feature hợp gu k10s nhất: quan hệ sở hữu vốn là cây, mà bảng
phẳng thì che nó đi. "Deploy này đang chạy pod nào, và pod nào của RS cũ" là
câu hỏi thường xuyên mà hôm nay phải trả lời bằng cách đối chiếu 3 bảng.

**Files** — `internal/k8s/tree.go` (mới), `internal/domain/domain.go`,
`internal/mock/extra.go`, `internal/ui/treeview.go` (mới),
`internal/ui/model.go`, `internal/ui/view.go`

**Design**

```go
type Tree interface {
    Tree(kind, ns, name string) (*domain.Node, error)
}
type Node struct {
    Kind, Name, Status string
    Healthy            bool
    Children           []*Node
}
```

- Xuống: theo `OwnerReferences` ngược (`internal/k8s/actions.go` đã đọc
  ownerRef, dùng lại).
- Lên: từ pod đi ngược lên deploy — cùng một hàm, chỉ khác điểm bắt đầu.
- Service → endpoints → pods cũng là quan hệ đáng vẽ, dù không phải ownerRef.
- Nút không khoẻ tô `theme.Danger`; mặc định mở hết, `space` gập.
- `enter` trên nút = mở object đó trong main panel như bảng thường.
- Cây tính **on demand**, không tự refresh — không được đụng render path.

**Accept**

- [ ] `ctrl+r` trên Deployment → RS → Pods, số lượng khớp bảng.
- [ ] `ctrl+r` trên Pod → đi ngược lên tận Deployment.
- [ ] Pod CrashLoop → nút đỏ, cha cũng có dấu hiệu.
- [ ] Cây >200 nút vẫn render mượt.
- [ ] Không phá `TestKeypressLatency`.

---

## T15 — AI v2: stream + auto-context + redact

**Effort** L · **Deps** none · **Lane** E

**Goal** — AI từ "one-shot có biết tên pod" thành "thật sự đã xem object đó".

**Why** — hiện `internal/ai` chỉ nhét context/ns/kind/tên vào prompt rồi chờ
full response. Model không thấy describe, không thấy events, không thấy log —
tức là không thấy đúng những thứ trả lời được câu hỏi. Và prompt đang gửi đi
**chưa lọc secret**.

**Files** — `internal/ai/ai.go`, `internal/ai/redact.go` (mới),
`internal/ui/model.go`, `internal/ui/view.go`, `docs/commands.md`

**Design**

Bốn phần, làm được theo thứ tự này:

1. **Redact trước tiên** (bắt buộc, không optional). Trước khi gửi bất cứ gì:
   lọc giá trị Secret, `Authorization:` header, JWT (`eyJ…`), AWS key
   (`AKIA…`), private key block, `password=`/`token=`. Có test riêng cho từng
   pattern. Chưa xong bước này thì không merge 3 bước sau.
2. **Stream** — SSE cho cả OpenAI-compatible lẫn Anthropic, token hiện dần.
   `esc` huỷ giữa chừng.
3. **Auto-context** — `ctrl+a` khi đang chọn một object → gom
   `describe` + events (T04) + `logs --tail=100` + YAML đã rút gọn, kèm ngân
   sách token; vượt thì cắt log trước, YAML sau. Hiện cho user **chính xác**
   những gì sắp gửi + bytes, có nút huỷ. Không bao giờ gửi lén.
4. **Follow-up** — giữ history trong phiên, `ctrl+a` lần nữa là hỏi tiếp chứ
   không phải hỏi lại từ đầu. `:ai clear` xoá.

**Accept**

- [ ] Secret trong describe → không xuất hiện trong payload (test bằng
      httptest server, assert body).
- [ ] Response hiện dần, `esc` huỷ được.
- [ ] Preview context hiện đúng bytes trước khi gửi.
- [ ] Follow-up nhớ câu trước.
- [ ] Không có API key → AI mode tắt như cũ, không lỗi.

**Not** — chưa làm "apply patch AI đề xuất". Card riêng, sau T18.

---

## T16 — pulse dashboard

**Effort** M · **Deps** none · **Lane** E

**Goal** — mở k10s ra thấy **tình trạng cluster** ngay, không phải một bảng Pods.

**Why** — hôm nay màn hình đầu tiên là Pods của một namespace, thứ chưa trả lời
câu hỏi nào. Câu hỏi thật lúc mở app là "có gì cháy không".

**Files** — `internal/ui/pulse.go` (mới), `internal/ui/model.go`,
`internal/k8s/counts.go`, `internal/domain/domain.go`

**Design**

Ô, click được, mỗi ô nhảy tới view tương ứng:

```
nodes 4/4 ready · pods 128 (3 not ready, 1 crashloop)
restarts trong 1h: 7  · warning events: 12
pvc >80%: 2 · pod pending: 1 · job failed: 0
```

- Dữ liệu lấy từ **counts pass đã có** (`internal/k8s/counts.go`), không mở
  watch mới. Ràng buộc này không thương lượng — nó là lý do k10s khởi động
  nhanh.
- Refresh theo tick sẵn có, không tick riêng.
- Số nào chưa biết → hiện `—`, không hiện `0`. (Xem `domain.CountUnknown`.)
- Bật/tắt bằng `--home pulse|pods` (T05) + config `home:`.

**Accept**

- [ ] Startup vẫn 0 watch; `TestNewStoreReturnsFast` xanh.
- [ ] Click "3 not ready" → Pods đã filter sẵn.
- [ ] Cluster chưa sync → `—`, không phải `0`.
- [ ] `--home pods` giữ hành vi cũ.

---

## T17 — `can-i` / RBAC introspect

**Effort** M · **Deps** T06 · **Lane** A

**Goal** — action nào bạn không có quyền thì không hiện, thay vì click xong mới
ăn 403.

**Why** — Actions pane là lời hứa "đây là những gì bạn làm được với thứ này".
Với token bị giới hạn, lời hứa đó đang sai.

**Files** — `internal/k8s/access.go` (mới), `internal/domain/domain.go`,
`internal/ui/actions.go`, `internal/mock/extra.go`

**Design**

- `SelfSubjectAccessReview` cho `(verb, resource, ns)`.
- **Cache theo (verb, resource, ns)**, TTL 5 phút. Không được gọi SSAR trong
  render path — pane vẽ mỗi frame.
- Warm bất đồng bộ khi đổi kind/ns; chưa biết thì **hiện** action (fail-open).
  Fail-closed sẽ làm action nhấp nháy biến mất lúc mới mở, tệ hơn nhiều.
- `:cani <verb> <resource>` in kết quả trực tiếp.
- Bị từ chối → tooltip "forbidden: needs delete on pods".

**Accept**

- [ ] Token chỉ đọc → Delete/Edit/Scale không hiện.
- [ ] Không SSAR nào được gọi từ render path (test guard).
- [ ] Cache: đổi kind qua lại không tạo request mới trong TTL.
- [ ] SSAR lỗi → fail-open, action vẫn hiện.

---

## T18 — diff trước khi apply

**Effort** M · **Deps** none · **Lane** B

**Goal** — `e` edit → thấy diff server-side dry-run → mới confirm.

**Why** — `Apply` hiện ghi thẳng. Sửa YAML trong `$EDITOR` rồi ghi mù vào
cluster là chỗ dễ mất `resourceVersion`, dễ ghi đè thay đổi của người khác.

**Files** — `internal/k8s/actions.go`, `internal/domain/domain.go`,
`internal/ui/model.go`, `internal/ui/diffview.go` (mới)

**Design**

- Optional interface `Differ { Diff(kind, ns, name, yaml string) (string, error) }`
  dùng server-side apply `dryRun=All`, so với object hiện tại.
- Diff màu, `+`/`-`, chỉ hiện field đổi, bỏ qua `managedFields`/
  `resourceVersion`/`generation` — nhiễu che mất thứ thật.
- Không đổi gì → "no changes", không apply.
- Conflict (object đã đổi từ lúc mở editor) → báo, cho chọn reload hoặc force.

**Accept**

- [ ] Sửa replicas → diff chỉ hiện dòng đó.
- [ ] Không sửa gì → "no changes", không gọi apply.
- [ ] Object bị người khác đổi giữa chừng → cảnh báo conflict.
- [ ] Read-only → không vào được edit.

---

## T19 — helm releases

**Effort** L · **Deps** none · **Lane** A

**Goal** — kind mới `:helm`: list / history / values / rollback.

**Why** — phần lớn thứ trong cluster do helm cài. Xem được release mà không rời
k10s là khoảng cách rõ ràng với `kubectl`-only.

**Files** — `internal/k8s/helm.go` (mới), `internal/k8s/kinds.go`,
`internal/mock/data.go`, `internal/domain/domain.go`, `docs/commands.md`

**Design**

- **Không** import SDK `helm.sh/helm/v3` — nó kéo theo cả rừng dependency. Đọc
  thẳng release secret (`type=helm.sh/release.v1`), base64 + gzip + JSON. Format
  ổn định từ Helm 3.
- Cột: `NAME · NAMESPACE · REVISION · STATUS · CHART · APP VERSION · UPDATED`.
- Action: `y` values · `h` history · `R` rollback (risky, chịu T06) ·
  `d` describe.
- Rollback thì shell ra `helm` binary nếu có trên PATH; không có thì ẩn action
  và nói lý do. Tự cài đặt lại release bằng tay là sai.

**Accept**

- [ ] Release cài bằng helm → hiện đúng revision + status.
- [ ] `h` ra history đủ các revision.
- [ ] Không có helm binary → rollback ẩn, có giải thích.
- [ ] Cluster không có release nào → bảng rỗng, không lỗi.

---

## T20 — ephemeral debug container + node shell

**Effort** M · **Deps** T02 · **Lane** C

**Goal** — shell được vào pod distroless và vào node.

**Why** — `s` hôm nay hỏng với image distroless/scratch (không có shell) —
đúng loại image mà production hay dùng.

**Files** — `internal/k8s/exec.go`, `internal/k8s/shell.go`,
`internal/domain/domain.go`, `internal/ui/actions.go`

**Design**

- Pod: `shift+s` → ephemeral container (`kubectl debug` equivalent qua
  subresource `ephemeralcontainers`), image mặc định
  `busybox:1.36` đổi được qua config `debug_image`.
- Node: `s` trên Node → pod privileged `nsenter` trên node đó, tự dọn khi
  đóng session.
- Cả hai đều **risky** → confirm + chịu read-only (T06).
- Cluster không cho ephemeral container (feature gate tắt) → báo rõ, không
  treo.
- Pod debug tạo ra phải dọn kể cả khi k10s bị kill — đặt label
  `k10s.io/debug=true` và dọn ở startup lần sau.

**Accept**

- [ ] Pod distroless → `shift+s` vào được shell.
- [ ] `s` trên node → shell trên node.
- [ ] Đóng session → pod debug biến mất.
- [ ] k10s bị kill giữa chừng → lần chạy sau dọn nốt.
- [ ] Read-only → cả hai bị chặn.

---

# P3

## T21 — cosign verify cho self-update

**Effort** M · **Deps** none · **Lane** D

**Goal** — update kiểm chữ ký, không chỉ checksum.

**Why** — `docs/roadmap.md` tự ghi: checksum chứng minh file khớp với thứ
release công bố, **không** chứng minh ai công bố. k10s tự ghi đè binary đang
chạy — đây là đường tấn công đắt giá nhất trong repo. Nên làm **trước khi** có
người ngoài dùng, không phải sau.

**Files** — `internal/update/verify.go` (mới), `internal/update/update.go`,
`.github/workflows/release.yml`, `Justfile`, `docs/update.md`

**Design**

- Release ký bằng cosign keyless (OIDC GitHub Actions) → `.sig` + `.pem` cạnh
  archive.
- Verify **trong** binary, không shell ra `cosign`. Pin identity
  (`repo == p10node/k10s`, issuer GitHub).
- Không có chữ ký (release cũ) → cảnh báo rõ + hỏi, không im lặng cho qua và
  cũng không cấm cứng.
- `K10S_UPDATE_REPO` trỏ fork → identity đổi theo repo đó, và nói rõ cho user.

**Accept**

- [ ] Archive bị sửa → verify fail, không cài.
- [ ] Chữ ký của repo khác → fail.
- [ ] Release chưa ký → cảnh báo + hỏi.
- [ ] `RealDist` test vẫn xanh.

---

## T22 — API key vào OS keychain

**Effort** M · **Deps** none · **Lane** D

**Goal** — key AI không nằm plaintext trong `config.yaml`.

**Why** — roadmap đã liệt kê là known limit. `~/.k10s/config.yaml` hay bị
dotfile-sync lên git.

**Files** — `internal/config/secret_darwin.go`, `_linux.go`, `_windows.go`
(mới), `internal/config/config.go`, `internal/ui/settings.go`

**Design**

- Giữ nguyên field `AI.APIKey` trong struct — UI không đổi.
- Backend: macOS `security`, Linux `secret-tool` (libsecret), Windows
  credential manager. Không có → fallback plaintext như hiện tại **kèm cảnh báo
  hiện rõ trong `/settings`**.
- Migrate: key đang có trong file → chuyển vào keychain, ghi
  `api_key: "keychain"` vào file.

**Accept**

- [ ] macOS: key vào Keychain, file không còn plaintext.
- [ ] Không có keychain → vẫn chạy, có cảnh báo.
- [ ] Config cũ tự migrate một lần.
- [ ] Không thêm cgo dependency.

---

## T23 — custom keybindings + custom columns

**Effort** M · **Deps** T07 · **Lane** D

**Goal** — parity với `views.yaml` của k9s: đổi phím, đổi cột hiển thị.

**Files** — `internal/config/views.go`, `internal/ui/model.go`,
`internal/k8s/kinds.go`, `docs/config.md`

**Design** — `~/.k10s/views.yaml` (chung file với T12, section khác):
cột theo kind, cột từ label (`labels.app`), cột từ JSONPath đơn giản. Keybinding
map `action → key`, phát hiện trùng lúc load và báo, không im lặng lấy cái cuối.

**Accept** — [ ] đổi cột Pods thêm `labels.app` chạy được · [ ] phím trùng báo
lỗi rõ · [ ] file hỏng không chặn startup.

---

## T24 — export CSV/JSON + clipboard OSC 52

**Effort** S · **Deps** T07, T08 · **Lane** B

**Goal** — `ctrl+e` xuất bảng đang xem, `Y` copy YAML vào clipboard.

**Design** — export tôn trọng filter + sort + marked (T08). Clipboard dùng
**OSC 52** để copy được cả khi chạy qua ssh/tmux; fallback ghi file + toast.

**Accept** — [ ] CSV mở được bằng spreadsheet · [ ] copy qua ssh vào được
clipboard máy local · [ ] terminal không hỗ trợ OSC 52 → fallback file.

---

## T25 — packaging: brew / scoop / nix

**Effort** M · **Deps** T21 · **Lane** D

**Design** — homebrew tap `p10node/tap`, scoop bucket, nix flake; workflow
release tự bump. Giữ `install.sh` làm đường mặc định.

**Accept** — [ ] `brew install p10node/tap/k10s` chạy · [ ] tag mới tự bump
formula · [ ] self-update biết mình cài bằng package manager thì bảo user dùng
package manager, không tự ghi đè.

---

## T26 — kinds còn thiếu

**Effort** S · **Deps** none · **Lane** A

IngressClass · VPA · PriorityClass · MutatingWebhookConfiguration ·
ValidatingWebhookConfiguration · Lease · VolumeSnapshot · CSIDriver.

Theo đúng 5 bước `docs/dev.md` § "Adding a resource kind" cho từng cái. Mỗi kind
một commit riêng để review được.

**Accept** — [ ] mỗi kind có row formatter + mock mirror + test · [ ]
`RowCount` không mở watch · [ ] sidebar count đúng.

---

## Prompt template

Dán nguyên khối này cho worker agent, thay `<ID>`:

```
Bạn làm việc trên repo k10s (Kubernetes TUI, Go, bubbletea) tại thư mục hiện tại.

Nhiệm vụ: hoàn thành đúng task <ID> trong docs/plan.md.

Bắt buộc, theo thứ tự:

1. Đọc docs/plan.md — TOÀN BỘ section "Worker contract" và card <ID>.
   Worker contract là các invariant của repo; phá là CI đỏ hoặc UI vỡ âm thầm.
2. Đọc các file trong mục "Files" của card, cộng docs/architecture.md.
   Nếu card đụng internal/k8s hoặc render path, đọc thêm docs/performance.md.
3. Implement đúng phần "Design". Chỉ sửa file trong "Files" của card.
   Thấy cần file khác → làm xong rồi ghi vào báo cáo, đừng tự mở rộng scope.
4. Viết test cho từng gạch đầu dòng trong "Accept". Test backend dùng fake
   clientset qua newTestStore; nhớ syncKinds(t, s, kPods, …) trước khi assert
   rows.
5. Chạy `just check` (fmt-check + vet + test). Phải xanh.
6. Feature có thay đổi UI → chạy `just shot 140 44 "<keys>"` và dán frame vào
   báo cáo.
7. Cập nhật docs theo "Definition of done" mục 4.
8. Tick card <ID> trong docs/plan.md § Board.

Giới hạn cứng:
- KHÔNG thêm method vào domain.Source. Capability mới đi bằng optional
  interface + type-assert, cả internal/k8s và internal/mock đều impl.
- KHÔNG I/O trong render path. KHÔNG watch mới lúc startup.
- KHÔNG thêm dependency mới trừ khi card nói rõ.
- KHÔNG commit, KHÔNG push, KHÔNG tag. Để diff ở working tree.
- KHÔNG refactor ngoài phạm vi card.

Báo cáo cuối, đúng 5 mục:
- Đã làm gì (theo từng gạch đầu dòng Accept)
- File đã sửa + vì sao
- Test đã thêm + output `just check`
- Frame `just shot` nếu có đổi UI
- Điều gì lệch khỏi card, và lý do
```

## Lane prompt template

Cho lane mode. Thay `<LANE>`, `<TÊN LANE>`, `<VÙNG SỞ HỮU>`, `<DANH SÁCH CARD>`,
`<DEPS CẮT NGANG>`:

```
Bạn là worker agent của lane <LANE> (<TÊN LANE>) trên repo k10s
(Kubernetes TUI, Go, bubbletea) tại thư mục hiện tại.

Lane của bạn sở hữu: <VÙNG SỞ HỮU>
Bốn lane khác đang chạy song song trên vùng file khác. Ra khỏi vùng của mình
là gây conflict cho người khác.

SETUP (làm một lần):
  git checkout -b lane/<LANE>-<tên> main

Đọc trước khi sửa bất cứ gì:
  - docs/plan.md § "Worker contract" — TOÀN BỘ. Đây là invariant của repo.
  - docs/plan.md § "Dispatch protocol" — luật chống conflict giữa 5 lane.
  - docs/architecture.md

CARD, làm TUẦN TỰ đúng thứ tự này:
<DANH SÁCH CARD>

Với MỖI card, lặp lại đủ vòng:
  1. git rebase main   (lane khác có thể đã merge thứ bạn cần)
  2. Đọc card trong docs/plan.md: Goal, Why, Files, Design, Accept, Tests, Not.
     Card đụng internal/k8s hoặc render path → đọc thêm docs/performance.md.
  3. Implement đúng Design. Chỉ sửa file trong "Files" của card.
  4. Viết test cho TỪNG gạch đầu dòng trong "Accept". Test backend dùng fake
     clientset qua newTestStore; nhớ syncKinds(t, s, kPods, …) trước khi assert
     rows.
  5. `just check` phải xanh.
  6. Có đổi UI → `just shot 140 44 "<keys>"`, giữ frame cho báo cáo.
  7. Cập nhật docs theo Definition of done mục 4, tick card ở § Board.
  8. git commit -m "<ID>: <tiêu đề card>"   — KHÔNG push, KHÔNG tag.
  9. Sang card kế tiếp.

DEPS CẮT NGANG LANE:
<DEPS CẮT NGANG>
  Dep chưa có trên main → BỎ QUA card đó, làm card kế tiếp, ghi "hoãn" vào báo
  cáo. TUYỆT ĐỐI không tự implement card của lane khác.

GIỚI HẠN CỨNG:
  - KHÔNG thêm method vào domain.Source. Capability mới = optional interface
    + type-assert; cả internal/k8s và internal/mock đều impl.
  - File dùng chung (domain.go, model.go, actions.go, internal/mock/) CHỈ ĐƯỢC
    THÊM, append cuối, một block một card, kèm comment card ID. Không sắp xếp
    lại, không đổi thứ tự field cũ, không sửa row mock đã có.
  - KHÔNG I/O trong render path. KHÔNG watch mới lúc startup.
  - KHÔNG thêm dependency, KHÔNG go mod tidy, KHÔNG đổi go.mod.
  - KHÔNG push, KHÔNG tag, KHÔNG merge vào main.
  - KHÔNG refactor ngoài phạm vi card.
  - Đụng file ngoài vùng sở hữu của lane → DỪNG, báo dispatcher, đừng tự quyết.

BÁO CÁO CUỐI (sau khi hết card), mỗi card một mục:
  - Card ID + xong / hoãn (lý do)
  - Từng gạch Accept: đạt hay không
  - File đã sửa
  - Test đã thêm + output `just check`
  - Frame `just shot` nếu có đổi UI
  - Điều gì lệch khỏi card, và lý do
  Cuối báo cáo: tên branch + `git log --oneline main..HEAD`.
```
