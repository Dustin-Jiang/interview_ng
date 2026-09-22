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

- **事件流（出站）**：state 写操作在同一临界区内「先落库成功、后 `s.emit`」→ service 经 `broadcast.Manager.Publish` → handler 注册的 Sink → WS 客户端。事件带全局单调 `Seq`；消息带候选人维度 `msg_id` 作续传游标。事件类型与载荷见 `internal/state/event.go`。
- **RBAC 旁路**：`internal/auth`（JWT + `RequireAuth`/`RequirePerm` 中间件）读 `internal/rbac/cache.go` 内存缓存（DB 权威 + 内存加速）；改密 bump `token_version` 踢旧 token。
- **候选人七档状态机**（`internal/model/candidate.go`）：`NOT_CHECKED_IN → CHECKED_IN_PENDING_ASSIGN → ASSIGNED → IN_PROGRESS → COMPLETED（面试已结束）→ ADMISSION_PENDING（待录取）→ ADMITTED（已录取）`，`StatusTransitions` 严格转移图 + `guardTransition`；状态写库统一走 `mem_store.go#statusUpdates`，据此维护 `interview_started_at`（进入面试中打点、离开即清空）。
- **候选人学号**（`internal/model/candidate.go`）：`student_no` 是**身份键**——必填、纯数字（1–64 位；全角数字折半角、去首尾空白）、唯一（`uniqueIndex` 兜底），前导零有意义（`00123` ≠ `123`，文本列存储）；三条写入路径（`POST /api/candidates`、`PUT /api/candidates/:id`、导入）统一走 `ValidateStudentNo`，撞号 → `ErrStudentNoExists` → **409**。该列为 NOT NULL，**旧库必须重置**（AutoMigrate 无法给已有数据的表加 NOT NULL 列）。
- **候选人资料字段**（`state.CandidateInfo` = 新建/编辑/导入共用契约）：`student_no / name / profile / first_choice（第一志愿）/ second_choice（第二志愿）/ accept_adjust（是否接受调剂，bool）/ phone / qq / email`。除学号外均可选；后端统一 `TrimSpace`（空白即空串）；编辑与导入为**全量覆盖**（零值清空旧值，`Updates` 走 map 以覆盖 `false`/`""`）。志愿与调剂另有**独立小权限** `candidates.preferences`（`interviewer` 默认持有）与专用入口 `PATCH /api/candidates/:id/preferences`（契约 = `state.CandidatePreferences`，只覆盖这三列，不触碰其他资料与运行态；三处界面入口共用 `CandidatePreferenceDialog`，其中第一/第二志愿是**部门下拉**——取值来自 `GET /api/departments`（浏览任意登录，增删改 users.manage），并保留候选人现有取值作为额外选项以免静默清空历史自由文本）。
- **批量导入**（`internal/state/mem_store.go` 的 `ImportCandidates` + `POST /api/candidates/imports`）：浏览器解析 Excel 并映射后提交行数组（行 = `CandidateImportRow`，内嵌 `CandidateInfo`，JSON 扁平）；服务端**单事务全或无**按学号 upsert（只写资料列，运行态不动；批内同学号后者覆盖前者），校验失败一次返回全部行级错误（`*state.ImportError` → `400 {error, rows:[{index,error}]}`）。
- **系统四阶段**（`internal/model/system_status.go`）：`interview / admission / leftover / settlement`。**进入结算阶段即按出价自动结算全部竞拍**（最高价部门录取、其余出价部门放弃、争议一并仲裁；幂等封盘），并批量同步录取档（唯一 admitted → 已录取，其余 → 待录取）；结算不提供逐个手动入口。
- **录取决定**（`candidate_admissions`，按部门分别记录）：`pending`（待定）/`admitted`（录取）/`withdrawn`（放弃）。**未记录的 (候选人 × 部门) = 未表态 → 视作弃权**（前端 `domain/admission.ts` 的预览与结论、`useAdmissions.othersOf` 跨部门徽章均按此补默认值），故「一家录取 + 其余无记录」= 已录取；只有**显式 `pending`** 才使结论保持未定。服务端判定一律只看 `admitted` 计数（预算/封盘/结算），未记录与弃权在后端等价，不为未表态部门落库。
- **捡漏竞拍**（`internal/model/bid.go`）：预算 `max(500, (预期人数−已录取)×100)`；**「已录取」只算唯一录取（封盘）的候选人**——争议（≥2 家 admitted）不计入已录取、不缩预算基数，且其出价照常占用 `spent`；只有唯一录取赢家的出价才随录取释放（`deptSpent` 的排除条件必须带「该候选人 admitted 数 == 1」）；出价跨部门保密（事件不带金额）；**出价 ≥ 0（0 合法，负数 `invalid_amount`）**，(candidate, department) 唯一、改价覆盖同一行（用 `found` 判存在，不能用金额当哨兵）；唯一 admitted 才算已结算封盘，多家录取属争议进捡漏仲裁；出价步长 `bid_step`（默认 10）存于系统状态单行；系统状态页在**捡漏/结算阶段**用 `GET /api/leftover/bids` + `GET /api/leftover/projections`（只读预览：赢家 = 当前最高出价部门，`resolved` = 已正式落库）展示「竞拍情况 + 预览录取结果」，无出价的候选人回退到决定矩阵结论。

