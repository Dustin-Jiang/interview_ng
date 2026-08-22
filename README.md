# interview_ng — 高并发实时面试系统（Monorepo）

基于 **Gin + Gorm + Postgres + WebSocket** 的后端，配 **Vue 3 + shadcn-vue** 前端控制台。面试官可浏览面试者、分配面试者到房间、开始面试，并以群聊式交流记录面试信息。

按需求"高并发多客户端 WS 同步 + 系统内部状态唯一 + 良好的恢复、避免频繁状态不同步"设计。

---

## Monorepo 结构

采用 **pnpm workspaces**，目录布局为惯用的 `apps/` + `packages/`：

```
interview_ng/
├── apps/                     # 应用
│   ├── server/               # Go 后端（Gin + Gorm + Postgres + WebSocket）
│   │   ├── cmd/              #   main.go / wssmoke
│   │   ├── internal/         #   handler / service / state / broadcast / model
│   │   └── go.mod
│   └── web/                  # Vue 3 + shadcn-vue 前端控制台
│       ├── src/
│       │   ├── composables/  #   useXXX() 组合式函数（函数式 ViewModel）
│       │   ├── domain/       #   纯函数领域层
│       │   ├── api/          #   REST/WS Service
│       │   ├── views/        #   页面视图
│       │   └── components/   #   shadcn-vue UI 组件
│       └── package.json
├── packages/                 # 共享包（预留）
├── docker-compose.yml        # Postgres 基础服务
├── pnpm-workspace.yaml       # pnpm 工作区（apps/*、packages/*）
└── package.json              # 根：聚合 build/dev/test 脚本
```

根 `package.json` 提供跨应用统一脚本：

| 命令 | 说明 |
|---|---|
| `pnpm install` | 安装全部 workspace 依赖 |
| `pnpm build` | 构建 web + server |
| `pnpm dev` | 启动 web dev server |
| `pnpm dev:server` | 启动 Go 后端 |
| `pnpm typecheck` / `pnpm test` | 类型检查 / 全量测试 |



## 需求 → 设计映射

| 需求 | 落点 |
|---|---|
| 1. 多客户端 WS 长连接同步状态 | `internal/handler/ws.go` + `internal/broadcast`；WS 按房间连接，事件扇出到房间内所有连接 |
| 2. 系统内部状态唯一 | `internal/state`；单 director 进程内存为权威，所有写经 `StateStore` 原子操作 + 五档状态机，非法转移拒绝 |
| 3. 良好的状态恢复 | 房间内消息 `id` 作续传游标；重连拉"房间快照 + 消息增量"；事件带稳定 `Seq` 幂等对齐；「先落库后广播」杜绝"客户端看到但库没有" |

### 已定的关键决策（对齐记录）
- **状态来源**：单 director 进程内存为权威，Gorm/Postgres 持久化；`StateStore` 接口化预埋未来切 Redis。
- **StateStore 语义**：只暴露业务级原子操作，返回不可变变更事件（不暴露内部结构）。
- **广播解耦**：`StateStore` 只产出事件；`BroadcastManager` 订阅事件按房间扇出；读给瞬时一致快照。
- **恢复**：消息 `id` 为主续传游标（**按候选人维度**）；事件带 `Seq` 幂等；「先落库成功 → 后广播」。
- **消息/日志**：全部落库且**按候选人归属**（候选人换房历史随人走）；候选人删除级联删消息；面试官删除后其消息保留（sender 置空）。
- **房间**：独立于候选人的物理会议室记录（`candidate_id` 可空，可先建房后绑人、重置解绑后房保留）；**无房间状态机**，房间状态 = 候选人状态的查询投影；仅空房可删；不归档。
- **分配**：候选人被房间内面试官**拉取**（`pull_candidate`），取代"页面推分配"；并发拉取由状态机原子拒绝。
- **候选人与状态机**：五档状态 `NOT_CHECKED_IN → CHECKED_IN_PENDING_ASSIGN → ASSIGNED → IN_PROGRESS → COMPLETED` 为唯一权威；管理端支持"重置到任意档"（向后自动解绑房间、向前须已有房间）。
- **完成后自动清房**：候选人完成（无论房间内推进到 `COMPLETED`，还是管理端重置到 `COMPLETED`）自动清空房间绑定（`rooms.candidate_id` 与候选人 `room_id` 置空），房间转空闲、成员留守，可立即拉取下一位候选人；消息仍**按候选人归档保留**，新候选人会话从零开始。
- **鉴权**：登录 + JWT（7 天，`ver` 吊销计数）；RBAC 角色↔权限（9 枚权限目录），权限判断走内存缓存即时生效；`users.manage` 下可管理用户与角色。

