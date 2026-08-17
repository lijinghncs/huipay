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
- **运行时**：Node.js 24 + Go 1.25.0（已安装于 /usr/local/go）

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
│   ├── internal/            # 业务逻辑（order/payment/account/merchant/split/store/stats/admin/recon）
│   │   └── split/           # 分账模块（P0-P5 重构后：高内聚低耦合）
│   │       ├── alloc/       # 分配方案纯函数计算（无外部依赖，可单测）
│   │       ├── event/       # 领域事件 + outbox 轮询 + 内存总线（P3）
│   │       │   ├── event.go     # 事件类型定义与载荷
│   │       │   ├── outbox.go    # Outbox 仓储（t_outbox_event 表）
│   │       │   ├── bus.go       # 内存事件总线（发布-订阅）
│   │       │   ├── worker.go    # 后台轮询投递
│   │       │   └── handler.go   # 事件处理器（日志/监控）
│   │       ├── service/     # 编排层，按 UseCase 拆为 5 文件：
│   │       │   ├── service.go     # Service 结构体 + NewService + 共享基础设施
│   │       │   ├── ordersplit.go  # 单笔订单分账（Execute / Get / Preview / Retry / ListExecutions）
│   │       │   ├── periodbill.go  # 时段分账 + 审批流（ExecuteByPeriod / GenerateBill / ApproveBill / RejectBill）
│   │       │   ├── reconcile.go   # 差错中心 + 对账差异（ListExceptions / ListReconcileDiffs / ResolveReconcileDiff / ListAudits）
│   │       │   └── rules.go       # 规则 CRUD（ListRules / CreateRule / UpdateRule / SetRuleStatus / DeleteRule）
│   │       ├── handler/               # HTTP 层，拆为 5 文件（handler + split_order + bill + reconcile + rule）
│   │       │   ├── handler.go             # Handler 结构体 + New + parsePageQuery
│   │       │   ├── split_order_handler.go  # Execute / Get / ListExecutions / Preview / Retry
│   │       │   ├── bill_handler.go         # ExecuteByPeriod / GenerateBill / ApproveBill / RejectBill
│   │       │   ├── reconcile_handler.go    # ListExceptions / ListReconcileDiffs / ResolveReconcileDiff
│   │       │   └── rule_handler.go         # ListRules / CreateRule / UpdateRule / SetRuleStatus
│   │       ├── executor/    # 分账执行器，拆为 5 组件（P4）
│   │       │   ├── executor.go         # 类型定义 + NewExecutor + helper 函数
│   │       │   ├── orchestrator.go      # Execute 主流程编排（errgroup 并行处理）
│   │       │   ├── query.go            # 查询方法（ListByOrderNo / ListByMerchant）
│   │       │   ├── channel_caller.go    # 通道调用 + 转账 + 钱包解析（单次查询合并）
│   │       │   ├── status_sync.go       # 状态回写 + 指标 + 事件发布 + determineFinalStatus
│   │       │   └── balance_gate.go      # 余额预校验
│   │       ├── rule/        # 规则引擎（DSL 解析 + 匹配）
│   │       ├── scheduler/   # 补偿调度（依赖 ports.Executor 接口）+ 重算
│   │       └── repository/  # 数据访问（10 个表）
│   ├── internal/recon/      # 对账域（独立限界上下文，V2 重构后）
│   │   ├── domain/          # Diff/DiffType/CheckResult 等纯领域模型
│   │   ├── compare/         # 纯函数比对器（Totals/Rows/MatchBills）
│   │   ├── ports/           # DiffStore/AuditRecorder/RunLogger/Observer + Fetcher 端口
│   │   ├── engine/          # ScheduledJob 契约
│   │   ├── job/             # precheck（前置）/ postsplit（执行后）/ channel（渠道）
│   │   ├── adapter/         # gorm 取数适配器（口径 SQL 集中）+ MySQL 冒烟测试
│   │   ├── repository/      # DiffStore：t_reconcile_diff 唯一写入/查询入口
│   │   └── scheduler/       # framework 任务注册与窗口
│   └── Makefile             # 本地开发命令
└── docs/                    # 产品方案/技术方案/设计文档
```

## 关键入口 / 核心模块

- **后端入口**：`huipay-backend/cmd/server/main.go` — Gin 路由装配 + 定时任务调度
- **API 路由**：`/v1/` 前缀，含收银台、商户、分账、管理后台、门店等模块
- **前端入口**：各 package 独立 Vite 应用，`pnpm dev:merchant` / `pnpm dev:admin` / `pnpm dev:sdk`
- **部署端口**：后端服务端口 5000（通过 `HUIPAY_HTTP_PORT` 环境变量覆盖，默认 8080）
- **分账模块（P0-P5 重构 + 优化 Q1-Q5）**：
  - `internal/split/alloc/` — 分配方案纯函数计算，无 DB/通道依赖，可独立单测 ✅
  - `internal/split/state/` — 状态机集中（8 种状态，IsTerminal / IsClaimable / IsException / Transition）✅
  - `internal/split/event/` — 领域事件 + outbox 仓储 + 内存总线（每 5s 轮询投递）+ 告警通知 ✅
  - `internal/split/splitcfg/` — 配置常量集中管理 ✅
  - `internal/split/ports/` — 依赖倒置接口（WalletResolver / Executor / Prechecker）✅
  - `internal/split/service/` — 按 UseCase 拆为 5 文件，各 <= 600 行，handler 零改动 ✅
  - `internal/split/executor/` — 拆为 5 组件，多接收方并行处理（errgroup）✅
  - `internal/split/handler/` — 拆为 5 文件 ✅
  - `internal/split/scheduler/compensate.go` — 依赖 ports.Executor 接口 ✅
  - 入口：`handler.New(svc, logger)` → `service.NewService(...)`
- **对账域（V2 重构，docs/reconcile-architecture-v2.md）**：
  - `internal/recon/` — 独立限界上下文，三层对账（前置/执行后/渠道）统一编排
  - 依赖方向：split/payment → recon（recon 不 import 任何业务域）；跨域取数走 ports Fetcher，main.go 装配适配器
  - `repository.DiffStore` — t_reconcile_diff 唯一写入/查询入口；幂等按「商户+业务日+类型+未核销」清理重写（修复渠道对账按 biz_date 全清误删分账差异的缺陷）
  - 调度任务：`split_daily_reconcile`（02:30）、`reconcile_daily`（09:00），均注册 framework Runner（自带 runlog）+ 手动触发
  - 前置对账异常 runlog 名 `split_precheck`；渠道 SHORT 单商户归属经 t_order 关联回填（查不到留 NULL）
  - 差错中心/管理端查询：split service 与 admin service 直接使用 recon DiffStore（未设 query 包）；ListExceptions/ListAudits 仍属 split 域
- **单元测试覆盖**：
  - `alloc` — 比例分配、固定金额、末笔补齐、ALL_STORES 展开、超总额拒绝、边界 ✅
  - `state` — 8 状态穷举 + 非法转移 ✅
  - `rule` — 条件匹配、优先级、分配方案解析 ✅
  - `event/bus` — 单/多订阅者、错误传播、无订阅者、不同类型 ✅
  - `event/handler` — DEAD 告警、SUCCESS 不告警、无效载荷、各事件类型 ✅
  - `event/outbox` — SQLite 集成测试：Insert/Poll/MarkProcessed/MarkFailed/Delete/PublishEvent/OnConflict ✅
  - `executor/status_sync` — determineFinalStatus 纯函数：SUCCESS/PARTIAL/TIMEOUT/DEAD 穷举 ✅
  - `recon/compare` — Totals/Rows/MatchBills（LONG/SHORT/MISMATCH/双键匹配）✅
  - `recon/repository` — SQLite 集成测试：写入幂等、已核销保留、跨类型隔离、查询过滤、核销 ✅
  - `recon/adapter` — 口径 SQL 断言 + MySQL 冒烟测试（HUIPAY_SMOKE_DSN 门控，默认跳过）✅
  - 待补充：`service/`、`handler/`、`scheduler/`、`repository/`
- **事件类型**：`SPLIT_ORDER_EXECUTED`、`SPLIT_BILL_APPROVED`、`SPLIT_BILL_REJECTED`、`RECONCILE_DIFF_RESOLVED`

## 运行与预览

### 预览（huipay-web 子项目，3 portal 统一入口 + 后端 API 代理）

- **预览入口**：`http://localhost:5000`（路由服务器，自动分发到各 Vite dev server）
- **访问路径**：
  - `/merchant/` → 商家工作台（merchant-portal，Vite dev :5170）
  - `/admin/` → 管理后台（admin-portal，Vite dev :5171）
  - `/checkout/` → 收银台 SDK（checkout-sdk，Vite dev :5173）
  - `/checkout/h5` → 收银台 H5 页
  - `/v1/*` → 代理到后端 API（`localhost:5001`）
