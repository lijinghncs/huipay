#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# 1. 构建前端（每个 portal 独立设置 base path）
echo "=== Building frontend (huipay-web) ==="
cd "$PROJECT_DIR/huipay-web"
pnpm install --frozen-lockfile

echo "--- merchant-portal (base: /merchant/) ---"
pnpm --filter huipay-merchant-portal exec vite build --base /merchant/

echo "--- admin-portal (base: /admin/) ---"
pnpm --filter huipay-admin-portal exec vite build --base /admin/

echo "--- checkout-sdk (base: /checkout/) ---"
pnpm --filter @huipay/checkout-sdk exec vite build --base /checkout/

# 2. 构建后端
echo "=== Building backend (huipay-backend) ==="
cd "$PROJECT_DIR/huipay-backend"
mkdir -p bin
go build -o bin/huipay-server ./cmd/server

echo "=== Build complete ==="