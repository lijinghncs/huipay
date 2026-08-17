# 分账前置对账 + 每日执行记录 迭代方案（V2 合并版）

> 本文档为单一权威方案，包含：
> 1. V1 的功能需求（前置对账、每日执行记录、门店日报已分账字段、列表展示）
> 2. V2 的关键修订（双层对账、统一口径、状态机补偿、跨规则防护、审计顺势落地、管理端分两期）
> 3. 性能评审（DB 索引、SQL 重写、连接池、归档策略）
>
> 取代：[split-precheck-and-execution-record.md](./split-precheck-and-execution-record.md)（V1）与 [split-precheck-v2-db-perf-review.md](./split-precheck-v2-db-perf-review.md)（独立性能评审）。

---

## 一、需求与背景

### 1.1 业务目标
1. **分账前置对账**：按时间段分账前，按「门店 × 日」逐日校验 `t_order` 未分账订单实收合计 vs `t_store_daily_stats.paid_amount`；任意门店 × 日不平则整批拒绝分账。
2. **每日分账执行记录**：新建 `t_split_daily_execution`，按 `(merchant_id, batch_no)` 记录每日分账执行情况。
3. **门店日报「已分账」状态**：在 `t_store_daily_stats` 增加 `split_status / split_batch_no / split_at / split_total_amount`，并在商户/管理端报表中展示。
4. **商户/管理端门店统计列表增加「是否分账」字段**。

### 1.2 现状速查

| 关注点 | 现状 |
|---|---|
| `t_store_daily_stats` | 已建（迁移 0024）：`(store_id, biz_date)` 唯一键，存 `order_count / paid_amount / channel_breakdown / status_breakdown`；无分账状态字段 |
| 分账执行明细 | `t_split_execution`（按 `order_no, receiver_entity_id` 记录每笔接收方分账）；索引 `uk_order_receiver(order_no, receiver_entity_id)` |
| 分账批次记录 | `t_split_bill`（按时间段分账后落地一条；含 `batch_no/rule_code/start_time/end_time/total_amount/order_nos/status`） |
| 按时间段分账入口 | `Service.ExecuteByPeriod`：拿时间段内商户实收总额 → 规则分配 → executor 落地 → 写 `t_split_bill` → 回填 `t_order.split_batch_no` |
| 排除已分账 | `StoreRevenueRepo.splitExclusion()`：`t_split_execution.status='SUCCESS'` 或 `t_split_bill.status IN (PENDING/APPROVED/EXECUTED)`——**该语义错误**：PENDING/APPROVED 是审批态，不应排除 |
| 现有对账机制 | **无** 「门店日报 vs 未分账订单」前置对账 |
| 商户端门店统计 | `merchant-portal/pages/StoreStats/index.tsx`：列表 + CSV 导出；无分账字段 |
| 管理端报表 | `admin/handler/store_stats.go` 已实现 `List / Summary / Daily`；前端**未实现** admin StoreStats 页 |
| `t_order` 索引 | `uk_order_no` / `uk_merchant_order` / `idx_status_created` / `idx_channel_trade` / `idx_merchant_created` / `idx_store(store_id)` / `idx_split_batch(merchant_id, split_batch_no, store_id)`——**缺 `(merchant_id, status, paid_at)`** |
| 迁移号 | 已到 `0026_split_precheck_and_daily_execution`；本轮新增 `0027_*` 性能索引 |

---

## 二、关键决策

| 决策点 | 取值 | 理由 |
|---|---|---|
| 对账粒度 | **双层**：商户级总额 + 门店×日明细 | 单层都有盲点；商户级秒级阻断大偏差，门店×日定位错配 |
| 容差 | **0 容差**（用户要求） | 严格平账；通过 `SameScopeFilter` 消除口径错位导致的必然失败 |
| 比较口径 | 统一函数 `SameScopeFilter()`：`PAID + paid_at ∈ [start,end] + store_id NOT NULL + t_store.status=1 + 排除 SUCCESS execution + 排除 EXECUTED bill` | 所有聚合 SQL（日报、Prechecker、补跑、回算 split_status）共用 |
| 失败处理 | 阻断 + 自动 Backfill 重试 1 次 → 仍失败返回 `RECONCILE_FAILED` + 写入 `t_reconcile_diff` | 自动兜底常见「日报晚到」场景 |
| 跨规则重复防护 | 入口查时段内已存在账单 → 拒绝并返回 `SPLIT_PERIOD_OVERLAPPED` + `existing_batches[]` | 阻止同规则/不同规则对同一时段重复执行 |
| `split_status` 计算 | **异步汇总**：新增 `recompute_store_split_status` 任务（每 5 分钟 + 分账完成后事件触发），从 `t_order.split_batch_no` 反算 | 不在分账热路径写，规避事务边界问题 |
| `daily_execution` 角色 | 仅存执行轨迹 + 失败 reconcile 引用；不再存规则/金额等账单属性 | 与 `t_split_bill` 职责分离 |
| 审计日志 | 顺势落地 `t_split_audit`（split-resilience D2 规划），记录 `EXECUTE / EXECUTE_FAILED / RECONCILE_PASSED / RECONCILE_FAILED / RESET / MANUAL_OVERRIDE` | 长期不可缺 |
| 商户端列表 | merchant-portal StoreStats 增加 `已分账` 字段（颜色徽章） | 用户要求 |
| 管理端 StoreStats | 分两期：本期 Analytics 概览加卡片 + 商户详情子表；下期独立页面 | 控制本期范围 |

---

## 三、专家评审（V1 → V2 修订要点）

