# Miyabi 重构方案

- 日期：2026-09-19
- 基线：`master / 85228f7`
- 范围：四条工作线。① 全局代理；② 整体结构重构（含冗余清理）；③ 磁力聚合（JavDB + JavBus）；④ javdb-cli 接口补齐评估。
- 约束：前端所有样式与动效原样沿用，前端只做结构性调整；预览视频、字幕、DMM、第三方图库不在本轮范围；`AUDIT.md`、`AUDIT_RESPONSE.md` 及其临时实验产物随本轮一并删除，已经合并进代码的修复保留。

---

## 目录

1. 目标架构
2. 工作线 A：全局代理
3. 工作线 B：结构重构
4. 工作线 C：磁力聚合
5. 工作线 D：javdb-cli 接口评估
6. 执行顺序与里程碑
7. 验证与回归边界
8. 需要你确认的事项

---

## 1. 目标架构

### 1.1 包布局

```
cmd/miyabi/                 main.go 只解析参数和信号；装配移到 internal/app
internal/
  app/                      组合根：配置 → 存储 → 网络 → 服务 → 任务注册 → 路由
  config/                   环境变量 + 运行时常量（超时、限速、缓存尺寸、轮询间隔）
  domain/                   纯模型与错误，无外部依赖
  netx/                     代理管理器、HTTP 客户端工厂（resty / tls-client 两种）
  storage/                  ent 客户端、迁移、原生索引、settings 读写
  tasks/                    任务队列、worker pool、处理器注册表、SSE 总线、typed payload
  drive/                    115 账号与挂载目录状态、SourceSession（原 service/pan_*.go）
  pan/                      115 HTTP client（现有，基本不动）
  javdb/                    JavDB App API client（现有，返回 domain 类型）
  javbus/                   JavBus HTML client（新）
  catalogue/                目录服务：搜索/浏览/详情/标签/媒体，缓存与路由，本地状态投影
  magnet/                   磁力源接口、聚合器、质量推断
  library/                  影片索引、扫描、观看记录（原 library*.go、watch_history.go）
  library/scrape/           刮削、封面、sidecar、元数据快照（原 scrape.go、cover.go、metadata_snapshot.go）
  offline/                  离线下载
  monitor/                  新片监控
  playback/                 播放会话与 HLS 代理
  maintenance/              数据目录统计与缓存清理（原 data.go）
  api/                      gin 路由、bind/respond 助手、错误映射、DTO
  image/ nfo/ codeid/ logging/   不动
```

依赖方向自上而下单向：`api → 业务包 → drive/tasks/catalogue → pan/javdb/javbus/netx → domain`。业务包之间不得互相 import 具体类型，只能通过在 `domain` 或调用方定义的接口交互。

### 1.2 三个核心接口

```go
// drive.Session：一次业务操作期间对 115 的受控访问。签发时捕获账号、挂载目录、
// 授权版本；每次调用前后比对版本；Commit 在持有 drive 提交锁的前提下开事务并复查。
type Session interface {
    Source() domain.LibrarySource
    List(ctx context.Context, dirID string, page int) (pan.FilePage, error)
    Info(ctx context.Context, fileID string) (pan.FileInfo, error)
    Read(ctx context.Context, pickCode string, limit int64) ([]byte, error)
    Upload(ctx context.Context, dirID, name string, body []byte) error
    Commit(ctx context.Context, fn func(tx *ent.Tx) error) error
}

// tasks.Handler：每种任务由所属包实现并注册，队列不再知道任何领域规则。
type Handler interface {
    Kind() tasks.Kind
    Handle(ctx context.Context, job tasks.Job) error
    Finished(ctx context.Context, tx *ent.Tx, job tasks.Job, result error) error // 可选钩子
}

// magnet.Source：按番号提供磁力。
type Source interface {
    Name() string
    Find(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, error)
}
```

### 1.3 身份与数据模型

