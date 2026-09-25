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
├── Dockerfile                # 部署镜像（单进程：后端同时提供 API/WS 与前端产物）
├── docker-compose.yml        # 开发用 Postgres + 部署用整栈（postgres + app）
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
| 1. 多客户端 WS 长连接同步状态 | `internal/handler/ws.go` + `internal/broadcast`；房间通道按房间扇出，看板通道（`/ws/board`）扇出全部业务事件，列表页（候场大屏 `/board`、房间列表、候选人记录）据此防抖重拉实时刷新 |
| 2. 系统内部状态唯一 | `internal/state`；单 director 进程内存为权威，所有写经 `StateStore` 原子操作 + 五档状态机，非法转移拒绝 |
| 3. 良好的状态恢复 | 房间内消息 `id` 作续传游标；重连拉"房间快照 + 消息增量"；事件带稳定 `Seq` 幂等对齐；「先落库后广播」杜绝"客户端看到但库没有" |

### 已定的关键决策（对齐记录）
- **状态来源**：单 director 进程内存为权威，Gorm/Postgres 持久化；`StateStore` 接口化预埋未来切 Redis。
- **StateStore 语义**：只暴露业务级原子操作，返回不可变变更事件（不暴露内部结构）。
- **广播解耦**：`StateStore` 只产出事件；`BroadcastManager` 订阅事件按房间扇出；读给瞬时一致快照。
- **恢复**：消息 `id` 为主续传游标（**按候选人维度**）；事件带 `Seq` 幂等；「先落库成功 → 后广播」。增量 sync 只能补「新消息」——**已存在消息上的改动（编辑后的正文、别人的表情回复）补不回来**，故房间页重连后还会按 REST 归档快照再对齐一次（同 id 以权威为准，本地新到的保留），否则断线期间别人回的表情/改的字永远看不到。
- **消息/日志**：全部落库且**按候选人归属**（候选人换房历史随人走）；候选人删除级联删消息；面试官删除后其消息保留（sender 置空）。
- **房间**：独立于候选人的物理会议室记录（`candidate_id` 可空，可先建房后绑人、重置解绑后房保留）；**无房间状态机**，房间状态 = 候选人状态的查询投影；仅空房可删；不归档。**面试结束即留档房间**：推进到「面试已结束」时把当时绑定的房间写到候选人身上（`interview_room_id` + 名字快照 `interview_room_name`），随后才解绑——房间之后改名或删除也仍能回答「这场面试在哪间做的」，候选人详情页的「**面试房间**」行即此（没面完的候选人不显示该行）。
- **房间成员（席位 = 此刻在场）**：`room_members` 的一条行意味着「这个人现在在这间房里」——进房写入（WS `auth` 自动 JoinRoom，或管理端 `POST /api/rooms/:id/members`），WS 断开即退房；故后端**启动时清空整张表**（进程刚起来时没有任何连接，残留行都是旧进程被强杀留下的陈旧席位）。**不限制一个人同时在几间房**（可多标签页各守一间），同一间房重复进房（断线重连/刷新）幂等。
- **分配**：候选人被房间内面试官**拉取**（`PUT /api/rooms/:id/candidate`），取代"页面推分配"；并发拉取由状态机原子拒绝。房间侧栏的「拉取候选人」列表 = 当前处于「已签到待分配」的候选人，按**候场队列顺序**排列并标出 1..N 名次（数据源是 `useWaitingQueue`——候场队列的唯一共享组合式，按看板通道事件防抖重拉，与候场大屏档内同源同序）。**候场大屏 `/board` 同理**：按状态分档（正在面试 > 等待开始 > 等待分配 > 其他），档内用同一条候场队列顺序（`domain/status.ts#compareWaiting`）；它只拉「未定局」的档位（`GET /api/board/candidates`），已定局的录取档不参与这次拉取（已定局者本来也不上屏）。
- **候场队列顺序**（前后端唯一口径，`mem_store.go#compareWaiting` ≡ `domain/status.ts#compareWaiting`，改一处必须改另一处）：`waiting_priority` 升序（NULL 排在所有显式序之后）→ `checked_in_at` 升序 → `created_at` 升序 → `id` 升序。没有做过任何手动调序时，就是纯「先签到先叫号」（未签到者没有签到时刻，退到创建顺序）。
- **手动调序**（候场大屏操作列 ↑/↓，`PUT /api/candidates/:id/priority` `{direction:"up"|"down"}`，权限同签到 `candidates.checkin`）：**只对「已签到待分配」档开放**——它是候场队列的唯一消费方（房间拉取池）；正在面试的顺序由面试进程决定、未签到者还没到队，调了没有读取方。与同档相邻一位交换；任一侧没有显式序号时，把整档按当时显示顺序固化成 1..n（**单事务**，失败整体回滚），此后调序只在显式序之间互换，后续新签到（NULL）接在已固化成员之后。**手动序号属于「排队会话」**：重新签到或重置回「未签到」都会清空（重新排队 = 回队尾），因此离开档位后不会留下会无声插队的陈旧序号。已在档首/档尾是**幂等 no-op**（`200 {ok:true, moved:false}`，不是错误）。顺序变更发 `candidate_priority_changed`，大屏与房间侧栏据此防抖重拉。
- **数据导入**：设置页 `/settings/imports`（需 `candidates.manage`）三步导入候选人——① 选 `.xlsx`（单工作表，≤2000 行 / ≤5MB）② 每行成为 JSON 对象（首行表头为 key、值做类型推断、全空行跳过）并用 JMESPath 逐字段映射（中文列名须写成 `"列名"`，界面提供可复制列名清单）③ 确认提交；**解析与映射全在浏览器**（服务端零依赖、无 multipart），提交只发映射后的行，由 `POST /api/candidates/imports` 单事务全或无落库。学号取单元格**原始值**（格式化文本可能带千分位，会破坏纯数字校验）。映射结果会按 **JSON 字符串转义集**解释字面量转义（`\n`、`\t`、`\uXXXX` 等），因此「单元格内真实换行」与「字面量 `\n`」两种写法都能正确显示；未知转义（如正则里的 `\d`）原样保留，需保留反斜杠请写 `\\`。**「更新时间」列可选**：映射它以后，源表该行时间早于库中记录的行判为旧数据、**跳过不覆盖**（避免重复导入旧表格冲掉系统内的改动）；未映射即不比较，照旧全量覆盖；无法识别的时间按行级错误整批拒绝，不会静默关掉保护。
- **学号（身份键）**：`student_no` 必填、纯数字（1–64 位，全角数字按半角归一化、首尾空白去除）、唯一（DB 唯一索引兜底），前导零有意义（`00123` ≠ `123`）；新增/编辑/导入三条写入路径同一套校验，单条编辑允许改学号（撞号 → 409「学号已存在」，改号不影响运行态：房间绑定/消息/录取决定/出价均随候选人 id 保留）。**该列为 NOT NULL，故旧库必须重置**（AutoMigrate 无法给已有数据的表加 NOT NULL 列）：服务端启动时若发现这种旧库会**拒绝启动并打印处置提示**，`DB_RESET=1`（或 `just db-reset`）先删全部业务表再重建，**数据不可恢复**——这是唯一路径，不会自动清库（开发与部署共用同一个 Postgres）。
- **候选人与状态机**：五档状态 `NOT_CHECKED_IN → CHECKED_IN_PENDING_ASSIGN → ASSIGNED → IN_PROGRESS → COMPLETED` 为唯一权威；管理端支持"重置到任意档"（向后自动解绑房间、向前须已有房间）。
- **完成后自动清房**：候选人完成（无论房间内推进到 `COMPLETED`，还是管理端重置到 `COMPLETED`）自动清空房间绑定（`rooms.candidate_id` 置空，绑定唯一权威在房间侧），房间转空闲、成员留守，可立即拉取下一位候选人；消息仍**按候选人归档保留**，新候选人会话从零开始。
- **面试结档即关闭消息通道**：结档档位不止 `COMPLETED`——`ADMISSION_PENDING`（待录取）/`ADMITTED`（已录取）位于面试完成之后，把**已绑定房间**的候选人重置到这两档同样自动解绑房间（并留档「在哪间房间面的」）。房间消息只写给「面试进行中」（`ASSIGNED`/`IN_PROGRESS`）的候选人：结档后这段记录**只读归档**（空房间与已结档候选人的房间发消息均被拒，`model.CandidateStatus.Interviewing` 为唯一判据），界面上房间输入区同步禁用。**归档本身仍可写**：候选人查看页的「面试记录」卡带补充入口（`POST /api/candidates/:id/messages`，需 `rooms.chat`），结档后照样能补记录——房间通道服务进行中的面试，归档补充不依赖房间与在场成员。
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
- **model**：`User / Department / Candidate / Room / RoomMember / Message / SystemStatus / CandidateAdmission / Bid / OidcConfig / OidcRoleRule / OidcDeptRule` + 状态枚举/转移动图。

