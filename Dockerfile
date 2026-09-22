# syntax=docker/dockerfile:1
#
# interview_ng 部署镜像：**单进程、单镜像**——Go 后端同时提供 API/WS 与前端产物。
#
#   builder 1  web-build → Vite 产物（apps/web）
#   builder 2  app-build → Go 静态二进制（apps/server）
#   运行层              → alpine + 该二进制 + /srv/www 里的前端产物；
#                          WEB_ROOT=/srv/www 让后端把静态文件一并发出去（同一端口 ⇒ 天然同源）。
#
# 不再需要 Caddy/nginx 之类的前置：前端请求都是相对路径（见 apps/web/src/api/http.ts 的
# baseURL '/api' 与 apps/web/src/api/ws.ts 的 '/ws'），后端本来就监听在同一个端口上，
# 静态与接口同源，没有第二跳。代价是不会自动压缩（见 README「部署」的说明与实测数据）。
#
# 构建上下文 = 仓库根：`podman compose build`（或 `podman build -t interview_ng:local .`）。

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

# ---------- 运行层：单进程 ----------
FROM docker.io/library/alpine:3.22
# ca-certificates：OIDC 的发现文档 / JWKS 走 HTTPS；tzdata：无它时 time.Local 退化成 UTC
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 app
COPY --from=app-build /out/interview_ng /usr/local/bin/interview_ng
COPY --from=web-build /src/apps/web/dist /srv/www
# WEB_ROOT：后端据此把前端产物发出去（目录里没有 index.html 时该功能自动不启用）
ENV ADDR=:8080 WEB_ROOT=/srv/www
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/interview_ng"]
