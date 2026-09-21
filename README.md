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
| 1. 多客户端 WS 长连接同步状态 | `internal/handler/ws.go` + `internal/broadcast`；房间通道按房间扇出，看板通道（`/ws/board`）扇出全部业务事件，列表页（候场大屏/房间列表/候选人记录）据此防抖重拉实时刷新 |
| 2. 系统内部状态唯一 | `internal/state`；单 director 进程内存为权威，所有写经 `StateStore` 原子操作 + 五档状态机，非法转移拒绝 |
| 3. 良好的状态恢复 | 房间内消息 `id` 作续传游标；重连拉"房间快照 + 消息增量"；事件带稳定 `Seq` 幂等对齐；「先落库后广播」杜绝"客户端看到但库没有" |

### 已定的关键决策（对齐记录）
- **状态来源**：单 director 进程内存为权威，Gorm/Postgres 持久化；`StateStore` 接口化预埋未来切 Redis。
- **StateStore 语义**：只暴露业务级原子操作，返回不可变变更事件（不暴露内部结构）。
- **广播解耦**：`StateStore` 只产出事件；`BroadcastManager` 订阅事件按房间扇出；读给瞬时一致快照。
- **恢复**：消息 `id` 为主续传游标（**按候选人维度**）；事件带 `Seq` 幂等；「先落库成功 → 后广播」。
- **消息/日志**：全部落库且**按候选人归属**（候选人换房历史随人走）；候选人删除级联删消息；面试官删除后其消息保留（sender 置空）。
- **房间**：独立于候选人的物理会议室记录（`candidate_id` 可空，可先建房后绑人、重置解绑后房保留）；**无房间状态机**，房间状态 = 候选人状态的查询投影；仅空房可删；不归档。
- **分配**：候选人被房间内面试官**拉取**（`PUT /api/rooms/:id/candidate`），取代"页面推分配"；并发拉取由状态机原子拒绝。
- **数据导入**：设置页 `/settings/imports`（需 `candidates.manage`）三步导入候选人——① 选 `.xlsx`（单工作表，≤2000 行 / ≤5MB）② 每行成为 JSON 对象（首行表头为 key、值做类型推断、全空行跳过）并用 JMESPath 逐字段映射（中文列名须写成 `"列名"`，界面提供可复制列名清单）③ 确认提交；**解析与映射全在浏览器**（服务端零依赖、无 multipart），提交只发映射后的行，由 `POST /api/candidates/imports` 单事务全或无落库。学号取单元格**原始值**（格式化文本可能带千分位，会破坏纯数字校验）。
- **学号（身份键）**：`student_no` 必填、纯数字（1–64 位，全角数字按半角归一化、首尾空白去除）、唯一（DB 唯一索引兜底），前导零有意义（`00123` ≠ `123`）；新增/编辑/导入三条写入路径同一套校验，单条编辑允许改学号（撞号 → 409「学号已存在」，改号不影响运行态：房间绑定/消息/录取决定/出价均随候选人 id 保留）。**该列为 NOT NULL，故旧库必须重置**（AutoMigrate 无法给已有数据的表加 NOT NULL 列）。
- **候选人与状态机**：五档状态 `NOT_CHECKED_IN → CHECKED_IN_PENDING_ASSIGN → ASSIGNED → IN_PROGRESS → COMPLETED` 为唯一权威；管理端支持"重置到任意档"（向后自动解绑房间、向前须已有房间）。
- **完成后自动清房**：候选人完成（无论房间内推进到 `COMPLETED`，还是管理端重置到 `COMPLETED`）自动清空房间绑定（`rooms.candidate_id` 置空，绑定唯一权威在房间侧），房间转空闲、成员留守，可立即拉取下一位候选人；消息仍**按候选人归档保留**，新候选人会话从零开始。
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
- **broadcast**：`Manager` 订阅事件流，按房间扇出到该房间所有 WS 写队列；`BindGlobal` 注册的全局订阅者（看板通道）接收所有事件。
- **model**：`User / Department / Candidate / Room / RoomMember / Message / SystemStatus / CandidateAdmission` + 状态枚举/转移动图。

