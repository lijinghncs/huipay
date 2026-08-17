## 项目概述

**HuiPay（汇聚付）** — 商户收款 + 分账系统 monorepo。
核心业务：聚合收银（微信 + 支付宝）、商户进件、资金分账（一级/多级）、门店管理、对账中心。

## 技术栈

- **前端** (`huipay-web/`)：Vite 5 + React 18 + TypeScript + AntD Pro + pnpm workspace
  - 5 packages：`merchant-portal`（商家工作台）、`admin-portal`（管理后台）、`checkout-sdk`（收银台 SDK）、`shared`（共享类型）、`ui-kit`（UI 组件）
- **后端** (`huipay-backend/`)：Go 1.22 + Gin + GORM + MySQL 8.0
  - 配置：Viper（config.yaml + HUIPAY_* 环境变量覆盖）
  - 端口：默认 8080（可通过 HUIPAY_HTTP_PORT 覆盖）
  - 迁移：golang-migrate（内嵌迁移文件，自动在启动时执行）
- **运行时**：Node.js 24 + Go 1.25 (go.mod)

## 目录结构

```
/workspace/projects/
├── .coze                    # 根部署配置（monorepo）
├── .gitignore
├── AGENTS.md
├── scripts/
│   ├── build.sh             # 部署构建脚本（构建前端 + 后端）
│   ├── run.sh               # 部署运行脚本（启动静态文件服务器 + 后端）
│   └── serve-static.mjs     # 部署静态文件服务器（3 portal + API 代理）
├── huipay-web/              # 前端 monorepo（pnpm workspace）
│   ├── .coze                # sub_id=d310f0de, project_type=web
│   ├── .preview             # 预览端口声明（expose_port=5000）
│   ├── scripts/
│   │   ├── build.sh         # 预览构建脚本（pnpm install）
│   │   ├── run.sh           # 预览运行脚本（启动 3 个 Vite dev server + 路由）
│   │   └── preview-router.mjs # 预览路由服务器（路径分发到各 Vite dev server）
│   ├── packages/
│   │   ├── merchant-portal/ # 商家工作台（Vite dev: 5170, base: /merchant/）
│   │   ├── admin-portal/    # 管理后台（Vite dev: 5171, base: /admin/）
│   │   ├── checkout-sdk/    # 收银台 SDK（Vite dev: 5173, base: /checkout/）
│   │   ├── shared/          # 共享类型/API Client/工具
│   │   └── ui-kit/          # 共享 UI 组件
│   └── pnpm-workspace.yaml
├── huipay-backend/          # Go 后端单体
│   ├── .coze                # sub_id=16285581, project_type=backend
│   ├── cmd/server/main.go   # 启动入口
│   ├── infra/               # 基础设施层（config/db/migrator/obs/prom）
│   ├── internal/            # 业务逻辑（order/payment/account/merchant/split/store/stats/admin）
│   └── Makefile             # 本地开发命令
└── docs/                    # 产品方案/技术方案/设计文档
```

## 关键入口 / 核心模块

- **后端入口**：`huipay-backend/cmd/server/main.go` — Gin 路由装配 + 定时任务调度
- **API 路由**：`/v1/` 前缀，含收银台、商户、分账、管理后台、门店等模块
- **前端入口**：各 package 独立 Vite 应用，`pnpm dev:merchant` / `pnpm dev:admin` / `pnpm dev:sdk`
- **部署端口**：后端服务端口 5000（通过 `HUIPAY_HTTP_PORT` 环境变量覆盖，默认 8080）

## 运行与预览

### 预览（huipay-web 子项目，3 portal 统一入口）

- **预览入口**：`http://localhost:5000`（路由服务器，自动分发到各 Vite dev server）
- **访问路径**：
  - `/merchant/` → 商家工作台（merchant-portal，Vite dev :5170）
  - `/admin/` → 管理后台（admin-portal，Vite dev :5171）
  - `/checkout/` → 收银台 SDK（checkout-sdk，Vite dev :5173）
  - `/checkout/h5` → 收银台 H5 页
- **启动命令**：`cd huipay-web && bash scripts/run.sh`（自动启动 3 个 Vite dev server + 路由服务器）
- **预览脚本**：`.coze [dev]` → `scripts/build.sh`（安装依赖）+ `scripts/run.sh`（启动全门户预览）

### 部署（根 .coze 编排，方案 C）

- **构建**：`scripts/build.sh` → 先构建 3 个前端 portal（pnpm build:all，指定各自 base 路径），再构建后端 Go 二进制
- **运行**：`scripts/run.sh` → `scripts/serve-static.mjs`（Node.js 静态文件服务器，端口 5000）+ 后端（后台端口 8080）
- **部署访问路径**（同预览路径）：
  - `/merchant/` → merchant-portal 构建产物
  - `/admin/` → admin-portal 构建产物
  - `/checkout/` → checkout-sdk 构建产物
  - `/v1/*` → 代理到后端 API
  - `/` → 重定向到 `/merchant/`
- **后端端口**：默认 8080（`HUIPAY_HTTP_PORT` 覆盖）

### 本地开发

- 前端：`cd huipay-web && pnpm dev:merchant`（端口 5170）
- 后端：`cd huipay-backend && make run`（端口 8080，需 MySQL）

## 用户偏好与长期约束

- 包管理器：前端 pnpm **强制**，后端 Go modules
- 提交规范：Conventional Commits
- 端口：部署统一 5000；本地开发前端 5170/5171/5173，后端 8080

## 常见问题和预防

- 后端可跳过数据库启动：`HUIPAY_SKIP_DB=true`（默认已配置，部署脚本中启用）
- 后端端口通过环境变量覆盖：`HUIPAY_HTTP_PORT=5000`（部署脚本中设置）
- 前端为多包 workspace，单独构建每个 portal