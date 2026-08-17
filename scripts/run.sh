#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

PORT=5000
BACKEND_PORT=5001

usage() {
  echo "Usage: $0 -p <port>"
  exit 1
}

while getopts "p:h" opt; do
  case "$opt" in
    p) PORT="$OPTARG" ;;
    h) usage ;;
    \?) usage ;;
  esac
done

# 清理残留（绝不碰 9000）
for p in "$PORT" "$BACKEND_PORT"; do
  PID=$(ss -lptn "sport = :${p}" 2>/dev/null | grep -oP 'pid=\K\d+' | head -1)
  if [ -n "$PID" ]; then
    kill -9 "$PID" 2>/dev/null || true
  fi
done
sleep 1

# 1. 启动后端服务（后台，端口 5001）
export HUIPAY_HTTP_PORT="$BACKEND_PORT"
export HUIPAY_SKIP_DB="${HUIPAY_SKIP_DB:-true}"
export HUIPAY_GIN_MODE="${HUIPAY_GIN_MODE:-release}"

cd "$PROJECT_DIR/huipay-backend"
nohup ./bin/huipay-server > /tmp/backend.log 2>&1 &
echo "Backend started on :${BACKEND_PORT}"

# 等待后端就绪
sleep 2

# 2. 启动静态文件服务器（前台，绑定 0.0.0.0:PORT）
cd "$PROJECT_DIR"
export PORT="$PORT"
export BACKEND_PORT="$BACKEND_PORT"
exec node scripts/serve-static.mjs