- **启动命令**：`cd huipay-web && bash scripts/run.sh`（自动启动 3 个 Vite dev server + 路由服务器）

### 后端开发（huipay-backend 子项目）

- **数据库**：MySQL 8.0（本地安装，root 密码 `root123`，业务库 `huipay`）
- **后端启动**：`HUIPAY_HTTP_PORT=5001 /tmp/huipay-server`（编译产物在 `/tmp/huipay-server`）
- **跳过数据库启动**：`HUIPAY_SKIP_DB=true`（可在无 MySQL 时启动）
- **端口**：开发默认 8080，预览/部署用 5001 避免与前端冲突
- **启动命令**：`cd huipay-web && bash scripts/run.sh`（自动启动 3 个 Vite dev server + 路由服务器）
- **预览脚本**：`.coze [dev]` → `scripts/build.sh`（安装依赖）+ `scripts/run.sh`（启动全门户预览）
- **事件系统**：outbox 轮询每 5s 拉取 `t_outbox_event` 待处理事件，投递到内存总线

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
- 单元测试使用 SQLite 内存数据库（`github.com/glebarez/sqlite`），无需 MySQL
- `CURRENT_TIMESTAMP(3)` 是 MySQL 语法，SQLite 测试中需用 `CURRENT_TIMESTAMP` 替代