---

## 分层架构

```
handler  →  service  →  state(StateStore)  →  model(Gorm/Postgres)
                          └→ broadcast(订阅事件流扇出)
```

- **handler**：Gin 路由、HTTP + WS 收发、JSON 信封编解、命令分发、续传同步。不碰库。
- **service**：`InterviewService` 业务编排；强制「先落库后广播」顺序。
- **state**：`StateStore` 接口 + `MemStateStore` 实现。状态唯一性权威、原子写、转移动图、内存快照读；内部经 Gorm 落库。
- **broadcast**：`Manager` 订阅事件流，按房间扇出到该房间所有 WS 写队列。
- **model**：`User / Candidate / Room / RoomMember / Message` + 状态枚举/转移动图。

---

## 数据模型

| 表 | 关键列 | 角色 |
|---|---|---|
| `users` | id, username(唯一), password_hash, name, token_version | 面试官（登录用户，登录名不可改） |
| `roles` | id, name(唯一), description | 角色（权限组，RBAC） |
| `role_permissions` | role_id, permission（联合唯一） | 角色↔权限关联 |
| `user_roles` | user_id, role_id（联合唯一） | 用户↔角色 M2M |
| `candidates` | id, name, profile, status, room_id | 候选人（非登录用户） |
| `rooms` | id, candidate_id(可空) | 房间 = 独立物理会议室记录，`candidate_id` 可空；无状态机、无主持人，"状态"= 候选人状态的查询投影 |
| `room_members` | room_id, user_id（`idx_room_user` 唯一） | 房间成员，一次一活跃房间 |
| `messages` | id, candidate_id, sender_id(可空), content | 群聊记录（长存），**按候选人归属**，`id` 即候选人维度续传游标 |

权限目录（9 枚）：`users.manage`、`candidates.manage`、`candidates.create`、`candidates.checkin`、`candidates.assign`、`rooms.view`、`rooms.chat`、`rooms.move_phase`、`rooms.manage`。预置角色：`admin`（全部）、`interviewer`（6 枚流程权限）。

---

## WS 协议（JSON 信封）

连接：`GET /ws/room/:roomId`（RESTful 路径，**不带任何 query 参数**）。连接建立后首条消息必须为 `auth`（携带 JWT），鉴权成功后才可收发业务命令；10 秒内未鉴权将断开。

请求（客户端 → 服务端）：
```json
{"op":"auth",        "req_id":"a1", "data":{"token":"<jwt>"}}
{"op":"sync",        "req_id":"r1", "data":{"last_msg_id":0}}
{"op":"send_msg",    "req_id":"r2", "data":{"content":"hello"}}
{"op":"move_phase",  "req_id":"r4", "data":{"to":"IN_PROGRESS"}}
```

> 连接已绑定房间（路径决定），故业务命令不再携带 room_id；操作者身份一律取自鉴权后的连接，不信任消息体。

服务端推送（事件 / 回复）：
```json
{"type":"message_appended","room_id":3,"seq":12,"msg_id":101,"data":{"RoomID":3,"CandidateID":5,"SenderID":1,"Content":"hello"}}
{"type":"reply","req_id":"r2","data":{"ok":true,...}}
```

> `seq` 全局单调事件序；`msg_id` 为**候选人维度**续传游标——消息按候选人归属，候选人换房后历史随人走。空房间（无候选人）可入房但 `send_msg` 会被拒。

## 认证与鉴权

- 登录：`POST /api/auth/login`（JWT，7 天有效，`JWT_SECRET` 环境变量配置）；无登录接口的旧版已移除。
- RBAC：角色↔权限存库，权限判断走**内存 RBAC 缓存**，角色/权限/改密变更即时生效（改密 bump `token_version`，旧 token 立即失效）。
- 错误语义：未登录/无效 token → **401**；有身份但缺权限 → **403**。
- 种子：启动时若无用户则创建 `admin`（全权限）+ `interviewer` 角色与默认账号 `admin/admin`（可用 `ADMIN_INIT_PASSWORD` 覆盖）。

