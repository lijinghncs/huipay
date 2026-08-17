#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# 1. 构建前端
echo "=== Building frontend (huipay-web) ==="
cd "$PROJECT_DIR/huipay-web"
pnpm install --frozen-lockfile
pnpm build:all

# 2. 构建后端
echo "=== Building backend (huipay-backend) ==="
cd "$PROJECT_DIR/huipay-backend"
mkdir -p bin
go build -o bin/huipay-server ./cmd/server

echo "=== Build complete ==="