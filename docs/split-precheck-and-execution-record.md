# 分账前置对账 + 每日执行记录 迭代方案

> 目标：
> 1. **分账前置对账**：按时间段分账前，按「门店 × 日」逐日校验 `t_order` 未分账订单实收合计 vs `t_store_daily_stats.paid_amount`；任意门店 × 日不平则整批拒绝分账。
> 2. **每日分账执行记录**：新建 `t_split_daily_execution`，按 `(merchant_id, biz_date)` 记录每日分账执行情况（成功/失败、批次号、规则、金额、接收方、对账差异快照）。
> 3. **门店日报「已分账」状态**：在 `t_store_daily_stats` 增加分账状态字段（`split_status / split_batch_no / split_at`），并在商户/管理端报表中展示。
> 4. **商户/管理端门店统计列表增加「是否分账」字段**：merchant-portal 与 admin-portal 的 StoreStats 列表展示 + 详情下钻。
>
> 范围：基于已有 `t_store_daily_stats`、`t_split_bill`、`t_split_execution` 扩展；不重复实现支付对账。

---

## 一、现状速查

| 关注点 | 现状 |
|---|---|
| `t_store_daily_stats` | 已建（迁移 0024）：`(store_id, biz_date)` 唯一键，存 `order_count / paid_amount / channel_breakdown / status_breakdown / generated_at`；无分账状态字段 |
| 分账执行明细 | `t_split_execution`（按 `order_no, receiver_entity_id` 记录每笔接收方分账） |
| 分账批次记录 | `t_split_bill`（按时间段分账后落地一条；含 `batch_no/rule_code/start_time/end_time/total_amount/order_nos/status`） |
| 按时间段分账入口 | `Service.ExecuteByPeriod`：拿时间段内商户实收总额 → 规则分配 → executor 落地 → 写 `t_split_bill` → 回填 `t_order.split_batch_no` |
| 排除已分账 | `StoreRevenueRepo.splitExclusion()`：`t_split_execution.status='SUCCESS'` 或 `t_split_bill.status IN (PENDING/APPROVED/EXECUTED)` |
| 现有对账机制 | **无** 「门店日报 vs 未分账订单」前置对账 |
| 商户端门店统计 | `merchant-portal/pages/StoreStats/index.tsx`：列表 + CSV 导出；无分账字段 |
| 管理端报表 | `admin/handler/store_stats.go` 已实现 `List / Summary / Daily`；前端**未实现** admin StoreStats 页 |
| `t_order.split_status` | 已有 `split_status / split_batch_no` 字段 |

---

## 二、关键决策（已与用户对齐）

| 决策点 | 取值 | 理由 |
|---|---|---|
| 对账粒度 | **门店 × 日**（store_id × biz_date） | 用户选择；细粒度更早发现日报与订单不一致 |
| 容差 | **0 容差** | 严格平账才分账 |
| 对账范围 | 「按时间段分账」入口专用；单笔 `Execute` 不前置对账 | 单笔分账只对一笔订单，不与日报绑定 |
| 比较口径 | `SUM(paid_amount)` of 未分账订单（PAID、`paid_at` ∈ [T 日 00:00, T+1 日 00:00)、`store_id IS NOT NULL`、排除 `t_split_execution.status='SUCCESS'` 与未驳回账单） vs `t_store_daily_stats.paid_amount` | 与日报聚合 SQL 的口径一致（含 PAID，排除 REFUNDED），避免口径不一致导致的对账失败 |
| 失败处理 | 拒绝执行；返回 `RECONCILE_FAILED` 错误码 + 差异明细 `diffs: [{store_id, biz_date, t_order_total, stats_total, diff}]` | 0 容差下必须阻断，否则资金错配 |
| 重试策略 | 失败后可让商户/运维先重跑门店日报 → 再重试分账 | 与 `Backfill` 接口复用 |
| 每日执行记录粒度 | `(merchant_id, biz_date)` 一行；同一日多次分账（不同规则/重试）以**最新一次**覆盖，分账明细走 `t_split_execution` 与 `t_split_bill` | 与现有 `t_split_bill` 互补：bill 按「批次」，daily_exec 按「日」 |
| 已分账标识 | `split_status: PENDING/SUCCESS/PARTIAL/FAILED` + `split_batch_no` + `split_at`；SUCCESS 需 `executor` 完全成功；PARTIAL 表示部分接收方成功 | 与订单级 `t_split_order_status` 一致 |
| 重复分账防护 | 同 `(merchant_id, biz_date, store_id)` 已有 SUCCESS 记录的，再次发起分账视为「已分账，跳过」 | 配合 daily_exec 唯一键 |
| 商户端列表展示 | merchant-portal StoreStats：增加 `split_status / split_at` 列 + 颜色徽章 | 用户要求 |
| 管理端列表展示 | admin-portal 新增 StoreStats 页（含相同字段） | 用户要求 |

