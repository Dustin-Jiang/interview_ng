# Repository Guidelines

## 项目概述

面试管理系统（interview_ng）：面试官协作面试、多部门录取决定、捡漏阶段按预算竞拍补录候选人。pnpm monorepo：

- `apps/web`（`@interview-ng/web`）：Vue 3 + Vite + TS + shadcn-vue 前端
- `apps/server`（Go module `interview_ng`，Gin + Gorm）：单进程后端，**已从 pnpm workspace 排除**（`pnpm-workspace.yaml` 中 `!apps/server`）
- 文档与代码注释一律使用中文

## 架构与数据流

后端分层（依赖单向向下）：

```
cmd/server/main.go（装配：AutoMigrate 12 表 → seed → rbac → auth → state → broadcast → service → 路由）
  handler（HTTP /api + WS /ws/rooms/:roomId、/ws/board）
    → service.InterviewService（用例编排）
      → state.StateStore（MemStateStore，唯一权威）
        → gorm / Postgres
```

- **事件流（出站）**：state 写操作在同一临界区内「先落库成功、后 `s.emit`」→ service 经 `broadcast.Manager.Publish` → handler 注册的 Sink → WS 客户端。事件带全局单调 `Seq`；消息带候选人维度 `msg_id` 作续传游标。事件类型与载荷见 `internal/state/event.go`。
- **RBAC 旁路**：`internal/auth`（JWT + `RequireAuth`/`RequirePerm` 中间件）读 `internal/rbac/cache.go` 内存缓存（DB 权威 + 内存加速）；改密 bump `token_version` 踢旧 token。
- **候选人七档状态机**（`internal/model/candidate.go`）：`NOT_CHECKED_IN → CHECKED_IN_PENDING_ASSIGN → ASSIGNED → IN_PROGRESS → COMPLETED（面试已结束）→ ADMISSION_PENDING（待录取）→ ADMITTED（已录取）`，`StatusTransitions` 严格转移图 + `guardTransition`。
- **候选人学号**（`internal/model/candidate.go`）：`student_no` 是**身份键**——必填、纯数字（1–64 位；全角数字折半角、去首尾空白）、唯一（`uniqueIndex` 兜底），前导零有意义（`00123` ≠ `123`，文本列存储）；三条写入路径（`POST /api/candidates`、`PUT /api/candidates/:id`、导入）统一走 `ValidateStudentNo`，撞号 → `ErrStudentNoExists` → **409**。该列为 NOT NULL，**旧库必须重置**（AutoMigrate 无法给已有数据的表加 NOT NULL 列）。
- **批量导入**（`internal/state/mem_store.go` 的 `ImportCandidates` + `POST /api/candidates/imports`）：浏览器解析 Excel 并映射后提交行数组；服务端**单事务全或无**按学号 upsert（只写 `student_no/name/profile`，运行态不动；批内同学号后者覆盖前者），校验失败一次返回全部行级错误（`*state.ImportError` → `400 {error, rows:[{index,error}]}`）。
- **系统四阶段**（`internal/model/system_status.go`）：`interview / admission / leftover / settlement`。**进入结算阶段即按出价自动结算全部竞拍**（最高价部门录取、其余出价部门放弃、争议一并仲裁；幂等封盘），并批量同步录取档（唯一 admitted → 已录取，其余 → 待录取）；结算不提供逐个手动入口。
- **捡漏竞拍**（`internal/model/bid.go`）：预算 `max(500, (预期人数−已录取)×100)`；出价跨部门保密（事件不带金额）；唯一 admitted 才算已结算封盘，多家录取属争议进捡漏仲裁；出价步长 `bid_step`（默认 10）存于系统状态单行。

前端（`apps/web/src`）为 MVVM 函数式，无 Pinia：

- `api/`：axios 单例（`/api` baseURL、Bearer 注入、401 统一登出）、各资源 Service 对象（`http.ts`）、WS 客户端 `ws.ts`（首条消息必须 auth，断线 1.5s 重连，重连后增量补拉）
- `composables/`：函数式 ViewModel；`useAsync(loader)` 是所有请求的基础原语（`{data, loading, error, run}`）；`useBoardChannel` 是唯一模块级单例 + 引用计数，`useBoardRefresh(events, cb)` 300ms 防抖重拉是列表页实时刷新标准模式
- `models/index.ts`：与后端 JSON 契约一一对应的纯类型层
- `domain/`：无 Vue 依赖的纯函数（状态机、消息合并、录取预览、**学号归一化校验**、**Excel 解析与 JMESPath 映射**）
- `presenters/`：状态 → 中文标签 + Badge variant 的展示映射
- **数据导入页**（`/settings/imports`，需 `candidates.manage`）：`xlsx`（官方 CDN tarball，import `xlsx/dist/xlsx.mini.min.js`）与 `@jmespath-community/jmespath` **只在该分区的懒加载 chunk 里引入**；解析、映射、预览全在浏览器完成，服务端零新增依赖、无 multipart。JMESPath 里中文列名必须加引号（`"姓名"`），界面给出可复制的列名清单。