| # | 严重度 | V1 问题 | V2 修订 |
|---|---|---|---|
| 🔴1 | 严重 | 「门店×日」平账与 `ExecuteByPeriod.total` 口径不一致 | **双层对账**：先商户级总额粗校验，再门店×日细校验 |
| 🔴2 | 严重 | `splitExclusion()` 包含 `t_split_bill PENDING/APPROVED`，与日报口径错位 | **统一口径**：只排除 `EXECUTED` + `t_split_execution.status='SUCCESS'`，所有聚合 SQL 共用 `SameScopeFilter` |
| 🔴3 | 严重 | 同一时段多规则会重复计算基数并执行 | `ExecuteByPeriod` 入口校验「该时段是否已有未驳回账单」，命中则返回 `SPLIT_PERIOD_OVERLAPPED` |
| 🔴4 | 严重 | Executor 返回值假设（要返回成功接收方 ID）破坏现有契约 | **不扩展 Executor**：直接 `SELECT receiver_entity_id, status FROM t_split_execution WHERE order_no=?` 反查事实 |
| 🔴5 | 严重 | 试图单事务包 executor.Execute，跨自治事务包不住 | **状态机补偿**：步骤 4–7 各自治，失败由补偿脚本/告警兜底，不靠事务回滚 |
| 🔴6 | 严重 | `split_status='PARTIAL'` 在「门店×日」粒度语义不清 | **异步汇总**：split_status 由定时任务或事件汇总 `t_order.split_batch_no` 算出，不在分账热路径写 |
| 🔴7 | 性能 | `SameScopeFilter` 中两个 NOT EXISTS 三层嵌套 → 3000 万行 × 2 ≈ 6000 万次嵌套评估 | **改 LEFT JOIN + 物化**（见 §七.7.2），单次评估 O(N) |
| 🔴8 | 性能 | `t_order` 缺 `(merchant_id, status, paid_at)` 复合索引 | **迁移 0027 加 3 个复合索引**（在线 DDL） |
| 🔴9 | 性能 | `t_reconcile_diff` 批量 INSERT 与 09:00 对账并发持锁 | **幂等 INSERT ON DUPLICATE KEY** + 自增锁调优 |
| 🔴10 | 性能 | async 汇总 5 分钟 × 多商户并发 → 索引 page latch | **增量触发**（`executed_at > now - 5min`）+ **队列去重** |
| 🔴11 | 性能 | `t_split_audit` 缺 `idx_biz_time`，千万级查询全表扫 | **加 `idx_biz_time` + 月度分区 + 90 天归档** |
| 🟠12 | 高 | `diff_snapshot` 存大 JSON 不利查询 | **复用 `t_reconcile_diff`**（已有，扩展 `diff_type='SPLIT_PRECHECK'`），`daily_execution` 只存 diff_id |
| 🟠13 | 高 | `t_split_daily_execution` 与 `t_split_bill` 字段重叠 | **去重**：`daily_execution` 只存 `run_id/biz_date/batch_no/status/timing/error/reconcile_diff_id/operator`；规则/金额/时段从 `t_split_bill` JOIN |
| 🟠14 | 高 | 重复请求同一规则同时段会被 executor 幂等跳过但仍写 SUCCESS 误导日志 | **幂等短路**：入口 `bill_repo.GetByBatchNo` 命中直接返回已存在结果 |
| 🟠15 | 高 | `t_split_bill.biz_dates` JSON 不可索引 | **新增关联表** `t_split_bill_biz_date(bill_id, biz_date)`，JSON 字段保留作为冗余 |
| 🟡16 | 中 | 0 容差 + 口径不一致 → 必然失败 | 统一口径函数 `SameScopeFilter` 提前解决 |
| 🟡17 | 中 | 日报缺失需手工补跑 | **自动 Backfill 一次**，仍缺失才报错 |
| 🟡18 | 中 | 单笔 execute 命中已 SUCCESS 门店×日拒绝，但缺覆盖入口 | 拒绝 + 返回 `admin_reset_supported=true`，管理端提供"重置该门店×日分账状态"按钮 |
| 🟡19 | 中 | 管理端 StoreStats 页工程量大 | **分两期**：本期 Analytics 概览加卡片 + 商户详情子表；下期独立页面 |
| 🟡20 | 中 | 错误信息可能含敏感字段且过长 | 截断至 1000 字符 + 订单号脱敏 |
| 🟡21 | 性能 | 连接池与收银回调竞争 | **读写分离**：Prechecker / async 用专用只读连接池 |
| 🟡22 | 性能 | `SameScopeFilter` 子查询不如 LEFT JOIN 友好 | Prechecker 改用 LEFT JOIN；SameScopeFilter 保留为单元测试断言 |

---

## 四、数据模型

### 4.1 迁移 0026（业务表）