---

## 三、数据模型（迁移 0026）

```sql
-- 0026_add_split_precheck_and_daily_execution.up.sql

-- 1) t_store_daily_stats 增加分账相关字段
ALTER TABLE t_store_daily_stats
  ADD COLUMN split_status       VARCHAR(16)  NOT NULL DEFAULT 'PENDING' COMMENT 'PENDING/SUCCESS/PARTIAL/FAILED' AFTER status_breakdown,
  ADD COLUMN split_batch_no     VARCHAR(64)  NULL     COMMENT '对应分账批次号' AFTER split_status,
  ADD COLUMN split_at           DATETIME(3)  NULL     COMMENT '分账完成时间(SUCCESS/PARTIAL/FAILED)' AFTER split_batch_no,
  ADD COLUMN split_receiver_count INT         NOT NULL DEFAULT 0 COMMENT '分账接收方数(SUCCESS/PARTIAL 时统计)' AFTER split_at,
  ADD KEY idx_split_status (split_status, biz_date);

-- 2) 每日分账执行记录表
CREATE TABLE t_split_daily_execution (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  merchant_id     BIGINT UNSIGNED NOT NULL,
  biz_date        DATE            NOT NULL COMMENT '业务日期(与 store_daily_stats 一致)',
  rule_id         BIGINT UNSIGNED NOT NULL COMMENT '执行的规则 ID',
  rule_code       VARCHAR(64)     NOT NULL,
  rule_name       VARCHAR(128)    NOT NULL,
  batch_no        VARCHAR(64)     NOT NULL COMMENT '对应 t_split_bill.batch_no',
  start_time      DATETIME(3)     NOT NULL COMMENT '执行时段起(UTC+8)',
  end_time        DATETIME(3)     NOT NULL COMMENT '执行时段止(UTC+8)',
  store_count     INT             NOT NULL DEFAULT 0 COMMENT '本次分账覆盖门店数',
  total_amount    BIGINT          NOT NULL DEFAULT 0 COMMENT '本次分账总额(分)',
  status          VARCHAR(16)     NOT NULL DEFAULT 'RUNNING' COMMENT 'RUNNING/SUCCESS/FAILED',
  error_code      VARCHAR(64)     NULL     COMMENT '业务错误码',
  error_message   VARCHAR(1024)   NULL,
  diff_snapshot   JSON            NULL     COMMENT '对账差异快照(失败时记录)',
  started_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  finished_at     DATETIME(3)     NULL,
  operator_type   VARCHAR(16)     NOT NULL DEFAULT 'SYSTEM' COMMENT 'SYSTEM/MERCHANT/ADMIN',
  operator_id     BIGINT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_merchant_date (merchant_id, biz_date, batch_no) COMMENT '同日同批次号幂等',
  KEY idx_merchant_started (merchant_id, started_at),
  KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每日分账执行记录';

-- 3) t_split_bill 增加关联 biz_date（便于追溯「该账单对应哪几日门店日报」）
ALTER TABLE t_split_bill
  ADD COLUMN biz_dates JSON NULL COMMENT '账单覆盖的业务日期(YYYY-MM-DD 列表)' AFTER order_nos;
```

字段说明：
- `split_receiver_count`：分账执行器返回的成功接收方数；用于展示分账覆盖门店数（与 `store_count` 区别：daily_exec 反映本次执行，store_daily_stats 反映该门店当日最终是否完成分账）。
- `diff_snapshot`：失败时记录 `[{store_id, biz_date, t_order_total, stats_total, diff}]` 供排查；不存明细订单号（已存在于 `t_split_bill.order_nos`）。
- `uk_merchant_date` 唯一键：`(merchant_id, biz_date, batch_no)`，避免同日同批次重入重复记录。

---

## 四、模块变更

### 4.1 新建 `internal/split/recon/precheck.go`（前置对账）

