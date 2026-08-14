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
- **恢复**：每房间消息 `id` 为主续传游标；事件带 `Seq` 幂等；「先落库成功 → 后广播」。
- **消息/日志**：全部落库（`messages` 长存），另存房间快照（`rooms`）避免频繁重建。
- **候选人与状态机**：候选人非登录用户；五档状态 `NOT_CHECKED_IN → CHECKED_IN_PENDING_ASSIGN → ASSIGNED → IN_PROGRESS → COMPLETED`。
- **房间**：一对一候选**人**（多面试官对一个候选人）；`room_members.user_id` 唯一约束保证一次一个活跃房间。

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
| `users` | id, name, role | 面试官（登录用户） |
| `candidates` | id, name, profile, status, room_id | 候选人（非登录用户） |
| `rooms` | id, candidate_id, current_interviewer_id | 面试房间（一对一候选人） |
| `room_members` | room_id, user_id（`idx_room_user` 唯一） | 房间成员，一次一活跃房间 |
| `messages` | id, room_id, sender_id, content | 群聊记录（长存），`id` 即续传游标 |

---

## WS 协议（JSON 信封）

请求（客户端 → 服务端）：
```json
{"op":"sync",       "req_id":"r1", "data":{"room_id":3,"last_msg_id":0,"last_seq":0}}
{"op":"send_msg",   "req_id":"r2", "data":{"room_id":3,"content":"hello"}}
{"op":"join",       "req_id":"r3", "data":{"room_id":3}}
{"op":"move_phase", "req_id":"r4", "data":{"room_id":3,"to":"IN_PROGRESS"}}
```

连接：`GET /ws/room?user_id=1&room_id=3`（建立时若尚非成员则自动 JoinRoom）。

服务端推送（事件 / 回复）：
```json
{"type":"message_appended","room_id":3,"seq":12,"msg_id":101,"data":{...}}
{"type":"reply","req_id":"r2","data":{"ok":true,...}}
```

> `seq` 全局单调事件序，`msg_id` 房间内消息游标——客户端同时用两者做重连续传与幂等去重。

---

## 本地运行

```bash
# 1) 启动 Postgres（podman 或 docker 均可）
podman-compose up -d          # 或 docker compose up -d

# 2) 运行服务（默认 :8080，可用 DATABASE_DSN/ADDR 覆盖），后端在 apps/server/ 下
cd apps/server
go run ./cmd/server           # 或从仓库根 pnpm run dev:server
```

前端（另一终端）：

```bash
pnpm install                  # 首次，在仓库根安装所有 workspace 依赖
pnpm dev                      # http://localhost:8000 （vite 已把 /api 与 /ws 代理到 :8080）
```

接口：
- `GET  /api/health`
- `GET  /api/candidates?status=&limit=&offset=`
- `POST /api/candidates` `{name, profile?}`
- `GET  /api/candidates/:id`
- `POST /api/candidates/:id/checkin`
- `POST /api/candidates/:id/assign` `{room_id?}`
- `GET  /api/rooms`
- `GET  /api/rooms/:id`
- `GET  /ws/room?user_id=&room_id=`

---

## 测试

按层级分开目录，**并发/集成测试独立收集**，与源码解耦：

```
internal/state/mem_store_test.go     # 源码包旁的功能单测（状态机/生命周期/订阅）
tests/
└── concurrency/
    ├── store_concurrent_test.go     # StateStore 并发断言（-race）
    └── ws_e2e_test.go               # WS 多客户端端到端并发 + 断线续传
```

运行：
```bash
go test -race ./...                  # 全量含竞态检测
go test -race -v ./tests/concurrency  # 单独跑并发套件
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
