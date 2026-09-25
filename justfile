# 开发常用命令。web :8000，server :8080，均代理到同一后端。
set shell := ["bash", "-uc"]

# 列出全部命令
default:
    @just --list

# 启动 Postgres（docker 或 podman 二选一，见 docker-compose.yml）
# 只起 postgres 服务：docker-compose.yml 里还有部署用的 app（单镜像），不会被捎带起来
db:
    mkdir -p data && docker compose up -d postgres || podman compose up -d postgres

# 启动后端 Go 服务（从 apps/server 运行，缓存放在仓库内）
server:
    cd apps/server && GOCACHE="$PWD/.gopath/gocache" go run ./cmd/server

# 用途：旧库 schema 与本版本不兼容（AutoMigrate 只会建表/加列，改不了既有列的约束）
# 重置数据库 schema：删全部业务表后重建（破坏性，数据不可恢复）
db-reset:
    cd apps/server && GOCACHE="$PWD/.gopath/gocache" DB_RESET=1 go run ./cmd/server

# 启动前端 web（Vite，:8000）
web:
    pnpm run dev:web

# 前端类型检查
typecheck:
    pnpm run typecheck

# 后端测试（带 -race）
test-server:
    cd apps/server && GOCACHE="$PWD/.gopath/gocache" go test -race ./...

# 前端 typecheck + 后端测试
test:
    just typecheck
    just test-server

# 构建
build:
    pnpm run build