---

## 数据模型

| 表 | 关键列 | 角色 |
|---|---|---|
| `users` | id, username(唯一), password_hash, name, department_id, **oidc_subject(可空唯一)**, token_version | 面试官（登录用户，登录名唯一，管理端可改）；`oidc_subject` = 单点登录身份键（ID token 的 `sub`），密码用户为 NULL（Postgres 唯一索引允许多个 NULL） |
| `departments` | id, name(唯一), description | 部门（面试官所属组织单元） |
| `roles` | id, name(唯一), description | 角色（权限组，RBAC） |
| `role_permissions` | role_id, permission（联合唯一） | 角色↔权限关联 |
| `user_roles` | user_id, role_id（联合唯一） | 用户↔角色 M2M |
| `candidates` | id, **student_no(唯一, NOT NULL)**, name, profile, first_choice, second_choice, accept_adjust, phone, qq, email, status, **interview_started_at(可空)**, **interview_completed_at(可空)**, **checked_in_at(可空)**, **waiting_priority(可空)**, **interview_room_id(可空)**, **interview_room_name** | 候选人（非登录用户）；**学号 = 身份键**（纯数字、唯一、前导零有意义），志愿/联系方式为可选资料字段，当前房间归属为查询投影（`rooms.candidate_id` 主导，`room_id` 不落库）；`interview_started_at` = 进入「面试中」时打点、离开该档即清空（NULL = 不在面试中），前端面试计时以此为准（`updated_at` 会被任意资料编辑刷新，不能当计时起点）；`interview_completed_at` = 进入「面试已结束」时打点、**开始下一次面试才作废**（与 `interview_room_*` 快照同族：结档后仍是事实，名册里「面试过的那组按面试时间升序、排在没面试记录者之后」以它为准）；`checked_in_at` = **签到那一刻打点，只有重置回「未签到」才清空**（被拉进房间 / 开始面试 / 完成都不清空——大屏要让已签到的各档按到达先后排列）；房间「拉取候选人」列表与候场大屏据此先来后到（同理不能用 `updated_at`：导入/编辑资料会刷新它，排队顺序会被打乱）；`interview_room_id` / `interview_room_name` = **这场面试在哪间房间做的**，在推进到「面试已结束」的那一刻记录（房间随即解绑，不留档就查不到），名字存快照 —— 房间之后改名或删除仍能回答「当时在哪间」，房间删除时 id 置空、快照保留（候选人查看页的「面试房间」筛选取值即 `interview_room_id`，选项名优先用快照） |
| `rooms` | id, **name**, candidate_id(可空) | 房间 = 独立物理会议室记录，`name` 为可选别名（空串=未命名，UI 回退「房间 #id」，不要求唯一），`candidate_id` 可空；无状态机、无主持人，"状态"= 候选人状态的查询投影 |
| `room_members` | room_id, user_id（`idx_room_user` 唯一） | 房间成员（**席位 = 此刻在场**：进程启动时清空，WS 断开即退房；一人可同时在多间房） |
| `messages` | id, candidate_id, sender_id(可空), content | 群聊记录（长存），**按候选人归属**，`id` 即候选人维度续传游标 |
| `message_reactions` | message_id, user_id, emoji（三者联合唯一）, created_at | 记录上的表情回复：**一人对一条记录的同一表情只能一份**（重复提交幂等）；表情限 `model.ReactionEmojis` 允许集（24 枚，按「态度/评价/关注/其他」分组，前端同序镜像）；撤回消息 / 删除候选人 / 删除用户时显式连带清理 |
| `system_status` | id=1(单行), phase(interview/admission/leftover/settlement) | 系统状态：当前面试 / 录取 / 捡漏 / 结算阶段，管理端可切换 |
| `candidate_admissions` | candidate_id + department_id（联合唯一）, status(pending/admitted/withdrawn) | 各部门对候选人的录取决定（候选人无固定部门，按部门分别记） |
| `bids` | candidate_id + department_id（联合唯一）, amount | 捡漏阶段部门出价（candidate+department 唯一；金额对其他部门保密、事件不带金额，持 `candidates.browse_all` 的管理端跨部门可见） |
| `oidc_configs` | id=1(单行), enabled, issuer, client_id, client_secret, scopes, redirect_url, auto_provision | 单点登录（OIDC）配置（单行懒建，同 `system_status` 范式）；`client_secret` 明文存库但**读取接口只回 `client_secret_set` 布尔**，明文永不下发 |
| `oidc_role_rules` | id, position, expression, role_id | 「声明 → 角色」映射规则：对 ID token 声明求值 JMESPath 表达式，命中即赋予该角色；**自上而下、首个命中生效**（`position` 即界面行序，保存时整体替换） |
| `oidc_dept_rules` | id, position, expression, department_id | 「声明 → 部门」映射规则：同上语义，命中即把账号的 `department_id` 设为该部门；**未命中不拒绝登录**，只是不改动账号现有部门（部门不是权限） |

