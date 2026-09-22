# syntax=docker/dockerfile:1
#
# interview_ng 部署镜像：**前后端同一个镜像**（一个容器 = Caddy + Go 后端）。
#
#   builder 1  web-build → Vite 产物（apps/web）
#   builder 2  app-build → Go 静态二进制（apps/server）
#   运行层     runtime   → caddy:2-alpine：Caddy 发静态产物，并把 /api、/ws 反代给
#                          同容器内 127.0.0.1:8080 的 Go 进程；两个进程由
#                          deploy/entrypoint.sh 一起拉起、一起退出（任一死即容器退出）。
#
# 构建上下文 = 仓库根：`podman compose build`（或 `podman build -t interview_ng:local .`）。
#
# 前端请求全部走相对路径（见 apps/web/src/api/http.ts 的 baseURL '/api' 与
# api/ws.ts 的 '/ws'），所以必须同源——Caddy 既发静态又反代这两条前缀，
# 与 vite dev 的 server.proxy 是同一套语义，只是把「跨容器」换成了「同容器回环」。

# ---------- 前端：Vite 产物 ----------
FROM docker.io/library/node:22-alpine AS web-build
WORKDIR /src
# packageManager 钉的是 pnpm@11.7.0（与开发机一致），这里显式装同一版本
RUN npm i -g pnpm@11.7.0
# 先只拷依赖清单：源码改动不会让依赖层失效
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/package.json
# pnpm-workspace.yaml 里的 allowBuilds 必须一起进镜像，否则 esbuild 的 postinstall 被拦，vite 构建会失败
RUN pnpm install --frozen-lockfile
COPY apps/web apps/web
RUN pnpm --filter @interview-ng/web build

# ---------- 后端：静态编译 ----------
FROM docker.io/library/golang:1.26-alpine AS app-build
WORKDIR /src
COPY apps/server/go.mod apps/server/go.sum ./
RUN go mod download
COPY apps/server/ ./
# 纯 Go 依赖（gin/gorm/pgx），关掉 CGO 才能落到 alpine 运行时
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/interview_ng ./cmd/server

# ---------- 运行层：Caddy（静态 + 反代）+ Go 后端 ----------
FROM docker.io/library/caddy:2-alpine AS runtime

# tzdata：容器默认 UTC，装上后可用 TZ / DSN 的 TimeZone 对齐时区；
# ca-certificates 已由 caddy 基础镜像自带（ACME 也要用），无需重复安装。
RUN apk add --no-cache tzdata

COPY --from=app-build /out/interview_ng /usr/local/bin/interview_ng
COPY --from=web-build /src/apps/web/dist /srv/www
COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY deploy/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# Go 后端只监听容器内的 127.0.0.1:8080（不对外发布），唯一入口是 Caddy 的 :80
ENV ADDR=127.0.0.1:8080
EXPOSE 80

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