---

## 数据模型

| 表 | 关键列 | 角色 |
|---|---|---|
| `users` | id, username(唯一), password_hash, name, department_id, token_version | 面试官（登录用户，登录名唯一，管理端可改） |
| `departments` | id, name(唯一), description | 部门（面试官所属组织单元） |
| `roles` | id, name(唯一), description | 角色（权限组，RBAC） |
| `role_permissions` | role_id, permission（联合唯一） | 角色↔权限关联 |
| `user_roles` | user_id, role_id（联合唯一） | 用户↔角色 M2M |
| `candidates` | id, **student_no(唯一, NOT NULL)**, name, profile, status | 候选人（非登录用户）；**学号 = 身份键**（纯数字、唯一、前导零有意义），当前房间归属为查询投影（`rooms.candidate_id` 主导，`room_id` 不落库） |
| `rooms` | id, candidate_id(可空) | 房间 = 独立物理会议室记录，`candidate_id` 可空；无状态机、无主持人，"状态"= 候选人状态的查询投影 |
| `room_members` | room_id, user_id（`idx_room_user` 唯一） | 房间成员，一次一活跃房间 |
| `messages` | id, candidate_id, sender_id(可空), content | 群聊记录（长存），**按候选人归属**，`id` 即候选人维度续传游标 |
| `system_status` | id=1(单行), phase(interview/admission/leftover/settlement) | 系统状态：当前面试 / 录取 / 捡漏 / 结算阶段，管理端可切换 |
| `candidate_admissions` | candidate_id + department_id（联合唯一）, status(pending/admitted/withdrawn) | 各部门对候选人的录取决定（候选人无固定部门，按部门分别记） |
| `bids` | candidate_id + department_id（联合唯一）, amount | 捡漏阶段部门出价（candidate+department 唯一；金额对其他部门保密、事件不带金额，持 `candidates.browse_all` 的管理端跨部门可见） |

权限目录（11 枚）：`users.manage`、`candidates.manage`、`candidates.browse_all`（跨部门浏览录取状态与捡漏出价）、`candidates.create`、`candidates.checkin`、`candidates.assign`、`rooms.view`、`rooms.chat`、`rooms.move_phase`、`rooms.manage`、`admissions.record`（记录本部门录取决定/捡漏出价）。预置角色：`admin`（全部）、`interviewer`（6 枚流程权限，不含录取状态浏览/记录）。

**捡漏阶段**（`phase=leftover`）：各部门按预算竞拍补录候选人。预算 = `max(500, (预期人数 − 已确认录取人数) × 100)`（已结算赢家的出价随录取释放，不重复占用）；出价受剩余预算约束（`PUT /api/leftover/bids/:candidateId`），各部门间出价金额互不可见、持 `candidates.browse_all` 的管理端可见全部门出价与各部门预算占用；`GET /api/leftover/results` 公开已结算的赢家与成交金额。

**结算阶段**（`phase=settlement`，进入即自动结算）：切换到结算阶段时后端自动把**全部有出价的竞拍按出价结算落库**——每个候选人最高出价部门录取（admitted）、其余出价部门 withdrawn，随后**归一化候选人状态**（唯一录取确定 → `ADMITTED`，其余录取档 → `ADMISSION_PENDING`），并 emit `leftover_resolved`。该过程**幂等**（已在库的唯一录取封盘 / 无出价者跳过，不重复成交、不重复广播）；结算时可逆地切回捡漏阶段继续出价/改价，再进结算阶段按最新出价重算。结算阶段竞拍数据只读——出价（`PUT /api/leftover/bids/:candidateId`）被拒绝（`not_leftover_phase`），手动逐个结算入口**已移除**（`POST /api/leftover/results` 不再存在）。`GET /api/leftover/projections` 保留为**未结算前的只读预演/校验**：由当前出价计算每个候选人的赢家与成交额，`resolved` 标记该结果是否已正式落库（与结算结果对照可用于自检）。