权限目录（12 枚）：`users.manage`、`candidates.manage`、`candidates.preferences`（修改候选人志愿与调剂）、`candidates.browse_all`（跨部门浏览录取状态与捡漏出价）、`candidates.create`、`candidates.checkin`、`candidates.assign`、`rooms.view`、`rooms.chat`、`rooms.move_phase`、`rooms.manage`、`admissions.record`（记录本部门录取决定/捡漏出价）。预置角色：`admin`（全部）、`interviewer`（8 枚流程权限：候选人创建/签到/拉取、志愿与调剂、房间浏览/聊天/推进阶段、本部门录取记录；不含资料与房间管理、跨部门浏览）。

**捡漏阶段**（`phase=leftover`）：各部门按预算竞拍补录候选人。预算 = `max(500, (预期人数 − 已确认录取人数) × 100)`（**只有唯一录取（封盘）才算已确认录取**；争议候选人不缩预算基数，其出价照常占用预算；唯一录取赢家的出价随录取释放、不重复占用）；出价受剩余预算约束（`PUT /api/leftover/bids/:candidateId`），**出价须为 ≥ 0 的整数**（0 是合法出价，即零元出价；同候选人 + 同部门唯一，改价即覆盖该行），各部门间出价金额互不可见、持 `candidates.browse_all` 的管理端可见全部门出价与各部门预算占用；`GET /api/leftover/results` 公开已结算的赢家与成交金额。

**结算阶段**（`phase=settlement`，进入即自动结算）：切换到结算阶段时后端自动把**全部有出价的竞拍按出价结算落库**——每个候选人最高出价部门录取（admitted）、其余出价部门 withdrawn，随后**归一化候选人状态**（唯一录取确定 → `ADMITTED`，其余录取档 → `ADMISSION_PENDING`），并 emit `leftover_resolved`。该过程**幂等**（已在库的唯一录取封盘 / 无出价者跳过，不重复成交、不重复广播）；结算时可逆地切回捡漏阶段继续出价/改价，再进结算阶段按最新出价重算。结算阶段竞拍数据只读——出价（`PUT /api/leftover/bids/:candidateId`）被拒绝（`not_leftover_phase`），手动逐个结算入口**已移除**（`POST /api/leftover/results` 不再存在）。`GET /api/leftover/projections` 保留为**未结算前的只读预演/校验**：由当前出价计算每个候选人的赢家与成交额，`resolved` 标记该结果是否已正式落库（与结算结果对照可用于自检）。

