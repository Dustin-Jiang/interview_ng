# Repository Guidelines

## 项目概述

面试管理系统（interview_ng）：面试官协作面试、多部门录取决定、捡漏阶段按预算竞拍补录候选人。pnpm monorepo：

- `apps/web`（`@interview-ng/web`）：Vue 3 + Vite + TS + shadcn-vue 前端
- `apps/server`（Go module `interview_ng`，Gin + Gorm）：单进程后端，**已从 pnpm workspace 排除**（`pnpm-workspace.yaml` 中 `!apps/server`）
- 文档与代码注释一律使用中文

## 架构与数据流

后端分层（依赖单向向下）：

```
cmd/server/main.go（装配：AutoMigrate 15 表 → seed → rbac → state → auth → broadcast → service → 路由）
  handler（HTTP /api + WS /ws/rooms/:roomId、/ws/board；OIDC 的发现/授权/回调/探测由 internal/oidcauth 承担）
    → service.InterviewService（用例编排）
      → state.StateStore（MemStateStore，唯一权威）
        → gorm / Postgres
```

- **单点登录（OIDC）**：`internal/oidcauth`（发现文档 + provider 缓存 10 分钟、PKCE S256 授权跳转、回调换 token 并验签、ID token 声明上的 JMESPath 规则求值）；配置存 `oidc_configs`（单行懒建，范式同 `system_status`）+ `oidc_role_rules` 与 `oidc_dept_rules`（`position` 即优先级，保存时整体替换）。**两类规则共用一套求值核心**（`mapping.go` 的 `match` + `MatchRole`/`MatchDepartment`），语义都是「顺序求值、首个命中生效」：角色规则**未命中 = 拒绝登录**，部门规则**未命中 = 只保持账号现有部门**（部门不是权限，不能被空规则清掉），两类规则**求值失败一律拒绝登录**（`ErrRuleEval`，文案标明第几条角色/部门规则）。`state` 侧新增 `GetOidcConfig/SetOidcConfig/FindUserByOidcSubject/SyncOidcUser`（新增错误码见 `SetOidcConfig` 注释；`SyncOidcUser` 的 `departmentID` 为 nil 表示不动现有部门）。角色/显示名/部门以 IdP 为权威，每次登录覆盖。`auth.Manager` 依赖 `state`（自动开通走 `CreateUser`），故 `main.go` 中 `store` 必须先于 `auth.New` 构造。

