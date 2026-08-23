# AGENTS.md

pnpm monorepo：`apps/web`（Vue 3 + Vite + TS + shadcn-vue，包名 `@interview-ng/web`）与 `apps/server`（Go，module `interview_ng`，**已从 pnpm workspace 排除**，见 `pnpm-workspace.yaml`）。文档与代码注释使用中文书写；新增文档/注释请保持一致。

## 命令

- 根脚本：`pnpm dev`（仅 web）、`pnpm dev:server`、`pnpm build`、`pnpm typecheck`（仅 web，`vue-tsc --noEmit`）、`pnpm test`（web typecheck + `go test ./...`）。`pnpm test:server` **不带** `-race`。
- Go 命令必须从 `apps/server/` 运行。根脚本会设置 `GOCACHE="$PWD/apps/server/.gopath/gocache"`，把构建缓存放在仓库内（已 gitignore）。Go 工具请同样设置，勿依赖全局缓存。
- 单包命令：`pnpm --filter @interview-ng/web typecheck` / `build`（build = `vue-tsc -b && vite build`）。

## 运行环境

- Postgres 通过 `docker-compose.yml`（`podman-compose up -d` 或 `docker compose up -d`；库 `interview`，postgres/postgres，:5432）。服务启动时 AutoMigrate —— 无迁移工具。
- 服务端环境变量：`DATABASE_DSN`（默认 DSN 见 `cmd/server/main.go`）、`ADDR`（默认 `:8080`）、`JWT_SECRET`（JWT 签发密钥，缺省 dev 值）、`ADMIN_INIT_PASSWORD`（种子 admin 初始密码，缺省 `admin`）。
- 登录鉴权：`POST /api/auth/login` 签发 7 天 JWT；`GET /api/me` 返回用户/角色/权限并集。RBAC 权限判断走 `internal/rbac` 内存缓存，变更即时生效；改密 bump `token_version` 踢旧 token。
- Vite 将 `/api` 与 `/ws` 代理到 `:8080`；web dev 跑在 `:8000`（`apps/web/vite.config.ts`）。
- 无开放注册。Web 端身份来自登录态（token 存 localStorage），不再写死 `CURRENT_USER_ID`。

## 架构（单进程、内存权威）

分层：`handler` → `service` → `state(StateStore)` → `model(Gorm)`，`broadcast` 订阅状态事件。详见根 README。

- `internal/state/store.go`：`StateStore` 是唯一权威。当前实现为 `MemStateStore`（内存权威，Gorm 持久化）。所有写操作必须先落库成功，再产出事件（“先落库后广播”）——持久化成功前绝不广播。
- 候选人状态机：`NOT_CHECKED_IN → CHECKED_IN_PENDING_ASSIGN → ASSIGNED → IN_PROGRESS → COMPLETED`；非法迁移通过 `state.Error` 错误码拒绝（`store.go:59`）。管理端「重置到任意档」（`PUT /api/candidates/:id/status`）向后自动解绑房间、向前须已有房间。
- 房间是**独立于候选人的物理会议室记录**：`candidate_id` 可空，无房间状态机，房间“状态”= 候选人状态的查询投影；仅空房可删。消息**按候选人归属**（`messages.candidate_id`），候选人维度续传游标，删候选人级联删其消息。候选人完成（推进或重置到 `COMPLETED`）**自动清房**（`rooms.candidate_id` 置空，绑定唯一权威在房间侧，候选人 `room_id` 为只读投影），房间转空闲、成员留守，可立即拉取下一位；消息仍按候选人归档保留。
- **分配 = 房间内拉取**（`POST /api/rooms/:id/pull_candidate`），取代旧的 `POST /api/candidates/:id/assign`。
- 一个房间 = 一个候选人 + 多个面试官；`room_members` 对 `(room_id, user_id)` 唯一，因此一个用户至多同时处于一个活跃房间。
- 事件带全局单调 `Seq`；消息带候选人维度 `id` 作续传游标。WS 走 RESTful 路径 `GET /ws/room/:roomId`（无 query 参数），连接后首条消息必须为 `auth`（携带 JWT，10 秒超时），成功后自动 JoinRoom；JSON 信封 `{op, req_id, data}`。

## 测试

- 后端测试：`internal/state/mem_store_test.go`、`internal/state/manage_test.go`、`internal/handler/handler_test.go`（含登录/权限矩阵/WS 端到端），均用内存 SQLite `:memory:`。根 README 里的 `tests/concurrency/` 套件与 `cmd/wssmoke` **只有文档、尚未实现** —— 相关命令会失败。验证请从 `apps/server/` 运行 `go test -race ./...`。

## 前端约定

- 无 Pinia/全局 store。MVVM 通过组合式函数（`src/composables/useXxx`，状态在调用方作用域内自管理，`onScopeDispose` 清理）+ 纯函数 `src/domain/` + `src/api/` 服务层实现。视图只绑定 VM；`src/models/` 与后端 JSON 契约一一对应。
- **视觉 token 一律符合全局设计**：颜色/圆角/边框/阴影只能取自 `src/assets/index.css`（`:root`/`.dark` CSS 变量）与 `tailwind.config.cjs`（`theme.extend` 的语义色映射、radius 档位）的语义 token（如 `bg-muted`、`text-muted-foreground`、`bg-border`、`bg-accent`），禁止硬编码 hex/rgb/hsl 或任意值色。新增/扩展组件须沿用 `components/ui/<name>/index.ts` + cva 变体约定（参考 button/badge/icon-badge），并在既有变体基础上扩展；状态只通过带文字标签的 Badge 传达，不依赖颜色单通道。
- **无 caption 小字**：禁止在标题/栏目标题之下附加小字号说明文字（页头 description、卡片副标题、对话框说明、空态 hint、表单说明行等）。出现 caption 即意味着标题不够明确——要么删掉冗余说明，要么把标题改写得足够准确、自明。数据内容（如个人简介）与功能元信息（字段标签、时间戳、空态主文案、`-`/`无` 占位）不受此限。