**录取决定与默认值**：录取决定按部门分别记录（`pending` 待定 / `admitted` 录取 / `withdrawn` 放弃）。**面试阶段起各部门就能在候选人查看页表态**（面试现场直接给结论，不必等到录取阶段）；界面入口的阶段策略在 `domain/admission.ts#isAdmissionRecordingPhase`（面试/录取/捡漏给入口，结算阶段不给），**服务端不设阶段门**（`UpsertCandidateAdmission` 只校验候选人/部门/状态，幂等）——阶段只决定界面是否给入口。**未记录的 (候选人 × 部门) 组合 = 该部门未表态 → 视作弃权**：不表态不影响录取判定，所以「一家录取 + 其余无记录」即「录取到该部门」；只有**显式记录的 `pending`** 才表示结论未定。服务端全部判定只看 `admitted` 计数（预算、封盘、结算），因此「未记录」与「弃权」在后端等价，无需为未表态的部门落库记录。

**录取争议仲裁**：已结算 = 恰好一家部门 admitted（唯一录取确定，封盘）；0 家未定可竞拍；≥2 家同时录取属争议——该候选人不算已结算、自动进入捡漏竞拍，进入结算阶段按出价自动仲裁：最高出价部门录取（admitted），其余部门（含未出价的手动录取记录）一律改 withdrawn。

**阶段切换同步**：切换到 捡漏/结算 阶段时批量同步录取档状态——恰好一家 admitted（唯一录取确定）→ `ADMITTED`（已录取）；其余（未定/争议/无决定）→ `ADMISSION_PENDING`（待录取）；未完成候选人不动。切换到结算阶段时**先按出价结算全部竞拍落库、再同步状态**，故自动成交的候选人随之推进到 `ADMITTED`。

---

## WS 协议（JSON 信封）

两条 WS 通道，连接建立后首条消息必须为 `auth`（携带 JWT），鉴权成功后才可收发；10 秒内未鉴权将断开。

**房间通道** `GET /ws/rooms/:roomId`（RESTful 路径，**不带任何 query 参数**）：要求 `rooms.chat` 权限，成功后自动 JoinRoom（同一间房幂等重入；**不限制一人在几间房**，多标签页可各守一间），收发房间业务命令。

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
{"type":"message_appended","room_id":3,"seq":12,"msg_id":101,"data":{"RoomID":3,"CandidateID":5,"SenderID":1,"SenderName":"张三","SenderDepartment":"技术部","Content":"hello"}}
{"type":"candidate_signed_in","room_id":0,"seq":11,"data":{"CandidateID":5}}
{"type":"reply","req_id":"r2","data":{"ok":true,...}}
```

> `seq` 全局单调事件序；`msg_id` 为**候选人维度**续传游标——消息按候选人归属，候选人换房后历史随人走。空房间（无候选人）可入房但 `send_msg` 会被拒（`not_found`）；面试已结档（`COMPLETED` 及其后的录取档）同样被拒（`interview_finished`，记录只读）。结档后仍要补记录走归档入口 `POST /api/candidates/:id/messages`（不需房间与在场成员；候选人无房间时该事件全局扇出，房间页按候选人过滤，不串档）。
> `message_appended` 同时带发送者展示名与**部门名**（`SenderName` / `SenderDepartment`，无部门为空串）：消息头部要显示「姓名 + 部门头衔 + 时间」，实时事件自带这两项，前端不必二次查询；历史消息（`GET /api/candidates/:id/messages` 与 `sync` 回执）同样预加载了 `sender.department`，故归档回放与实时聊天显示一致（`domain/messages.ts#senderDepartmentLabel`）。
> 捡漏类事件：`leftover_bid`（出价变更，载荷 `{CandidateID, DepartmentID}`，**不含金额**）、`leftover_resolved`（结算，载荷 `{CandidateID, DepartmentID, Amount}`）——均全局扇出。
>
> 事件类型：`candidate_signed_in`（全局）、`candidate_assigned`、`room_phase_changed`、`message_appended`、`message_updated`、`message_deleted`、`message_reactions_changed`、`member_joined/left`（以上带 room_id）、`candidate_created/updated/deleted`、`candidate_priority_changed`（候场队列手动调序）与 `room_created/deleted/renamed`（全局，载荷 `{CandidateID}` / `{RoomID}`）。
> 消息编辑 / 撤回（`message_updated` 载荷 `{CandidateID, MessageID, Content}`、`message_deleted` 载荷 `{CandidateID, MessageID}`）同样按「候选人所属房间」路由——归档页与房间页都能就地改/撤；**窗口 2 分钟且仅发送者本人**（`state.MessageModifyWindow`，`EditMessage`/`DeleteMessage` 复核，编辑不延长窗口）。
> 表情回复（`message_reactions_changed` 载荷 `{CandidateID, MessageID, Emoji, UserID, Added, UserName, UserDepartment}`）是**与观察者无关的一条增量**：各端据 `UserID`/`Added` 加减，计数、是否「我回的」与**回复人明细**（右键菜单「谁回了什么」）自行聚合——因此同一事件可直接广播给所有人。**事件自带回复人展示名与部门**（与 `message_appended` 带 `SenderName`/`SenderDepartment` 同理），只靠事件得知的回复也能显示姓名，前端不必查用户表。状态未变化的重复提交不广播（幂等）。

## 认证与鉴权