- **事件流（出站）**：state 写操作在同一临界区内「先落库成功、后 `s.emit`」→ service 经 `broadcast.Manager.Publish` → handler 注册的 Sink → WS 客户端。事件带全局单调 `Seq`；消息带候选人维度 `msg_id` 作续传游标。事件类型与载荷见 `internal/state/event.go`。消息事件额外带发送者的展示名与**部门名**（`SenderName` / `SenderDepartment`，见 `AppendMessage`），历史消息则预加载 `Sender.Department`——聊天与归档的消息头部都显示「姓名 + 部门头衔 + 时间」（前端取 `domain/messages.ts#senderDepartmentLabel`，实时与历史两路归一到 `sender.department.name`）。
- **RBAC 旁路**：`internal/auth`（JWT + `RequireAuth`/`RequirePerm` 中间件）读 `internal/rbac/cache.go` 内存缓存（DB 权威 + 内存加速）；改密 bump `token_version` 踢旧 token。
- **候选人七档状态机**（`internal/model/candidate.go`）：`NOT_CHECKED_IN → CHECKED_IN_PENDING_ASSIGN → ASSIGNED → IN_PROGRESS → COMPLETED（面试已结束）→ ADMISSION_PENDING（待录取）→ ADMITTED（已录取）`，`StatusTransitions` 严格转移图 + `guardTransition`；状态写库统一走 `mem_store.go#statusUpdates`，据此维护 `interview_started_at`（进入面试中打点、离开即清空）与 **`checked_in_at`（签到那一刻在 `CheckIn` 里打点；只有重置回「未签到」才清空，其余流转一律不动）**。**候场队列顺序**由 `mem_store.go#compareWaiting` ≡ `domain/status.ts#compareWaiting` 唯一确定（改一处必须改另一处）：`waiting_priority` 升序（NULL 最后）→ `checked_in_at` 升序 → `created_at` 升序 → `id` 升序；未调序时即纯「先签到先叫号」。**手动调序**（候场大屏 ↑/↓，`PUT /api/candidates/:id/priority`，权限同签到）**只对「已签到待分配」档开放**（它是房间拉取池 `useWaitingQueue` 的 queue 的唯一数据源，其余档位没有读取方），与同档相邻一位交换、首次调序把整档固化成 1..n（**单事务**），已在档首/档尾为幂等 no-op（`200 {moved:false}`）；**`waiting_priority` 属于「排队会话」**——`CheckIn` 与重置回「未签到」都清空它（重新排队 = 回队尾），故不会留下无声插队的陈旧序号；顺序变更发 `candidate_priority_changed`（与资料编辑的 `candidate_updated` 分开）；**名册左栏（`RosterList`）的排序由组件自己统一应用**：`domain/status.ts#sortRoster` —— **没有面试记录的在前**（视作更早，按 `created_at` 添加顺序升序），面试过的在后（在面试用 `interview_started_at`、已结束用 `interview_completed_at`，升序）；故调用方只传筛选结果、不要再各自排序；不能改用 `created_at`（那是导入顺序）或 `updated_at`（资料编辑/导入会刷新它，一边排队一边导入就会打乱顺序）。
- **候选人学号**（`internal/model/candidate.go`）：`student_no` 是**身份键**——必填、纯数字（1–64 位；全角数字折半角、去首尾空白）、唯一（`uniqueIndex` 兜底），前导零有意义（`00123` ≠ `123`，文本列存储）；三条写入路径（`POST /api/candidates`、`PUT /api/candidates/:id`、导入）统一走 `ValidateStudentNo`，撞号 → `ErrStudentNoExists` → **409**。该列为 NOT NULL，**旧库必须重置**（AutoMigrate 只能建表/加列，给已有数据的表补 NOT NULL 列会 `contains null values`）。旧库启动即失败：`main.go#incompatibleSchemaHint` 识别该形状并打印处置提示，**默认拒绝自动清库**（部署与开发共用同一个 Postgres/`./data`），重建走 `DB_RESET=1`（或 `just db-reset`），**数据不可恢复**。
- **候选人资料字段**（`state.CandidateInfo` = 新建/编辑/导入共用契约）：`student_no / name / profile / first_choice（第一志愿）/ second_choice（第二志愿）/ accept_adjust（是否接受调剂，bool）/ phone / qq / email`。除学号外均可选；后端统一 `TrimSpace`（空白即空串）；编辑与导入为**全量覆盖**（零值清空旧值，`Updates` 走 map 以覆盖 `false`/`""`）。志愿与调剂另有**独立小权限** `candidates.preferences`（`interviewer` 默认持有）与专用入口 `PATCH /api/candidates/:id/preferences`（契约 = `state.CandidatePreferences`，只覆盖这三列，不触碰其他资料与运行态；三处界面入口共用 `CandidatePreferenceDialog`，其中第一/第二志愿是**部门下拉**——取值来自 `GET /api/departments`（浏览任意登录，增删改 users.manage），并保留候选人现有取值作为额外选项以免静默清空历史自由文本）。
- **批量导入**（`internal/state/mem_store.go` 的 `ImportCandidates` + `POST /api/candidates/imports`）：浏览器解析 Excel 并映射后提交行数组（行 = `CandidateImportRow`，内嵌 `CandidateInfo`，JSON 扁平；另可带可选 `updated_at` = 源表「更新时间」列）；服务端**单事务全或无**按学号 upsert（只写资料列，运行态不动；批内同学号后者覆盖前者），校验失败一次返回全部行级错误（`*state.ImportError` → `400 {error, rows:[{index,error}]}`）。**过期行保护**：行带 `updated_at` 且早于库中记录的 `updated_at` → 跳过不覆盖（报告 `status=skipped` + `stored_updated_at`，不写库不广播），比较基准是导入开始时的库中快照（故批内后行不因前行写回而被误判）；源表不给该列即不比较、照旧全量覆盖。前端镜像同一口径做预览（`domain/import.ts#mapSheet` 的 `existing: Map<学号, updated_at>` + `parseRowTimestamp`，预览/结果表都有「更新时间」列与「跳过」态）。
- **系统四阶段**（`internal/model/system_status.go`）：`interview / admission / leftover / settlement`。**进入结算阶段即按出价自动结算全部竞拍**（最高价部门录取、其余出价部门放弃、争议一并仲裁；幂等封盘），并批量同步录取档（唯一 admitted → 已录取，其余 → 待录取）；结算不提供逐个手动入口。
- **录取决定**（`candidate_admissions`，按部门分别记录）：`pending`（待定）/`admitted`（录取）/`withdrawn`（放弃）。**面试阶段起各部门就能在候选人查看页表态**（`useAdmissions` 的控件从面试阶段起展示；阶段策略是 `domain/admission.ts#isAdmissionRecordingPhase`，面试/录取/捡漏给入口、结算阶段不给）——**服务端不设阶段门**（`UpsertCandidateAdmission` 只校验候选人/部门/状态），阶段只决定界面给不给入口，别在后端另加阶段判断。**未记录的 (候选人 × 部门) = 未表态 → 视作弃权**（前端 `domain/admission.ts` 的预览与结论、`useAdmissions.othersOf` 跨部门徽章均按此补默认值），故「一家录取 + 其余无记录」= 已录取；只有**显式 `pending`** 才使结论保持未定。服务端判定一律只看 `admitted` 计数（预算/封盘/结算），未记录与弃权在后端等价，不为未表态部门落库。
- **捡漏竞拍**（`internal/model/bid.go`）：预算 `max(500, (预期人数−已录取)×100)`；**「已录取」只算唯一录取（封盘）的候选人**——争议（≥2 家 admitted）不计入已录取、不缩预算基数，且其出价照常占用 `spent`；只有唯一录取赢家的出价才随录取释放（`deptSpent` 的排除条件必须带「该候选人 admitted 数 == 1」）；出价跨部门保密（事件不带金额）；**出价 ≥ 0（0 合法，负数 `invalid_amount`）**，(candidate, department) 唯一、改价覆盖同一行（用 `found` 判存在，不能用金额当哨兵）；唯一 admitted 才算已结算封盘，多家录取属争议进捡漏仲裁；出价步长 `bid_step`（默认 10）存于系统状态单行；系统状态页在**捡漏/结算阶段**用 `GET /api/leftover/bids` + `GET /api/leftover/projections`（只读预览：赢家 = 当前最高出价部门，`resolved` = 已正式落库）展示「竞拍情况 + 预览录取结果」，无出价的候选人回退到决定矩阵结论。

前端（`apps/web/src`）为 MVVM 函数式，无 Pinia：