**录取争议仲裁**：已结算 = 恰好一家部门 admitted（唯一录取确定，封盘）；0 家未定可竞拍；≥2 家同时录取属争议——该候选人不算已结算、自动进入捡漏竞拍，进入结算阶段按出价自动仲裁：最高出价部门录取（admitted），其余部门（含未出价的手动录取记录）一律改 withdrawn。

**阶段切换同步**：切换到 捡漏/结算 阶段时批量同步录取档状态——恰好一家 admitted（唯一录取确定）→ `ADMITTED`（已录取）；其余（未定/争议/无决定）→ `ADMISSION_PENDING`（待录取）；未完成候选人不动。切换到结算阶段时**先按出价结算全部竞拍落库、再同步状态**，故自动成交的候选人随之推进到 `ADMITTED`。

---

## WS 协议（JSON 信封）

两条 WS 通道，连接建立后首条消息必须为 `auth`（携带 JWT），鉴权成功后才可收发；10 秒内未鉴权将断开。

**房间通道** `GET /ws/rooms/:roomId`（RESTful 路径，**不带任何 query 参数**）：要求 `rooms.chat` 权限，成功后自动 JoinRoom（一用户至多一活跃房间），收发房间业务命令。

**看板通道** `GET /ws/board`：要求 `rooms.view` 权限，不 JoinRoom、无成员语义、仅接受 `auth` 一条命令。扇出所有业务事件（房间级 + 全局级，各一份不重复），供候场大屏 / 房间列表 / 候选人记录等列表页感知变化后防抖重拉。

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
{"type":"candidate_signed_in","room_id":0,"seq":11,"data":{"CandidateID":5}}
{"type":"reply","req_id":"r2","data":{"ok":true,...}}
```

> `seq` 全局单调事件序；`msg_id` 为**候选人维度**续传游标——消息按候选人归属，候选人换房后历史随人走。空房间（无候选人）可入房但 `send_msg` 会被拒。
> 捡漏类事件：`leftover_bid`（出价变更，载荷 `{CandidateID, DepartmentID}`，**不含金额**）、`leftover_resolved`（结算，载荷 `{CandidateID, DepartmentID, Amount}`）——均全局扇出。
>
> 事件类型：`candidate_signed_in`（全局）、`candidate_assigned`、`room_phase_changed`、`message_appended`、`member_joined/left`（以上带 room_id）、`candidate_created/updated/deleted` 与 `room_created/deleted`（全局，载荷 `{CandidateID}` / `{RoomID}`）。

## 认证与鉴权

- 登录：`POST /api/sessions`（JWT，7 天有效，`JWT_SECRET` 环境变量配置）；无登录接口的旧版已移除。
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
pnpm dev                      # http://localhost:3000 （vite 已把 /api 与 /ws 代理到 :8080）
```