- 登录：`POST /api/sessions`（JWT，7 天有效，`JWT_SECRET` 环境变量配置）；无登录接口的旧版已移除。
- RBAC：角色↔权限存库，权限判断走**内存 RBAC 缓存**，角色/权限/改密变更即时生效（改密 bump `token_version`，旧 token 立即失效）。
- 错误语义：未登录/无效 token → **401**；有身份但缺权限 → **403**。
- 种子：启动时若无用户则创建 `admin`（全权限）+ `interviewer` 角色与默认账号 `admin/admin`（可用 `ADMIN_INIT_PASSWORD` 覆盖）。**不预置任何部门**——部门与用户归属由管理员在设置页显式创建/分配（admin 不隶属任何部门）。

### 单点登录（OIDC）

- 配置入口：「登录认证」设置分区（`/settings/authentication`，需 `users.manage`）：开关、Issuer、Client ID / Secret、Scopes、回调地址、是否自动开通，以及「声明 → 角色」「声明 → 部门」两张 **JMESPath 规则表**（`OidcRuleTable`）与规则验证（`OidcClaimsPreview`，粘贴 ID token 声明同时试算两类规则的命中结果）。
- 回调地址：界面**只填主机**（控制台与 `/api` 必须同源），路径固定为后端回调端点 `/api/oidc/sessions`；**注册到 IdP 的 redirect_uri 必须是该完整地址** `<主机>/api/oidc/sessions`。管理员只改主机的另一个原因：后端从回调地址的主机推导登录页来源（`spaOrigin`），换主机即换站点，路径却只能由本服务提供。
- 规则写法：**表达式必须容忍声明缺失/类型不符**——对 `null` 取 `length()` 之类会**求值失败**，整条登录链就此中断（错误码 `oidc_rule_eval_failed`，服务端日志会打出第几条**角色/部门**规则与表达式原文）。写法对比（同一份「ID token 无 `groups`」声明）：`length(groups[? ends_with(@, '-admin')]) > \`0\`` ❌ 求值失败；`type(groups) == 'array' && length(groups[? ends_with(@, '-admin')]) > \`0\`` ✅ 判为未命中；`groups[? ends_with(@, '-admin')] | [0]` ✅ 判为未命中。**声明本身要存在**：Keycloak 需给客户端加上 `groups` 作用域（Group Membership mapper，勾选 Add to ID token），否则 IdP 侧的组信息根本没进 ID token。
- 流程：`GET /api/oidc/authorization` 发现文档 → 生成 `state` / `nonce` / PKCE（S256，始终启用）→ 302 到 IdP；IdP 回调 `GET /api/oidc/sessions?code=&state=` → 换 token 并用 go-oidc 校验签名/iss/aud/exp（**nonce 由本服务手工比对**，库不校验）→ 命中角色规则（未命中即拒绝）与部门规则（未命中不动现有部门）→ 建号/同步 → 签发同款 JWT → 302 回前端并携带**一次性登录码**（60 秒有效，token 不出现在 URL）；前端 `POST /api/oidc/sessions {code}` 换取会话。`state`（10 分钟有效）与登录码均为**单次使用**。
- 身份与授权：身份键 = IdP 的 `sub`（`users.oidc_subject`，可空唯一）；显示名、角色与部门**以 IdP 为权威**，每次登录覆盖。**未命中任何角色规则 → 拒绝登录**（`oidc_role_unmapped`，无默认角色；要兜底就加一条恒真规则 `@`）；**未命中任何部门规则 → 只保持账号现有部门**（`users.department_id` 可被管理员手工设置，不能被空规则清掉）；两类规则**求值失败一律拒绝登录**（无法判定授权/归属就不放行）。首次登录按 `auto_provision` 自动建号（用户名取 `preferred_username` → 邮箱前缀 → `sub`，非法字符剔除并截断 64 字符；重名追加 `-<sub 前 6 位>`）。
- 密钥：`client_secret` 明文存库，读取接口只回 `client_secret_set`；保存时省略该字段 = 保持原值，`""` = 清除，非空 = 覆盖。
- 失败一律 302 回 `<前端源>/login?oidc_error=<码>`（`oidc_not_configured` / `oidc_discovery_failed` / `oidc_state_invalid` / `oidc_exchange_failed` / `oidc_nonce_invalid` / `oidc_claims_invalid` / `oidc_rule_eval_failed` / `oidc_role_unmapped` / `oidc_user_unknown` / `oidc_username_taken` / `oidc_login_failed`），登录页映射为中文提示；**换 token 与规则求值失败会在服务端记日志**（含底层原因，是唯一能看到细节的地方）。
- 流程与一次性登录码是**单进程内存态**：后端重启后在途登录失效，用户重新点按钮即可。本地密码登录**保留**，登录页同时展示两种方式；同名密码账号不会被 OIDC 接管（避免冒名）。规则映射**角色与部门**（不映射岗位等其他字段）；不使用 userinfo 端点（只用 ID token 声明）。

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
> **旧库起不来**（例如 `column "student_no" ... contains null values`，即 AutoMigrate 改不动的旧 schema）：
> `just db-reset`（等价于 `DB_RESET=1` 启动）先删全部业务表再重建，**数据不可恢复**。

前端（另一终端）：

```bash
pnpm install                  # 首次，在仓库根安装所有 workspace 依赖
pnpm dev                      # http://localhost:3000 （vite 已把 /api 与 /ws 代理到 :8080）
```

**深色模式**：顶栏与登录页右上角的主题按钮提供「浅色 / 深色 / 跟随系统」三档，默认跟随系统；选择存 localStorage（`interview_ng_theme`，跨标签页同步），首屏由 `index.html` 的内联脚本先行应用以免白屏闪烁。颜色全部走语义 token（`assets/index.css` 的 `.dark` 覆盖同名变量 + `color-scheme`），因此 `xlsx` 导入预览、弹窗、提示条、原生表单控件与原生滚动条都随主题切换。

