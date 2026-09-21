# TODO：管理员数据导入（Excel → 行 JSON → JMESPath 映射 → 实时预览）

> 贯穿实现的不变量：**先落库后广播**；候选人状态机为唯一权威；消息**按候选人归属**；房间是独立物理会议室记录（`candidate_id` 可空）；文档/注释用中文；Go 命令从 `apps/server/` 运行并设 `GOCACHE`。
> 本任务新增不变量：**候选人必须有唯一纯数字学号**（`student_no` NOT NULL + 唯一索引）；**导入在浏览器解析，服务端只做全或无落库**。

## 已完成（历史清单）

- [x] **数据模型**：`users.username/password_hash/token_version`、`roles/role_permissions/user_roles`（权限目录）、`rooms.candidate_id` 可空（FK `SET NULL`）、`messages.candidate_id`（消息按候选人归属，`id` 即候选人维度续传游标）。
- [x] **认证与 RBAC**：JWT（7 天）+ `token_version` 吊销；RBAC 内存缓存即时生效；种子 `admin`（全权限）/ `interviewer`；`POST /api/sessions` 登录 / `GET /api/me` / `PUT /api/me/password` 自助改密；401/403 分离。
- [x] **WS 改造**：`GET /ws/rooms/:roomId` 无 query 参数；首条 `auth` 消息（JWT + `rooms.chat`，10 秒超时），成功自动 JoinRoom；`send_msg`/`move_phase` 按权限与连接身份；候选人维度断线续传。
- [x] **管理接口**（StateStore → service 先落库后广播）：用户/角色/部门 CRUD；候选人 CRUD、重置状态（向后解绑/向前须有房）、级联删消息；房间 CRUD、成员管理、`PUT /api/rooms/:id/candidate`（拉取式分配）。
- [x] **前端**：登录鉴权基础设施（`useAuth`/路由守卫/401 登出）；候选人管理页（编辑/删除/重置状态/搜索）；房间列表与 `RoomChat`（拉取候选人、成员管理、空房禁发消息）；面试官管理页。
- [x] **候选人完成 → 自动清房**：`MovePhase`/`ResetCandidateStatus` 推进到 `COMPLETED` 自动解绑房间；消息按候选人归档保留；前端 `clearRoomCandidate` + `applyEvent` 处理 `COMPLETED` 清空会话。
- [x] **去掉所有 Skeleton**：4 个视图的"加载态 / 连接中"分支改为留空（不加替换元素）；`components/ui/skeleton/*` 库文件保留但不再被引用。
- [x] **前端拆分重构（单文件 ≤400 行）**：抽出 `components/app/` 9 个原语（`MasterDetailSplit`/`RosterList`/`RosterPager`/`CandidateDetailHeader`/`DataTableSection`/`FormDialog`/`RefreshButton`/`ListSkeleton`/`ErrorAlert`）与 8 个 composable（`useRosterSelection`/`useRosterRouteSync`/`useLeftover`/`useAdmissions`/…）；`initialsOf`/`toastError`/`PHASE_PRESENTATION` 收敛重复实现；两页 672/658 行 → 397/390 行。
- [x] **URL 统一为 RESTful**：页面集合/条目分离（`/candidates`、`/candidates/:candidateId`、`/rooms`、`/rooms/:roomId`、`/leftover/candidates/:candidateId`、`/settings/system/status`，旧路径保留重定向）；接口动词路径清零（`POST /api/sessions`、`PUT /api/candidates/:id/check-in`、`PUT /api/rooms/:id/candidate`、`PUT /api/admissions/:candidateId`、`PUT /api/users/:id/password`），`PUT /api/leftover/*` 收敛为 `GET /api/leftover`、`PUT /api/leftover/bids/:candidateId`、`GET /api/leftover/projections`，`PATCH /api/system/status` 合并 bid-step。
- [x] **typecheck 修复**：`vue-tsc --noEmit` 因 solution 式 tsconfig（`files: []` + references）实际空跑（实测 0 文件、注入错误仍 exit 0），改为 `vue-tsc -b --noEmit`。
- [x] **管理员数据导入**：设置页 `/settings/imports` 三步导入（选工作簿 → JMESPath 字段映射 + 实时预览 → 确认提交）；解析与映射全在浏览器（`xlsx` mini + `@jmespath-community/jmespath`），服务端 `POST /api/candidates/imports` 单事务全或无按学号 upsert；同时把 **`student_no` 立为候选人身份键**（NOT NULL + 唯一，纯数字、前导零有意义），新增/编辑/导入三条路径统一校验、撞号 409；`6e5d25a`（依赖）、`9e98350`（后端）、`acbcc2c`（前端）。
- [x] **结算自动化**：**进入结算阶段即按出价自动结算全部竞拍**（幂等、封盘、可逆回捡漏重算），删除逐个手动结算入口（`POST /api/leftover/results` 路由与 handler、service/store 接口一并移除）；系统状态页进入结算阶段前二次确认。