## 关键目录

| 路径 | 用途 |
|---|---|
| `apps/server/cmd/server/main.go` | 唯一入口：连接、迁移、装配、路由 |
| `apps/server/internal/handler/` | `http.go`（全部 REST 路由与 `stateErr` 错误映射）、`ws.go`（WS 泵与命令分发）、`hub.go`、`envelope.go` |
| `apps/server/internal/state/` | `store.go`（StateStore 接口 + `Error{Code,Msg}` + 哨兵错误）、`mem_store.go`（全部实现）、`event.go` |
| `apps/server/internal/{auth,rbac,broadcast,seed}/` | JWT/中间件、权限缓存、事件扇出、启动种子（幂等 reconcile） |
| `apps/web/src/api/`、`composables/`、`models/`、`domain/`、`presenters/` | 见上 |
| `apps/web/src/views/` | 页面（组装层：只做筛选/展示派生 + 组合下方共享组件与 composable） |
| `apps/web/src/components/ui/` | shadcn-vue 组件，`index.ts` + cva 变体约定 |
| `apps/web/src/components/app/` | 自研业务外壳：`PageShell`、`EmptyState`、`SearchInput`、`ConfirmDialog`、`MessageTranscript`（通用）+ `MasterDetailSplit`、`RosterList`、`RosterPager`、`CandidateDetailHeader`（名册↔详情布局）、`DataTableSection`、`FormDialog`、`RefreshButton`、`ListSkeleton`、`ErrorAlert`（列表/表单/状态骨架）、`FileDropInput`、`CandidateFormFields`、`ImportFileStep`/`ImportMappingStep`/`ImportPreviewTable`/`ImportSubmitStep`（数据导入） |

## 开发命令

```bash
pnpm dev              # 仅 web（vite，端口 3000）
pnpm dev:server       # Go 服务（:8080）
pnpm build            # web 构建（vue-tsc -b && vite build）
pnpm typecheck        # web 类型检查（vue-tsc -b --noEmit，走 project references）
pnpm test             # typecheck + go test ./...
pnpm test:server      # go test ./...（不带 -race）
just test-server      # go test -race ./...
just db               # docker compose up -d（或 podman compose up -d）
```

- Go 命令**必须从 `apps/server/` 运行**，且设置 `GOCACHE="$PWD/.gopath/gocache"`（构建缓存进仓库内，已 gitignore）。推荐 `go test -race ./...`。
- Postgres：`docker-compose.yml`（postgres:16-alpine，库 `interview`，postgres/postgres，:5432）。Schema 由 Gorm AutoMigrate 在启动时创建——**没有迁移工具**。
- 服务端环境变量：`DATABASE_DSN`、`ADDR`（默认 `:8080`）、`JWT_SECRET`（缺省 dev 值）、`ADMIN_INIT_PASSWORD`（默认 `admin`）。无开放注册。

## 代码约定与常见模式

- **后端**
  - 所有写操作走 `MemStateStore`：`s.mu` 临界区内「先落库成功，后 emit 事件」，持久化失败绝不广播。
  - 错误：领域层返回 `&state.Error{Code, Msg}`（机器码如 `not_leftover_phase`/`budget_exceeded`）或哨兵错误；handler 用 `stateErr()` 统一映射 HTTP 状态码。
  - 状态变更必须经 `guardTransition` / `validStatus` 校验；房间是独立物理记录，绑定的唯一权威在 `rooms.candidate_id`，候选人 `room_id` 是只读投影；候选人完成即自动清房；消息按候选人归属、删除候选人级联删消息。