**移动端适配**：手机（<768px）用**底部导航**替换顶栏横向导航（6 个目的地一屏可达，此前顶栏横滑会把末尾的「设置」藏出屏幕）；顶栏只留品牌/主题/退出，并避开刘海（`viewport-fit=cover` + `env(safe-area-inset-*)`）。管理页的宽表格在 <768px 自动变成**一记录一卡片**（字段标签来自列头，操作列铺底），无需各页单独适配——见 `components/ui/table/data-table.vue`；设置页在 <768px 把左侧栏换成顶部可横滑的分区标签条。触屏（≤1023px）下按钮/输入框/下拉一律 ≥44px，输入框按 16px 渲染（否则 iOS 聚焦会自动放大页面）。高度链用 `dvh`，viewport 声明 `interactive-widget=resizes-content`，因此软键盘弹出时房间聊天的输入区不被遮挡。

接口：
- `GET  /api/health`
- `POST /api/sessions` `{username, password}`（登录，签发 JWT）
- `GET  /api/me`（当前用户 + 角色 + 权限并集）
- `GET  /api/authentication`（**公共**：可用登录方式 `{password, oidc:{enabled}}`；配置读取失败一律按 `enabled=false` 返回）
- `GET  /api/oidc/authorization`（**公共**：302 跳到 IdP 授权端点；未配置完整 → 302 回登录页 `oidc_error=oidc_not_configured`）
- `GET  /api/oidc/sessions?code=&state=`（**公共**：IdP 回调，成功后 302 回 `<前端源>/login?oidc_code=<一次性登录码>`，失败 302 `?oidc_error=<码>`）
- `POST /api/oidc/sessions` `{code}`（**公共**：用一次性登录码换会话，响应与 `POST /api/sessions` 一致；失效 → 401）
- `GET  /api/oidc/config`（`users.manage`：OIDC 配置 + 两类规则 `role_rules` / `department_rules`（元素为 `{id, position, expression, role_id}` / `{…, department_id}`）；`client_secret` 明文**不下发**，只回 `client_secret_set`）
- `PUT  /api/oidc/config` `{enabled, issuer, client_id, client_secret?, scopes, redirect_url, auto_provision, role_rules:[{expression, role_id}], department_rules:[{expression, department_id}]}`（`users.manage`：整体覆盖保存，规则顺序即优先级；`client_secret` 省略 = 保持、`""` = 清除、非空 = 覆盖；校验失败 → 400 `oidc_issuer_invalid` / `oidc_client_id_required` / `oidc_redirect_url_invalid` / `oidc_scopes_invalid` / `oidc_rule_invalid` / `oidc_rule_role_missing` / `oidc_dept_rule_invalid` / `oidc_dept_rule_department_missing`）
- `POST /api/oidc/probes` `{issuer}`（`users.manage`：拉取发现文档做连通性自检，返回 `issuer` / `authorization_endpoint` / `token_endpoint` / `jwks_uri`）
- `PUT  /api/me/password` `{old_password, new_password}`（自助改密）
- `GET  /api/candidates?status=&q=&limit=&offset=`（管理面全集）
- `GET  /api/board/candidates?q=&limit=&offset=`（**候场大屏名单**：只含「未定局」的档位——面试已结束及其后的录取档不在其中，它们各有自己的页面；分页参数同 `/api/candidates`）
- `POST /api/candidates` `{student_no, name, profile?, first_choice?, second_choice?, accept_adjust?, phone?, qq?, email?}`（学号必填唯一；重复 → 409；文本字段服务端裁剪首尾空白）
- `GET  /api/candidates/:id`
- `PUT  /api/candidates/:id/check-in`（签到，无请求体）
- `PUT  /api/candidates/:id/priority` `{direction:"up"|"down"}`（**候场队列手动调序**，需 `candidates.checkin`：只对「已签到待分配」档有效，与同档相邻一位交换，首次调序把整档固化成显式序号；已在档首/档尾 → `200 {ok:true, moved:false}` 幂等 no-op，其余档位 → `400`）
- `PUT  /api/candidates/:id` `{student_no, name, profile?, first_choice?, second_choice?, accept_adjust?, phone?, qq?, email?}`（编辑，含学号；**全量覆盖**——缺省的可选字段按空串/false 清空旧值；撞号 → 409）
- `DELETE /api/candidates/:id`（级联删消息并解绑房间）
- `PATCH /api/candidates/:id/preferences` `{first_choice, second_choice, accept_adjust}`（**志愿与调剂**：独立权限 `candidates.preferences`（面试官默认持有），只覆盖这三列，其他资料与运行态不动；三项须完整给出，bool 无缺省语义）
- `PUT  /api/candidates/:id/status` `{status}`（重置到任意档：向后自动解绑、向前须已有房间）
- `GET  /api/candidates/:id/messages`（候选人历史面试记录归档，完成 / 换房后仍可查）
- `POST /api/candidates/:id/messages`（向归档补充一条记录，需 `rooms.chat`；**不依赖房间与在场成员**，面试结档后仍可写——这是「面试完成后仍要补记录」的唯一入口，房间通道只服务进行中的面试）
- `PATCH /api/candidates/:id/messages/:messageId` `{content}` / `DELETE /api/candidates/:id/messages/:messageId`（**编辑 / 撤回自己的记录**，需 `rooms.chat` + 发送者本人 + 距发送 ≤ 2 分钟：他人记录 → `403`，超窗口 → `409`，消息不在该候选人名下或不存在 → `404`；撤回为物理删除）
- `PUT|DELETE /api/candidates/:id/messages/:messageId/reactions/:emoji`（**表情回复**，需 `rooms.chat`；PUT 加上、DELETE 撤回，均幂等且**不设时间窗口**——任何档位、任何时间都能回；表情限允许集，之外 → `400`。消息的 `reactions` 字段随归档读取带出：`[{user_id, emoji, user}]`（`user` 为回复人展示名与部门，供右键菜单显示「谁回了什么」；不含凭据字段），计数与「我回没回」由前端按当前用户聚合）
- `POST /api/candidates/imports` `{rows:[{student_no, name, profile, first_choice, second_choice, accept_adjust, phone, qq, email, updated_at?}]}`（**批量导入**，需 `candidates.manage`）：单事务**全或无**，按学号 upsert（命中即覆盖全部资料列，值相同也写；批内同学号后者覆盖前者），只写资料列，运行态一律不动；`updated_at`（可选，RFC3339）早于库中该行 `updated_at` 的行判为**过期 → 跳过不覆盖**（不写库不广播，报告 `status:"skipped"` 并带 `stored_updated_at`；比较基准是导入开始时的库中快照，故批内后行不会因前行写回而被误判）；成功 `200 {created, updated, skipped, rows:[{index, status, candidate_id, stored_updated_at?}]}`，任一行的硬错误 → `400 {error, rows:[{index, error}]}`（整批未落库）。单次上限 2000 行
- `GET  /api/admissions`、`PUT /api/admissions/:candidateId` `{status}`（录取决定：默认本部门可见，记录需 `admissions.record`）
- `GET  /api/rooms`、`GET /api/rooms/:id`
- `POST /api/rooms` `{name?}`（建空房，`rooms.manage`；缺省/空串 = 未命名）
- `PATCH /api/rooms/:id` `{name}`（房间命名/改名，`rooms.manage`；空串 = 清除命名；超 64 字符 → 400）
- `DELETE /api/rooms/:id`（仅空房可删）
- `POST /api/rooms/:id/members` `{user_id}`、`DELETE /api/rooms/:id/members/:userId`
- `PUT  /api/rooms/:id/candidate` `{candidate_id}`（**拉取式分配**：候选人从待分配池被拉入房间）
- `GET/POST /api/users`、`PUT/DELETE /api/users/:id`、`PUT /api/users/:id/password` `{new_password}`（`users.manage`；后者为管理员重置他人密码）
- `GET/POST /api/roles`、`PUT/DELETE /api/roles/:id`（`users.manage`，角色管理）
- `GET /api/departments`（部门名单，**任意登录用户可读**——志愿下拉需要）、`POST /api/departments`、`PUT/DELETE /api/departments/:id`（`users.manage`，部门管理）
- `GET /api/system/status`（读取当前阶段，任意登录）、`PATCH /api/system/status` `{phase?, bid_step?}`（切换阶段 / 出价步长，`users.manage`，两项至少给一项）
- `GET /api/leftover`（捡漏总览：各部门预算，spent/remaining 默认仅本部门可见、`browse_all` 全可见）、`GET /api/leftover/bids`（默认本部门出价，`browse_all` 返回全部门）
- `PUT /api/leftover/bids/:candidateId` `{amount}`（出价/改价，`admissions.record`，仅捡漏阶段、受剩余预算约束；**amount ≥ 0**，0 合法，负数 → 400 `invalid_amount`）
- `GET /api/leftover/results`（已结算赢家与成交金额，全员可见；结算由切换到结算阶段自动触发）
- `GET /api/leftover/projections`（未结算前的只读预演：由出价计算的最终录取结果；保密语义与出价一致——默认仅已成交或本部门的进行中出价可见，`candidates.browse_all` 全量）
- `GET  /ws/rooms/:roomId`（房间通道，连接后首条 `auth` 消息）
- `GET  /ws/board`（看板通道，扇出全部业务事件供列表页实时刷新）