---

## 已完成：管理员数据导入（实现记录，Excel → 行 JSON → JMESPath 映射 → 实时预览）

### 背景与目标

现状：候选人只能逐个手工录入（`POST /api/candidates`，仅校验 `name` 非空）。

目标：管理员在设置页上传 `.xlsx`，**每行成为一个 JSON object**（首行表头为 key），用 **JMESPath 表达式**把行映射成候选人字段，**边写表达式边实时预览**，确认后一次性导入；导入的数据形态与手工录入完全一致（同一套校验规则）。

---

### 已定决策（设计访谈结论，逐条）

1. **实体**：仅候选人（页面预留实体维度，不实现多实体）。
2. **入口**：设置页新分区 `/settings/imports`（无动词），权限 **`candidates.manage`**（入口与提交均要求，因为导入会覆盖既有记录）。
3. **映射模型**：**每个目标字段一条 JMESPath 表达式**（学号必填、姓名必填、简介可选）+ **只读列名清单**（中文列名必须写成 `"姓名"`，可直接复制）；不做列选择下拉、不做"单表达式产出整个对象"。
4. **行对象**：首行表头为 key；值类型推断（数字→number、布尔→boolean、日期→ISO 字符串（**UTC 固定解释**）、空→null、其余 string）；跳过全空行；**仅单 sheet、仅 `.xlsx`**；**≤2000 行 / ≤5MB**（超限就地报错）。
5. **预览**：三段对照（原始行 → 映射后 JSON → 校验状态），默认前 10 行（可切"显示全部"），表达式输入 **300ms 防抖**，顶部统计条（总 / 新建 / 更新 / 失败）。
6. **步骤**：三步 stepper（① 选择文件 ② 字段映射 + 实时预览 ③ 确认与提交），**仅进度指示、不可跳步**；可回退且**保留已选文件与已填映射**；③ 提交后**就地**显示行级报告，不自动跳转。
7. **提交语义**：**单事务全或无**；命中既有学号 → **更新**（以最新一次提交为准，值相同也写）；未命中 → 新建；批内同学号 → **后行覆盖前行**（预览标注"覆盖第 N 行"）；**运行态（状态机 / 房间绑定 / 消息 / 录取决定 / 捡漏出价）一律不动**，只写 `student_no` / `name` / `profile`。
8. **身份键与校验**：`student_no` **必填、纯数字 `^\d{1,64}$`、NFKC 归一（全角数字→半角）+ trim、精确匹配**（`00123` ≠ `123`，大小写无关因纯数字）；姓名必填；**同名不同学号 = 两条独立记录**（姓名不参与匹配，删除"姓名折叠空白 + 小写"的比对键）。
9. **唯一性**：`student_no` 建 **NOT NULL + 唯一索引**（DB 层强制）；单条编辑**允许改学号**，改到已存在学号 → 明确错误（**409「学号已存在」**，不走 500）。
10. **迁移**：**不兼容既有数据**——直接**删库重建**（AutoMigrate 建新 schema + 种子）；**不写启动自检、不写回填逻辑**。
11. **预设**：映射表达式存 **localStorage** 命名预设（可切换）；**不保留导入历史**；**不提供模板下载**。
12. **依赖**：`xlsx`（官方 CDN tarball `0.20.3`，导入子路径 `xlsx/dist/xlsx.mini.min.js`，**86KB gzip**，Apache-2.0，带类型）+ `@jmespath-community/jmespath@1.3.0`（MPL-2.0，9.6KB，TS + ESM，41 函数）；不引 `zod`/`papaparse`/`read-excel-file`；**服务端零新增依赖、无 multipart 通道**。
13. **学号取值**：取**原始值 `v`**（字符串化）而非格式化文本 `w`（`w` 会把带千分位格式的数字渲染成 `1,234`，破坏纯数字校验）；**不对"数字型单元格丢失前导零"做任何特殊处理**（用户明确不操心）。

---

### 事实依据（已实测，实现时写进注释/文档）