---

## 本地运行

```bash
# 1) 启动 Postgres（podman 或 docker 均可）
podman-compose up -d          # 或 docker compose up -d

# 2) 运行服务（默认 :8080，可用 DATABASE_DSN/ADDR 覆盖），后端在 apps/server/ 下
cd apps/server
go run ./cmd/server           # 或从仓库根 pnpm run dev:server
```

> 首次启动自动建表 + 种子：默认账号 `admin / admin`（可用环境变量 `ADMIN_INIT_PASSWORD` 覆盖初始密码，`JWT_SECRET` 配置签发密钥）。

前端（另一终端）：

```bash
pnpm install                  # 首次，在仓库根安装所有 workspace 依赖
pnpm dev                      # http://localhost:8000 （vite 已把 /api 与 /ws 代理到 :8080）
```

接口：
- `GET  /api/health`
- `POST /api/auth/login` `{username, password}`
- `GET  /api/me`（当前用户 + 角色 + 权限并集）
- `POST /api/auth/password` `{old_password, new_password}`（自助改密）
- `GET  /api/candidates?status=&q=&limit=&offset=`
- `POST /api/candidates` `{name, profile?}`
- `GET  /api/candidates/:id`
- `POST /api/candidates/:id/checkin`
- `PUT  /api/candidates/:id` `{name, profile}`（编辑）
- `DELETE /api/candidates/:id`（级联删消息并解绑房间）
- `PUT  /api/candidates/:id/status` `{status}`（重置到任意档：向后自动解绑、向前须已有房间）
- `GET  /api/rooms`、`GET /api/rooms/:id`
- `POST /api/rooms`（建空房，`rooms.manage`）
- `DELETE /api/rooms/:id`（仅空房可删）
- `POST /api/rooms/:id/members` `{user_id}`、`DELETE /api/rooms/:id/members/:userId`
- `POST /api/rooms/:id/pull_candidate` `{candidate_id}`（**拉取式分配**：候选人从待分配池被拉入房间，取代旧的 `POST /api/candidates/:id/assign`）
- `GET/POST /api/users`、`PUT/DELETE /api/users/:id`、`POST /api/users/:id/reset_password`（`users.manage`）
- `GET/POST /api/roles`、`PUT/DELETE /api/roles/:id`（`users.manage`，角色管理）
- `GET  /ws/room/:roomId`（连接后首条 `auth` 消息）

---

## 测试

按层级分开目录，**并发/集成测试独立收集**，与源码解耦：

```
internal/state/mem_store_test.go    # 状态机/生命周期/订阅（源码包旁功能单测）
internal/state/manage_test.go       # 拉取并发、重置联动、级联删除、删房规则、用户/角色生命周期
internal/handler/handler_test.go    # 登录/me/401/403/权限矩阵 + WS(auth 消息→sync) 端到端
tests/
└── concurrency/                    # 仅文档、尚未实现
```

运行：
```bash
go test -race ./...                  # 全量含竞态检测
```

并发测试覆盖的断言：
- 同一候选人并发分配 / 并发推进阶段，仅一个成功（状态唯一）。
- 并发发消息：`id` 严格递增、无丢失、可增量续传。
- 多 WS 客户端并发发消息，每个客户端收到等量广播事件（db 消息数精确核对）。
- 断线重连：离线消息经 `last_msg_id` 增量补齐。

> 测试用内存 SQLite（`MaxOpenConns(1)` 共享库）；WS 用 `httptest.Server` + gorilla 客户端，不依赖真实 Postgres。

### 对运行中服务的手工冒烟（真实 Postgres 之上）

`cmd/wssmoke` 连入运行中的服务做端到端 WS 验证：

```bash
go run ./cmd/wssmoke -room 1 -who 1 -peer 2 -msgs 5                # 发消息+对端广播
go run ./cmd/wssmoke -room 1 -who 1 -mode resume -msgs 5           # 断线后增量续传
```

> 说明：`room_members.user_id` 外键引用 `users.id`，冒烟前须先存在对应面试官 user（可通过 psql 预置），否则 JoinRoom 会因外键失败。