---

## 部署（容器，单进程单镜像）

`Dockerfile` 把 Vite 产物与 Go 二进制装进**同一个镜像**，运行起来只有**一个进程**：
Go 后端同时提供 `/api`、`/ws` 与前端产物（`WEB_ROOT=/srv/www`）。前端请求都走相对路径
（`/api`、`/ws`），与静态文件同端口、天然同源，**不需要 Caddy/nginx 之类的前置**，
也没有第二跳。纯 HTTP；要 TLS 就在更外层终结。

```bash
export JWT_SECRET=...            # 必填：登录令牌签发密钥（没有安全缺省值）
podman compose up -d --build     # 或 docker compose up -d --build
# 打开 http://localhost:8080 （WEB_PORT 可换宿主机端口），默认账号 admin / admin

podman compose logs -f app       # 后端日志（含「serving web assets from /srv/www」）
podman compose down              # 停止（数据在 ./data，不会被删）

podman build -t interview_ng:local .   # 只构建镜像
```

`WEB_ROOT` 是唯一的开关：设了且目录里有 `index.html` 才接管静态（未命中磁盘的 GET 回落
`index.html`，因此 `/candidates/:id`、`/settings/*` 深链刷新不 404）；不设则退化成「只提供
API/WS」——本地开发就是这样，前端仍走 Vite :3000。