前端（`apps/web/src`）为 MVVM 函数式，无 Pinia：

- `api/`：axios 单例（`/api` baseURL、Bearer 注入、401 统一登出）、各资源 Service 对象（`http.ts`）、WS 客户端 `ws.ts`（首条消息必须 auth，断线 1.5s 重连，重连后增量补拉）
- `composables/`：函数式 ViewModel；`useAsync(loader)` 是所有请求的基础原语（`{data, loading, error, run}`）；`useBoardChannel` 是唯一模块级单例 + 引用计数，`useBoardRefresh(events, cb)` 300ms 防抖重拉是列表页实时刷新标准模式；**`useSystemStatus` 是系统状态（阶段 / 出价步长）的唯一共享数据源**（模块级单例 + 并发去重 + 写入方落库后强制刷新），捡漏页/候选人页/系统状态页一律消费它，禁止各自再拉一份；`useOidcSettings` 收编登录认证设置页的配置/角色名单/部门名单（两类规则的目标下拉）/保存/探测/清除密钥；`useTheme` 是深色模式的唯一数据源（模式 + 解析结果 + 落 `<html>.dark`）
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
| `apps/web/src/components/app/` | 自研业务外壳：`PageShell`、`EmptyState`、`SearchInput`、`ConfirmDialog`、`MessageTranscript`（通用）+ `MasterDetailSplit`、`RosterToolbar`（名册栏工具条：标题 + 元信息 + 动作）、`RosterList`、`RosterPager`、`CandidateDetailHeader`（名册↔详情布局）、`DataTableSection`（`nowrapHeaders` 收编表头不换行的包裹 div）、`FormDialog`、`RefreshButton`、`ListSkeleton`、`ErrorAlert`（列表/表单/状态骨架）、`FileDropInput`、`CandidateFormFields`、`CandidateStatusSelect`（候选人状态下拉：筛选带「全部」、重置状态不带）、`ClampText`（长文折叠 + Popover）、`CandidatePreferenceDialog`（志愿与调剂编辑，三处入口共用）、`CallNumberDialog`（候场大屏叫号弹窗）、`RoomSidebar`（面试房间左栏：候选人信息/简介/拉取/阶段控制）、`ImportFileStep`/`ImportMappingStep`/`ImportPreviewTable`/`ImportSubmitStep`（数据导入）、`OidcRuleTable`（登录认证：角色/部门规则表共用，泛型 + `targetOf`/`makeRule` 存取器）/`OidcClaimsPreview`（规则验证）、`ThemeToggle`（深色模式三档切换，顶栏与登录页共用） |

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
  - **URL 一律 RESTful**：路径段只能是资源名词（集合用复数、条目 `/:id`、单例子资源用单数、多词 kebab-case），**禁止动词/动名词路径段**（`login`、`assign`、`pull_candidate`、`waiting` 这类历史写法一律改为资源操作）；集合项用 POST/GET/DELETE，单例子资源用 PUT，部分更新用 PATCH。
    - 页面路由（`router/index.ts`）：`/`、`/login`、`/candidates`、`/candidates/:candidateId`、`/board`（候场大屏，看板资源）、`/rooms`、`/rooms/:roomId`、`/leftover`、`/leftover/candidates/:candidateId`、`/settings/{candidates,imports,users,roles,departments,system/status,authentication}`；筛选/搜索态留在 query（`?q=`、`?status=`），选中条目走路径参数。**旧页面路径保留重定向**（`/waiting`、`/candidates/waiting`、`/room/:roomId`、`/candidates/manage`、`/users`），接口不留别名。顶部导航高亮按**路径归属**判定（`domain/nav.ts#isNavPathActive`：同路径或位于其 `/` 子层级下），**不要用 `route.name` 相等**——RESTful 下条目页/子页是集合页的**兄弟路由**（`/candidates/:id` 的 name 是 `candidate`、设置子页是 `settings-*`），`RouterLink` 自带的 `router-link-active` 同样失效（它要求目标 route record 出现在当前 `matched` 链里）。
    - 接口路由（`handler/http.go`）：`POST /api/sessions`（登录）、`/api/authentication`、`/api/me`、`/api/candidates`(+`/:id`、`/:id/messages`、`/:id/check-in`、`/:id/status`、`/:id/preferences`、`/imports`)、`/api/rooms`(+`/:id`、`/:id/members/:userId`、`/:id/candidate`)、`/api/users`(+`/:id`、`/:id/password`)、`/api/roles`、`/api/departments`、`/api/admissions/:candidateId`、`/api/leftover`(+`/bids/:candidateId`、`/projections`、`/results`)、`GET|PATCH /api/system/status`、`GET /api/oidc/authorization`、`GET|POST /api/oidc/sessions`、`GET|PUT /api/oidc/config`、`POST /api/oidc/probes`、`GET /api/health`；WS 通道 `/ws/rooms/:roomId`、`/ws/board`。
  - **先复用后新写**：「左名册 + 右详情」用 `MasterDetailSplit` + `RosterToolbar` + `RosterList` + `RosterPager`（选中/键盘/深链状态在 `useRosterSelection`，键位在 `useRosterHotkeys`）；管理列表用 `DataTableSection`（骨架→空态→表格）；增改对话框用 `FormDialog` + `useEntityDialog`（`open/target/form/saving` + 校验/提交/成功提示一套收口）；二次确认用 `useConfirmAction` + `ConfirmDialog`；加载/错误/刷新用 `ListSkeleton` / `ErrorAlert` / `RefreshButton`；异常提示用 `lib/toast.ts` 的 `toastError`。视图层只保留筛选与展示派生。
  - **单文件行数**：视图/组件/组合式函数尽量 ≤ 400 行；超标即按上述原语拆分，避免超长文件难以维护。
  - WS 消息经 `domain/` 纯函数不可变更新（如 `mergeMessages` 按 id 去重升序）。
  - **视觉 token**：颜色/圆角/边框/阴影只能取自 `src/assets/index.css`（`:root`/`.dark` 语义变量）与 `tailwind.config.cjs` 语义映射（如 `bg-muted`、`text-muted-foreground`），**禁止硬编码 hex/rgb/hsl**。新组件沿用 `components/ui/<name>/index.ts` + cva 变体；状态只通过带文字标签的 Badge 传达，不依赖颜色单通道。
  - **深色模式**：三档「浅色 / 深色 / 跟随系统」（默认跟随系统）存 localStorage `interview_ng_theme`；`useTheme()` 把解析结果 toggle 到 `<html>` 的 `.dark` 类上（`domain/theme.ts` 是纯逻辑，`usePreferredDark` 供系统偏好，`useStorage` 负责持久化与跨标签页同步），切换入口 `ThemeToggle` 在**顶栏与登录页右上角**两处共用。组件**一律不写 `dark:` 变体**——`.dark` 覆盖同名语义变量即全站生效。`color-scheme` 随 `.dark` 一起切换（原生表单控件、原生滚动条因此跟随），`vue-sonner` 的 `Toaster` 绑 `resolved`。**首屏防白闪在 `index.html` 的内联脚本里按同一键与规则先应用一次——改动 `domain/theme.ts` 的键或规则必须同步改它**（内联脚本不能 import，那是唯一的第二份实现）。
  - **房间展示名**：任何界面都不显示房间编号，统一走 `domain/room.ts#roomLabel(room)`——入参是**房间对象**（不是 name 字符串），未命名/非字符串 name → 「未命名」；手上是房间对象时直接调用，只有候选人 `room_id` 的列表页/大屏用 `composables/useRoomNames.ts`（挂载拉取 + 房间 CRUD 事件重拉，映射里存的就是已解析的展示名）按 id 反查。
  - **键盘**：键位放在**各页自己的 window 处理**里，但注册/注销与守卫收在 `useRosterHotkeys`（`useRosterSelection` 只提供状态与 `goPrev/goNext`，不注册全局键；名册也不做 ↑/↓ 列表导航）。两个名册页统一 **↑/↓ 切换候选人**；捡漏页另用 **←/→ 按步长调整报价**、**Enter 保存报价**——焦点在出价输入框内同样生效（该框只放数字、无光标需求，标记 `data-bid-input` 并作为 `isHotkeyInput` 传入），其它输入控件（搜索框等）与真按钮/链接一律让位（守卫 `lib/dom.ts#isEditableTarget` / `isActivatableElement`）。捡漏页的出价框是普通 `Input`（草稿即文本、输入即时同步），**不要换回 `ui/number-field`**：它自带 ↑/↓ 步进，会与新键位打架。
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