```sql
-- 0026_split_precheck_and_daily_execution.up.sql

-- 1) t_store_daily_stats 增加分账状态字段（异步汇总写，热路径不写）
ALTER TABLE t_store_daily_stats
  ADD COLUMN split_status         VARCHAR(16) NOT NULL DEFAULT 'PENDING'
      COMMENT 'PENDING/SUCCESS/PARTIAL/FAILED，由后台任务根据 t_order.split_batch_no 汇总',
  ADD COLUMN split_batch_no       VARCHAR(64) NULL
      COMMENT '最近一次成功分账的批次号(冗余便于展示)',
  ADD COLUMN split_at             DATETIME(3) NULL
      COMMENT '最近一次分账完成时间(SUCCESS/PARTIAL 时)',
  ADD COLUMN split_total_amount   BIGINT NOT NULL DEFAULT 0
      COMMENT '该门店×日已被分账的订单合计金额(分)',
  ADD KEY idx_split_status (merchant_id, split_status, biz_date);

-- 2) 每日分账执行轨迹表（与 t_split_bill 职责分离）
CREATE TABLE t_split_daily_execution (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_id            VARCHAR(64) NOT NULL COMMENT '幂等键 SP_RUN-{merchant}-{batch_no}-{ts}',
  merchant_id       BIGINT UNSIGNED NOT NULL,
  biz_date          DATE NOT NULL,
  batch_no          VARCHAR(64) NOT NULL,
  status            VARCHAR(16) NOT NULL DEFAULT 'RUNNING'
      COMMENT 'RUNNING/SUCCESS/PARTIAL/FAILED',
  started_at        DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  finished_at       DATETIME(3) NULL,
  duration_ms       INT NULL,
  error_code        VARCHAR(64) NULL,
  error_message     VARCHAR(1024) NULL COMMENT '截断+脱敏',
  reconcile_diff_id BIGINT UNSIGNED NULL COMMENT '失败时关联 t_reconcile_diff.id',
  operator_type     VARCHAR(16) NOT NULL DEFAULT 'SYSTEM',
  operator_id       BIGINT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_run_id (run_id),
  KEY idx_merchant_started (merchant_id, started_at),
  KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每日分账执行轨迹';

-- 3) 账单-业务日期关联表（替代 JSON 过滤）
CREATE TABLE t_split_bill_biz_date (
  bill_id  BIGINT UNSIGNED NOT NULL,
  biz_date DATE NOT NULL,
  PRIMARY KEY (bill_id, biz_date),
  KEY idx_biz_date (biz_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='账单-业务日期关联表';

-- 4) t_split_bill 增加冗余 JSON（保留向后兼容，不再用于过滤）
ALTER TABLE t_split_bill
  ADD COLUMN biz_dates JSON NULL COMMENT '冗余：账单覆盖业务日期列表(由 t_split_bill_biz_date 派生)';

-- 5) t_reconcile_diff 已存在，扩展（无 schema 变更，应用层定义 diff_type='SPLIT_PRECHECK'）
--    补 biz_date 列便于管理端按日查询 + 索引
ALTER TABLE t_reconcile_diff
  ADD COLUMN biz_date DATE NULL AFTER biz_date_orig,
  ADD KEY idx_diff_type_biz_date (diff_type, biz_date);
-- 注：原表已有 biz_date 字段（迁移 0009），本处仅加索引；如字段名冲突需先 RENAME

-- 6) t_split_audit（split-resilience D2，顺势落地）
CREATE TABLE t_split_audit (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  biz_type      VARCHAR(32) NOT NULL COMMENT 'DAILY_SPLIT/SPLIT_BILL/SPLIT_EXEC',
  biz_id        VARCHAR(64) NOT NULL COMMENT 'biz_date 或 batch_no',
  action        VARCHAR(32) NOT NULL
      COMMENT 'EXECUTE/EXECUTE_FAILED/RECONCILE_PASSED/RECONCILE_FAILED/RESET/MANUAL_OVERRIDE',
  operator_type VARCHAR(16) NOT NULL COMMENT 'SYSTEM/MERCHANT/ADMIN',
  operator_id   BIGINT UNSIGNED NOT NULL DEFAULT 0,
  detail        JSON NULL,
  created_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_biz (biz_type, biz_id),
  KEY idx_biz_time (biz_type, created_at),
  KEY idx_action_time_status (action, status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分账审计日志';
-- 后续迭代加月度分区：PARTITION BY RANGE (TO_DAYS(created_at))
```

### 4.2 迁移 0027（性能索引，在线 DDL）

```sql
-- 0027_add_perf_indexes.up.sql（gh-ost 或 pt-online-schema-change）

-- 1) Prechecker / 聚合查询主索引
ALTER TABLE t_order
  ADD KEY idx_merchant_status_paidat (merchant_id, status, paid_at);

-- 2) split_batch_no IS NOT NULL 过滤辅助（async 汇总用）
ALTER TABLE t_order
  ADD KEY idx_merchant_split (merchant_id, split_batch_no, status, paid_at);

-- 3) 单笔订单所属门店×日反查（async 汇总用）
ALTER TABLE t_order
  ADD KEY idx_store_paidat (store_id, status, paid_at);
```

**索引代价**：每行多 ~30 字节 × 3000 万 = **900 MB**；InnoDB buffer pool 命中率下降 ~5%。
**写入代价**：每次 INSERT/UPDATE 多写 3 个二级索引；用 gh-ost 几乎 0 阻塞。

---

## 五、模块变更

### 5.1 `internal/split/scope/`（新建）

**`scope.go` — 统一口径函数**：

```go
// SameScopeFilter 返回「同一聚合口径」的 WHERE 片段（与日报聚合 SQL 一致）。
// 所有读 t_order 的聚合查询（日报生成、Prechecker、补跑、回算 split_status）必须使用。
// 实际执行路径用 LEFT JOIN 优化版（见 prechecker.go），本函数仅用于单元测试断言。
func SameScopeFilter(merchantID uint64, from, to time.Time) (where string, args []any) {
    where = `
        o.merchant_id = ? AND o.status = 'PAID' AND o.deleted_at IS NULL
        AND o.paid_at >= ? AND o.paid_at < ?
        AND o.store_id IS NOT NULL
        AND EXISTS (SELECT 1 FROM t_store s WHERE s.id = o.store_id AND s.status = 1)
        AND NOT EXISTS (SELECT 1 FROM t_split_execution se
                        WHERE se.order_no = o.order_no AND se.status = 'SUCCESS')
        AND NOT EXISTS (SELECT 1 FROM t_split_bill sb
                        WHERE sb.merchant_id = o.merchant_id
                          AND sb.status = 'EXECUTED'
                          AND sb.id IN (
                              SELECT bill_id FROM t_split_bill_biz_date WHERE biz_date = DATE(o.paid_at)
                          ))
    `
    args = []any{merchantID, from, to}
    return
}
```

### 5.2 `internal/split/recon/`（新建）

#### 5.2.1 `prechecker.go` — LEFT JOIN 优化版

