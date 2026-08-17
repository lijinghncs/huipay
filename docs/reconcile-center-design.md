# 三步对账过程与结果 · 管理端「对账中心」详细设计文档

> 版本：v1.0
> 日期：2026-08-14
> 状态：已确认关键决策，待评审

## 1. 背景与目标

当前系统存在三层对账机制（前置对账 / 渠道对账 / 分账执行后对账），但仅**对账结果**（差异）在管理端 SplitManage「对账差异」Tab 与商户端「差错中心」可见；**对账过程**（运行日志、比对记录、耗时、执行状态）后端已有接口但前端无页面，且**前置对账（同步执行）无独立运行日志**。

**目标**：在**平台管理端**新增「对账中心」页面，使三层对账的**过程与结果**均可统一查看，形成可追溯、可审计、可人工介入的对账观测视图。

## 2. 关键决策（已确认）

| 决策点 | 结论 | 理由 |
|---|---|---|
| 查看入口 | **平台管理端** | 对账是平台级资金安全机制，跨商户、跨渠道；运行日志/比对口径属平台内部机制，不应暴露给商户；商户端差错中心已覆盖自查+核销诉求 |
| 前置对账留痕 | **补充独立运行日志（仅异常时记录）** | 前置对账同步触发可能高频，仅记录异常/不平场景避免日志膨胀；同时使"过程"可回溯 |
| 管理端核销 | **仅查看，不提供核销** | 差异核销保留在商户端差错中心（商户自查自核）；管理端聚焦观测与触发 |
| 交付形式 | 落成 md 文档 | 本文档 |

## 3. 三层对账数据现状

| 对账层 | 触发时机 | 过程数据源 | 结果差异类型 |
|---|---|---|---|
| ① 前置对账（Prechecker） | 分账执行/试算/生成账单时（同步） | 审计 `RECONCILE_*` + **新增异常 runlog `split_precheck`** | `SPLIT_TOTAL` / `SPLIT_DETAIL` |
| ② 渠道对账 | 每日 09:00（T+1 定时） | runlog `reconcile_daily` | `LONG` / `SHORT` / `MISMATCH` |
| ③ 分账执行后对账 | 每日 02:30（T+1 定时） | runlog `split_daily_reconcile` | `SPLIT_POST` / `SPLIT_DEGRADED` |

## 4. 总体架构

```
┌──────────────────────────────────────────────────────────┐
│              平台管理端 · 对账中心（admin-portal）           │
│  ┌──────────┬──────────┬──────────┬──────────┬─────────┐ │
│  │ 运行概览 │ 对账任务 │ 前置对账 │ 渠道对账 │ 执行后 │ │
│  │  (卡片)  │  (日志)  │ (差异+审计)│ (差异)  │ (差异)  │ │
│  └────┬─────┴────┬─────┴────┬─────┴────┬─────┴────┬────┘ │
└───────┼──────────┼──────────┼──────────┼──────────┼──────┘
        │  /v1/admin/scheduler/runs  │  /v1/admin/reconcile-diffs  │  /v1/admin/split/audit
        ▼                           ▼                            ▼
┌──────────────────────────────────────────────────────────────────┐
│  t_scheduler_run_log（三层对账过程）   t_reconcile_diff（结果）   │
│  t_split_audit（前置对账审计）                                     │
└──────────────────────────────────────────────────────────────────┘
```

## 5. 后端改造

### 5.1 前置对账补充独立运行日志（仅异常时记录）

**现状**：`Prechecker.Check` 为同步执行，仅写审计与差异，无运行日志。

**改造方案**：在 `internal/split/recon/prechecker.go` 的 `Check` 中，对**不平/失败场景**调用 `framework.RunLogged` 写入运行日志（`name = "split_precheck"`）：
- 运行状态：`FAILED`（不平时）
- 耗时 `duration_ms`
- 影响行 `rows_affected`（差异条数）
- 失败原因 `error_message`（含 `SPLIT_TOTAL` / `SPLIT_DETAIL` 差异摘要）

**策略要点**：仅在前置对账**发现不平/异常**时记录，通过（`PASS`）场景不写运行日志；避免分账高频触发导致 `t_scheduler_run_log` 膨胀。审计仍保留全量留痕（`RECONCILE_PASSED` / `RECONCILE_FAILED`）。

**涉及文件**：
- `internal/split/recon/prechecker.go`
- `internal/split/service/service.go`（调用 Prechecker 处）

### 5.2 管理端接口（复用为主）

| 接口 | 方法 / 路径 | 用途 | 状态 |
|---|---|---|---|
| 运行日志列表 | `GET /v1/admin/scheduler/runs?name=&status=&start=&end=&page=&page_size=` | 三层对账过程 | 已有 |
| 运行日志详情 | `GET /v1/admin/scheduler/runs/:id` | 单次运行明细 | 已有 |
| 任务列表（含最近运行） | `GET /v1/admin/scheduler/tasks` | 运行概览卡片数据 | 已有 |
| 手动触发任务 | `POST /v1/admin/scheduler/tasks/:name/run` | 手动重跑对账 | 已有 |
| 对账差异列表 | `GET /v1/admin/reconcile-diffs?diff_type=&merchant_id=&start_date=&end_date=` | 三层差异结果 | 已有 |
| 审计日志列表 | `GET /v1/admin/split/audit?biz_type=&action=` | 前置对账过程 | 已有 |