接口：
- `GET  /api/health`
- `POST /api/sessions` `{username, password}`（登录，签发 JWT）
- `GET  /api/me`（当前用户 + 角色 + 权限并集）
- `PUT  /api/me/password` `{old_password, new_password}`（自助改密）
- `GET  /api/candidates?status=&q=&limit=&offset=`
- `POST /api/candidates` `{student_no, name, profile?}`（学号必填唯一；重复 → 409）
- `GET  /api/candidates/:id`
- `PUT  /api/candidates/:id/check-in`（签到，无请求体）
- `PUT  /api/candidates/:id` `{student_no, name, profile}`（编辑，含学号；撞号 → 409）
- `DELETE /api/candidates/:id`（级联删消息并解绑房间）
- `PUT  /api/candidates/:id/status` `{status}`（重置到任意档：向后自动解绑、向前须已有房间）
- `GET  /api/candidates/:id/messages`（候选人历史面试记录归档，完成 / 换房后仍可查）
- `POST /api/candidates/imports` `{rows:[{student_no, name, profile}]}`（**批量导入**，需 `candidates.manage`）：单事务**全或无**，按学号 upsert（命中即覆盖姓名/简介，值相同也写；批内同学号后者覆盖前者），只写 `student_no/name/profile`，运行态一律不动；成功 `200 {created, updated, rows:[{index, status, candidate_id}]}`，任一行的硬错误 → `400 {error, rows:[{index, error}]}`（整批未落库）。单次上限 2000 行
- `GET  /api/admissions`、`PUT /api/admissions/:candidateId` `{status}`（录取决定：默认本部门可见，记录需 `admissions.record`）
- `GET  /api/rooms`、`GET /api/rooms/:id`
- `POST /api/rooms`（建空房，`rooms.manage`）
- `DELETE /api/rooms/:id`（仅空房可删）
- `POST /api/rooms/:id/members` `{user_id}`、`DELETE /api/rooms/:id/members/:userId`
- `PUT  /api/rooms/:id/candidate` `{candidate_id}`（**拉取式分配**：候选人从待分配池被拉入房间）
- `GET/POST /api/users`、`PUT/DELETE /api/users/:id`、`PUT /api/users/:id/password` `{new_password}`（`users.manage`；后者为管理员重置他人密码）
- `GET/POST /api/roles`、`PUT/DELETE /api/roles/:id`（`users.manage`，角色管理）
- `GET/POST /api/departments`、`PUT/DELETE /api/departments/:id`（`users.manage`，部门管理）
- `GET /api/system/status`（读取当前阶段，任意登录）、`PATCH /api/system/status` `{phase?, bid_step?}`（切换阶段 / 出价步长，`users.manage`，两项至少给一项）
- `GET /api/leftover`（捡漏总览：各部门预算，spent/remaining 默认仅本部门可见、`browse_all` 全可见）、`GET /api/leftover/bids`（默认本部门出价，`browse_all` 返回全部门）
- `PUT /api/leftover/bids/:candidateId` `{amount}`（出价/改价，`admissions.record`，仅捡漏阶段、受剩余预算约束）
- `GET /api/leftover/results`（已结算赢家与成交金额，全员可见；结算由切换到结算阶段自动触发）
- `GET /api/leftover/projections`（未结算前的只读预演：由出价计算的最终录取结果；保密语义与出价一致——默认仅已成交或本部门的进行中出价可见，`candidates.browse_all` 全量）
- `GET  /ws/rooms/:roomId`（房间通道，连接后首条 `auth` 消息）
- `GET  /ws/board`（看板通道，扇出全部业务事件供列表页实时刷新）

---

## 依赖说明

- 前端 `xlsx`（SheetJS，Apache-2.0）取自**官方 CDN tarball**（非 npm registry）：`https://cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz`，导入子路径 `xlsx/dist/xlsx.mini.min.js`（mini 构建，仅读 `.xlsx`，体积约为 full 的 1/3）。
- 前端 `@jmespath-community/jmespath`（MPL-2.0）用于导入页的字段映射表达式求值。
- 后端**零新增依赖**：导入的行解析、映射与校验都在浏览器完成，服务端只做校验与事务落库。

---

## 测试

按层级分开目录，**并发/集成测试独立收集**，与源码解耦：

```
internal/state/mem_store_test.go    # 状态机/生命周期/订阅（源码包旁功能单测）
internal/state/manage_test.go       # 拉取并发、重置联动、级联删除、删房规则、用户/角色生命周期
internal/handler/handler_test.go    # 登录/me/401/403/权限矩阵 + WS(auth 消息→sync) 端到端
internal/state/import_test.go       # 学号校验/唯一冲突/导入 upsert（含批内覆盖、全或无回滚、量级上限）/关键词检索
internal/handler/import_test.go     # 导入端点端到端（行级报告、整批回滚、403、409）
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
