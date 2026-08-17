#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

PORT=5000

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
fuser -k "${PORT}/tcp" 2>/dev/null || true
sleep 1

# 启动后端服务（可通过 HUIPAY_HTTP_PORT 覆盖端口）
export HUIPAY_HTTP_PORT="$PORT"
export HUIPAY_SKIP_DB="${HUIPAY_SKIP_DB:-true}"
export HUIPAY_GIN_MODE="${HUIPAY_GIN_MODE:-release}"

cd "$PROJECT_DIR/huipay-backend"
exec ./bin/huipay-server