要点：

- **启动顺序**：后端启动时自己重试连库（`DB_CONNECT_TIMEOUT`，默认 60s，`0` = 不重试），
  连上才继续 migrate/seed。compose 的 `depends_on` 只保证容器创建顺序，`podman-compose`
  也不等 `healthcheck` 变健康（实测 app 比 pg 首次 health 通过早约 15s 启动），首次 `initdb`
  期间必然连不上——所以这段等待放在进程里，不依赖编排工具的实现差异，也不依赖外层脚本。
- **密钥**：`JWT_SECRET` 缺失时 compose 直接报错（没有安全缺省值）；`ADMIN_INIT_PASSWORD`
  只在首次创建默认 admin 时生效。
- **端口**：只发布一个端口（默认宿主机 8080 → 容器 8080）。
- **可观测性（可选，OTLP → GreptimeDB）**：**不走环境变量**——部署后在管理面板
  「设置 → 可观测性」（users.manage）里配置 Endpoint / 库名 / Basic 账号密码（只写不读），
  保存即生效、无需重启；compose **不自带** greptime 服务，观察引擎自备（GreptimeDB
  standalone 的 HTTP :4000 即可，地址须 http(s):// 开头）。也可以用
  `OTEL_EXPORTER_OTLP_*` 环境变量作首次种子（仅当面板从没配过 endpoint 时生效）。
  **库不存在时后端会先自动建库**（`CREATE DATABASE IF NOT EXISTS`）：GreptimeDB 的「自动生成
  表结构」只覆盖表，不建库——往不存在的库推 OTLP 会 400 `Failed to find schema`，引擎也没有
  自动建库的配置项（实测 0.11.0 与 1.2.1 一致）。
  内容：HTTP RED 指标（`http_server_requests_total`、`http_server_request_duration_seconds`；后者
  按 Prometheus 兼容命名落成 `…_bucket` / `…_count` / `…_sum` 三张表 + `greptime_physical_table`，
  没有同名单表，查询时按拆出来的表名写）、
  WS 在线连接/消息/丢帧（`ws_connections`、`ws_messages_appended`、`ws_messages_dropped`），
  以及 slog 日志双写（stdout + GreptimeDB 的 `opentelemetry_logs` 表）；未配置/关闭时
  观测全程 noop 零开销。
- **数据库**：沿用开发用的 `./data` 绑定目录与同一套账号（`postgres/postgres`）。换库或改
  密码时，`DATABASE_DSN` 与 `postgres` 服务两处要同时改。**库 schema 落后于本版本时后端拒绝启动**
  （AutoMigrate 只会建表/加列，改不了既有列的约束），提示里给出处置办法；`DB_RESET=1` 启动
  会先删全部业务表再重建（`docker-compose.yml` 的 `app` 服务可临时加该变量），数据不可恢复。
- **缓存**：`/assets/*`（带内容哈希）回 `Cache-Control: immutable`，`index.html` 回 `no-cache`。
- **未压缩**：后端不发 `Content-Encoding`。实测（本机、回环、HTTP/1.1）292KB 打包产物
  gzip 后 105KB，但压缩侧吞吐从 66k rps 掉到 3.5k rps——内网部署不值得，故不做；若需要，
  给静态路由加一个 gzip 中间件即可（约 15 行）。

---

## 依赖说明

- 前端 `xlsx`（SheetJS，Apache-2.0）取自**官方 CDN tarball**（非 npm registry）：`https://cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz`，导入子路径 `xlsx/dist/xlsx.mini.min.js`（mini 构建，仅读 `.xlsx`，体积约为 full 的 1/3）。
- 前端 `@jmespath-community/jmespath`（MPL-2.0）用于导入页的字段映射表达式求值，以及设置页「登录认证」的规则验证试算（两处均在懒加载 chunk 内，不进入口包）。
- 后端导入功能**零新增依赖**：导入的行解析、映射与校验都在浏览器完成，服务端只做校验与事务落库。
- 后端 OIDC 依赖：`github.com/coreos/go-oidc/v3`（发现文档 + ID token 验签）、`golang.org/x/oauth2`（授权码 + PKCE）、`github.com/jmespath/go-jmespath`（规则表达式，保存时编译校验 + 回调时求值）。

---

## 测试

按层级分开目录，**并发/集成测试独立收集**，与源码解耦：

```
internal/state/mem_store_test.go    # 状态机/生命周期/订阅（源码包旁功能单测）
internal/state/manage_test.go       # 拉取并发、重置联动、级联删除、删房规则、用户/角色生命周期
internal/handler/handler_test.go    # 登录/me/401/403/权限矩阵 + WS(auth 消息→sync) 端到端
internal/state/import_test.go       # 学号校验/唯一冲突/导入 upsert（含批内覆盖、全或无回滚、量级上限、更新时间过期行跳过）/关键词检索
internal/handler/import_test.go     # 导入端点端到端（行级报告、整批回滚、403、409、updated_at 过期行契约）
internal/oidcauth/mapping_test.go   # JMESPath 命中语义/首个命中（角色与部门）、求值失败归因、用户名派生、声明取值
internal/state/oidc_test.go         # OIDC 配置懒建/密钥三段语义/两类规则校验与整体替换/按 sub 查号与显示名·角色·部门同步
internal/handler/oidc_test.go       # OIDC 端到端（假 IdP + 真 RS256 ID token：PKCE+nonce、登录码换会话、state 一次性、未映射拒绝、部门写入与未命中保持、配置响应契约、探测）
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