- **前端**
  - 列表页模式：多个独立 `useAsync` 资源 + `useBoardRefresh([...事件], reloadAll)` + `RefreshButton`；筛选态同步 URL query、选中条目同步路径参数（`router.replace`，均可深链）。
  - **URL 一律 RESTful**：集合用复数名词 + 条目 `/:id`（页面 `/candidates`、`/candidates/:candidateId`、`/rooms`、`/rooms/:roomId`、`/leftover`、`/leftover/candidates/:candidateId`；接口 `/api/candidates`、`/api/rooms/:id/candidate`），路径段 kebab-case，**禁止动词路径**（如 login/checkin/pull 这类动词改为资源：`POST /api/sessions`、`PUT /api/candidates/:id/check-in`、`PUT /api/rooms/:id/candidate`）；集合项用 POST/GET/DELETE，单例子资源用 PUT，部分更新用 PATCH（`PATCH /api/system/status`）。旧页面路径保留重定向，接口不留别名。
  - **先复用后新写**：「左名册 + 右详情」用 `MasterDetailSplit` + `RosterList` + `RosterPager`（选中/键盘/深链状态在 `useRosterSelection`）；管理列表用 `DataTableSection`（骨架→空态→表格）；增改表单用 `FormDialog`；二次确认用 `useConfirmAction` + `ConfirmDialog`；加载/错误/刷新用 `ListSkeleton` / `ErrorAlert` / `RefreshButton`；异常提示用 `lib/toast.ts` 的 `toastError`。视图层只保留筛选与展示派生。
  - **单文件行数**：视图/组件/组合式函数尽量 ≤ 400 行；超标即按上述原语拆分，避免超长文件难以维护。
  - WS 消息经 `domain/` 纯函数不可变更新（如 `mergeMessages` 按 id 去重升序）。
  - **视觉 token**：颜色/圆角/边框/阴影只能取自 `src/assets/index.css`（`:root`/`.dark` 语义变量）与 `tailwind.config.cjs` 语义映射（如 `bg-muted`、`text-muted-foreground`），**禁止硬编码 hex/rgb/hsl**。新组件沿用 `components/ui/<name>/index.ts` + cva 变体；状态只通过带文字标签的 Badge 传达，不依赖颜色单通道。
  - **无 caption 小字**：禁止在标题下附加小字号说明文字（页头 description、表单说明行、空态 hint 等）——要么删掉冗余说明，要么改写标题。数据内容（如个人简介）与功能元信息（字段标签、时间戳、`-` 占位）不受限。
  - 导航/操作提示用 `vue-sonner` toast；确认类操作用 `components/app/ConfirmDialog.vue`。

## 重要文件

- `apps/server/cmd/server/main.go` —— 装配入口（改模型后核对 AutoMigrate 列表）
- `apps/server/internal/state/store.go` —— StateStore 接口 = 权威层契约（新增能力先改这里）
- `apps/server/internal/state/mem_store.go` —— 全部业务实现（~1400 行）
- `apps/server/internal/handler/http.go` —— 路由 + 权限中间件 + `stateErr` 映射
- `apps/web/src/api/http.ts` —— axios 封装与全部 REST Service 对象
- `apps/web/src/models/index.ts` —— 前后端契约类型（改后端 JSON 必同步）
- `apps/web/vite.config.ts` —— dev 端口 **3000**；`/api` 与 `/ws` 代理到 `:8080`
- `README.md` —— 架构、数据模型、WS 协议、API 端点清单（中文，最完整）

## 运行时与工具链

- Node 侧一律 **pnpm**（`packageManager: pnpm@11.7.0`）；Go 1.26.5。
- 依赖注入是手工构造（`main.go` 一条链），无框架。
- UI 组件经 shadcn-vue 约定（`components.json`，style default，lucide 图标）；本仓库 `reka-ui` + cva + tailwind-merge。
- `tests/concurrency/` 与 `cmd/wssmoke` **只有文档、没有实现**——相关命令会失败，勿引用。

## 测试与 QA

- 仅 Go 侧有测试，全部用内存 SQLite `:memory:`（每测试独立建库 + AutoMigrate）：
  - `internal/state/mem_store_test.go`（状态机/捡漏/阶段同步）、`manage_test.go`（管理操作 + 并发拉取仅一成功）
  - `internal/handler/handler_test.go`（最重的集成层：seed+rbac+auth+broadcast+service 全栈进 gin.TestMode，REST 走 httptest，WS 走 gorilla 客户端）
  - `internal/seed/seed_test.go`、`internal/broadcast/manager_test.go`
- 前端**没有测试框架**，`pnpm test:web` 只是 `vue-tsc -b --noEmit`——回归保障主要在 Go 测试 + 手动浏览器验证。
- 验证标准：`cd apps/server && go test -race ./...` 全绿；UI 改动需启动实际服务（Postgres + :8080 + :3000）在浏览器确认。