- **SheetJS mini 与 full 在 xlsx 路径逐字节等价**：数值 `123` + 格式 `00000` → 两者 `w:"00123"`；`dateNF:"yyyy-mm-dd"` 生效；`sheet_to_json({raw:false})` 用 `w`；共享字符串中文 / 富文本 / `inlineStr` / 公式缓存值均正常。mini **不读 `.xls`**（本项目不需要）。包 `exports` 直接暴露 `./dist/xlsx.mini.min.js`（带类型）。
- **Postgres 迁移语义**：非空表 `ADD COLUMN ... NOT NULL` → `ERROR: contains null values`；`DEFAULT ''` 路线在建唯一索引时因重复空值失败；**空表**加 NOT NULL + 唯一索引**成功**；可空列 + 唯一索引允许多个 NULL。
- **当前 dev 库牵连面**（删库会全部消失，已获授权）：`candidates=6 / messages=14 / admissions=8 / bids=6 / rooms_bound=0 / room_members=1`；种子只重建 `admin`（admin/admin）+ `interviewer` 角色，**技术/记账部门与全部演示数据不再存在**。
- **JMESPath 规范**：不以 `A-Za-z_` 起始的标识符**必须加引号**；JS×2 与 Go×2 四个实现实测一致拒绝裸中文、一致接受 `"姓名"`。
- **前端现状**：无 xlsx/jmespath/上传相关依赖与代码、无 Worker；无 tabs/progress/file-input 原语（步骤用既有 `ui/stepper`，文件选择用原生 `input[type=file]` + 拖拽容器）；所有 view 走路由懒加载（解析库落在独立 chunk）。

---

### 落地清单

#### A. 数据模型与契约

- [x] `model/candidate.go`：`StudentNo string \`gorm:"size:64;not null;uniqueIndex" json:"student_no"\``（文本列，前导零在类型层面安全）。
- [x] 前端 `models/index.ts` 的 `Candidate` 增 `student_no: string`。
- [x] 三条写入路径（单条新增 / 单条编辑 / 导入）统一学号校验：必填、NFKC + trim、`^\d{1,64}$`、唯一。

#### B. 后端

- [x] `listCandidates` 的 `q` 匹配范围加入 `student_no`（搜索含学号）。
- [x] 单条新增/编辑：学号校验 + 冲突返回明确错误码（`state.Error{Code:"student_no_exists"}` → `stateErr` 映射 **409**）。
- [x] 新增 **`POST /api/candidates/imports`**（权限 `candidates.manage`，无动词路径）：
  - 请求 `{rows: [{student_no, name, profile}]}`（服务端不解析 Excel，只收规范化后的行）；
  - **单事务全或无**：按学号匹配 → 更新 `name/profile`（值相同也写）；未命中 → 新建；
  - 逐条 emit `candidate_created`（复用既有事件，**不动看板扇出语义**）；
  - 响应：全部成功 → `200 {created, updated, rows:[{index, status:"created"|"updated", candidate_id}]}`；任一行硬错误 → `400 {error, rows:[{index, error}]}`（整批未落库）。
- [x] `state` 层批量 upsert 方法（单全局锁内先查后写，无并发竞争）。
- [x] Go 测试：全或无回滚、命中更新（含值相同）、批内后行覆盖、行级错误报告、权限 403、唯一冲突 409、学号正则/长度边界、搜索命中。

#### C. 前端

- [x] 依赖：加入 `xlsx`（CDN tarball）+ `@jmespath-community/jmespath`。
- [x] 路由 + 设置分区：`/settings/imports`（name `settings-imports`），左侧导航按 `candidates.manage` 显隐。
- [x] 新页面 `views/SettingsImportsView.vue`：三步 stepper + 三个分区组件；文件选择新建小组件（原生 input + 拖拽）。
- [x] `composables/useCandidateImport.ts`（文件/解析/映射/预览/提交/预设）；解析与映射的纯函数放 `domain/` 便于复用。
- [x] 预览：三段对照表 + 统计条 + 前 10 行/全部切换；提交后行级报告就地展示。
- [x] 学号落点：候选人管理表格列、单条新增/编辑表单（必填 + 纯数字 + 「学号已存在」）、候选人查看页元信息、列表搜索 placeholder。
- [x] 约束遵守：单文件 ≤400 行、先复用既有 `components/app/*` 与 `ui/stepper`、不新增 UI 库、无 caption 小字、视觉只取语义 token。

#### D. 文档与运维

- [x] README：导入流程与规则、`student_no` 不变量、依赖来源（CDN tarball 非 npm registry）、**旧库必须重置**说明。
- [x] AGENTS：URL/原语约定处补导入页与 `POST /api/candidates/imports`；不变量补"学号唯一必填"。
- [x] 删库重建：drop `interview` → 启动服务端 → 确认 12 表 + 种子 + `student_no NOT NULL UNIQUE`。
- [x] 收尾：删除验证用测试候选人，保持库干净（除非要求保留 demo）。