```go
// Diff 门店×日对账差异。
type Diff struct {
    StoreID    uint64 `json:"store_id"`
    StoreName  string `json:"store_name"`
    BizDate    string `json:"biz_date"`
    OrderTotal int64  `json:"order_total"`  // 未分账订单实收合计（分）
    StatsTotal int64  `json:"stats_total"`  // t_store_daily_stats.paid_amount（分）
    Diff       int64  `json:"diff"`         // OrderTotal - StatsTotal
}

// Prechecker 门店×日平账前置校验。
type Prechecker struct {
    db          *gorm.DB
    dailyStats  *statsrepo.StoreDailyStatsRepo  // 注入依赖
    orderRepo   OrderPaidSumQuerier             // 接口：按门店×日聚合未分账订单实收
    logger      *zap.Logger
}

// Check 执行前置对账。
// 入参：merchantID、start、end、storeIDs(可选)。
// 返回：diffs（空表示全部平账）、error（业务错误如「日报缺失」）。
func (p *Prechecker) Check(ctx, merchantID, start, end, storeIDs) (diffs []Diff, error)
```

核心 SQL：
```sql
-- 每日每门店未分账订单实收合计
SELECT DATE(o.paid_at) AS biz_date,
       o.store_id,
       COALESCE(SUM(o.paid_amount), 0) AS order_total
FROM t_order o
WHERE o.merchant_id = ?
  AND o.status = 'PAID' AND o.deleted_at IS NULL
  AND o.paid_at >= ? AND o.paid_at < ?
  AND o.store_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM t_split_execution se
                  WHERE se.order_no = o.order_no AND se.status='SUCCESS')
  AND NOT EXISTS (SELECT 1 FROM t_split_bill sb
                  WHERE sb.merchant_id = o.merchant_id
                    AND sb.status IN ('PENDING','APPROVED','EXECUTED')
                    AND JSON_CONTAINS(sb.order_nos, JSON_QUOTE(o.order_no)))
GROUP BY DATE(o.paid_at), o.store_id;

-- 与 t_store_daily_stats 逐行 LEFT JOIN 比对
-- 找出 order_total != stats_total 的行；缺失门店×日也视为 diff=order_total
```

校验规则：
- 「时段内任意营业日」**必须**在 `t_store_daily_stats` 中存在；缺失（如日报未跑）→ 整批拒绝，错误码 `STATS_NOT_READY`，文案「门店日报未生成，请先执行门店日报 T+1 任务」。
- 差异总数 ≤ 0 容差 → 返回 `diffs`，错误码 `RECONCILE_FAILED`，附 `diffs` JSON 让前端展示哪些门店×日不平。

### 4.2 改造 `internal/split/service/service.go`

#### 4.2.1 `ExecuteByPeriod` 改造（前置对账 + 落地记录）

```
ExecuteByPeriod(ctx, merchantID, req):
    1) 解析时间区间（已实现）
    2) 选定规则（已实现）
    3) ★ 新增：Prechecker.Check(ctx, merchantID, start, end, nil)
        → 非空 diffs：return errs.New(RECONCILE_FAILED, "...", 200, diffs)
        → 报日报未生成：return errs.New(STATS_NOT_READY, "...", 200)
    4) 写 daily_execution RUNNING
    5) 计算 total = SumPaid(...)（已实现）
    6) executor.Execute(...)（已实现）
    7) ★ 新增：写 daily_execution SUCCESS/FAILED（含 store_count/total_amount/finished_at）
    8) ★ 新增：CAS 更新 t_store_daily_stats.split_status / split_batch_no / split_at
        - 全成功：所有覆盖门店×日 → split_status='SUCCESS', split_batch_no, split_at=NOW
        - 部分成功：split_status='PARTIAL', split_receiver_count = 成功接收方数
        - 失败：split_status='FAILED'（若对账通过但执行失败）
    9) 写 t_split_bill（已实现，新增 biz_dates JSON）
    10) 回填 t_order.split_batch_no（已实现）
```

#### 4.2.2 `Execute`（单笔）改造

- 单笔分账不属于「按门店×日」口径，**不**前置对账；
- 但若该订单所属门店×日已是 `SUCCESS`，应**拒绝**重复发起，返回 `ALREADY_SPLIT`；
- 若已是 `PARTIAL/FAILED`，允许覆盖（重试语义）。

