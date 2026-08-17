#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# 从 .preview 读取 expose_port，读取不到 fallback 5000
EXPOSE_PORT=$(awk -F '[ =]+' '/^expose_port/ {gsub(/[^0-9]/, "", $2); print $2; exit}' .preview 2>/dev/null || echo 5000)

# 清理残留（绝不碰 9000）
fuser -k "${EXPOSE_PORT}/tcp" 2>/dev/null || true
sleep 1

# 启动 merchant-portal Vite dev server
cd "$PROJECT_DIR/packages/merchant-portal"
exec pnpm exec vite --host 0.0.0.0 --port "$EXPOSE_PORT"