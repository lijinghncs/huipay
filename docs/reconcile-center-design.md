# 三步对账过程与结果 · 管理端「对账中心」详细设计文档

> 版本：v1.1
> 日期：2026-08-17
> 状态：已对齐 recon V2 架构（docs/reconcile-architecture-v2.md），待评审
>
> v1.1 变更：①§5.1 前置对账异常 runlog 已随 V2 重构落地，标记完成；②修正「管理端不核销」决策与代码现实的矛盾（§2）；③新增「全过程联动」设计（§6.5：运行→差异跳转、业务日时间线）；④文件路径对齐 internal/recon/*。

## 1. 背景与目标

当前系统存在三层对账机制（前置对账 / 渠道对账 / 分账执行后对账），但仅**对账结果**（差异）在管理端 SplitManage「对账差异」Tab 与商户端「差错中心」可见；**对账过程**（运行日志、比对记录、耗时、执行状态）后端已有接口但前端无页面，且**前置对账（同步执行）无独立运行日志**。

**目标**：在**平台管理端**新增「对账中心」页面，使三层对账的**过程与结果**均可统一查看，形成可追溯、可审计、可人工介入的对账观测视图。

## 2. 关键决策（已确认）

| 决策点 | 结论 | 理由 |
|---|---|---|
| 查看入口 | **平台管理端** | 对账是平台级资金安全机制，跨商户、跨渠道；运行日志/比对口径属平台内部机制，不应暴露给商户；商户端差错中心已覆盖自查+核销诉求 |
| 前置对账留痕 | **补充独立运行日志（仅异常时记录）** | 前置对账同步触发可能高频，仅记录异常/不平场景避免日志膨胀；同时使"过程"可回溯 |
| 管理端核销 | **提供核销（v1.1 修订）** | v1.0 曾决策「仅查看」，但代码现实是管理端核销接口（`POST /v1/admin/reconcile-diffs/:id/resolve`）与 SplitManage 前端核销按钮均已存在；运营场景（商户未及时处理平台侧差异）也需要管理端兜底。故修订为保留，核销留痕走审计 |
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

### 5.1 前置对账补充独立运行日志（仅异常时记录）✅ 已实现

**现状**：`Prechecker.Check` 为同步执行，仅写审计与差异，无运行日志。

**改造方案**：在 `Check` 中，对**不平/失败场景**经 `ports.RunLogger` 写入运行日志（`name = "split_precheck"`）：
- 运行状态：`FAILED`（不平时）
- 耗时 `duration_ms`
- 影响行 `rows_affected`（差异条数）
- 失败原因 `error_message`（含 `SPLIT_TOTAL` / `SPLIT_DETAIL` 差异摘要）

**策略要点**：仅在前置对账**发现不平/异常**时记录，通过（`PASS`）场景不写运行日志；避免分账高频触发导致 `t_scheduler_run_log` 膨胀。审计仍保留全量留痕（`RECONCILE_PASSED` / `RECONCILE_FAILED`）。

**实现落点**（reconcile-architecture-v2 重构 P1 已落地）：
- `internal/recon/job/precheck/precheck.go` — 不平路径经 `ports.RunLogger` 写 `split_precheck` FAILED runlog（`bizDate = start`，`rows_affected = 差异条数`）
- `cmd/server/main.go` — `runLoggerAdapter` 包装 `framework.RunLogged` 注入

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

- 对账中心以**只读观测**为主；差异核销作为运营兜底能力提供（见 §2 决策），核销操作记录审计（操作人、时间）。
- 手动触发为管理端运营操作，需 Popconfirm 二次确认。

### 6.5 全过程联动（可跟踪的关键）

对账「可跟踪」不止是分 Tab 展示，还要能沿两条线索串联全过程：

**线索一：运行 → 差异（从过程追结果）**
- 「对账任务运行」Tab 的每行 runlog 提供「查看关联差异」操作，跳转对应差异 Tab 并自动带入筛选：
  - `split_daily_reconcile` → 执行后 Tab，`diff_type ∈ {SPLIT_POST, SPLIT_DEGRADED}` + `biz_date = 运行业务日`
  - `reconcile_daily` → 渠道 Tab，`diff_type ∈ {LONG, SHORT, MISMATCH}` + `biz_date = 运行业务日`
  - `split_precheck` → 前置 Tab，`diff_type ∈ {SPLIT_TOTAL, SPLIT_DETAIL}` + 时间范围 = 运行起止时间
- 关联依据为「任务名 → 差异类型映射 + 业务日/时间窗」，**无需改表结构**。runlog 的 `rows_affected` 即该次运行写入的差异条数，可与差异列表计数互验。
- 注意幂等重写语义：同一业务日重跑会替换未核销差异，历史 runlog 关联到的是**当前存活**差异；差异被重跑替换属预期行为，页面在差异 Tab 以 `created_at` 体现最近一次写入时间。

**线索二：业务日 → 三层全景（从时间追过程）**
- 「运行概览」Tab 提供业务日选择器：选定某业务日后，纵向展示该日三层对账轨迹：
  1. 前置对账：该日 `RECONCILE_PASSED/FAILED` 审计事件数、`split_precheck` 异常 runlog
  2. 执行后对账：`split_daily_reconcile` 该业务日运行记录（状态/耗时/差异条数）
  3. 渠道对账：`reconcile_daily` 该业务日运行记录
  4. 结果汇总：各 diff_type 差异条数与未核销条数
- 数据来自 §5.2 现有接口（diffs 按日期过滤、audit 按 biz_id=日期过滤）+ 一处小后端补充：`GET /v1/admin/scheduler/runs` 增加 `biz_date` 过滤参数（RunFilter 现有 name/status/started_at，无业务日过滤），前端按业务日聚合展示。

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

| # | 任务 | 端 | 说明 | 状态 |
|---|---|---|---|---|
| 1 | 前置对账补充异常运行日志 | 后端 | 5.1 | ✅ 已随 recon V2 重构落地 |
| 1.5 | 运行记录接口增加 biz_date 过滤 | 后端 | 6.5 线索二 | 待实施 |
| 2 | 新增对账中心路由与菜单 | 管理端 | 6.1 | 待实施 |
| 3 | 运行概览 Tab（含业务日全景，6.5 线索二） | 管理端 | 6.2 / 6.5 | 待实施 |
| 4 | 对账任务运行 Tab（含关联差异跳转，6.5 线索一） | 管理端 | 6.2 / 6.5 | 待实施 |
| 5 | 前置对账 Tab | 管理端 | 6.2 | 待实施 |
| 6 | 渠道对账 Tab | 管理端 | 6.2 | 待实施 |
| 7 | 分账执行后 Tab | 管理端 | 6.2 | 待实施 |
| 8 | 扩展状态徽章 + 核销操作（审计留痕） | 管理端 | 6.3 / §2 | 待实施 |
| 9 | 联调与验证 | 全部 | | 待实施 |

## 9. 评审备注

- 前置对账仅异常记录策略已确认，避免高频日志膨胀；审计仍全量留痕。
- 管理端核销按 v1.1 决策保留（运营兜底），核销留痕走审计；商户端差错中心核销不受影响。
- 全过程联动（§6.5）基于现有接口与「任务名→差异类型+业务日」关联实现，不改表结构、不新增接口。
- 后端复用现有接口，无新增查询接口，改动聚焦前置对账日志 + 前端页面。