### 4.3 新建 `internal/split/repository/daily_execution_repo.go`

```go
type DailyExecutionModel struct { /* 与迁移 0026 一一对应 */ }

type DailyExecutionRepo struct{ db *gorm.DB }

func (r *DailyExecutionRepo) Create(ctx, m *DailyExecutionModel) error
func (r *DailyExecutionRepo) MarkStatus(ctx, id uint64, status, errorCode, errorMessage string, diffSnapshot any) error
func (r *DailyExecutionRepo) GetByMerchantDate(ctx, merchantID uint64, bizDate time.Time, batchNo string) (*DailyExecutionModel, error)
func (r *DailyExecutionRepo) ListByMerchantDateRange(ctx, merchantID, start, end, offset, limit) ([]DailyExecutionModel, int64, error)
```

唯一键冲突处理：`Create` 命中 `uk_merchant_date` → 视为「已记录」，返回 nil；外部通过 `GetByMerchantDate` 决定是否覆盖更新。

### 4.4 改造 `internal/stats/repository/store_daily_stats_repo.go`

新增：
```go
func (r *StoreDailyStatsRepo) BatchUpdateSplitStatus(ctx, merchantID, bizDate, batchNo string, storeIDs []uint64, status string, successReceiverCount int) error
    // 批量更新 (store_id, biz_date) IN (...) 的 split_status / split_batch_no / split_at
    // 使用单条 SQL: UPDATE t_store_daily_stats SET ... WHERE merchant_id=? AND biz_date=? AND store_id IN (?)
func (r *StoreDailyStatsRepo) MarkSplitStatusByBatch(ctx, merchantID, batchNo, status string) error
    // 兜底：失败时回滚全部受影响门店×日为 PENDING（或保持不变，按策略）
```

### 4.5 新增 `t_split_audit` 触发点（沿用 split-resilience D2）

- `ExecuteByPeriod` 成功后写 `t_split_audit(biz_type='DAILY_SPLIT', biz_id=biz_date, action='EXECUTE', detail={batch_no, rule_code, total, store_count})`；
- 失败时 `action='EXECUTE_FAILED'` + diff_snapshot 摘要。

---

## 五、接口与组件变更

### 5.1 新增/改造 HTTP 接口

| 方法 | 路径 | 变更 | 说明 |
|---|---|---|---|
| POST | `/v1/merchant/split/execute-period` | **改造** | 对账失败返回 `RECONCILE_FAILED` 错误体含 `diffs[]`；前端展示差异明细 |
| GET | `/v1/admin/split/daily-executions` | **新增** | 管理端查看每日执行记录，参数 `merchant_id? / start_date / end_date / status? / page` |
| GET | `/v1/admin/split/daily-executions/:id` | **新增** | 单次执行详情（含 `diff_snapshot`） |
| GET | `/v1/admin/store-stats` | **改造** | 返回字段新增 `split_status / split_batch_no / split_at / split_receiver_count` |
| GET | `/v1/admin/store-stats/summary` | **改造** | 汇总行新增 `split_status: 'ALL' / 'PARTIAL' / 'NONE'`（按门店×日 split_status 投票） |
| GET | `/v1/admin/stores/:id/daily-stats` | **改造** | 返回每日明细含 `split_status / split_batch_no / split_at` |
| GET | `/v1/merchant/store-stats` | **改造** | 商户端列表字段同样扩展 |
| GET | `/v1/merchant/store-stats/stores/:id/daily` | **改造** | 同上 |

### 5.2 错误码新增

| 错误码 | HTTP | 文案 | 备注 |
|---|---|---|---|
| `STATS_NOT_READY` | 200 | 门店日报未生成，请先执行门店日报任务 | 前端引导去手动补跑 |
| `RECONCILE_FAILED` | 200 | 分账前置对账失败：{差异条数} | body 附带 `diffs` |
| `ALREADY_SPLIT` | 200 | 该订单所属门店×日已完成分账 | 单笔分账时返回 |

### 5.3 前端变更

#### 5.3.1 `merchant-portal` `pages/StoreStats/index.tsx`

- 列表列扩展：`已分账`（徽章 `已分账/部分分账/未分账/分账失败` + 对应颜色）、`分账时间`、`分账批次号`（点击复制）；
- 行操作：若 `split_status='FAILED'` 显示「查看失败原因」（弹窗展示执行记录 `error_message`）；
- 范围选择新增筛选：`split_status` 多选；
- 表头汇总 KPI 增加：「已分账门店×日数 / 总数」。