```go
type Diff struct {
    StoreID    uint64 `json:"store_id"`
    StoreName  string `json:"store_name"`
    BizDate    string `json:"biz_date"`
    OrderTotal int64  `json:"order_total"`
    StatsTotal int64  `json:"stats_total"`
    Diff       int64  `json:"diff"`
}

type Prechecker struct {
    db         *gorm.DB
    statsSvc   StatsBackfiller  // 接口：Backfill(start, end)
    diffRepo   *repo.ReconcileDiffRepo
    auditRepo  *repo.AuditRepo
    logger     *zap.Logger
}

// Check 双层对账。
func (p *Prechecker) Check(ctx, merchantID, start, end, storeIDs) (diffs []Diff, diffID *uint64, err error) {
    // 1. 自动补跑：仅在日报缺失时
    if p.statsSvc.HasMissing(ctx, merchantID, start, end) {
        if err := p.statsSvc.Backfill(ctx, start, end); err != nil {
            return nil, nil, errs.New(STATS_NOT_READY, "日报补跑失败", 200)
        }
    }

    // 2. Layer A - 商户级总额
    orderTotal := p.sumOrderTotal(ctx, merchantID, start, end)
    statsTotal := p.sumStatsTotal(ctx, merchantID, start, end)
    if orderTotal != statsTotal {
        diffID := p.diffRepo.Write(ctx, "SPLIT_PRECHECK", "TOTAL", start, end, ...)
        return nil, &diffID, errs.New(RECONCILE_FAILED_TOTAL, "商户级总额不平", 200)
    }

    // 3. Layer B - 门店×日
    rows := p.sumOrderByStoreAndDate(ctx, merchantID, start, end)
    stats := p.fetchStatsRows(ctx, merchantID, start, end)
    diffs = compareRows(rows, stats)  // 逐行比对
    if len(diffs) > 0 {
        diffID := p.diffRepo.Write(ctx, "SPLIT_PRECHECK", "DETAIL", start, end, diffs)
        p.auditRepo.Write(ctx, "DAILY_SPLIT", start, "RECONCILE_FAILED", ...)
        return diffs, &diffID, errs.New(RECONCILE_FAILED_DETAIL, "门店×日不平", 200)
    }

    p.auditRepo.Write(ctx, "DAILY_SPLIT", start, "RECONCILE_PASSED", ...)
    return nil, nil, nil
}
```

#### 5.2.2 Layer A/B 关键 SQL（LEFT JOIN 优化版）

**Layer A（商户级总额）**：

```sql
SELECT COALESCE(SUM(o.paid_amount), 0) AS order_total
FROM t_order o USE INDEX (idx_merchant_status_paidat)
INNER JOIN t_store s ON s.id = o.store_id AND s.status = 1
LEFT JOIN t_split_execution se ON se.order_no = o.order_no AND se.status='SUCCESS'
LEFT JOIN t_split_bill sb ON sb.merchant_id = o.merchant_id AND sb.status='EXECUTED'
LEFT JOIN t_split_bill_biz_date bd
       ON bd.bill_id = sb.id AND bd.biz_date = DATE(o.paid_at)
WHERE o.merchant_id = ? AND o.status='PAID' AND o.deleted_at IS NULL
  AND o.paid_at >= ? AND o.paid_at < ?
  AND o.store_id IS NOT NULL
  AND se.order_no IS NULL
  AND bd.bill_id IS NULL;
```

**Layer B（门店×日明细）**：

```sql
SELECT DATE(o.paid_at) AS biz_date, o.store_id,
       COALESCE(SUM(o.paid_amount), 0) AS order_total
FROM t_order o USE INDEX (idx_merchant_status_paidat)
INNER JOIN t_store s ON s.id = o.store_id AND s.status = 1
LEFT JOIN t_split_execution se ON se.order_no = o.order_no AND se.status='SUCCESS'
LEFT JOIN t_split_bill sb ON sb.merchant_id = o.merchant_id AND sb.status='EXECUTED'
LEFT JOIN t_split_bill_biz_date bd
       ON bd.bill_id = sb.id AND bd.biz_date = DATE(o.paid_at)
WHERE o.merchant_id = ? AND o.status='PAID' AND o.deleted_at IS NULL
  AND o.paid_at >= ? AND o.paid_at < ?
  AND o.store_id IS NOT NULL
  AND se.order_no IS NULL
  AND bd.bill_id IS NULL
GROUP BY DATE(o.paid_at), o.store_id;
```

**对比日报聚合**：

```sql
SELECT COALESCE(SUM(paid_amount), 0) AS stats_total
FROM t_store_daily_stats USE INDEX (idx_merchant_date)
WHERE merchant_id = ? AND biz_date >= ? AND biz_date < ?;
```

#### 5.2.3 `order_paid_sum_querier.go`（接口）

```go
type OrderPaidSumQuerier interface {
    SumPaidByStoreAndDate(ctx, merchantID, from, to) ([]StoreDateSum, error)
    SumPaidTotal(ctx, merchantID, from, to) (int64, error)
}
```

实现位于 `internal/order/repository/order_repo.go`，使用上述 LEFT JOIN SQL。

### 5.3 `internal/split/repository/daily_execution_repo.go`（新建）

```go
type DailyExecutionModel struct {
    ID, RunID, MerchantID, BizDate, BatchNo string
    Status, StartedAt, FinishedAt, ErrorCode, ErrorMessage
    ReconcileDiffID *uint64
    OperatorType, OperatorID
}

func (r *DailyExecutionRepo) CreateWithRunID(ctx, runID, merchantID, bizDate, batchNo) (id, error)
func (r *DailyExecutionRepo) MarkStatus(ctx, id, status, errorCode, errorMessage, diffID, durationMs) error
func (r *DailyExecutionRepo) GetByRunID(ctx, runID) (*Model, error)
func (r *DailyExecutionRepo) ListByMerchantDateRange(ctx, merchantID, start, end, status, page) ([]Model, int64, error)
```

