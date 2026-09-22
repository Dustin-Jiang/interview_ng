#!/bin/sh
# interview_ng 单镜像入口：一个容器里跑两个进程——
#   Caddy（:80，静态产物 + 反代 /api、/ws，纯 HTTP）
#   Go 后端（127.0.0.1:8080，只提供 API/WS，不提供静态文件）
#
# 做三件事：
#
# 1) 等数据库接受连接再拉起后端。
#    compose 的 depends_on 只保证「创建顺序」，**不等** postgres 的 healthcheck 变健康
#    （实测 podman-compose 1.6：app 比 pg 首次 health 通过早 ~15s 启动），而 gorm.Open
#    连不上就 log.Fatalf。首次启动要 initdb，必然踩到这个竞态，所以这里自己等 TCP，
#    不让部署成败取决于编排工具的实现差异。
#
# 2) 一起拉起 Caddy 与后端。
#
# 3) 任一进程退出即整体退出。容器里没有 init/进程管理器：若 Caddy 前台而后端后台，
#    API 挂掉时容器仍显示健康、编排层不会重启它。以失败码退出，交给 compose 的
#    `restart: unless-stopped` 重启。
#
# POSIX sh 没有 `wait -n`，所以用 1s 轮询（退出延迟最多 1s，可接受）。

set -eu

# ---------- 1) 等数据库就绪 ----------
# 只解析 compose 用的 key=value DSN（`host=... port=...`）；URL 形式或未设置时跳过等待，
# 由后端自己报错——即与加这段之前的行为一致，不会凭空多等。
dsn="${DATABASE_DSN:-}"
db_host=""
db_port=""
if [ -n "$dsn" ]; then
  db_host=$(printf '%s\n' "$dsn" | tr ' ' '\n' | sed -n 's/^host=//p' | head -n 1)
  db_port=$(printf '%s\n' "$dsn" | tr ' ' '\n' | sed -n 's/^port=//p' | head -n 1)
fi

if [ -n "$db_host" ] && [ -n "$db_port" ]; then
  limit="${DB_WAIT_SECONDS:-60}"
  waited=0
  until nc -z "$db_host" "$db_port" 2>/dev/null; do
    waited=$((waited + 1))
    if [ "$waited" -ge "$limit" ]; then
      echo "entrypoint: 等 ${db_host}:${db_port} 就绪超时（${limit}s），仍启动后端（由重启策略兜底）" >&2
      break
    fi
    sleep 1
  done
fi

# ---------- 2) 拉起两个进程 ----------
app_pid=0
caddy_pid=0
terminating=0

kill_children() {
  kill "$app_pid" "$caddy_pid" 2>/dev/null || true
}

on_signal() {
  terminating=1
  kill_children
}
trap on_signal INT TERM

/usr/local/bin/interview_ng &
app_pid=$!

caddy run --config /etc/caddy/Caddyfile --adapter caddyfile &
caddy_pid=$!

# ---------- 3) 任一退出即整体退出 ----------
while kill -0 "$app_pid" 2>/dev/null && kill -0 "$caddy_pid" 2>/dev/null; do
  sleep 1
done

kill_children
wait 2>/dev/null || true

# 正常停机（INT/TERM）退出 0；否则判定为「有进程挂了」，以失败退出让编排层重启。
# 注意这里不能复用 on_signal：否则崩溃也会被记成 0，`restart: on-failure` 之类的策略就失效了。
if [ "$terminating" = 1 ]; then
  exit 0
fi
echo "entrypoint: 后端或 Caddy 已退出，容器随之退出" >&2
exit 1