#### 5.3.2 `merchant-portal` 服务定义（`services/user.ts`）

- `StoreStatItem` 类型扩展：`split_status / split_batch_no / split_at / split_receiver_count`；
- 新增 `listDailyExecutions(params)` / `getDailyExecution(id)`（供管理员弹窗调用，不强制商户用）。

#### 5.3.3 `admin-portal` 新增 `pages/StoreStats/index.tsx`

- 与 merchant 版同结构，外加 `merchant_id` 筛选；
- 新增「分账执行记录」Tab（`pages/StoreStats/ExecutionsTab.tsx`）：列表 + 失败详情抽屉（含 `diff_snapshot` 表格）。

#### 5.3.4 `admin-portal` 服务定义（`services/admin.ts`）

- `listStoreStats / getStoreStatsSummary / getStoreDailyStats` 同步扩展返回字段；
- 新增 `listDailyExecutions / getDailyExecution`；
- 路由 `config/routes.ts`：`/store-stats` 菜单保留；如已有可不动。

---

## 六、对账前置详细时序图（按时间段分账）

```
[商户] 点击"按时间段分账"
  │
  ▼
POST /v1/merchant/split/execute-period {start, end, rule_code}
  │
  ▼
Service.ExecuteByPeriod
  ├─ 1. 解析时间/选定规则
  ├─ 2. Prechecker.Check
  │     ├─ 2.1 扫描 t_order 聚合「未分账订单实收」按 (biz_date, store_id)
  │     ├─ 2.2 LEFT JOIN t_store_daily_stats 比较 paid_amount
  │     ├─ 2.3 若任意差异或日报缺失 → return RECONCILE_FAILED / STATS_NOT_READY
  │     └─ 2.4 全部平账 → 返回空 diffs
  ├─ 3. daily_execution_repo.Create(RUNNING)
  ├─ 4. SumPaid → total（已实现）
  ├─ 5. executor.Execute → 返回 success_count
  │     ├─ 全成功 → daily_execution_repo.MarkStatus(SUCCESS)
  │     └─ 部分/失败 → daily_execution_repo.MarkStatus(FAILED)
  ├─ 6. store_daily_stats_repo.BatchUpdateSplitStatus
  │     ├─ 全成功 → split_status='SUCCESS'
  │     └─ 部分成功 → split_status='PARTIAL', split_receiver_count=N
  ├─ 7. bill_repo.Create + 回填 split_batch_no（已实现）
  └─ 8. audit_repo.Write(EXECUTE / EXECUTE_FAILED)
```

---

## 七、迁移与兼容性

| 维度 | 处理 |
|---|---|
| 老数据 | `t_store_daily_stats` 新字段默认 `PENDING`；历史「未分账」记录继续展示 |
| `t_split_bill.biz_dates` | 新字段，nullable，老数据为 NULL，不影响现有查询 |
| 唯一键 | `uk_merchant_date(merchant_id, biz_date, batch_no)` 仅新增记录时生效 |
| 接口兼容 | `/v1/merchant/store-stats` 与 `/v1/admin/store-stats` 增加字段，旧前端忽略即可；前端必须升级后才能展示新字段 |
| 执行器调用 | `executor.Execute` 内部已统计 `success_count`，新增「返回成功接收方 ID 列表」以驱动 daily_exec.store_count + stats 批量更新 |
| 重跑门店日报（Backfill） | 不重置 `split_status`；已 SUCCESS 的行不被覆盖（聚合 SQL 不更新分账相关列），需管理员手动重置 |

---

## 八、任务清单