### 5.4 `internal/split/repository/audit_repo.go`（新建）

```go
func (r *AuditRepo) Write(ctx, bizType, bizID, action, opType, opID, detail) error
func (r *AuditRepo) List(ctx, bizType, bizID, page) ([]Model, int64, error)
```

### 5.5 `internal/split/repository/bill_biz_date_repo.go`（新建）

```go
func (r *BillBizDateRepo) Bind(ctx, billID, bizDates) error  // 批量 INSERT
func (r *BillBizDateRepo) ListBillsByDate(ctx, merchantID, bizDate) ([]BillID, error)
func (r *BillBizDateRepo) BackfillFromBillJSON(ctx) error    // 历史 JSON → 关联表
```

### 5.6 `internal/stats/repository/store_daily_stats_repo.go`（改造）

```go
// RecomputeByMerchantDate 异步汇总：从 t_order.split_batch_no 反算 split_status
func (r *StoreDailyStatsRepo) RecomputeByMerchantDate(ctx, merchantID, bizDate) error {
    // 1. SELECT store_id, COUNT(split_batch_no IS NOT NULL), SUM(amount)
    //    FROM t_order USE INDEX (idx_merchant_split)
    //    WHERE merchant_id=? AND DATE(paid_at)=? AND status='PAID'
    //    GROUP BY store_id
    // 2. UPDATE t_store_daily_stats
    //    SET split_status='SUCCESS/PARTIAL/PENDING', split_batch_no=?, split_at=?, split_total_amount=?
    //    WHERE merchant_id=? AND biz_date=? AND store_id IN (...)
    // 3. 已分账比例 = 0 → split_status='PENDING'
    //    已分账比例 = 100 → split_status='SUCCESS' + split_batch_no=最近一次成功批次号
    //    0 < 比例 < 100 → split_status='PARTIAL'
}

// HasMissing 检查 [start, end] 区间内是否有 biz_date 在 t_store_daily_stats 缺失
func (r *StoreDailyStatsRepo) HasMissing(ctx, merchantID, start, end) (bool, error)
```

### 5.7 `internal/split/service/service.go`（V2 改造）

**`ExecuteByPeriod` 完整流程**：

```
1. 解析时段/规则（同 V1）
2. ★ 幂等短路：
   if bill, _ := billRepo.GetByBatchNo(merchantID, batchNo); bill != nil:
       return &ExecuteByPeriodResponse{BatchNo: bill.BatchNo, ...已存在账单}, nil
3. ★ 跨规则重复防护：
   if overlapped := billBizDateRepo.ListBillsByDate(merchantID, bizDate)
                  ∪ billRepo.ListByMerchantRange(merchantID, start, end); len(overlapped) > 0:
       return errs.New(SPLIT_PERIOD_OVERLAPPED, "时段已有账单", existing_batches)
4. ★ Prechecker.Check(merchantID, start, end, nil)
   失败 → audit_repo.Write(RECONCILE_FAILED) + 写 t_reconcile_diff
       return errs.New(RECONCILE_FAILED, "...", diffID)
   通过 → audit_repo.Write(RECONCILE_PASSED)
5. ★ daily_execution_repo.CreateWithRunID(RUNNING)
6. total = SumPaid (使用 LEFT JOIN 优化 SQL)
7. executor.Execute (内部自治，不外层包事务)
8. ★ readExecutionFacts(batchNo) 反查 t_split_execution 真实状态：
       SELECT status, receiver_entity_id FROM t_split_execution WHERE order_no=?
   status := 全部SUCCESS ? "SUCCESS" : (部分SUCCESS ? "PARTIAL" : "FAILED")
9. ★ daily_execution_repo.MarkStatus(status, errorCode, errorMessage, diffID, durationMs)
10. ★ audit_repo.Write(EXECUTE 或 EXECUTE_FAILED)
11. bill_repo.Create (含 biz_dates 冗余 JSON)
12. bill_biz_date_repo.Bind(billID, bizDates[])
13. ★ 回填 t_order.split_batch_no（同 V1）
14. ★ 触发 async 汇总：
        go statsSvc.RecomputeByMerchantDate(merchantID, 每个 biz_date)
       （本期同步调用，迭代 2 改为 outbox）
```

### 5.8 异步汇总任务

`internal/stats/scheduler/store_split_status_recompute.go`：

- 每 5 分钟跑一次（用 monitoring framework 的 ticker，沿用上一迭代方案）
- **增量策略**（优化后）：
  ```sql
  -- 每分钟扫一次最近 5 分钟的 t_split_execution 变更
  SELECT DISTINCT merchant_id, DATE(o.paid_at) AS biz_date
  FROM t_split_execution se
  INNER JOIN t_order o ON o.order_no = se.order_no
  WHERE se.executed_at > NOW() - INTERVAL 5 MINUTE;
  -- 把 (merchant_id, biz_date) 入队列，去重后逐日回算
  ```
- **应用层去重**：维护内存 `map[merchantID_bizDate]bool` + 持久化队列（Redis 或 DB 表），串行处理避免锁竞争

### 5.9 错误码

| 错误码 | 触发条件 | HTTP |
|---|---|---|
| `STATS_NOT_READY` | Backfill 后仍缺日报 | 200 |
| `RECONCILE_FAILED_TOTAL` | 商户级总额不平 | 200 |
| `RECONCILE_FAILED_DETAIL` | 门店×日明细不平 | 200 |
| `SPLIT_PERIOD_OVERLAPPED` | 时段已有未驳回账单 | 200 |
| `ALREADY_SPLIT` | 单笔分账命中已 SUCCESS 门店×日 | 200 |
| `SPLIT_STORE_RESET` | 管理端重置门店×日分账状态 | — |