- `movie.javdb_id`、`actor.javdb_id`、`tag.javdb_id` 三列保留原名，NFO 的 `uniqueid type="javdb"` 不变。JavDB 是唯一的目录身份来源，JavBus 只按番号补充。
- `domain.MovieRef{Code, JavDBID}` 是所有增强源的输入。
- `domain.Magnet` 新增 `Sources []string`、`Tags []string`、`Inferred bool`。现有字段和 JSON 名保持不变，前端零破坏。

---

## 2. 工作线 A：全局代理

### 2.1 现状

- 只有环境变量 `MIYABI_PROXY`，进程启动时一次性传给 `javdb.Options.Proxy` 与 `pan.Options.Proxy`（`cmd/miyabi/main.go:59,67`）。
- 四处各自构造 HTTP 客户端：`javdb/transport.go:54`（tls-client）、`javdb/media.go:21`（resty）、`pan/client.go:30,32`（resty ×2）。运行时不可改，UI 不可见。
- 115 走代理通常适得其反（国内直连更快，且代理出口可能触发风控），JavDB/JavBus 通常必须走代理。所以"全局"应理解为：一个全局代理地址，按目标可开关。

### 2.2 设计

**配置模型**（settings 表，key `network.proxy`）

```json
{
  "url": "http://127.0.0.1:7890",
  "targets": { "javdb": true, "javbus": true, "pan": false }
}
```

- `MIYABI_PROXY` 保留：首次启动且 settings 无记录时作为初始值写入；之后以 settings 为准。
- 空 `url` 表示关闭。目标缺省值：javdb 开、javbus 开、pan 关。

**`internal/netx`**

```go
type Target string // TargetJavDB / TargetJavBus / TargetPan

type ProxyManager struct { current atomic.Pointer[ProxyConfig]; subs []chan struct{} }
func (m *ProxyManager) Resolve(t Target) *url.URL     // 每个请求调用
func (m *ProxyManager) Update(ctx, ProxyConfig) error  // 持久化 + 通知
func (m *ProxyManager) Subscribe() <-chan struct{}

// 客户端工厂：所有 HTTP 客户端只能从这里创建
func NewRestyClient(m *ProxyManager, t Target, opts RestyOptions) *resty.Client
func NewFingerprintClient(m *ProxyManager, t Target, opts FingerprintOptions) (tlsclient.HttpClient, error)
```

- resty：不用 `SetProxy`，而是注入自定义 `http.Transport{Proxy: func(*http.Request) (*url.URL, error) { return m.Resolve(t), nil }}`，每个请求实时取值，改代理无需重建客户端、无并发竞态。
- tls-client：代理只能在构造时指定。JavDB 的 transport 本来就在每次路由安装时重建（`javdb/client.go:installRoute`），因此 `javdb.Client` 订阅代理变更后调用 `Reinstall()` 重建当前路由的 transport 即可；JavBus 客户端同理。
- `cmd/miyabi/healthcheck.go` 保持不用代理。