#### E. 验证

- [x] 后端 `go test -race -count=1 ./...` 全绿。
- [x] 前端 `pnpm --filter @interview-ng/web build`（`vue-tsc -b --noEmit` + vite build）通过。
- [x] 浏览器实测：造 xlsx → 选文件 → 写表达式（含中文列名引号）→ 实时预览 → 提交 → 行级报告；单条新增必填学号；编辑改学号冲突；搜索命中；查看页显示。
- [x] 边界：超 2000 行 / 超 5MB / 非 xlsx / 空表头 / 学号非纯数字 → 就地报错且文案自明。

#### F. 提交拆分

- [x] `feat(server)!: 候选人新增必填唯一学号，支持批量导入端点`
- [x] `feat(web)!: 设置页数据导入（Excel → JSON → JMESPath 映射 + 实时预览）与学号落点`
- [x] `chore(web): 引入 xlsx(CDN tarball, mini) 与 jmespath-community`
- [x] `docs: README/AGENTS（导入流程、学号不变量、依赖来源、旧库重置）`

---

### 实现差异（相对上方计划）

- **步骤条**：改用页内只读步骤指示，**没有**复用 `components/ui/stepper` —— reka 的 `StepperSeparator` 必须在 `StepperItem` 内使用，且「总步数」由 `StepperTrigger` 注册，纯指示用法会渲染出错误的读屏文案（`Step 1 of 0`）。`components/ui/stepper/*` 库文件未改动（本就未被引用）。
- **错误文案**：行级错误不带冗余字段前缀（"学号只能包含数字"，而非"学号：学号只能包含数字"）。
- **额外落点**（搜索按学号命中后能看见学号）：候场大屏加学号列、捡漏名册按学号检索、候选人新增/编辑表单字段抽为 `CandidateFormFields`（两个对话框共用，顺带压下 `SettingsCandidatesView` 行数）。
- **提交顺序**：依赖提交排在前端功能提交之前（先装依赖后写引用它的代码，避免中间提交不可构建）。

### 验证记录（本轮实测）

- `go test -race -count=1 ./...` 全绿；`pnpm test`（`vue-tsc -b --noEmit` + `go test ./...`）全绿；`pnpm --filter @interview-ng/web build` 通过（导入分区懒加载 chunk 316KB / gzip 102KB，即为 mini 构建，未引入 full）。
- 删库重建：`DROP/CREATE DATABASE interview` 后启动服务，AutoMigrate 建出 `student_no varchar(64) NOT NULL` + `idx_candidates_student_no UNIQUE`，12 张表 + 种子 `admin` 就绪。
- 浏览器端到端（真实 Postgres + 真实 xlsx 文件）：正常批次 4 行 → 预览「总 4 / 新建 4」→ 提交报告逐行「新建」+ 学号 `00240004` 前导零保留；覆盖批次 → 「更新 2」；批内同学号 → 第 3 行标注「覆盖第 2 行」且最终只留一条（后行生效）；非法批次（坏学号 / 空学号 / 空姓名）→ 预览「失败 3」+ 逐行原因，且"下一步"禁用；单条新增：非数字学号被拒、全角学号归一为半角落库、撞号 toast「学号已存在」；编辑可改学号；列表/查看页/候场大屏均显示学号，按学号搜索命中。
- 接口探针（对运行中的服务）：非法批次 `400 {"error":"导入数据校验未通过","rows":[{index,error}...]}` 且候选人数不变；2001 行 `400 单次导入最多 2000 行`；撞号新增 `409 学号已存在`。
- 收尾：验证用候选人已全部删除，dev 库回到仅种子状态。

---

### 风险与注意

- **删库不可逆**：dev 库现有全部业务数据（含 6 位候选人及其消息/录取决定/出价、技术/记账部门）会消失（已获授权）。
- `student_no` 为 NOT NULL ⇒ **任何旧库都必须重置**；README 必须写清，否则他人拉起旧库会看到 `contains null values` 这类底层报错。
- `xlsx` 依赖来自 CDN tarball（非 npm registry）；lockfile 会记录 tarball 与完整性哈希，离线安装/审计策略需知悉。
- 学号列若在 Excel 中以数字存储且带前导零，会**静默丢失**（按用户指示不做特殊处理）——预览三段对照可肉眼发现（`123` vs `00123`）。
- 导入的"全或无"意味着**一行坏数据会让整批不落库**；报告必须给出行号 + 字段 + 原因，便于一次改完重传。