### 5.10 HTTP 接口

| 方法 | 路径 | 变更 | 说明 |
|---|---|---|---|
| POST | `/v1/merchant/split/execute-period` | **改造** | 对账失败返回 `RECONCILE_FAILED` 错误体含 `diffs[]` |
| GET | `/v1/admin/split/daily-executions` | **新增** | 管理端查看每日执行记录 |
| GET | `/v1/admin/split/daily-executions/:id` | **新增** | 单次执行详情 |
| GET | `/v1/admin/split/audit` | **新增** | 审计日志（biz_type/action/分页） |
| GET | `/v1/admin/reconcile-diffs` | **新增** | `diff_type='SPLIT_PRECHECK'` 列表 |
| POST | `/v1/admin/store-stats/{merchant}/{store}/{biz_date}/recompute` | **新增** | 主动触发单个 (merchant, store, biz_date) 重算 |
| POST | `/v1/admin/store-stats/{merchant}/{store}/{biz_date}/reset-split-status` | **新增** | 重置门店×日分账状态（CAS + 审计） |
| GET | `/v1/admin/store-stats` | **改造** | 返回字段新增 `split_status / split_batch_no / split_at / split_total_amount` |
| GET | `/v1/admin/store-stats/summary` | **改造** | 同上 |
| GET | `/v1/admin/stores/:id/daily-stats` | **改造** | 同上 |
| GET | `/v1/merchant/store-stats` | **改造** | 商户端列表字段扩展 |

---

## 六、前端变更

### 6.1 merchant-portal（本期必须）

- `pages/StoreStats/index.tsx`：
  - 列扩展：`已分账`（徽章 `已分账/部分分账/未分账/分账失败` + 颜色 + Tooltip）
  - 操作列：`失败行 → 查看原因`（弹窗调 `getDailyExecution`）
  - 范围筛选：增加 `split_status` 多选
  - CSV 导出：增加「是否分账」列
- `services/user.ts`：`StoreStatItem` 类型扩展

### 6.2 admin-portal（分两期）

**本期（必做）**：
- `pages/Merchants/detail.tsx` 新增子 Tab「门店分账概览」：按 (store, biz_date) 列表，含 `已分账` 字段
- `pages/Analytics/index.tsx` 真实化：
  - 新增「门店分账覆盖率」KPI 卡
  - 「近 7 日分账成功率」图表（用真接口）
  - 「异常差异」卡（指向 `/v1/admin/reconcile-diffs?diff_type=SPLIT_PRECHECK`）
- 新增 API：`listDailyExecutions / getDailyExecution / listReconcileDiffs / listAudit / recomputeStoreStats / resetStoreStats`

**下期（迭代 2）**：
- 独立 `pages/StoreStats/index.tsx`（与 merchant 版同结构）
- 独立 `pages/SplitExecutions/index.tsx`（执行记录列表 + 详情）
- 独立 `pages/AuditLog/index.tsx`（审计日志）

### 6.3 组件复用

- 抽 `components/SplitStatusBadge.tsx`（merchant/admin 复用）
- 抽 `components/ReconcileDiffList.tsx`（展示 `t_reconcile_diff` 列表）

---

## 七、性能与容量

### 7.1 假设
- `t_order` 累计 3000 万行（3 年 × 200 商户 × 日均 1370 笔）
- 单商户日均订单 1500 笔，峰值 8000 笔/日
- 单商户时段分账请求 5 次/日
- 单商户时段跨度 7–30 天（多数 1 天）
- 门店数/商户平均 20 家，最大 200 家
- `t_split_execution` ≈ 4500 万行（PAID × 1.5 接收方）

### 7.2 SQL 优化要点

**Prechecker**：
- NOT EXISTS 三层嵌套 → LEFT JOIN 优化版（见 §5.2.2）
- 配合索引 `idx_merchant_status_paidat` → 单次评估 O(N)
- 单商户×30 日预估 **< 300 ms P99**

**async 汇总**：
- 全表 5 分钟 × 多商户 × 多日 → 增量触发（`executed_at > now - 5min`）
- 应用层队列去重（`map[merchantID_bizDate]bool`）
- 单次 recompute 商户级 **< 50 ms P99**

**日报生成**（沿用 0024 SQL）：
- 已用 `idx_merchant_created` + GROUP BY，**不需改**

### 7.3 索引策略

| 索引 | 服务于 |
|---|---|
| `idx_merchant_status_paidat` | Prechecker Layer A/B、Backfill、日报生成 |
| `idx_merchant_split` | async 汇总 `split_batch_no IS NOT NULL` 过滤 |
| `idx_store_paidat` | RecomputeByStore 单门店回算 |
| `idx_diff_type_biz_date` (t_reconcile_diff) | 管理端按日查询差异 |
| `idx_biz_time` (t_split_audit) | 千万级审计日志分页 |

**索引代价**：t_order 每行多 ~30 字节 × 3000 万 = **900 MB**；InnoDB buffer pool 命中率下降 ~5%。
**写入代价**：收银回调 INSERT 多写 3 个二级索引 → 性能影响 < 5%（压测验证）。

### 7.4 连接池拆分

```go
// cmd/server/main.go
masterDB := initMasterDB()                    // 主写池：收银回调、分账写
masterDB.SetMaxOpenConns(100)
masterDB.SetMaxIdleConns(20)

readonlyDB := initReadonlyDB(masterDB)        // 只读池：Prechecker、async 汇总、补跑
readonlyDB.SetMaxOpenConns(30)
readonlyDB.SetMaxIdleConns(10)
readonlyDB.SetConnMaxLifetime(5 * time.Minute)

// async 汇总错峰：从 5 分钟改为 10 分钟 + 随机抖动（避免与 09:00 对账重叠）
```

### 7.5 容量规划（1 年）

