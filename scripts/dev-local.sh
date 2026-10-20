#!/usr/bin/env bash
# 本地开发一键启动：后端(8907) + 前端 Vite(3000)，不使用 Docker。
# 用法：
#   ./scripts/dev-local.sh start    启动两个服务
#   ./scripts/dev-local.sh stop     停止两个服务
#   ./scripts/dev-local.sh status   查看运行状态
set -uo pipefail

APP_PORT="${APP_PORT:-8907}"
WEB_PORT="${WEB_PORT:-3000}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$ROOT_DIR/dev/logs"

export PATH="$HOME/.local/go-install/go/bin:$HOME/.local/bin:$PATH"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.google.cn}"
export GOCACHE="${GOCACHE:-/tmp/bill-go-build}"

mkdir -p "$LOG_DIR"

# 与 Makefile 保持一致的版本注入（数据库 schema 版本校验依赖它）
APP_VER="$(tr -d '\n' < "$ROOT_DIR/VERSION" 2>/dev/null)"
APP_VER="${APP_VER:-dev}"
BUILD_TIME="$(date +%Y-%m-%dT%H:%M:%S)"
GIT_COMMIT="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null)"
GIT_COMMIT="${GIT_COMMIT:-unknown}"
LDFLAGS="-X smallgo/server/version.Version=$APP_VER"
LDFLAGS="$LDFLAGS -X smallgo/server/version.BuildTime=$BUILD_TIME"
LDFLAGS="$LDFLAGS -X smallgo/server/version.GitCommit=$GIT_COMMIT"
LDFLAGS="$LDFLAGS -X smallgo/server/version.AppName=bill"

port_pid() {
  lsof -nP -tiTCP:"$1" -sTCP:LISTEN 2>/dev/null | head -1
}

wait_port() {
  local port="$1" tries="$2" i=0
  while [ "$i" -lt "$tries" ]; do
    if [ -n "$(port_pid "$port")" ]; then
      return 0
    fi
    sleep 0.5
    i=$((i + 1))
  done
  return 1
}

stop_one() {
  local port="$1" pid
  pid="$(port_pid "$port")"
  if [ -n "$pid" ]; then
    echo "  stop PID $pid on port $port"
    kill "$pid" 2>/dev/null
    wait_port "$port" 20
    if [ -n "$(port_pid "$port")" ]; then
      kill -9 "$pid" 2>/dev/null
      sleep 0.5
    fi
  fi
}

status() {
  local api_pid web_pid code
  api_pid="$(port_pid "$APP_PORT")"
  web_pid="$(port_pid "$WEB_PORT")"
  if [ -n "$api_pid" ]; then
    echo "backend  :$APP_PORT -> running (PID $api_pid)"
  else
    echo "backend  :$APP_PORT -> stopped"
  fi
  if [ -n "$web_pid" ]; then
    echo "frontend :$WEB_PORT -> running (PID $web_pid)"
  else
    echo "frontend :$WEB_PORT -> stopped"
  fi
  if [ -n "$api_pid" ]; then
    code="$(curl -s -o /dev/null -w '%{http_code}' --noproxy '*' --max-time 5 \
      "http://127.0.0.1:$APP_PORT/api/health" 2>/dev/null)"
    echo "health   /api/health -> HTTP ${code:-000}"
  fi
}

do_stop() {
  stop_one "$WEB_PORT"
  stop_one "$APP_PORT"
  echo "done."
}

do_start() {
  if ! command -v go >/dev/null 2>&1; then
    echo "go not found. run: source ~/.zshrc" >&2
    exit 1
  fi

  if [ ! -d "$ROOT_DIR/web/node_modules" ]; then
    echo "installing frontend deps..."
    cd "$ROOT_DIR/web" && npm ci --loglevel=error --no-audit --no-fund
    cd "$ROOT_DIR" || exit 1
  fi

  echo "free ports..."
  stop_one "$WEB_PORT"
  stop_one "$APP_PORT"

  echo "start backend on $APP_PORT..."
  cd "$ROOT_DIR/server" || exit 1
  # 必须注入版本号：数据库 schema 已升级到 0.0.1，binary 版本号低于它会拒绝启动
  nohup go run -ldflags "$LDFLAGS" . >"$LOG_DIR/backend.log" 2>&1 &
  cd "$ROOT_DIR" || exit 1

  if ! wait_port "$APP_PORT" 90; then
    echo "backend failed, tail of log:" >&2
    tail -20 "$LOG_DIR/backend.log" >&2
    exit 1
  fi
  echo "  backend is up"

  echo "start frontend on $WEB_PORT..."
  cd "$ROOT_DIR/web" || exit 1
  nohup npm run dev >"$LOG_DIR/frontend.log" 2>&1 &
  cd "$ROOT_DIR" || exit 1

  if ! wait_port "$WEB_PORT" 90; then
    echo "frontend failed, tail of log:" >&2
    tail -20 "$LOG_DIR/frontend.log" >&2
    exit 1
  fi
  echo "  frontend is up"

  local lan_ip
  lan_ip="$("$ROOT_DIR/scripts/lan-ip.sh")"

  echo ""
  echo "open in browser (use localhost, Vite binds IPv6 loopback only)"
  echo "  app    http://localhost:$WEB_PORT"
  echo "  api    http://localhost:$APP_PORT/api"
  if [ -n "$lan_ip" ]; then
    echo "  lan    http://$lan_ip:$WEB_PORT"
  fi
  echo ""
  echo "logs   $LOG_DIR/backend.log"
  echo "       $LOG_DIR/frontend.log"
  echo "stop   ./scripts/dev-local.sh stop"
}

case "${1:-start}" in
  start)
    do_start
    ;;
  stop)
    do_stop
    ;;
  status)
    status
    ;;
  *)
    echo "usage: $0 [start|stop|status]" >&2
    exit 2
    ;;
esac