- `api/`：axios 单例（`/api` baseURL、Bearer 注入、401 统一登出）、各资源 Service 对象（`http.ts`）、WS 客户端 `ws.ts`（首条消息必须 auth，断线 1.5s 重连，重连后增量补拉）
- `composables/`：函数式 ViewModel；`useAsync(loader)` 是所有请求的基础原语（`{data, loading, error, run}`）；`useBoardChannel` 是唯一模块级单例 + 引用计数，`useBoardRefresh(events, cb)` 300ms 防抖重拉是列表页实时刷新标准模式；**`useSystemStatus` 是系统状态（阶段 / 出价步长）的唯一共享数据源**（模块级单例 + 并发去重 + 写入方落库后强制刷新），捡漏页/候选人页/系统状态页一律消费它，禁止各自再拉一份；`useOidcSettings` 收编登录认证设置页的配置/角色名单/部门名单（两类规则的目标下拉）/保存/探测/清除密钥；`useWaitingQueue` 是候场队列（「已签到待分配」档）的唯一共享数据源（模块级单例 + 引用计数 + 请求序号：按看板通道事件防抖重拉，并丢弃并发重拉时先到的旧响应）——房间侧栏的拉取池与候场大屏的档内顺序因此同源同序，**禁止各自再拉一份**；`useTheme` 是深色模式的唯一数据源（模式 + 解析结果 + 落 `<html>.dark`）
- `models/index.ts`：与后端 JSON 契约一一对应的纯类型层
- `domain/`：无 Vue 依赖的纯函数（状态机、消息合并、录取预览、**学号归一化校验**、**Excel 解析与 JMESPath 映射**（含 `parseAcceptAdjust`：是/接受/y/yes/true/1 → true）、**OIDC 错误码文案** `oidc.ts`（登录页用，不引 jmespath）、**OIDC 规则编译与试算** `oidcRules.ts`（泛型：角色与部门规则共用，只要求 `{expression}`）、**深色模式纯逻辑** `theme.ts`（模式解析/解析结果/存储键））
- `presenters/`：状态 → 中文标签 + Badge variant 的展示映射
- **数据导入页**（`/settings/imports`，需 `candidates.manage`）：`xlsx`（官方 CDN tarball，import `xlsx/dist/xlsx.mini.min.js`）与 `@jmespath-community/jmespath` **只在该分区的懒加载 chunk 里引入**；解析、映射、预览全在浏览器完成，服务端零新增依赖、无 multipart。JMESPath 里中文列名必须加引号（`"姓名"`），界面给出可复制的列名清单。映射结果按 JSON 转义集解释字面量转义（`\n`/`\t`/`\uXXXX`…，见 `domain/import.ts#interpretEscapes`），换行等字符因此可真显示；含反斜杠的文本（如路径）需写 `\\`。
- **登录认证页**（`/settings/authentication`，需 `users.manage`）：OIDC 连接参数（开关 / Issuer / 客户端 / Scopes / 回调地址 / 自动开通 + 连通性检测）+ 「声明 → 角色」「声明 → 部门」两张 JMESPath 规则表（共用泛型组件 `OidcRuleTable`，上移/下移即调优先级，保存时下标即 `position`；两类规则只差目标字段名 `role_id`/`department_id`，由 `targetOf`/`makeRule` 存取器吸收，本组件不碰字段名）+ 规则验证（`OidcClaimsPreview`，粘贴 ID token 声明同时试算两类规则，并分别给出「未命中」的后果）。**未命中的后果两类不同**：角色规则未命中 = 拒绝登录；部门规则未命中 = 不改变账号现有部门。**「回调地址」只让改主机**（`domain/oidc.ts` 的 `normalizeCallbackOrigin`/`callbackOriginOf` 负责归一与拆解），路径固定为 `api/http.ts#OIDC_CALLBACK_PATH`（= 后端 `GET /api/oidc/sessions`，注册到 IdP 的 redirect_uri 即「主机 + 该路径」的完整地址）。规则表达式**对声明缺失/类型不符要健壮**（对 `null` 取 `length()` 会求值失败 → `oidc_rule_eval_failed` + 服务端日志；求值错误包 `oidcauth.ErrRuleEval` 并带第几条、**角色/部门**规则与表达式原文）。客户端密钥**只写不读**（界面只显示是否已配置，清除走单独按钮）。`domain/oidcRules.ts` 是 `@jmespath-community/jmespath` 的第二个懒加载引用点；登录页只引 `domain/oidc.ts`（不引 jmespath）。
- **登录页双入口**：`GET /api/authentication` 返回登录方式开关，启用 OIDC 时密码表单上方显示「统一身份认证登录」（整页跳转 `/api/oidc/authorization`，非 XHR）；回调落地 `/login?oidc_code=…` 用一次性登录码换会话（`useAuth().completeOidc`），失败 `/login?oidc_error=…` 转中文提示。路由守卫对带 `oidc_code` 的登录页放行。

## 关键目录