| 表 | 当前估算 | 1 年增量 | 备注 |
|---|---|---|---|
| `t_order` | 3000 万 | +1500 万 | 主索引已覆盖 |
| `t_split_execution` | 4500 万 | +2000 万 | `uk_order_receiver` 足够 |
| `t_split_bill` | 数十万 | +数十万 | 小表 |
| `t_split_bill_biz_date` | 0 | +数百万 | 与 bill 1:N |
| `t_reconcile_diff` | 0 | +数十万（SPLIT_PRECHECK + 已有） | 单条小 |
| `t_split_daily_execution` | 0 | +20 万 | 小表 |
| `t_split_audit` | 0 | **+1000 万**（千万级） | 需 90 天归档 |
| `t_store_daily_stats` | 数千 | +数万 | 小表 |

### 7.6 `t_split_audit` 归档策略

- **本期**：保留 90 天在线
- **迭代 2**：加月度分区 `PARTITION BY RANGE (TO_DAYS(created_at))`，每季度归档到 `t_split_audit_archive`（压缩表）
- **迭代 3**：超过 1 年的审计数据可下线冷存储

---

## 八、迁移与兼容性

### 8.1 迁移顺序

1. **迁移 0026**（业务表，瞬间完成，可灰度）
2. **迁移 0027**（性能索引，gh-ost 在线 DDL，5–10 分钟）
3. 应用层 `SameScopeFilter` 上线（双跑旧 `splitExclusion` 一周对比，确认无差异后切换）

### 8.2 回滚预案

- 删除迁移 0027 索引（`ALTER TABLE DROP KEY`）
- 删除迁移 0026 业务表 / 列
- 应用层 `if !hasColumn("split_status")` 兼容老逻辑

### 8.3 兼容性与回归

| 维度 | 处理 |
|---|---|
| `t_store_daily_stats.split_status` 默认 `PENDING` | 老数据继续展示为「未分账」，符合实际（无批次号关联） |
| `t_split_daily_execution` 与 `t_split_bill` 字段去重 | 老代码若读 `daily_execution.total_amount` 会编译报错，**强制**改为 JOIN |
| `t_split_bill_biz_date` 关联表需回填 | 一次性脚本：`INSERT IGNORE INTO t_split_bill_biz_date SELECT id, jt.biz_date FROM t_split_bill, JSON_TABLE(biz_dates, '$[*]' COLUMNS (biz_date DATE PATH '$')) AS jt WHERE biz_dates IS NOT NULL`（MySQL 8） |
| `SameScopeFilter` 与 `splitExclusion` 行为差异 | **所有**调用 `splitExclusion` 的地方（SumPaidByStore / SumPaid / ListUnsplitOrderNos）改为 `SameScopeFilter` 或 LEFT JOIN 优化版，行为变更需在迭代交付说明中明示 |
| Executor 不改 | 减少爆炸半径 |
| 异步汇总任务 | 5–10 分钟一次，本期上线后可观察数据准确性，再决定是否改为实时事件 |

---

## 九、依赖与顺序

```
A → B → C → D
       ↓
       └─ 与 split-resilience-error-handling.md 中 D2（审计日志）合并落地
```

### 迭代 A：数据 + 统一口径（前置）
| 任务 | 文件 |
|---|---|
| 迁移 0026（5 张表/列变更） | `infra/migrator/migrations/0026_*.sql` |
| 迁移 0027（性能索引，gh-ost） | `infra/migrator/migrations/0027_*.sql` |
| `scope.SameScopeFilter` + 替换 `splitExclusion` 3 处调用 | `internal/split/scope/`、`internal/split/repository/store_revenue_repo.go` |
| `order_repo.SumPaidByStoreAndDate` (LEFT JOIN 版) | `internal/order/repository/order_repo.go` |
| 单元测试：SameScopeFilter 行为对齐 | `internal/split/scope/scope_test.go` |

### 迭代 B：核心逻辑（资金安全）
| 任务 | 文件 |
|---|---|
| `prechecker.go` (LEFT JOIN 优化) + `daily_execution_repo` + `audit_repo` + `bill_biz_date_repo` + `reconcile_diff_repo` 扩展 | `internal/split/recon/`、`internal/split/repository/` |
| `Service.ExecuteByPeriod` 全量改造 | `internal/split/service/service.go` |
| `Executor` 不变；新增 `readExecutionFacts` | `internal/split/service/exec_facts.go` |
| 错误码新增（5 个） | `infra/errs/biz_error.go` |
| 单元测试：双层对账、跨规则防护、幂等短路 | `internal/split/{recon,service}/..._test.go` |

### 迭代 C：异步状态汇总 + 监控
| 任务 | 文件 |
|---|---|
| `stats_repo.RecomputeByMerchantDate` + `HasMissing` | `internal/stats/repository/store_daily_stats_repo.go` |
| `store_split_status_recompute` 调度器（10 分钟 + 抖动 + 增量触发） | `internal/stats/scheduler/` |
| `internal/split/recompute/queue.go` 队列去重 | 同上 |
| Prometheus 指标：`split_reconcile_total{result}`、`split_precheck_diff_total`、`split_status_recompute_duration_ms` | `infra/prom/prom.go` |
| 连接池拆分（Prechecker / async 用独立只读池） | `cmd/server/main.go` |

### 迭代 D：HTTP 接口与前端
| 任务 | 文件 |
|---|---|
| admin handlers: daily-executions / audit / reconcile-diffs / recompute / reset-split-status | `internal/admin/handler/`、`cmd/server/main.go` |
| merchant-portal StoreStats 增强 | `pages/StoreStats/index.tsx`、`services/user.ts` |
| admin-portal 商户详情子 Tab + Analytics 真实化 | `pages/Merchants/detail.tsx`、`pages/Analytics/index.tsx` |
| 公共组件 `SplitStatusBadge / ReconcileDiffList` | `components/` |

