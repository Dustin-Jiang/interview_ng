# TODO：管理界面与后端接口（面试官 / 面试者 / 面试房间）

> 基于已达成共识的设计树。本清单只列**待实现**的工作；已完成的决策（鉴权模型、RBAC、消息归属候选人、房间独立实体、拉取式分配等）已定为前提，不再重复论证。
> 贯穿实现的不变量：**先落库后广播**；候选人状态机为唯一权威；**401/403 分开**；文档/注释用中文；Go 命令从 `apps/server/` 运行并设 `GOCACHE`。

---

## A. 后端 — 数据模型 ✅

- [x] `apps/server/internal/model/user.go`：`User` 增 `Username`（`uniqueIndex`）、`PasswordHash`、`TokenVersion uint64`；移除无用 `Role string` 字段，`Name` 保留为显示名。
- [x] 新增 `apps/server/internal/model/role.go`：
  - `Role`：`id, name(unique), description, created_at, updated_at`
  - `RolePermission`：`role_id, permission`（联合唯一 `(role_id, permission)`）
  - `UserRole`：`user_id, role_id`（联合唯一 `(user_id, role_id)`，M2M）
  - `Permission` 常量集（9 枚）：`users.manage`、`candidates.manage`、`candidates.create`、`candidates.checkin`、`candidates.assign`、`rooms.view`、`rooms.chat`、`rooms.move_phase`、`rooms.manage`
- [x] `apps/server/internal/model/room.go`：`CandidateID` 改为 `*uint64`（可空，房间可独立存在）；FK 改 `OnDelete:SET NULL`，删除"房间阶段=候选人状态"的注释，补"房间是独立物理会议室记录，状态为候选人状态的查询投影"。
- [x] `apps/server/internal/model/message.go`：`RoomID` 改为 `CandidateID`（`index:idx_candidate_id`），`id` 为候选人维度续传游标；`SenderID` 对 `User` 保持 `OnDelete:RESTRICT`。
- [x] `apps/server/internal/model/room_member.go`：不变（`user_id` 已 CASCADE）。
- [x] `cmd/server/main.go`：`AutoMigrate` 增补 `Role / RolePermission / UserRole`。
- [x] 涟漪修正：`MemStateStore` 的 `AppendMessage`（空房间拒绝发送、消息按候选人落库）、`ListMessagesAfter(candidateID,…)`、`AssignCandidate`/`MovePhase` 指针解引用；测试同步适配（编译 + `go test` 通过）。

## B. 后端 — 认证与 RBAC ✅

- [x] 新增 `apps/server/internal/auth/`：JWT 签发/校验（stdlib HS256，claims 仅含 `user_id` + `ver`(token_version) + `exp`）。密钥取自环境变量 `JWT_SECRET`。
- [x] 新增 RBAC 内存缓存 `apps/server/internal/rbac/cache.go`：启动全量加载 `user → [role] → [permission]` 并集与 `token_version`；提供 `HasPermission` / `UserRoles` / `UserPermissions` / `TokenVersion` / `ReloadAll` / `ReloadUser`。
- [x] 认证中间件（Gin，挂在 `/api` 组）：白名单 `GET /api/health`、`POST /api/auth/login`；无 token/非法/过期/版本不符 → **401**；缺权限 → **403**。
- [x] `POST /api/auth/login` `{username, password}` → bcrypt 校验，返回 `{token, user, roles, permissions}`（7 天有效期）。
- [x] `GET /api/me` → `{user, roles, permissions}`。
- [x] `POST /api/auth/password` `{old_password, new_password}` → 自助改密（验旧密码、bcrypt 落库、`token_version++` 踢旧 token）。
- [x] 种子 `internal/seed/seed.go`：启动时若无用户则建 `admin`（全 9 权限）+ `interviewer`（6 枚流程权限）+ 默认 admin（`username=admin`，密码取 `ADMIN_INIT_PASSWORD`，缺省 `admin`）。

## C. 后端 — WS 改造 ✅

- [x] 路由：`GET /ws/room/:roomId`（RESTful 路径，移除 `user_id`/`room_id` query 参数）。
- [x] 新增 WS `auth` op：`{"op":"auth","req_id":...,"data":{"token":"..."}}`；校验 JWT + token_version + `rooms.chat` 权限，成功绑定 `user_id` 并按路径 `roomId` 自动 JoinRoom。
  - 鉴权一次机会、**10 秒超时**未 auth 即断开；未鉴权前其他 op 一律拒绝。
- [x] WS 各 op 权限与操作者身份：`send_msg` → `rooms.chat`；`move_phase` → `rooms.move_phase`；操作者一律取连接绑定的 `user_id`。
  - `send_msg` 于空房间（无候选人）拒绝。
- [x] `sync` 续传改为**候选人维度**：由房间当前绑定候选人取 `last_msg_id` 增量；`message_appended` 事件载荷带 `candidate_id` + `room_id`。

## D. 后端 — 管理接口（全部走 StateStore → service，先落库后广播）✅

**StateStore 接口扩充**（`internal/state/store.go` + `mem_store.go`）：
- [x] 用户：`CreateUser`、`GetUser`、`ListUsers(q)`、`UpdateUser`、`SetUserRoles`、`ResetUserPassword`（bump token_version）、`DeleteUser`。
- [x] 角色：`ListRoles`、`CreateRole`、`UpdateRole`、`DeleteRole`（被引用拒绝）。
- [x] 候选人：`UpdateCandidate`、`DeleteCandidate`（级联消息 + 解绑房间）、`ResetCandidateStatus`（向后解绑/向前须有房）、列表关键词搜索。
- [x] 房间：`CreateRoom`（空房）、`DeleteRoom`（仅空房）、`PullCandidate`（待分配池才可拉，并发原子）。