| 路径 | 用途 |
|---|---|
| `apps/server/cmd/server/main.go` | 唯一入口：连接、迁移、装配、路由 |
| `apps/server/internal/handler/` | `http.go`（全部 REST 路由与 `stateErr` 错误映射）、`ws.go`（WS 泵与命令分发）、`hub.go`、`envelope.go` |
| `apps/server/internal/state/` | `store.go`（StateStore 接口 + `Error{Code,Msg}` + 哨兵错误）、`mem_store.go`（全部实现）、`event.go` |
|`apps/server/internal/{auth,rbac,broadcast,seed}/`|JWT/中间件、权限缓存、事件扇出、启动种子（幂等 reconcile）|
|`apps/server/internal/oidcauth/`|OIDC：`mapping.go`（JMESPath 编译/命中语义/顺序匹配（角色与部门共用一个求值核心）、用户名派生）、`flow.go`（state 与一次性登录码，单进程内存态）、`provider.go`（发现、授权、回调、探测，provider 缓存）|
| `apps/web/src/api/`、`composables/`、`models/`、`domain/`、`presenters/` | 见上 |
| `apps/web/src/views/` | 页面（组装层：只做筛选/展示派生 + 组合下方共享组件与 composable） |
| `apps/web/src/components/ui/` | shadcn-vue 组件，`index.ts` + cva 变体约定 |
| `apps/web/src/components/app/` | 自研业务外壳：`PageShell`、`EmptyState`、`SearchInput`、`ConfirmDialog`、`MessageTranscript`（通用的消息列表：回放 + 右键菜单 = 表情网格/回复人明细 + 复制消息 + 本人 2 分钟内的编辑·撤回；正在编辑这条时触发区禁用，右键/长按让回系统原生菜单）+ `MasterDetailSplit`、`RosterToolbar`（名册栏工具条：标题 + 元信息 + 动作）、`RosterList`、`RosterPager`、`CandidateDetailHeader`（名册↔详情布局）、`DataTableSection`（`nowrapHeaders` 收编表头不换行的包裹 div）、`FormDialog`、`RefreshButton`、`ListSkeleton`、`ErrorAlert`（列表/表单/状态骨架）、`FileDropInput`、`CandidateFormFields`、`CandidateStatusSelect`（候选人状态下拉：筛选带「全部」、重置状态不带）、`CandidateStatusRadio`（同契约的单选组，筛选浮层这类「一点即选」场景替代下拉）、`ClampText`（长文折叠 + Popover）、`CandidatePreferenceDialog`（志愿与调剂编辑，三处入口共用）、`CallNumberDialog`（候场大屏叫号弹窗）、`RoomSidebar`（面试房间左栏：候选人信息/简介/拉取/阶段控制）、`ImportFileStep`/`ImportMappingStep`/`ImportPreviewTable`/`ImportSubmitStep`（数据导入）、`OidcRuleTable`（登录认证：角色/部门规则表共用，泛型 + `targetOf`/`makeRule` 存取器）/`OidcClaimsPreview`（规则验证）、`ThemeToggle`（深色模式三档切换，顶栏与登录页共用） |

## 开发命令

```bash
pnpm dev              # 仅 web（vite，端口 3000）
pnpm dev:server       # Go 服务（:8080）
pnpm build            # web 构建（vue-tsc -b && vite build）
pnpm typecheck        # web 类型检查（vue-tsc -b --noEmit，走 project references）
pnpm test             # typecheck + go test ./...
pnpm test:server      # go test ./...（不带 -race）
just test-server      # go test -race ./...
just db               # 只起 Postgres（docker compose up -d postgres，或 podman compose …）
just db-reset         # 旧库 schema 不兼容时重建（DB_RESET=1 启动：删全部业务表，数据不可恢复）
podman compose up -d --build   # 部署整栈（单镜像 app + postgres），入口宿主机 :8080
```

- Go 命令**必须从 `apps/server/` 运行**，且设置 `GOCACHE="$PWD/.gopath/gocache"`（构建缓存进仓库内，已 gitignore）。推荐 `go test -race ./...`。
- Postgres：`docker-compose.yml`（postgres:16-alpine，库 `interview`，postgres/postgres，:5432）。Schema 由 Gorm AutoMigrate 在启动时创建——**没有迁移工具**。
- 部署：**单进程单镜像**——根 `Dockerfile`（多阶段：Vite 产物 + Go 二进制 → `alpine`），运行层只有后端一个进程：`WEB_ROOT=/srv/www` 让它同时提供前端产物，静态与接口同端口、天然同源，无前置 web 服务器。后端启动时自己重试连库（`DB_CONNECT_TIMEOUT`，默认 60s），故不依赖 compose 的启动顺序/healthcheck。`docker-compose.yml` 的 `app` 服务即该镜像，`postgres` 与开发共用。细节见 README「部署（容器，单进程单镜像）」。
- 服务端环境变量：`DATABASE_DSN`、`ADDR`（默认 `:8080`）、`JWT_SECRET`（缺省 dev 值）、`ADMIN_INIT_PASSWORD`（默认 `admin`）、`WEB_ROOT`（设置且目录内有 `index.html` 时由后端直接提供前端产物；不设则只提供 API/WS）、`DB_CONNECT_TIMEOUT`（启动时重试连库的上限，默认 60s，`0` = 不重试）、`DB_RESET`（`1` = 启动时先删全部业务表再 AutoMigrate，用于旧库 schema 不兼容时重建；**破坏性**，默认关闭，因为部署与开发共用同一个 Postgres）。无开放注册。

## 代码约定与常见模式