### 迭代 E（后续）：归档与高级特性
- `t_split_audit` 月度分区 + 90 天归档
- async 汇总 outbox 改造（替代同步调用）
- 管理端独立 `StoreStats / SplitExecutions / AuditLog` 三页面

---

## 十、风险与缓解

| 风险 | 缓解 |
|---|---|
| `SameScopeFilter` 改变排除语义，影响已有按时间段分账基数 | 灰度切换：先在 Prechecker 内双跑新旧 SQL 一周对比；无差异后切换 |
| gh-ost 加索引失败回退 | 准备 ALGORITHM=INPLACE 备用方案；演练回滚脚本 |
| `Executor` 不返回成功接收方 → `readExecutionFacts` 多一次查询 | 接收方规模 ≤ 50，查询耗时 ms 级；可接受 |
| 异步汇总任务延迟，用户看到「未分账」但实际已分账 | UI 文案说明「分账状态每 5–10 分钟刷新」；提供 admin recompute 主动刷新 |
| `t_split_bill_biz_date` 数据膨胀 | 按月分区或定期归档已 EXECUTED > 90 天的记录 |
| 跨规则防护过严：商户故意想做"分段执行" | 提供 `force=true` 参数绕过防护，audit 必写 `MANUAL_OVERRIDE` |
| 0 容差导致罕见舍入误差仍失败 | 所有金额用分（int64），不存在浮点；通道分账与本地记账单位一致 |
| Executor 失败但 daily_execution 已 RUNNING → 永久悬挂 | 加 watchdog：`started_at < now - 30min AND status='RUNNING'` → 标 `STALE`，触发告警 |
| 自动 Backfill 启动失败 → Prechecker 永远 STATS_NOT_READY | 监控：Backfill 失败率指标 + 告警 |
| 读写连接池配置错误导致主库压力大 | 监控 readonly 池使用率 + 告警；只读账号权限隔离 |
| `t_split_audit` 千万级写入 I/O 压力 | 月度分区（迭代 E）+ 异步批量插入（本期先同步 INSERT，迭代 2 改批量） |

---

## 十一、验收标准

### 11.1 功能验收

1. **口径一致**：在 T 日插入 100 笔 PAID 订单，日报聚合 SQL 与 Prechecker Layer A 商户级聚合结果完全一致（误差 = 0）
2. **双层对账**：商户级不平但门店×日平 → 返回 `RECONCILE_FAILED_TOTAL` + 写 1 条 `t_reconcile_diff(level='TOTAL')`
3. **明细对账**：商户级平但某门店×日不平 → 返回 `RECONCILE_FAILED_DETAIL` + 写 N 条 `t_reconcile_diff(level='DETAIL')`
4. **幂等短路**：同一规则同时段二次请求 → 第二次直接返回第一次的 `ExecuteByPeriodResponse`，无 executor 调用
5. **跨规则防护**：时段内已存在批次 A，新请求批次 B → 返回 `SPLIT_PERIOD_OVERLAPPED` + `existing_batches=['A']`
6. **自动 Backfill**：T-1 日报缺失 → Prechecker 自动补跑 → 通过
7. **失败兜底**：executor 第 3 个接收方失败 → `daily_execution.status='FAILED'`, `split_status='PARTIAL'`（不是 SUCCESS），`error_message` 含失败原因
8. **异步汇总**：分账完成后 10 分钟内，商户端 StoreStats `split_status` 字段从 `PENDING` 变为 `SUCCESS`
9. **审计落地**：6 类事件（EXECUTE / EXECUTE_FAILED / RECONCILE_PASSED / RECONCILE_FAILED / RESET / MANUAL_OVERRIDE）均能在 `/v1/admin/split/audit` 查询到
10. **管理端**：通过 `/v1/admin/store-stats/{merchant}/{store}/{biz_date}/reset-split-status` 重置后，audit 写 `RESET`，下游 StoreStats 字段恢复 `PENDING`
11. **回归**：原有 `t_split_bill PENDING/APPROVED/EXECUTED` 三态流转不受影响

### 11.2 性能验收

| 项 | 目标 | 测试方法 |
|---|---|---|
| Prechecker Layer A 单商户×日 | < 50 ms P99 | sysbench + EXPLAIN ANALYZE |
| Prechecker Layer A 单商户×30 日 | < 200 ms P99 | 同上 |
| Prechecker Layer B 单商户×30 日×20 门店 | < 300 ms P99 | 同上 |
| Backfill 单商户×30 日 | < 500 ms | 同上 |
| async 汇总 单商户×日 | < 50 ms P99 | 同上 |
| 收银回调 INSERT 耗时 | 增加 < 5% | wrk 压测 |
| 并发 100 个 Prechecker 同时调用 | 总耗时 < 10 s | k6 |
| `t_split_audit` 千万级查询 | < 200 ms P99 | EXPLAIN |
| `t_reconcile_diff` SPLIT_PRECHECK 写入 | < 50 ms P99 | 同上 |

---

## 十二、落地前 Checklist

- [ ] 测试环境 100 万行 mock 数据 + `EXPLAIN ANALYZE` 验证 Prechecker LEFT JOIN 优化效果
- [ ] 准备 gh-ost 加索引脚本并演练回滚
- [ ] 双跑 `SameScopeFilter` 与旧 `splitExclusion` 一周对比
- [ ] Prechecker 重写后跑 chaos 测试（注入慢查询、连接池满）
- [ ] 与 DBA 评审 `t_order` 索引代价（900 MB buffer pool）
- [ ] 与 split-resilience-error-handling.md 评审消除 `t_split_audit` 重复定义
- [ ] 监控 dashboard 加 Prechecker / async / Backfill 失败率指标
- [ ] 告警规则：Backfill 失败率 > 5%、async 汇总 > 10s、daily_execution STALE > 0