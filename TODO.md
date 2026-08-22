# TODO：去掉所有 Skeleton（加载骨架屏样式）

> 贯穿实现的不变量：**先落库后广播**；候选人状态机为唯一权威；消息**按候选人归属**；房间是独立物理会议室记录（`candidate_id` 可空）；文档/注释用中文；Go 命令从 `apps/server/` 运行并设 `GOCACHE`。

## 已完成（历史清单）

- [x] **数据模型**：`users.username/password_hash/token_version`、`roles/role_permissions/user_roles`（9 枚权限）、`rooms.candidate_id` 可空（FK `SET NULL`）、`messages.candidate_id`（消息按候选人归属，`id` 即候选人维度续传游标）。
- [x] **认证与 RBAC**：JWT（7 天）+ `token_version` 吊销；RBAC 内存缓存即时生效；种子 `admin`（全权限）/ `interviewer`；登录 / `/api/me` / 自助改密；401/403 分离。
- [x] **WS 改造**：`GET /ws/room/:roomId` 无 query 参数；首条 `auth` 消息（JWT + `rooms.chat`，10 秒超时），成功自动 JoinRoom；`send_msg`/`move_phase` 按权限与连接身份；候选人维度断线续传。
- [x] **管理接口**（StateStore → service 先落库后广播）：用户/角色 CRUD；候选人 CRUD、重置状态（向后解绑/向前须有房）、级联删消息；房间 CRUD、成员管理、`POST /api/rooms/:id/pull_candidate`（拉取式分配）。
- [x] **前端**：登录鉴权基础设施（`useAuth`/路由守卫/401 登出）；候选人管理页（编辑/删除/重置状态/搜索）；房间列表与 `RoomChat`（拉取候选人、成员管理、空房禁发消息）；面试官管理页。
- [x] **验证**：`go test -race ./...` 全绿；`pnpm --filter @interview-ng/web build` 与根 `pnpm test` 通过；真实 Postgres 手工冒烟通过。
- [x] **文档**：根 `README.md`、`AGENTS.md`、`apps/web/README.md` 已对齐当前架构。
- [x] **候选人完成 → 自动清房**：`MovePhase`/`ResetCandidateStatus` 推进到 `COMPLETED` 自动解绑房间（`rooms.candidate_id` 与候选人 `room_id` 置空），房间转空闲可拉取下一位；消息按候选人归档保留；新增/更新后端测试（state 级 + WS 端到端），前端 `clearRoomCandidate` + `applyEvent` 处理 `COMPLETED` 清空会话，README/AGENTS 补不变量。

---

## 当前任务：去掉所有 Skeleton（加载骨架屏样式）

### 背景与目标

现状：4 个 Vue 视图的"加载态 / 连接中"分支使用了 shadcn 的 `<Skeleton>` 骨架屏，用户认为样式"很恶心"，要求去掉。

目标：**加载态留空即可**（不渲染任何骨架内容，也不加替换元素）；仅清理各文件中对 `Skeleton` 组件的引用（import 与模板块），不动业务逻辑；shadcn 的 `components/ui/skeleton/*` 库文件保留不动（仅不再被引用）。

---

### A. 清理每处 Skeleton 使用

- [x] `apps/web/src/views/CandidatesView.vue`：
  - 删除 `:19` `import { Skeleton } from '@/components/ui/skeleton'`；
  - 删除 `:279-281` 的"加载态"骨架块，并将空态判定改为 `v-if="!loading && candidates.length === 0"`（加载中留空，`loading` 期间列表数据为空直接落到空态/表格分支）。
- [x] `apps/web/src/views/UsersView.vue`：
  - 删除 `:16` import；
  - 删除 `:333-335` 的 `<div v-if="loading" class="space-y-2 py-4">…<Skeleton …/></div>`。
- [x] `apps/web/src/views/RoomView.vue`：
  - 删除 `:17` import；
  - 删除 `:109-112` 的"加载态"骨架块，并将空态判定改为 `v-if="!loading && rooms.length === 0"`（加载中留空）。
- [x] `apps/web/src/views/RoomChat.vue`：
  - 删除 `:17` import；
  - 删除 `:116-120` 的 `<div v-if="connecting">…<Skeleton …/></div>` 骨架块（连接中直接不显示，连接完成后由 `hasCandidate`/`messages` 分支接管）。

### B. 保留不动

- [x] `apps/web/src/components/ui/skeleton/Skeleton.vue` 与 `index.ts`：shadcn 库文件，仅不再被引用，**不删除**。

### C. 复核与验证

- [x] 全局复核：`rg "Skeleton|skeleton" apps/web/src --include '*.vue' --include '*.ts'` 确认除库文件外无残留引用。
- [x] 前端：`pnpm --filter @interview-ng/web typecheck` 通过（无未使用 import / 未定义组件报错）。
- [x] 前端：`pnpm --filter @interview-ng/web build` 通过；根 `pnpm test` 通过。
- [x] （可选）运行 Web 目视复核 4 个页面的加载/连接态均不出现骨架屏。