**Handler 路由**（`internal/handler/http.go`，权限按矩阵）：
- [x] `users.manage`：`GET/POST /api/users`、`PUT/DELETE /api/users/:id`、`POST /api/users/:id/reset_password`、角色管理 `GET/POST /api/roles`、`PUT/DELETE /api/roles/:id`。
- [x] `candidates.*`：`PUT /api/candidates/:id`、`DELETE /api/candidates/:id`、`PUT /api/candidates/:id/status`（重置）、`GET /api/candidates?q=` 搜索。
- [x] **移除** `POST /api/candidates/:id/assign`。
- [x] `rooms.manage`：`POST /api/rooms`、`DELETE /api/rooms/:id`、`POST /api/rooms/:id/members`、`DELETE /api/rooms/:id/members/:userId`、`PUT /api/rooms/:id/current_interviewer`。
- [x] `candidates.assign`：`POST /api/rooms/:id/pull_candidate` `{candidate_id}`。
- [x] 房间列表/详情返回绑定候选人（可空）与成员、主持人；空房间状态由前端按候选人聚合投影。

> 后端测试：`internal/state/manage_test.go`（拉取并发、重置联动、级联删除、删房规则、空房禁发、用户/角色生命周期）+ `internal/handler/handler_test.go`（登录/me/401/403/权限矩阵）——`go test -race ./...` 全绿。

## E. 前端 — 鉴权基础设施 ✅

- [x] `src/config.ts`：删除 `CURRENT_USER_ID` / `CURRENT_USER_NAME`（身份改由登录态）。
- [x] 新增 `src/composables/useAuth.ts`：`user / currentUserId / roles / permissions / hasPermission / login / logout / ensureAuthReady`；token 存 localStorage；启动读 token 调 `GET /api/me` 初始化；注册 HTTP 401 统一登出。
- [x] 新增 `src/views/LoginView.vue` + 路由 `{ name: 'login', path: '/login' }`。
- [x] `src/router/index.ts`：全局守卫——未登录访问非 `/login` 跳登录；已登录访问 `/login` 跳候选人管理。
- [x] `src/api/http.ts`：自动带 `Authorization: Bearer`；401 触发登出；新增 auth/user/role/room 管理端点；移除 `assign`。
- [x] `src/api/ws.ts`：URL 改 `/ws/room/:roomId`；连接后首条 `auth` 消息；移除 `join` 命令。
- [x] `src/models/index.ts`：契约更新——`User(+username/roles)`、`Role`/`RolePermission`、`Message.candidate_id/sender_id?`、`Room.candidate_id?`、`Permission` 常量、`UserProfile`。
- [x] `src/composables/useRoomChat.ts` / `useCandidates.ts` 适配新 WS 与 API（拉取、候选人维度续传、编辑/删除/重置）。

## F. 前端 — 管理页面 ✅

- [x] `src/App.vue`：导航增「面试官管理」（仅 `users.manage` 显示）+ 当前用户名与退出按钮。
- [x] 候选人页 `CandidatesView.vue`：移除「分配」；新增「编辑」「删除」「重置状态」对话框（表单约束：向前档须已有房间、向后档提示解绑）+ 关键词搜索；按钮按权限显隐。
- [x] 房间页 `RoomView.vue`：空房间派生状态「空闲」；「新建房间」与「删除空房」（`rooms.manage`）。
- [x] `RoomChat.vue`：侧栏「拉取候选人」（待分配池，`candidates.assign`）、「成员管理」（添加/移除/设为主持，`rooms.manage`）、空房禁发消息并提示。
- [x] 新增 `src/views/UsersView.vue`（面试官管理）：面试官/角色页签；用户 CRUD + 角色勾选 + 重置密码；角色 CRUD + 权限组勾选（`users.manage`）。
- [x] 新增 `src/composables/useUsers.ts`；`useRoomList` 的房间状态投影支持空房。

## G. 清理与文档 ✅

- [x] 根 `README.md`：数据模型（users/roles/messages 归属/rooms 可空）、WS 协议（RESTful 路径 + auth 消息 + 候选人游标）、认证与 RBAC、接口清单（删 assign、加 auth/me/users/roles/status/pull/rooms 管理）、关键决策、测试清单。
- [x] `AGENTS.md`：校正架构描述（房间独立记录、消息按候选人、拉取式分配、RBAC 内存缓存、WS auth 消息）。
- [x] `apps/web/README.md`：当前用户段落改为登录鉴权说明。

## H. 验证 ✅

- [x] 后端：`GOCACHE="$PWD/apps/server/.gopath/gocache" go test -race ./...` 全绿（state + handler 两包）。
- [x] 前端：`pnpm --filter @interview-ng/web build`（`vue-tsc -b && vite build`）通过；根 `pnpm test` 通过。
- [x] 手工冒烟（真实 Postgres）：401/登录 admin（9 权限）→ 建面试官并赋 interviewer 角色 → 403 验证 → 建候选人/签到 → admin 建房 + 加成员 → interviewer 拉取 → admin 重置解绑 → 改密踢旧 token。全部断言通过。