**API**

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/settings/network` | 返回当前配置（url 脱敏保留主机端口） |
| PUT | `/api/settings/network` | 校验 URL（http/https/socks5），保存并广播 |
| POST | `/api/settings/network/test` | 并发探测：JavDB `/api/v1/startup`、JavBus 首页、115 `/open/user/info`（已登录时），按当前 targets 配置分别走或不走代理，返回每项耗时与错误 |

**前端**：设置页新增"网络"分区，复用 `features/settings/shared.tsx` 现有布局原语与表单组件，一个地址输入框、三个开关、一个测试按钮和结果列表。不新增样式。

### 2.3 步骤

1. 新建 `internal/netx`，单测覆盖 Resolve 按目标开关、Update 广播、URL 校验。
2. `pan.New` 与 `javdb.New` 改为接收 `*netx.ProxyManager`；删除两个 `Options.Proxy` 字段。
3. `javdb.Client` 增加 `Reinstall()`，订阅变更；`installRoute` 复用。
4. settings 读写、API 三个端点、测试端点。
5. 前端分区。
6. 删除 `config.Proxy` 的直接使用，仅保留为初始种子。

工作量约 3 到 4 天。这一线不依赖结构重构，建议第一个做，因为 JavBus 源和后续所有上游访问都要用它。

---

## 3. 工作线 B：结构重构

### 3.1 原则

- 行为不变：对外 HTTP 契约、数据库 schema、任务 payload 的 JSON 形状、NFO 输出全部保持。用黄金测试锁定。
- 叶子优先：先抽出被依赖最多、自身依赖最少的部分（错误与模型、任务、drive 会话），再迁业务包。
- 每一步都是一个可独立合并的提交，`go test ./... -race` 全绿。
- 迁移时顺手删冗余，但不做行为优化。行为优化列入 3.7 单独处理。

### 3.2 步骤 B0：安全网（先于一切）

1. 端到端测试 `internal/app/e2e_test.go`：内存假 115（现有 `panStub` 模式）+ 固件 JavDB（现有 `fixtureTransport`），跑"选目录 → 扫描 → 刮削 → 封面 → 上传"，断言 NFO 字节、图片键、`scrape_status`、任务链。
2. 线格式黄金测试：`/api/discover/movies/:id`、`/magnets`、`/api/library/movies`、`/api/tasks`、`/api/offline/tasks`、`/api/monitors` 响应快照进 `internal/api/testdata`。
3. `errorMiddleware` 映射测试、SSE 三类事件测试。
4. 删除 `AUDIT.md`、`AUDIT_RESPONSE.md`、`.tmp/audit-*`。
5. 补 `.gitattributes`（`* text=auto eol=lf`），避免 CRLF 噪音污染重构 diff。

### 3.3 步骤 B1：`domain` 与错误

**模型迁移**

- `internal/javdb/model.go` 中的 `Movie、MovieDetail、MovieReference、Magnet、PreviewImage、Actor、Tag、TagOption、TagCategory、Series、Maker、Director、Zone、EntityType、SearchOptions、BrowseOptions` 迁入 `internal/domain`，JSON tag 不变。
- 第一步在 javdb 包留 `type Movie = domain.Movie` 别名保证编译，全仓库替换引用后删除别名。
- `service.DiscoverMovie` 等 DTO 改为内嵌 `domain.Movie`，黄金测试证明输出一致。
- `javdb.Options`、`RouteStatus`、`RouteCandidate`、`APIError`、`HTTPError` 留在 javdb（属于该客户端的运维概念）。

**错误模型**

```go
package domain
type Kind int // Invalid, Unauthorized, NotFound, Conflict, Busy, Upstream, Canceled, Internal
type Error struct { Kind Kind; Message string; Cause error }
func E(kind Kind, message string, cause error) *Error
func (e *Error) Error() string; Unwrap() error; PublicMessage() string
```

- `api/error.go` 只按 `Kind` 映射状态码，响应体只放 `Message`；`Cause` 进日志。sentinel switch 删除。
- 现有 42 处内联中文 `fmt.Errorf` 逐一改为 `domain.E(...)`，英文包装链保留在 `Cause`。
- `pan.apiError` 已有 `PublicMessage()`，映射为 `Kind=Upstream`，115 原文作为 Message。

### 3.4 步骤 B2：`tasks` 包

从 `service/task.go`（471 行）抽出，拆为：

| 文件 | 内容 |
| --- | --- |
| `kind.go` | `type Kind string`，常量 `KindScan/KindScrape/KindCover/KindOffline`；全仓库 8 个文件的字面量替换 |
| `queue.go` | `Claim / Finish / Recover / Pending`，`queue` 锁 |
| `registry.go` | `Register(Handler)`，pool 从注册表取处理器与 `Finished` 钩子 |
| `bus.go` | `Subscribe / Notify* / Revisions`，SSE 扇出 |
| `workflow.go` | `List / Info / workflowInfos`（scan+scrape+cover 折叠投影）与窗口函数查询 |
| `payload.go` | `Payload[T]` 泛型读写、`SetField`、`Path(...)` 集中定义 JSON 路径；`storage/indexes.go` 的 `json_extract` 表达式改为引用这里的常量 |
| `pool.go` | 从 `internal/worker/pool.go` 迁入 |

- `TaskService.Finish` 中修改 `movie.scrape_status` 的逻辑（`task.go:328-337`）移到 `library/scrape` 的 `Finished` 钩子。
- `OfflineService` 对未导出 `workflowInfos` 的调用（`offline.go:353`）改为公开的 `Workflows(ctx, ids)`。
- `PanService.SelectDirectory` 直接锁 `tasks.queue` 并调用 `ensureScanTask`（`pan_directory.go:79-88`）改为 `drive` 发布 `MountChanged` 事件，`library` 订阅并入队扫描。

### 3.5 步骤 B3：`drive` 包与 `Session`

原 `service/pan.go、pan_state.go、pan_token.go、pan_directory.go、pan_pagination.go`（776 行）迁入 `internal/drive`：

| 文件 | 内容 |
| --- | --- |
| `drive.go` | `Drive` 结构：凭据、挂载目录、三个版本号、提交锁、`Close` |
| `account.go` | `Account / BeginLogin / LoginStatus / Disconnect` |
| `mount.go` | `Files / SelectDirectory / ClearDirectory`，挂载目录只保留内存快照 + 版本号一处真相；`loadLibrarySource` 从各读路径删除 |
| `token.go` | `withPanToken` 的主动/被动刷新与 singleflight |
| `session.go` | `Open(ctx) (Session, error)`：签发时校验账号（带 60 秒 TTL 缓存，替代每次操作都打 `/open/user/info`），封装 `sourceState → withPanSourceToken → checkScanSource` 三段式与 `commitSource` |
| `pagination.go` | `WalkFilePages / WalkOfflinePages` |
| `events.go` | `MountChanged` 订阅 |

替换点（共十余处）：`library_source.go:29-64`、`library_scan.go:284-301`、`cover.go:218-244`、`play.go:110-136`、`offline.go:189-240`、`scrape.go:66-76` 等。完成后 `ScrapeService`、`PlayService`、`DataService` 对 `library.drive.*` 的 33 处穿透全部消失。

### 3.6 步骤 B4 到 B7：业务包迁移

按顺序，每迁一个包提交一次：

**B4 `library` + `library/scrape`**（约 2300 行）

- `library_scan.go`（649 行）拆为：`scan/walker.go`（BFS、分页、`scanPage`）、`scan/identity.go`（`identifyScanVideos`、单 NFO 启发式）、`scan/persist.go`（`processScanPage`、`savePage`）、`scan/reconcile.go`。`Scan()` 从 180 行压到调度骨架。
- `scrape.go`（435 行）拆为：`scrape/scrape.go`（任务处理器）、`scrape/nfo_source.go`（`findNFO / directoryNFO / readNFO`，同时吸收 `library_scan.go:236-263` 的重复 NFO 读取）、`scrape/mapping.go`（`movieNFO / detailNFO / saveMovieMetadata` 三份字段映射合并为一份 `domain.Movie ↔ nfo.Movie ↔ ent` 的双向映射）。
- `cover.go`、`metadata_snapshot.go` 迁入 `scrape/`。
- `watch_history.go`、`library.go`、`scan_observations.go` 迁入 `library/`。
- `library_source.go:13` 与 `pan_directory.go:53-61` 的路径拼接合并为 `drive.DirectoryPath`。

**B5 `offline`**（797 行）拆为 `add.go`（入口与锁）、`submit.go`（115 去重启发式）、`sync.go`（轮询与状态转换 `updateTask / markMissing / completeTask`）、`projection.go`（phase 计算 `submissions`）、`locks.go`（原 `offline_operations.go`）。对 `scanPayload` 的直接构造改为调用 `library.EnqueueTargetedScan(...)`。

**B6 `monitor`、`playback`、`maintenance`**：基本原样迁移；`play.go/play_stream.go` 拆为 `session.go / files.go / proxy.go / playlist.go`。

**B7 `catalogue`**：`discover.go、discover_cache.go、discover_tags.go、movie_state.go` 迁入。`MovieStates` 需要本地库状态，通过 `catalogue.LocalState` 接口由 `library` 实现注入。`DiscoverService.javdb` 改为 `catalogue.Provider` 接口，JavDB 是唯一实现；`Facets()` 暴露 zones、排序、分类槽位供前端后续数据驱动（本轮前端不接）。

**B8 `app` 组合根**：`cmd/miyabi/main.go` 的 `run()` 拆为 `internal/app/app.go`，`New(cfg) (*App, error)`、`Run(ctx)`；任务处理器由各包 `Register`；`internal/service` 目录删除。

### 3.7 步骤 B9：API 层与配置

- `api/helpers.go`：`bindJSON / bindQuery / bindURI` 与 `respond(c, value, err)`、`accepted(c, value, err)`；保留流式与 SSE 特殊路径。约 60 处样板收敛。
- `noStore()` 中间件定义一次（现 `router.go` 三处）。
- `config.Runtime`：收纳 pool 大小、离线轮询 30s、监控 5m、115 限速 2 req/s、超时 35s/45s/2m、播放会话 8h、缓存尺寸与 TTL、`minVideoSize`、视频后缀、sidecar 大小上限。环境变量 `MIYABI_*` 可覆盖，未设置用现值。
- 日志级别校验只留 `logging` 一处。

### 3.8 冗余与死代码清单

迁移时随手处理，每项都有明确位置：

| 位置 | 处理 |
| --- | --- |
| `errPanSourceChanged` vs `library_scan.go:116`、`scrape.go:73` 的同文案字面量 | 统一为 `drive.ErrSourceChanged`（`domain.Kind=Conflict`） |
| `library_scan.go:236-263` vs `scrape.go:256-291` NFO 读取 | 合并为 `scrape.readNFO` |
| `pan/*` 11 处 `json.Unmarshal + result.err()` 样板 | 抽 `apiRequest[T]`，仿现有 `authRequest[T]` |
| `pan/upload.go:37` ≤128KiB 时两次 SHA1 | 复用 |
| `javdb/transport.go:82-89` 先读全 body 再判状态码 | 先判状态码，非 2xx 只做有上限 drain |
| `javdb` 中 `slog.Warn` 与 `WarnContext` 混用 | 统一 `WarnContext` |
| `javdb/client.go:40` 字段 `selectRoute` 与包级函数同名 | 字段改名 `selector` |
| `pan/play.go:74` 基础设施层中文文案 | 改为 `domain.E` 由上层赋文案 |
| `pan/file.go:47-56` `Count/Size` 未用 `json.Number` | 与同结构其它字段一致 |
| `service/*` 直接用 `slog.Default()` 两处 | 注入 logger |
| `worker/offline.go`、`worker/monitor.go` 两个几乎相同的 ticker 循环 | 合并为 `tasks.RunPeriodic(name, interval, wake, fn)` |
| `internal/ent/enttest` 生成但未使用 | 保留（生成物），不手删 |
| 前端 `api/library.ts:8`、`api/offline.ts:7`、`api/watch-history.ts:5` 反向 import features | toast 移到调用方 `onSuccess/onError`；`watchSessions` 移入 `features/player` |
| 前端三份 `page` 校验器 | `lib/search-schema.ts` 一份 |
| 前端三处 `DiscoverResults` 参数展开 | 组件直接接收 `UseQueryResult` |
| 前端四处 `sameSource` 判断、两处 zone Select、两处确认对话框、两处卡片包装 | 各收敛为一份，DOM 与 class 不变 |
| 前端 `clamp` 手写 5 处 | `lib/utils.ts` 一份，明确 NaN 语义 |
| 前端 `features/tasks/task-progress-state.ts` 与 `scan-status.ts` 两套阶段映射 | 共享阶段定义，文案各自保留 |
| 前端 `lib/watch-progress.ts` 与 `features/player/watch-progress.ts` 撞名 | 后者更名 `watch-progress-writer.ts` |
| 前端 `shadcn` 在 `dependencies` | 移到 `devDependencies` |

### 3.9 前端边界

本轮前端只做：反向依赖清理、上表列出的去重、磁力卡片数据驱动（见工作线 C）、网络设置分区（见工作线 A）。不改路由结构、不改 zustand 与 URL 状态分工、不改任何 className、动效、骨架屏数量。每项去重都以"渲染结果 DOM 一致"为验收，用现有 `node --test` 补快照即可。

---

## 4. 工作线 C：磁力聚合

### 4.1 为什么只要 JavDB + JavBus

两者都是人工维护的聚合站，磁力都经过筛选并带站方标注的"高清/字幕"标签，质量远好于 Sukebei 这类原始索引。JavBus 的收录与 JavDB 有明显互补（JavBus 常先收录 DMM 系新片的磁力，且旧片资源保留更久），合并去重后覆盖面提升明显。两者都以番号为键，不需要身份映射。

### 4.2 `internal/javbus` 客户端

**传输**：`netx.NewFingerprintClient(TargetJavBus)`（Chrome 指纹，JavBus 前面有 Cloudflare，普通 Go client 容易被拦），cookie jar，限速 1 req/s，超时 15s。基础域名可配置，默认 `https://www.javbus.com`，备选镜像列表由用户填写。

**协议**（来自 JHS-enhance 与公开实现，需要用真实响应固件确认）：

1. 详情页 `GET {base}/{CODE}`，请求头带 `Cookie: existmag=all`（否则只显示有磁力的条目）。
2. 从页内脚本提取三个变量：`var gid = <数字>; var uc = <数字>; var img = '<封面URL>';`。
3. 磁力片段 `GET {base}/ajax/uncledatoolsbyajax.php?gid={gid}&lang=zh&img={img}&uc={uc}&floor={1..1000 随机}`，请求头必须带 `Referer: {base}/{CODE}` 与同一 cookie。
4. 响应是 HTML 片段，若干 `<tr>`：第 1 列 `a[href^=magnet:]` 名称与磁力链，同列内 `a.btn-primary` 文本"高清"、`a.btn-warning` 文本"字幕"；第 2 列体积（`1.23GB`，1024 进制）；第 3 列日期 `YYYY-MM-DD`。
5. 番号不存在时详情页返回 404，视为"无结果"而非错误。
6. 覆盖范围：censored、uncensored；FC2、western、anime 直接跳过不请求。

**解析**：用 `golang.org/x/net/html`（已在 go.sum，是间接依赖，需要提升为直接依赖，属于既有模块不算新装；若你希望用 goquery 需要你来安装）。

**缓存**：详情页 HTML 按番号缓存 5 分钟，为后续详情补缺复用。

**测试**：`internal/javbus/testdata/` 放详情页与 ajax 片段固件，覆盖：有磁力、无磁力、404、Cloudflare 挑战页（识别 `Just a moment` 返回 `Kind=Upstream`）。

### 4.3 `internal/magnet`

```go
type Aggregator struct { sources []Source; timeout time.Duration; logger *slog.Logger }
func (a *Aggregator) Find(ctx, ref domain.MovieRef) ([]domain.Magnet, error)
```

- errgroup 并发，每源独立 `context.WithTimeout`（默认 8s）；单源失败只记 `WarnContext`，全部失败才返回错误。
- 去重：infohash 小写；同 hash 合并 `Sources`（保持 javdb 在前）、`HasSubtitle/HD` 取 OR、`Size` 取非零最大、`Name` 取 JavDB 的、`CreatedAt` 取最早。
- 标签：`Tags` 由站方标注生成（`高清`、`字幕`），外加 `quality.Infer(name)` 补充 `4K`、`无码`、`破解`。推断规则移植 JHS 的 `classifyQuality`：
  - 中字：`(?:[^A-Za-z]|^)FHDC(?:[^A-Za-z]|$)`、`[-_](?:UC|CH?)(?:[^A-Za-z]|$)`、中文关键词表（中字/中文/字幕/繁中/汉化/内嵌/内封/双语…）、含汉字且不含假名。
  - 4K：`(?:[^A-Za-z0-9]|^)(?:4K(?:UHD)?|2160P)(?:[^A-Za-z0-9]|$)`。
  - 无码/破解：`破解|破坏|破壞|无码|無碼`、`\b(?:uncensored|mosaic)\b`、后缀 `-U`、`-UC`。
  - 先剥掉广告括号 `【…APP…】【…夸克…】【…域名…】`。
  - 推断得到而站方未标注的字段标记 `Inferred=true`。
- 排序不变：字幕 > 高清 > 体积 > 文件数；同分时 `Sources` 含 javdb 者在前。
- `catalogue` 现有 `magnets` 缓存（64 条 / 1 分钟）改缓存聚合结果。

### 4.4 接入点

- `GET /api/discover/movies/:id/magnets`：响应每项新增 `sources`、`tags`、`inferred`。
- `POST /api/discover/movies/:id/offline {hash}`：`OfflineService.Add` 校验 hash 时对聚合结果校验，JavBus 独有磁力也能推送。
- `monitor.checkOne`：使用聚合结果；首选含 javdb 的条目，其次含 javbus 的；策略在设置中可调（仅 JavDB / 两者）。
- 设置：`magnet.javbus.enabled`、`magnet.javbus.base_url`、`magnet.javbus.mirrors[]`、`monitor.sources`。
- 前端 `MagnetCard`：现有两个布尔徽章改为遍历 `tags` 渲染，同样式的 `Badge` 组件；新增来源小徽章沿用 `variant="outline"`。卡片布局、间距、动效不变。

工作量约 1 到 1.5 周，依赖工作线 A 与 B1 的 `domain`。

---

## 5. 工作线 D：javdb-cli 接口评估

以下都是匿名可用的 App API 端点，miyabi 现有 client 加一个方法即可调用；成本主要在前端呈现。

| 端点 | 用途 | 建议 | 理由 |
| --- | --- | --- | --- |
| `GET /api/v1/rankings?type={zone}&period=daily\|weekly\|monthly` | 影片排行 | **加** | 发现页多一个"排行"标签页，复用 `DiscoverResults`，无新样式 |
| `GET /api/v1/rankings/playback?filter_by=&period=` | 播放排行 | **加** | 同上，作为排行页的一个切换项 |
| `GET /api/v1/rankings/actors?type=daily\|weekly\|monthly` | 演员排行 | 可选 | 需要演员卡片列表，前端有新组件 |
| `GET /api/v1/movies/{id}/reviews?page=&limit=&sort_by=hotly` | 影片评论 | **加** | 详情页折叠区，选片时很有参考价值；一页 20 条不翻页 |
| `GET /api/v1/{actors\|series\|makers\|directors}/{id}` | 实体详情 | **加** | 现在 `/discover/search?kind=actor` 只有作品列表；加头部资料（名称、别名、作品数、头像） |
| `GET /api/v1/lists/related?movie_id=` | 相关合集 | 暂不 | 价值一般，合集页需要新 UI |
| `GET /api/v1/codes/{id}` | 番号前缀实体 | 不加 | 用处很小 |
| `POST /api/v1/sessions`、users/*、reviews 写操作、`movies/top` | 登录与个人状态 | **不加** | miyabi 有自己的观看记录；绑定 JavDB 账号引入封号与隐私风险 |
| `ResolveMovieID` 的"格式等价唯一匹配" | 番号解析 | **采纳** | 分隔符差异（`ABC00123` vs `ABC-123`）可接受，多候选拒绝；比现在严格相等更实用，直接影响刮削命中率 |
| 自动选线 `SelectAutoHost` | 路由 | 不动 | miyabi 现有实现更完整（持久化、手动选择、故障切换） |
| 以图搜番（avscan.cc） | 反搜 | 后议 | 上传截图到第三方，隐私边界需要你决定 |
| 资源下载与 HLS→MP4 重封装 | 预览视频 | 后议 | 属于预览视频范围 |

建议本轮实现"加"的四项加上 `ResolveMovieID` 规则，约 3 到 5 天，其中后端各半天，前端排行标签页与评论折叠区约两天。

---

## 6. 执行顺序与里程碑

| 序 | 里程碑 | 内容 | 估计 | 依赖 |
| --- | --- | --- | --- | --- |
| M0 | 安全网 | B0：e2e、黄金测试、删审计文档、`.gitattributes` | 3 天 | 无 |
| M1 | 全局代理 | 工作线 A 全部 | 3 到 4 天 | 无 |
| M2 | 模型与错误 | B1 | 4 天 | M0 |
| M3 | 任务与会话 | B2、B3 | 1.5 周 | M2 |
| M4 | 业务包迁移 | B4 到 B8 | 2.5 周 | M3 |
| M5 | API 与配置收口 | B9、3.8 后端清单 | 4 天 | M4 |
| M6 | 磁力聚合 | 工作线 C | 1 到 1.5 周 | M1、M2 |
| M7 | JavDB 接口补齐 | 工作线 D | 3 到 5 天 | M2 |
| M8 | 前端结构清理 | 3.8 前端清单、3.9 | 1 周 | 可与 M3 到 M5 并行 |

总计约 9 到 10 周单人工作量。M1、M6、M7、M8 都可以和主线并行推进。如果只能串行，顺序就是 M0 → M1 → M2 → M3 → M4 → M5 → M6 → M7 → M8。

---

## 7. 验证与回归边界

- 每个提交：`go test ./... -race`、`go vet`、前端 `node --test`、`tsc`、oxlint、Vite 构建。
- M2 起：黄金 JSON 测试证明所有列出的端点响应逐字节一致。
- M3：`drive.Session` 并发测试沿用现有 `pan_concurrency_test.go` 场景（登出、换目录、令牌刷新中途发生）。
- M4：e2e 测试在每个包迁出后重跑；`internal/service` 删除时 e2e 必须仍然通过。
- M6：JavBus 固件测试；聚合器测试覆盖单源超时、单源失败、重复 infohash 合并、`Inferred` 标记、排序稳定性。
- 浏览器行为按项目约定由你验证：设置页网络分区、磁力卡片徽章、排行标签页、评论折叠区。

---

## 8. 需要你确认的事项

1. **JavBus 固件采集。** 我需要一份真实的详情页 HTML 和一份 ajax 磁力片段做测试固件。这台机器若能通过代理访问 JavBus 我可以自己抓；否则请你保存两份响应到 `internal/javbus/testdata/`。
2. **`golang.org/x/net/html` 提升为直接依赖**是否可接受（它已在 `go.sum` 中，只是 `go.mod` 里标注 indirect）。
3. **代理默认值**：JavDB 开、JavBus 开、115 关。是否符合你的部署环境。
4. **监控自动推送策略**默认值：仅 JavDB 磁力，还是 JavDB 与 JavBus 都允许。
5. **`config.Runtime` 的环境变量前缀与命名**是否需要现在就定稿并写进 README，还是先以内部常量集中、后续再开放。