| 迭代 | 任务 | 涉及文件 |
|---|---|---|
| 数据 | 迁移 0026（store_daily_stats 增列 + daily_execution 新表 + bill.biz_dates） | `infra/migrator/migrations/0026_*.sql` |
| 数据 | `store_daily_stats_repo.BatchUpdateSplitStatus / MarkSplitStatusByBatch` | `internal/stats/repository/store_daily_stats_repo.go` |
| 数据 | 新建 `internal/split/repository/daily_execution_repo.go` | 同上 |
| 数据 | 新建 `internal/split/recon/prechecker.go` + `OrderPaidSumQuerier` 接口（`order_repo` 旁路） | `internal/split/recon/`、`internal/order/repository/` |
| 服务 | `Service.ExecuteByPeriod` 接入 Prechecker + daily_execution + store_daily_stats 更新 | `internal/split/service/service.go` |
| 服务 | `Service.Execute`（单笔）拒绝重复分账（门店×日已 SUCCESS） | `internal/split/service/service.go` |
| 服务 | 错误码 `STATS_NOT_READY / RECONCILE_FAILED / ALREADY_SPLIT` | `infra/errs/biz_error.go` |
| 服务 | executor 返回成功接收方 ID 列表（可选：扩展 `ExecuteResponse`） | `internal/split/executor/executor.go` |
| 接入 | main.go 注入 `prechecker / daily_execution_repo` 到 `splitSrv` | `cmd/server/main.go` |
| API | `/v1/admin/split/daily-executions[/:id]` handler/service/repo + 路由 | `internal/admin/handler/split_daily_exec.go`、`cmd/server/main.go` |
| 前端 | `merchant-portal/pages/StoreStats/index.tsx` 增加分账字段 | `merchant-portal/...` |
| 前端 | `merchant-portal/services/user.ts` 类型扩展 + `listDailyExecutions` | `merchant-portal/...` |
| 前端 | 新增 `admin-portal/pages/StoreStats/index.tsx` + 路由 + ExecutionsTab | `admin-portal/...` |
| 前端 | `admin-portal/services/admin.ts` 类型扩展 + daily_executions API | `admin-portal/...` |

---

## 九、依赖与顺序

1. 迁移 0026 + 仓储扩展 → 数据层就绪
2. Prechecker + executor 返回值扩展 → 计算层就绪
3. Service.ExecuteByPeriod 改造 → 主链路打通
4. admin handler + 前端 → 监控与展示
5. merchant-portal 列表增强 → 商户侧反馈

---

## 十、风险与缓解

| 风险 | 缓解 |
|---|---|
| 0 容差导致「日报微差异」全盘拒绝 | 文档明示「日报 T+1 跑完后才允许分账」；商户端在执行分账前展示日报状态 |
| `daily_execution` 写入失败但分账已成功 | 使用事务包住「executor.Execute → daily_execution.MarkStatus → store_daily_stats.Update」；写 daily_execution 失败则回滚（影响：分账已落库 → 必须手动修复，运营兜底） |
| 时区漂移导致 biz_date 偏差 | 沿用 `time.Local`；`diff_snapshot` 记录两侧都用的区间，前端展示便于核对 |
| 单笔 execute 误拒绝 | 仅当「所属门店×日已 SUCCESS」才拒；PARTIAL/FAILED 不拒，保留重试语义 |
| 历史已分账但未写入 daily_execution | 通过一次性「回填脚本」扫描 `t_split_bill` + `t_order.split_batch_no`，回填 `daily_execution` 与 `store_daily_stats.split_status`；首次上线前在迁移文件中以注释提供回填 SQL |
| 多门店×日同步更新 split_status 的并发 | 单条 SQL `UPDATE ... WHERE merchant_id=? AND biz_date=? AND store_id IN (...)`；命中行数明确；失败回退到逐行 |

---

## 十一、验收标准

1. 平账失败用例：在 T 日插一笔 PAID 订单，但 `t_store_daily_stats` 未生成 → 执行分账返回 `STATS_NOT_READY`；补跑日报后再次执行成功。
2. 平账失败用例：人为修改 `t_store_daily_stats.paid_amount += 1` → 执行分账返回 `RECONCILE_FAILED`，body 含 `diffs[]`，前端展示；恢复日报值后再次执行成功。
3. 成功用例：执行按时间段分账 → `t_split_daily_execution` 新增一行 status=SUCCESS → `t_store_daily_stats` 覆盖门店×日的 `split_status='SUCCESS' / split_batch_no / split_at` 正确更新。
4. 重复防护：对已 SUCCESS 的门店×日发起 `Execute` 单笔分账 → 返回 `ALREADY_SPLIT`。
5. 部分成功用例：构造「通道分账成功但内部转账失败」场景 → `daily_execution.status='FAILED'`，`store_daily_stats.split_status='PARTIAL'`，`error_message` 落库。
6. 商户/管理端 StoreStats 列表展示 `已分账` 字段：徽章颜色与状态一致；点击失败行可查看错误详情。