- **后端**
  - 所有写操作走 `MemStateStore`：`s.mu` 临界区内「先落库成功，后 emit 事件」，持久化失败绝不广播。
  - 错误：领域层返回 `&state.Error{Code, Msg}`（机器码如 `not_leftover_phase`/`budget_exceeded`）或哨兵错误；handler 用 `stateErr()` 统一映射 HTTP 状态码。
  - 状态变更必须经 `guardTransition` / `validStatus` 校验；房间是独立物理记录，绑定的唯一权威在 `rooms.candidate_id`，候选人 `room_id` 是只读投影；候选人完成即自动清房；消息按候选人归属、删除候选人级联删消息。**清房前先给候选人留档「这场面试在哪间房间做的」**（`interview_room_id` + 名字快照 `interview_room_name`，见 `mem_store.go#finishInterviewLocked` 收口的结档路径）——房间一旦解绑，这场面试发生在哪就再也查不到。候选人详情页据此显示「面试房间」行（没面完不显示）。
  - **消息通道只对「面试进行中」开放**：结档档位不止 `COMPLETED`——把**已绑定房间**的候选人重置到 `ADMISSION_PENDING`/`ADMITTED`（面试完成之后的录取档）同样自动解绑房间并留档，否则会停在「录取档却仍占着房间、还能继续写记录」。判据是 `model.CandidateStatus.Interviewing()`（`ASSIGNED`/`IN_PROGRESS`）：`AppendMessage` 对结档房间返回 `ErrInterviewFinished`（前端 `domain/status.ts#isInterviewing` 同一判据，房间输入区随之禁用），空房间仍返回 `NotFound`。
  - **房间成员是「此刻在场」，不是终身名册**：`room_members` 的一条行 = 一条 WS 连接（进房 `JoinRoom`、WS 断开的收尾 `RemoveRoomMember`），故 `main.go#clearRoomMembers` **启动时清空全表**——进程刚起来时没有任何连接，库里剩下的全是旧进程被强杀留下的陈旧席位（它们会挂在成员名单里，还让那间房因「仍有成员」删不掉）。**不限制面试官同时在几间房**：同一人可多标签页各守一间，同一间房重复进房（断线重连/刷新）幂等；`JoinRoom` 里原来那条「一人至多一活跃房间」的检查与 `state.ErrUserInRoom` 已彻底删除，不要再加回来。
  - **归档补充不受档位限制**：`AppendCandidateMessage`（`POST /api/candidates/:id/messages`，需 `rooms.chat`）只要求候选人存在——**不需要房间与在场成员**，故面试结档后仍可在候选人查看页补记录（房间通道服务进行中的面试，归档是那条记录的长期归宿）。消息落库与事件载荷统一走 `mem_store.go#appendMessageLocked`（内容 TrimSpace，空白 → `ErrInvalidContent`）；事件路由统一走 `#messageScopeRoomLocked`（候选人仍在房间内 → 按该房间扇出，否则全局扇出）——因此**房间页必须按 `CandidateID` 过滤**（`useRoomChat` 已做），否则别的候选人的归档会串进当前会话。
  - **记录可改可撤但有时限**：`EditMessage`/`DeleteMessage`（`PATCH|DELETE /api/candidates/:id/messages/:messageId`）只看三件事——消息属于该候选人、发送者是操作者本人、距 `created_at` 不超过 `state.MessageModifyWindow`（2 分钟，**编辑不延长窗口**；超时 → `ErrMessageWindowExpired`(409)、他人 → `ErrMessageNotOwner`(403)、错挂候选人/不存在 → `ErrNotFound`(404)）。撤回是**物理删除**，编辑只覆盖 `content`；两者都以 `message_updated`/`message_deleted`（载荷 `MessageRef{CandidateID, MessageID, Content}`）走同一路由扇出，前端据此就地改/撤（`domain/messages.ts#updateMessage`/`removeMessage`），**不要**靠重拉一次性解决。前端窗口口径镜像在 `MESSAGE_MODIFY_WINDOW_MS`（仅决定入口是否出现，服务端永远复核）。
  - **表情回复（`message_reactions`）是「旁注」而非记录**：`SetMessageReaction`（`PUT|DELETE /api/candidates/:id/messages/:messageId/reactions/:emoji`，需 `rooms.chat`）**不设时间窗口、不看档位、不看发送者**——只要求消息属于该候选人；`(message_id, user_id, emoji)` 联合唯一，开关都幂等，**状态未变化即返回 nil 事件（不广播）**；表情限 `model.ReactionEmojis`（24 枚，按「态度/评价/关注/其他」分组；前端 `REACTION_EMOJIS` 同口径**同顺序**镜像，改一处必须改另一处），之外 → `ErrInvalidReaction`(400)。载荷 `ReactionRef{CandidateID, MessageID, Emoji, UserID, Added}` **与观察者无关**（一条增量），故可直接广播给所有人、由各端自算计数与「我回没回」（读取路径 `ListMessagesAfter` 预加载 `Reactions.User.Department`，暴露 `{user_id, emoji, user}`；**事件载荷也带 `UserName`/`UserDepartment`**——两路都给足「谁回的」，前端不必查用户表）。撤回消息 / 删除候选人 / 删除用户都要**显式**连带清理表情行（不依赖 FK 级联在各库上的行为）。