> 结论：后端**无需新增查询接口**，仅需为前置对账补充异常 runlog 写入（5.1）。

## 6. 前端「对账中心」页面设计

### 6.1 路由与菜单

- 新增路由：`/reconcile-center` → `pages/ReconcileCenter/index.tsx`
- 新增菜单：平台管理端侧边栏「对账中心」（icon：`AuditOutlined`）
- 入口文件：`packages/admin-portal/src/App.tsx` + `src/layouts/BasicLayout.tsx`（`menuItems` 硬编码处）

### 6.2 页面结构（Tabs，均为只读 + 触发）

**Tab 1 · 运行概览**
- 三层对账任务卡片（`GET /v1/admin/scheduler/tasks`，按 name 过滤 3 个任务）
- 每卡片：任务名、最近状态（成功/失败/运行中）、最近运行时间、耗时、差异条数
- 点击可跳转「对账任务」Tab 过滤对应任务

**Tab 2 · 对账任务运行**
- 运行日志列表（`GET /v1/admin/scheduler/runs`）
- 筛选：任务名（3 个对账任务）、状态、时间范围
- 列：任务名、运行方式（自动/手动）、状态、业务日、耗时、差异条数、起止时间
- 操作：**查看详情**（Drawer，含错误信息）、**手动触发**（Popconfirm 确认）

**Tab 3 · 前置对账**
- 差异列表：`diff_type ∈ {SPLIT_TOTAL, SPLIT_DETAIL}`
- 过程联动：展示 `t_split_audit` 中 `RECONCILE_*` 的 check 明细（总额/门店日比对 JSON）

**Tab 4 · 渠道对账**
- 差异列表：`diff_type ∈ {LONG, SHORT, MISMATCH}`
- 含本地/通道金额、订单号、交易号

**Tab 5 · 分账执行后对账**
- 差异列表：`diff_type ∈ {SPLIT_POST, SPLIT_DEGRADED}`
- 含本地账本/执行金额、订单号

### 6.3 差异类型状态徽章（复用/扩展）

管理端 SplitManage 已有 `statusBadge`，扩展覆盖 `SPLIT_POST` / `SPLIT_DEGRADED`。

### 6.4 权限边界

- 对账中心为**管理端只读观测**页，不提供差异核销（核销在商户端差错中心）。
- 手动触发为管理端运营操作，需 Popconfirm 二次确认。

## 7. 数据字典

### 7.1 运行日志 `t_scheduler_run_log`（对账过程）

| 字段 | 说明 |
|---|---|
| name | 任务名：`reconcile_daily` / `split_daily_reconcile` / `split_precheck` |
| instance_id | 实例 ID |
| biz_date | 业务日 |
| run_mode | AUTO / MANUAL |
| status | RUNNING / SUCCESS / FAILED / TIMEOUT |
| started_at / finished_at | 起止时间 |
| duration_ms | 耗时 |
| rows_affected | 差异条数 / 影响行 |
| error_message | 失败原因 |

> 注：`split_precheck` 仅异常场景写入，`reconcile_daily` / `split_daily_reconcile` 为每次运行写入。

### 7.2 对账差异 `t_reconcile_diff`（结果）

| 字段 | 说明 |
|---|---|
| diff_type | SPLIT_TOTAL / SPLIT_DETAIL / LONG / SHORT / MISMATCH / SPLIT_POST / SPLIT_DEGRADED |
| biz_date | 业务日 |
| merchant_id | 商户 |
| order_no / transaction_id | 订单号 / 交易号 |
| local_amount / channel_amount | 本地金额 / 通道金额（分） |
| detail | 比对明细 JSON |
| resolved_at | 核销时间 |

## 8. 实施任务清单

| # | 任务 | 端 | 说明 |
|---|---|---|---|
| 1 | 前置对账补充异常运行日志 | 后端 | 5.1 |
| 2 | 新增对账中心路由与菜单 | 管理端 | 6.1 |
| 3 | 运行概览 Tab | 管理端 | 6.2 |
| 4 | 对账任务运行 Tab | 管理端 | 6.2 |
| 5 | 前置对账 Tab | 管理端 | 6.2 |
| 6 | 渠道对账 Tab | 管理端 | 6.2 |
| 7 | 分账执行后 Tab | 管理端 | 6.2 |
| 8 | 扩展状态徽章 | 管理端 | 6.3 |
| 9 | 联调与验证 | 全部 | |

## 9. 评审备注

- 前置对账仅异常记录策略已确认，避免高频日志膨胀；审计仍全量留痕。
- 对账中心为管理端只读 + 手动触发，核销保留在商户端差错中心。
- 后端复用现有接口，无新增查询接口，改动聚焦前置对账日志 + 前端页面。