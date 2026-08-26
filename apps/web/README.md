# interview_ng 前端（Vue 3 + Vite + TypeScript + shadcn-vue）

面试系统的管理控制台前端，包含两个视图：

- **候选人管理**（`/`）：列表 / 创建 / 签到 / 编辑 / 重置状态 / 搜索筛选，基于 REST。签到后的候选人进入「待拉取」排队池，由面试房间内按需拉取（银行叫号式）进入房间。
- **面试房间**（`/room` / `/room/:roomId`）：房间列表（每间显示绑定候选人及其当前状态），点击进入具体房间查看实时消息与阶段控制；房间内基于 WebSocket 群聊。

---

## 架构分层（MVVM + 组合式函数化的函数式风格）

前端同样遵循 **MVVM**，但 ViewModel 层不使用 Pinia，而是采用 **Vue 组合式函数（`useXXX()`）+ 函数式编程**：状态以纯函数派生、不可变更新、小函数组合。依赖方向单向：

```
Views（.vue 组件，纯绑定）
   ↓ 仅通过 useXXX() 消费 VM
ViewModels（src/composables：useAsync / useCandidates / useRoomList / useRoomChat）
   ↓ 组合
Pure Domain（src/domain：无副作用纯函数，不可变更新）
   ↓ 调用
Service（REST/WS 客户端，无 UI 状态）
   ↓ 收发
Model Domain Types（与后端 JSON 契约一一对应）
```

| 层 | 目录 | 职责 | 关键文件 |
|---|---|---|---|
| **View** | `src/views` | 模板渲染、事件绑定、表单交互；只读解构 VM 的 refs 并调用其函数，不含业务规则 | `CandidatesView.vue`, `RoomView.vue`, `RoomChat.vue` |
| **ViewModel** | `src/composables` | `useXXX()` 组合式：持有 reactive 状态，把 Service 事件/回执转成可绑定状态，`onScopeDispose` 自管清理 | `useCandidates.ts`, `useRoomList.ts`, `useRoomChat.ts`, `useBoardChannel.ts`, `useAsync.ts` |
| **Pure Domain** | `src/domain` | 无副作用纯函数：状态机推进、消息合并/去重、不可变房间更新 | `status.ts`, `messages.ts` |
| **Service** | `src/api` | 封装 REST 与 WS 收发，与框架解耦，只产出原始数据/回调 | `http.ts`, `ws.ts`, `ws-model.ts` |
| **Model** | `src/models` | 领域类型与 WS 协议信封，镜像后端契约 | `index.ts`, `ws-model.ts` |

函数式与组合式要点：

- **组合式函数替代全局 store**：每个 `useXXX()` 在其调用者（如组件）各自的作用域内自管理状态，`onScopeDispose` 自动清理（如 `useRoomChat` 自动断开 WS），无全局单例与生命周期泄漏。唯一例外是 `useBoardChannel`（看板 WS 通道）：模块级单例 + 引用计数，多视图共享一条 `/ws/board` 连接。
- **纯函数核心**：`src/domain` 里的状态推进与消息合并都是无副作用的（输入不改、返回新值），可独立单测。
- **函数组合**：`useAsync` 作为通用异步原语被 `useCandidates`/`useRoomList` 复用；`useRoomChat` 内部用 `domain` 纯函数转换 WS 事件。
- **实时刷新**：房间内数据走房间通道（`/ws/room/:id`，重连后按续传游标自动补拉增量）；列表页走看板通道（`useBoardRefresh(events, cb)`：事件到达 → 防抖 300ms 整表重拉），覆盖候场大屏、房间列表与候选人记录。
- **视图弱耦合**：视图在 setup 顶层解构 `useXXX()` 的 refs 与函数，模板直接引用（refs 自动解包）。


---

## 布局说明

- **全局导航**：仅在「候选人管理」与「房间列表」场景显示（`src/App.vue` 依据路由是否处于房间内部决定）。进入具体房间（`/room/:roomId`）时隐藏全局导航，由房间视图自绘头部。
- **房间内部**（`src/views/RoomChat.vue`）：头部为「返回键 + 房间名」，主区域为聊天界面，右侧栏展示面试人（候选人）信息与当前状态/状态机。
- **滚动策略**：整页固定为视口高度（`h-screen overflow-hidden`，不溢出滚动），滚动发生在**内部容器**——候选人列表、房间列表在各自视图内滚动；房间内聊天消息区在消息容器内滚动（新消息自动滚到底）。


---

## 启动

前置：后端服务运行在 `:8080`（`apps/server` 下 `go run ./cmd/server`，或根 `pnpm run dev:server`），且已通过根 `podman-compose up -d` 启动 Postgres，`users` 表含可用面试官。

```bash
# 在仓库根执行（本应用属于 pnpm monorepo 的 apps/web 工作区）
pnpm install        # 首次，安装全部 workspace 依赖
pnpm dev            # 开发：http://localhost:8000 （已配置 /api 与 /ws 代理到 :8080）
# 等价：pnpm --filter @interview-ng/web dev
```

生产构建：

```bash
pnpm build          # 根聚合：构建 web + server；或单独 pnpm --filter @interview-ng/web build
pnpm typecheck      # 或单独进入本目录：pnpm run typecheck / pnpm run build
```

> Vite 已把 `/api` 与 `/ws` 代理到 `http://localhost:8080`，浏览器直接访问 `http://localhost:8000` 即可，无需 CORS 配置。

---

## 当前用户与权限

后端提供登录鉴权：`POST /api/auth/login` 签发 7 天 JWT（默认种子账号 `admin / admin`）。前端登录态由 `useAuth` 组合式函数管理（token 存 localStorage，`GET /api/me` 拉取用户/角色/权限并集），路由守卫未登录跳 `/login`；各操作按钮按当前用户权限（`hasPermission`）显隐，后端矩阵强制兜底。WS 连接（房间通道 `/ws/room/:roomId` 或看板通道 `/ws/board`）后首条消息发送 `auth`（携带 JWT）完成鉴权；鉴权被拒等致命错误会停止自动重连，界面退化为手动刷新。