- **前端**
  - 列表页模式：多个独立 `useAsync` 资源 + `useBoardRefresh([...事件], reloadAll)` + `RefreshButton`；筛选态同步 URL query、选中条目同步路径参数（`router.replace`，均可深链）。
  - **URL 一律 RESTful**：路径段只能是资源名词（集合用复数、条目 `/:id`、单例子资源用单数、多词 kebab-case），**禁止动词/动名词路径段**（`login`、`assign`、`pull_candidate`、`waiting` 这类历史写法一律改为资源操作）；集合项用 POST/GET/DELETE，单例子资源用 PUT，部分更新用 PATCH。
    - 页面路由（`router/index.ts`）：`/`、`/login`、`/candidates`、`/candidates/:candidateId`、`/board`（候场大屏，看板资源）、`/rooms`、`/rooms/:roomId`、`/leftover`、`/leftover/candidates/:candidateId`、`/settings/{candidates,imports,users,roles,departments,system/status,authentication}`；筛选/搜索态留在 query（`?q=`、`?status=`、`?room=`——`room` 是候选人查看页的面试房间筛选，取值 = 留档的 `interview_room_id`），选中条目走路径参数。**旧页面路径保留重定向**（`/waiting`、`/candidates/waiting`、`/room/:roomId`、`/candidates/manage`、`/users`），接口不留别名。顶部导航高亮按**路径归属**判定（`domain/nav.ts#isNavPathActive`：同路径或位于其 `/` 子层级下），**不要用 `route.name` 相等**——RESTful 下条目页/子页是集合页的**兄弟路由**（`/candidates/:id` 的 name 是 `candidate`、设置子页是 `settings-*`），`RouterLink` 自带的 `router-link-active` 同样失效（它要求目标 route record 出现在当前 `matched` 链里）。
    - 接口路由（`handler/http.go`）：`POST /api/sessions`（登录）、`/api/authentication`、`/api/me`、`/api/candidates`(+`/:id`、`GET|POST /:id/messages`、`PATCH|DELETE /:id/messages/:messageId`、`PUT|DELETE /:id/messages/:messageId/reactions/:emoji`、`/:id/check-in`、`/:id/priority`、`/:id/status`、`/:id/preferences`、`/imports`)、`/api/board/candidates`（候场大屏名单：只含未定局档位）、`/api/rooms`(+`/:id`、`/:id/members/:userId`、`/:id/candidate`)、`/api/users`(+`/:id`、`/:id/password`)、`/api/roles`、`/api/departments`、`/api/admissions/:candidateId`、`/api/leftover`(+`/bids/:candidateId`、`/projections`、`/results`)、`GET|PATCH /api/system/status`、`GET /api/oidc/authorization`、`GET|POST /api/oidc/sessions`、`GET|PUT /api/oidc/config`、`POST /api/oidc/probes`、`GET /api/health`；WS 通道 `/ws/rooms/:roomId`、`/ws/board`。
  - **录取表态的阶段窗口**：`useAdmissions`（候选人查看页的录取控件 + 名册徽章 + 1/2/3 快捷键）共用 `showControls`，其阶段部分来自 `domain/admission.ts#isAdmissionRecordingPhase`——面试阶段起可用，结算阶段不展示（竞拍已自动结算、录取档已批量同步）；服务端不设阶段门，所以**这条策略就是唯一判据**，不要在各视图里另写阶段判断。
  - **先复用后新写**：「左名册 + 右详情」用 `MasterDetailSplit` + `RosterToolbar` + `RosterList` + `RosterPager`（选中/键盘/深链状态在 `useRosterSelection`，键位在 `useRosterHotkeys`）；管理列表用 `DataTableSection`（骨架→空态→表格）；增改对话框用 `FormDialog` + `useEntityDialog`（`open/target/form/saving` + 校验/提交/成功提示一套收口）；二次确认用 `useConfirmAction` + `ConfirmDialog`；加载/错误/刷新用 `ListSkeleton` / `ErrorAlert` / `RefreshButton`；异常提示用 `lib/toast.ts` 的 `toastError`。视图层只保留筛选与展示派生。
  - **单文件行数**：视图/组件/组合式函数尽量 ≤ 400 行；超标即按上述原语拆分，避免超长文件难以维护。
  - WS 消息经 `domain/` 纯函数不可变更新（如 `mergeMessages` 按 id 去重升序、`updateMessage`/`removeMessage` 就地改撤、`replaceMessages` 按权威快照对齐）。**断线重连**：增量 sync 补新消息，另按 REST 归档快照 `replaceMessages` 对齐一次（补回断线期间别人对**已有消息**的编辑与表情回复）。
  - **消息列表只有一处实现**：`MessageTranscript`（房间实时聊天与归档回看共用）——结构参照 shadcn `Message`/`Bubble`：**名头（姓名 + 部门 + 时间）→ 气泡 → 气泡下方的表情胶囊行**；同一发送者 3 分钟内（`domain/messages.ts#MESSAGE_GROUP_GAP_MS`/`continuesGroup`）的连续消息归一组（`BubbleGroup` 手法：组内只首条出**整块名头（姓名 + 部门 + 时间）**，后续消息**连时间一起隐藏**（时间以 `sr-only` 留在无障碍树里，不占视觉），气泡上圆角相连 `chatBubbleVariants({ grouped })`、间距 4px 对组间 12px；不同发送者或间隔 >3 分钟 → 重新出名头与时间）；胶囊在常规流里排在气泡下方（行内 4px 间距），**不要**改成绝对定位去「贴合气泡边缘/气泡宽度」——气泡盒子是收缩宽度，短消息会把胶囊挤成几十像素宽、折成一列并倒压到名头上（实测：气泡 31.5px → 胶囊盒 34px 宽 / 82px 高）。**本组件不自带 live region**（`role=log` 由房间页滚动容器持有，避免嵌套播报区）。编辑/撤回、表情回复（`MessageReactions`）、就地编辑（`MessageEditor`）、输入区（`MessageComposer`）都拆成独立组件；新增消息相关界面一律复用它们，不要再手写一份气泡/输入区。**编辑/撤回入口是右键菜单**（`ui/context-menu`，shadcn 移植：鼠标右键、触屏长按、键盘菜单键同效）——只有「自己发送且 2 分钟内」的消息触发区可点（`domain/messages.ts#canModifyMessage`），编辑就地改（Enter 保存 / Esc 取消），撤回走 `useConfirmAction` + `ConfirmDialog`（不可逆）；动作成功只 `emit('changed')`，内容由持有方按权威重拉或就地更新。
  - **表情回复**：`MessageReactions`（气泡下沿的胶囊：每个表情一枚 + 计数，超过 4 个折叠成「+N」，参照 shadcn `BubbleReactions`），点胶囊/在右键菜单的表情网格里点表情都走同一个开关函数（`candidateApi.setReaction`）；触发区只要「可回表情」就允许右键（**与 2 分钟编辑窗口无关**），计数、「我回没回」与**回复人明细**都由 `domain/messages.ts#summarizeReactions` 聚合（`reactors` 带展示名与部门；事件路径靠载荷的 `UserName`/`UserDepartment` 拼最小 `user` 对象，与 `messageFromEvent` 的 sender 同一手法）；胶囊表面用 `bg-muted` + `border-foreground/20` 描边（**不要**用 `bg-background`/`border-border`：深色下 `--background` 与页面同色、`--border` 又与 `--muted` 同色，胶囊会融进背景看不见），「我回的」用实底 `bg-primary text-primary-foreground`（与页签激活态同一语言，两种主题下都最醒目）；菜单顶部是 6×4 表情网格（24 枚，与 `REACTION_EMOJIS` 同序）（**已回的表情用实底 + `role=menuitemcheckbox`/`aria-checked` 标出**，避免误点成撤回），下方列出「每个表情 ×N → 回复人（姓名·部门，自己显示「我」）」（`ContextMenuContent` 限高 `max-h-[var(--reka-context-menu-content-available-height)]` + `overflow-y-auto`，回复人多时可滚），纯展示区**不是菜单项**（避免方向键停在无动作的行上）——因此**「已有表情明细」也要让触发区可右键**（`hasReactions`），**所有人（含无 `rooms.chat` 的只读用户）都能看到明细**。事件若没带 `UserName`（旧服务端 / 该行查不到用户），前端会按 REST 快照对齐一次把姓名补回来——只显示「面试官 {id}」说明那一行压根没拿到用户资料。 菜单含「复制消息」+ 本人 2 分钟内的「编辑」「撤回」。**正在编辑这条时右键触发区整块禁用**：编辑框要的是系统原生菜单（选中 / 复制 / 粘贴），reka 的 `disabled` 语义是不 `preventDefault`、也不拦长按（实测：启用时 `contextmenu.defaultPrevented=true` 且弹出我们的菜单；禁用时`false`、无 `[role=menu]`，原生菜单照常），取消编辑只走 Esc 或气泡下方的取消按钮。**复制消息走 `lib/clipboard.ts#copyText`，不要裸调 `navigator.clipboard`**：本项目部署是 `:8080` 上的 http 直供（README「部署」），从别的机器用 `http://<内网IP>:8080` 打开时 `navigator.clipboard` 是 undefined、裸调会静默失效；`copyText` 在该情形退回 `execCommand('copy')` + 临时 textarea（实测内网 http 下 `hasClipboardApi=false` 仍复制成功、无残留节点；localhost 走主路径），返回布尔值由调用方决定成功/失败文案。导入页的「复制列名」也走它（同一实现，别再写第二份）。
  - **输入区只有一处实现**：`MessageComposer`（房间聊天与归档页「补充记录」共用）——受控草稿 + `disabled`（不可用原因写进 placeholder）+ `sending`（按钮转 Spinner、输入框禁用）+ Enter 发送；房间聊天另有「回到最新」浮标（上翻读历史时出现，`MessageScroller` 的 jump-to-latest）。
  - **减弱动效**：全局 `prefers-reduced-motion` 收敛动画/过渡（`assets/index.css`），但**保留 `animate-spin`**——冻住 Spinner 会被误读成卡死。
  - **视觉 token**：颜色/圆角/边框/阴影只能取自 `src/assets/index.css`（`:root`/`.dark` 语义变量）与 `tailwind.config.cjs` 语义映射（如 `bg-muted`、`text-muted-foreground`），**禁止硬编码 hex/rgb/hsl**。新组件沿用 `components/ui/<name>/index.ts` + cva 变体；状态只通过带文字标签的 Badge 传达，不依赖颜色单通道。
  - **动效令牌**：时长/缓动/位移/缩放/模糊五档只定义在 `assets/index.css` 的 `:root`（动效不随深浅色变化，**不要**复制进 `.dark`；全局 `prefers-reduced-motion` 收敛会自动覆盖它们驱动的动画），组件引用变量、**不要写死 ms / cubic-bezier**。选档按**用途**而非就近取数：弹层/弹窗开 = `--duration-fast`(250ms)、关 = `--duration-quick`(150ms)（**开关不对等**，关闭要更快让开视线）；缩放 弹窗 `--scale-large`(0.96)、弹层打开 `--scale-medium`(0.97)、弹层关闭 `--scale-tiny`(0.99)（小于 ~0.9 就成了「变焦」）；面板类位移统一 `--ease-smooth-out`。**Tailwind 写法有坑**：本项目装了 `tailwindcss-animate`，它注册的 `duration-*`/`ease-*` **不接受任意值**——`duration-[var(--duration-quick)]`、`ease-[var(--ease-smooth-out)]` 实测生成 0 条规则（写了等于没写），必须写成**任意属性** `[animation-duration:var(--duration-fast)]` / `[transition-duration:var(--duration-quick)]` / `[animation-timing-function:var(--ease-smooth-out)]`；插件的 `zoom-in-[0.97]`/`zoom-out-[0.99]` 任意值正常生成。**全站的动效表面都按这套取值**（实测口径）：右键菜单 `ui/context-menu` 开 250ms/0.97、关 150ms/0.99；`ui/dialog`（含 Scroll 变体）遮罩与面板 开 250ms/关 150ms、缩放 `--scale-large`(0.96)；`ui/popover` 与 `ui/select` 开 250ms/0.97、关 150ms/0.99（弹层的 8px 侧向位移 `slide-in-from-*-2` 正好 = `--distance-base`，保持）；缓动一律 `--ease-smooth-out`。**另一个坑：核心 `duration-*` 只写 `transition-duration`，不驱动 `animate-in/out` 的动画时长**——弹窗原来的 `duration-200` 因此完全无效（实测动画一直是插件默认 150ms、缓动是 CSS 初始 `ease`），必须用任意属性写法。弹窗面板的 `slide-*-top-[48%]` 与居中用的 `-translate-y-1/2` 几乎抵消（实测位移 ≈ 面板高 2%，68px 面板 ≈ 1.4px），保留不动。**不纳入令牌的**：骨架 `animate-pulse`、旋转指示 `animate-spin`（无限循环，没有对应时长档）、`vue-sonner` 自带的进出动画（库内部时序，覆盖它要绑其内部选择器，不碰）；各处 `transition-colors` 无显式时长 = Tailwind 默认 150ms，本就在 `--duration-quick` 上。
  - **深色模式**：三档「浅色 / 深色 / 跟随系统」（默认跟随系统）存 localStorage `interview_ng_theme`；`useTheme()` 把解析结果 toggle 到 `<html>` 的 `.dark` 类上（`domain/theme.ts` 是纯逻辑，`usePreferredDark` 供系统偏好，`useStorage` 负责持久化与跨标签页同步），切换入口 `ThemeToggle` 在**顶栏与登录页右上角**两处共用。组件**一律不写 `dark:` 变体**——`.dark` 覆盖同名语义变量即全站生效。`color-scheme` 随 `.dark` 一起切换（原生表单控件、原生滚动条因此跟随），`vue-sonner` 的 `Toaster` 绑 `resolved`。**首屏防白闪在 `index.html` 的内联脚本里按同一键与规则先应用一次——改动 `domain/theme.ts` 的键或规则必须同步改它**（内联脚本不能 import，那是唯一的第二份实现）。
  - **房间展示名**：任何界面都不显示房间编号，统一走 `domain/room.ts#roomLabel(room)`——入参是**房间对象**（不是 name 字符串），未命名/非字符串 name → 「未命名」；手上是房间对象时直接调用，只有手上仅有候选人 `room_id` 的地方用 `composables/useRoomNames.ts`（挂载拉取 + 房间 CRUD 事件重拉，映射里存的就是已解析的展示名）按 id 反查——目前只有**候选人详情（房间行 / 「进入房间」按钮）、候选人管理表（房间列）、候场大屏（房间列）**。**左侧名册（`RosterList`）一律不显示房间名**（按需求去掉；它的「按项 `#meta`」插槽因此已删除，名册行只有姓名 / 状态徽章 / 简介）。
  - **键盘**：键位放在**各页自己的 window 处理**里，但注册/注销与守卫收在 `useRosterHotkeys`（`useRosterSelection` 只提供状态与 `goPrev/goNext`，不注册全局键；名册也不做 ↑/↓ 列表导航）。两个名册页统一 **↑/↓ 切换候选人**；捡漏页另用 **←/→ 按步长调整报价**、**Enter 保存报价**——焦点在出价输入框内同样生效（该框只放数字、无光标需求，标记 `data-bid-input` 并作为 `isHotkeyInput` 传入），其它输入控件（搜索框等）与真按钮/链接一律让位（守卫 `lib/dom.ts#isEditableTarget` / `isActivatableElement`）。捡漏页的出价框是普通 `Input`（草稿即文本、输入即时同步），**不要换回 `ui/number-field`**：它自带 ↑/↓ 步进，会与新键位打架。
  - **移动端（本次约定，勿回退）**：布局结构按**宽度**判断——`md`(768) 是手机/桌面分界（底部导航 ↔ 顶栏横向导航、表格 ↔ 卡片），`lg`(1024) 留给宽桌面分栏；**触控尺寸按 `max-lg`（≤1023 视为触屏）**抬到 ≥44px（`ui/button` 各档、`ui/input`、`select/SelectTrigger`、`number-field`、`ui/tokens.ts` 的 navItem/segmented 已内置；手写 `h-7`/`h-8` 图标按钮要自己补 `max-lg:h-11 max-lg:w-11`。**例外一：消息表情胶囊**（`MessageReactions`，触屏取 `max-lg:min-h-8` = 32px，与 Material Chip 同档）——胶囊是「emoji + 计数」的内联小控件，44px 会把它拉成近乎方形的高块（比气泡还厚重），横向改由 `max-lg:px-3.5` 拉宽成药丸、宽度仍 ≥44px；**例外二：消息右键菜单**在触屏放宽到 `max-lg:min-w-[17rem]`（`MessageTranscript`），否则 6 列表情网格每格只有 30px 宽而高度 44px，细得不像可点的目标（17rem 下约 41×44 见方）。取舍：不按 `pointer: coarse` 判断，故 1024 宽的平板横屏仍按桌面紧凑尺寸）。外壳用 `h-dvh` + `env(safe-area-inset-*)`（顶栏/底部导航/登录页），`index.html` 的 viewport 必须保留 `viewport-fit=cover` 与 **`interactive-widget=resizes-content`**（后者让 Android Chrome 软键盘收缩布局视口，房间聊天的输入区靠它才不被遮）。`components/ui/table/data-table.vue` 在 **<md 自动把每行渲染成卡片**：字符串 `header` 作字段标签、`id: 'actions'` 的列铺在卡片底部，卡片内 `whitespace-normal`/`min-w-0` 会覆盖单元格自身的 nowrap——**不要写死 `whitespace-nowrap` 与之对抗**（矩阵表那种 `[&_td]:whitespace-nowrap` 必须限定 `md:`）。弹窗底板 `DialogContent` 已自带 `max-h-[calc(100dvh-1.5rem)]` + 内部滚动 + 手机留边，表单直接用即可。任何改动都不得引入整页横向滚动（宽内容进自己的 `overflow-x-auto`）。
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
  - `internal/oidcauth/mapping_test.go`（命中语义/首个命中（角色与部门）/求值失败归因/用户名派生）、`internal/state/oidc_test.go`（配置懒建/密钥三段语义/两类规则的校验与整体替换/按 sub 查号与显示名·角色·部门同步）、`internal/handler/oidc_test.go`（端到端：`httptest` 假 IdP + go-jose 真 RS256 ID token，覆盖 PKCE/nonce、state 与登录码一次性、未映射拒绝、部门写入与未命中保持、配置响应契约、密钥掩码、探测）
- 前端**没有测试框架**，`pnpm test:web` 只是 `vue-tsc -b --noEmit`——回归保障在 Go 测试与类型/构建检查。
- 验证标准：`cd apps/server && go test -race ./...` 全绿，`pnpm typecheck`、`pnpm build` 通过。**不需要在浏览器中做端到端验证**：不启动 :3000/:8080 跑 UI 断言、不截图、不造测试数据（界面效果由用户自行检查）。agent 只需保证上述命令通过，并说明改动了哪些界面/交互。
