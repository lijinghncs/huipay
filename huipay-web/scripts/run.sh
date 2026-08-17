#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# 从 .preview 读取 expose_port，读取不到 fallback 5000
EXPOSE_PORT=$(awk -F '[ =]+' '/^expose_port/ {gsub(/[^0-9]/, "", $2); print $2; exit}' .preview 2>/dev/null || echo 5000)

# 清理残留（绝不碰 9000）
for p in 5170 5171 5173 "$EXPOSE_PORT"; do
  PID=$(ss -lptn "sport = :${p}" 2>/dev/null | grep -oP 'pid=\K\d+' | head -1)
  if [ -n "$PID" ]; then
    kill -9 "$PID" 2>/dev/null || true
  fi
done
sleep 1

# 1. 启动 merchant-portal Vite dev server（base: /merchant/）
nohup pnpm --filter huipay-merchant-portal exec vite --host 0.0.0.0 --port 5170 --base /merchant/ > /tmp/vite-merchant.log 2>&1 &
echo "merchant-portal started on :5170 (base /merchant/)"

# 2. 启动 admin-portal Vite dev server（base: /admin/）
nohup pnpm --filter huipay-admin-portal exec vite --host 0.0.0.0 --port 5171 --base /admin/ > /tmp/vite-admin.log 2>&1 &
echo "admin-portal started on :5171 (base /admin/)"

# 3. 启动 checkout-sdk Vite dev server（base: /checkout/）
nohup pnpm --filter @huipay/checkout-sdk exec vite --host 0.0.0.0 --port 5173 --base /checkout/ > /tmp/vite-checkout.log 2>&1 &
echo "checkout-sdk started on :5173 (base /checkout/)"

# 等待 dev server 就绪
sleep 3

# 4. 启动预览路由服务器（前台，绑定 0.0.0.0:EXPOSE_PORT）
export PORT="$EXPOSE_PORT"
exec node scripts/preview-router.mjs