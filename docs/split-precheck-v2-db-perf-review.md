# V2 方案 · 数据库性能评审

> 评审对象：[split-precheck-and-execution-record-v2.md](./split-precheck-and-execution-record-v2.md)
> 范围：迁移 0026、Prechecker 双层对账、SameScopeFilter、daily_execution、async 状态汇总、t_split_audit、t_split_bill_biz_date、t_reconcile_diff 扩展。
> 评估维度：索引覆盖 / 扫描行数 / 锁与 MVCC / 写入放大 / QPS / 数据膨胀 / GC/连接占用。

---

## 一、核心结论（先看）

**V2 方案在「单商户×单日」级别无明显性能问题；但在「批量对账、补跑、async 汇总」三条路径上有 5 个高风险点（🔴）、4 个中风险点（🟠）。** 集中表现为：

1. **`SameScopeFilter` 中的两个 `NOT EXISTS` 在没有合适索引时会扫描 `t_split_execution`/`t_split_bill_biz_date` 大表**
2. **`t_order` 缺 `(merchant_id, status, paid_at)` 复合索引**——所有按「商户×时段」聚合的查询全表扫或弱索引扫
3. **`t_reconcile_diff` 全表清空后批量 INSERT**（每日 Backfill + Prechecker 失败路径）会在大表上长时间持锁
4. **`t_split_daily_execution` 写入放大**——按 `uk(merchant, batch_no)` 唯一约束，并发分账同商户时 row-lock 热点
5. **`t_split_audit` 全量写**：每次分账/对账/失败都写一行，年写入量 = 365 × 商户数 × 5 事件 ≈ 千万级

下面逐路径量化 + 优化。

---

## 二、关键路径扫描行数估算

### 假设（中等规模）
- `t_order` 累计：**3000 万行**（3 年× 200 商户 × 日均 1370 笔）
- 单商户日均订单：**1500 笔**，峰值 **8000 笔/日**
- 单商户时段分账请求频次：**5 次/日**（5 个规则）
- 单商户时段跨度（按时间段分账）：**7–30 天**（多数 1 天，少量 7 天）
- 门店数/商户：**平均 20 家**，最大 **200 家**
- `t_split_execution` 行数 ≈ `t_order` PAID × 接收方数 ≈ **3000 万 × 1.5 = 4500 万**
- `t_split_bill` 行数 ≈ 商户数 × 时段请求频次 × 365 ≈ **数十万**

### 路径 P1：Prechecker Layer A（商户级总额）

```sql
SELECT SUM(o.paid_amount)
FROM t_order o
WHERE [SameScopeFilter]   -- 含 2 个 NOT EXISTS
  AND o.paid_at BETWEEN ? AND ?
  AND o.merchant_id = ?
```

| 索引现状 | 估算扫描 | 问题 |
|---|---|---|
| `idx_status_created(status, created_at)` 与 `idx_merchant_created(merchant_id, created_at)` 都不匹配 | 命中 `idx_merchant_created` 后扫所有状态行（含 CREATED/CLOSED/REFUNDED），3 年 = 3000 万 × 商户占比 | 🔴 索引不匹配，需扫商户全量订单 |
| 缺 `(merchant_id, status, paid_at)` 复合索引 | 30 万行 × 商户 | 🔴 |

**V2 未加索引时单次 Layer A 预估耗时**：150–500 ms（buffer pool 命中率 95% 时），IO 15–50 MB。

### 路径 P2：Prechecker Layer B（门店×日）

```sql
SELECT DATE(o.paid_at) AS biz_date, o.store_id, SUM(o.paid_amount)
FROM t_order o
WHERE [SameScopeFilter]
  AND o.paid_at BETWEEN ? AND ?
  AND o.merchant_id = ?
GROUP BY DATE(o.paid_at), o.store_id
```

- 在 Layer A 基础上叠加 GROUP BY，最多 **商户 30 天 × 200 门店 = 6000 行**输出；
- 但扫描行数同 Layer A。

### 路径 P3：`SameScopeFilter` 中两个 NOT EXISTS

```sql
NOT EXISTS (SELECT 1 FROM t_split_execution
            WHERE se.order_no = o.order_no AND se.status='SUCCESS')

NOT EXISTS (SELECT 1 FROM t_split_bill sb
            WHERE sb.merchant_id = o.merchant_id
              AND sb.status='EXECUTED'
              AND sb.id IN (
                SELECT bill_id FROM t_split_bill_biz_date
                WHERE biz_date = DATE(o.paid_at)
              ))
```

- 第一个 NOT EXISTS：`t_split_execution` 有 `uk_order_receiver(order_no, receiver_entity_id)`，**按 order_no 单条过滤快**，可优化为相关子查询 → hash semi-join。MySQL 8 对此可优化，但需要 FORCE INDEX 或 hint。
- 第二个 NOT EXISTS：**三层嵌套**（外层 t_order → t_split_bill → t_split_bill_biz_date），即使各表有索引，每行 o 触发 2 次索引查找 × 子查询嵌套：
  - `t_split_bill` 有 `idx_merchant_priority`（merchant_id, priority DESC），但**缺 `(merchant_id, status)`**，按 `EXECUTED` 过滤会回表；
  - `t_split_bill_biz_date` 有 `PRIMARY KEY(bill_id, biz_date)` 与 `idx_biz_date(biz_date)`，按 `biz_date` 反查是**点查，**但 bill_id 反向需要扫 bill 表；
  - 优化器通常会物化子查询，但每行 o 触发一次 `IN` 判断 = **3000 万次嵌套评估**。

**预估**：**5–30 秒**（含 innodb_buffer_pool miss 时磁盘 IO）。🔴 严重。

### 路径 P4：补跑日报 Backfill(start, end)

```sql
INSERT INTO t_store_daily_stats
SELECT ... FROM t_order o
WHERE o.paid_at >= ? AND o.paid_at < ?
GROUP BY merchant_id, store_id, DATE(paid_at)
ON DUPLICATE KEY UPDATE ...
```

- 现状迁移 0024 实现的 SQL 无 SameScopeFilter，所以日报生成本身**快**；
- 但 Prechecker 调用 Backfill 是**每日多次**（被外部触发或自检触发）→ 多次扫 `t_order`；
- 同时 Prechecker 自身也要扫 `t_order` 做对账 → **同一窗口内 `t_order` 被全表读两遍**。🟠

### 路径 P5：async 汇总 `RecomputeByMerchantDate`（每 5 分钟）

```sql
-- 假设按 updated_at 增量
SELECT store_id, COUNT(split_batch_no), SUM(amount)
FROM t_order
WHERE merchant_id = ? AND DATE(paid_at) = ? AND status = 'PAID'
  AND split_batch_no IS NOT NULL  -- 已分账订单
GROUP BY store_id;

UPDATE t_store_daily_stats
SET split_status=?, ...
WHERE merchant_id=? AND biz_date=?
```

- 现状 `t_order.split_batch_no` 上无独立索引（仅有 `idx_split_batch(merchant_id, split_batch_no, store_id)`）→ 单日 `split_batch_no IS NOT NULL` 过滤扫大量行；
- 每 5 分钟 × 商户数 × 近 7 日 = **批量子查询放大**；
- 单次 recompute 商户级 < 50ms，OK；但多商户并发时 + 多日回扫 → 总耗时长。🟠

### 路径 P6：`t_reconcile_diff` 写入

- 迁移 0026 已有索引 `idx_diff_type_date(diff_type, biz_date)`；
- Prechecker 失败时 N 条 INSERT，单事务一次性 → 锁 N 行相邻记录（InnoDB 主键聚簇，相邻插入触发 page latch）；
- 同商户不同日的差异不冲突，**OK**；
- 但 Prechecker 每天自动 Backfill → 同 `biz_date` 多次失败会**重复 INSERT**，需要先清空当日旧数据（同 `reconcile_test.go` 中 `SaveDiffs` 的 `DELETE WHERE biz_date=?` 模式）；
- 现有 `t_reconcile_diff` 已有 `idx_date_type`，无锁问题。

### 路径 P7：`t_split_bill_biz_date` 关联表

```sql
INSERT INTO t_split_bill_biz_date (bill_id, biz_date) VALUES (?,?), (?,?), ...;
```

- 单次分账插入 30 行（30 天），瞬时完成；无热点；
- 但 `idx_biz_date(biz_date)` 单纯反查「某日所有账单」→ 每次 Prechecker NOT EXISTS 子查询都需扫 `biz_date` 索引；
- 数据量小（数十万行），OK。

### 路径 P8：`t_split_audit` 写入

- 每次分账/对账/失败/重置都写，5 事件 × 商户 × 365 ≈ **千万行/年**；
- 单 INSERT，无锁问题；但查询（`GET /v1/admin/split/audit?biz_type=&biz_id=`）需索引；
- V2 仅 `idx_biz(biz_type, biz_id)` + `idx_action_time(action, created_at)`，无 `idx_biz_time`，分页查询全表扫隐患。🟠

### 路径 P9：`t_split_daily_execution`

- `uk_merchant_batch(merchant_id, batch_no)`：并发同商户多个 batch_no OK，但同商户同一 batch_no 二次请求触发唯一键冲突 → row-lock 等待；
- 每秒插入 QPS 极低（按时间段分账每天几次），**OK**。

---

## 三、🔴 高风险与优化

### H1：Prechecker 双层对账 NOT EXISTS 性能灾难

**问题**：每行 o 触发 2–3 次子查询，3000 万 × 2 = 6000 万次嵌套评估。

**优化方案**：

```sql
-- 用 LEFT JOIN + 聚合过滤替代 NOT EXISTS
SELECT DATE(o.paid_at) AS biz_date, o.store_id,
       COALESCE(SUM(o.paid_amount), 0) AS order_total
FROM t_order o
INNER JOIN t_store s ON s.id = o.store_id AND s.status = 1
LEFT JOIN t_split_execution se
       ON se.order_no = o.order_no AND se.status = 'SUCCESS'
LEFT JOIN t_split_bill sb ON sb.merchant_id = o.merchant_id AND sb.status = 'EXECUTED'
LEFT JOIN t_split_bill_biz_date bd
       ON bd.bill_id = sb.id AND bd.biz_date = DATE(o.paid_at)
WHERE o.merchant_id = ?
  AND o.status = 'PAID' AND o.deleted_at IS NULL
  AND o.paid_at >= ? AND o.paid_at < ?
  AND o.store_id IS NOT NULL
  AND se.order_no IS NULL     -- 已成功分账的订单排除
  AND bd.bill_id IS NULL      -- 已被账单覆盖的订单排除
GROUP BY DATE(o.paid_at), o.store_id;
```

- 优化器会把 3 个 LEFT JOIN 转 hash join，**单次评估 O(N)** 而非 O(N × M)；
- 配合下文的复合索引，可降到 **20–80 ms / 商户**。

### H2：`t_order` 缺 `(merchant_id, status, paid_at)` 复合索引

**问题**：所有「商户×时段×PAID」聚合都走弱索引。

**优化方案**：

```sql
-- 迁移 0027_add_perf_indexes.up.sql

-- 1) Prechecker / 聚合查询主索引
ALTER TABLE t_order
  ADD KEY idx_merchant_status_paidat (merchant_id, status, paid_at);

-- 2) split_batch_no IS NOT NULL 过滤辅助（已有 idx_split_batch 可复用，但缺 status）
ALTER TABLE t_order
  ADD KEY idx_merchant_split (merchant_id, split_batch_no, status, paid_at);

-- 3) 单笔订单所属门店×日反查（async 汇总用）
ALTER TABLE t_order
  ADD KEY idx_store_paidat (store_id, status, paid_at);
```

| 索引 | 服务于 |
|---|---|
| `idx_merchant_status_paidat` | Prechecker Layer A/B、Backfill、日报生成、所有「商户×时段」聚合 |
| `idx_merchant_split` | async 汇总 `split_batch_no IS NOT NULL` 过滤 |
| `idx_store_paidat` | RecomputeByStore 单门店回算 |

**索引代价**：每行多 30 字节 × 3000 万 = **900 MB**；InnoDB buffer pool 命中率下降 ~5%，可接受。

**⚠️ 写入代价**：每次 INSERT/UPDATE `t_order` 多写 3 个二级索引；当前 `INSERT` 路径为收银回调热点 → **收银写入 QPS 会受影响**。建议用 `pt-online-schema-change` 或 `gh-ost` 在线加索引，避免长时间表锁。

### H3：`t_reconcile_diff` Prechecker 失败批量 INSERT 的锁竞争

**问题**：单事务 INSERT 多行 → InnoDB 自增锁 + 行锁；与 09:00 微信对账任务并发时锁同一 page。

**优化方案**：
- 单次 Prechecker 失败最多 N 行（30 天 × 200 门店 × 商户数），实际商户失败通常 < 100 行；
- 用 `INSERT ... ON DUPLICATE KEY UPDATE` 代替 `DELETE + INSERT`（沿用现有 `SaveDiffs` 但改为幂等写入）；
- 主键自增锁的优化：MySQL 8 `innodb_autoinc_lock_mode=2`（默认已是）；如仍有问题可改用 UUID/雪花 ID。

### H4：async 汇总 5 分钟 × 多商户并发 → `t_order` 索引 page latch

**问题**：每 5 分钟 × 200 商户 × 近 7 日回算 = 1400 次扫表任务。

**优化方案**：
- **合并策略**：每轮按 `merchant_id` 分组，每商户仅跑最近一次分账的 `biz_date`（避免全商户全日回算）；
- **增量回算**：维护 `t_split_execution` 的 `executed_at` 触发增量更新（每分钟扫一次 `executed_at > now - 5min` 的记录对应的 `(merchant_id, biz_date)`）；
- **应用层去重**：所有 Recompute 请求入队列（`internal/split/recompute/queue.go`），按 `merchant_id × biz_date` 去重，串行处理。

### H5：`t_split_audit` 查询全表扫

**问题**：分页查询 `GET /v1/admin/split/audit?biz_type=DAILY_SPLIT&biz_id=...` 当 biz_id 为空时按时间分页 → 索引失效。

**优化方案**：

```sql
ALTER TABLE t_split_audit
  ADD KEY idx_biz_time (biz_type, created_at),
  ADD KEY idx_action_time_status (action, status, created_at);
```

---

## 四、🟠 中风险与优化

### M1：V2 中 `SameScopeFilter` 仍是子查询，性能不如 LEFT JOIN

**优化**：在 Prechecker 实际执行路径**不**使用 SameScopeFilter 字符串拼接，改用预编译的「LEFT JOIN + COALESCE」SQL（见 H1）；`SameScopeFilter` 保留作为**口径校验**的纯字符串函数，用于单元测试断言。

### M2：Backfill + Prechecker 在窗口内扫 `t_order` 两次

**优化**：
- Prechecker 入口 `Backfill` 仅在 `t_store_daily_stats` 缺失时调用（先 `SELECT 1 FROM t_store_daily_stats WHERE biz_date IN (...) LIMIT 1`）；
- 加缓存：`Prechecker.Check` 结果按 `(merchant_id, start, end, store_ids_hash)` 缓存 60 秒；同窗口内多次调用直接命中。

### M3：`t_store_daily_stats.split_status` 5 分钟延迟用户体验差

**优化**：
- 提供 `POST /v1/admin/store-stats/recompute` admin 触发立即重算（针对单一 `(merchant, biz_date)`）；
- 前端在「分账成功」Toast 后主动调用 recompute 并刷新列表。

### M4：`t_split_daily_execution.uk_merchant_batch` 写入热点（同商户并发批次）

**问题**：单商户并发多个 `ExecuteByPeriod` 请求时（如不同门店管理员同时操作）触发唯一键冲突。

**优化**：
- `batch_no` 已有 `SP{rule}-{start}-{end}` 唯一性，但商户可能 1 个规则 1 个时段 → 仍冲突；
- **改 uk 为 (merchant_id, batch_no, run_id)**（V2 已有 `uk_run_id`），并把 `run_id` 作为唯一键，去掉 `uk_merchant_batch`；
- 让并发安全由 `run_id` 唯一性保证。

### M5：`t_split_bill_biz_date` 索引覆盖不足

**优化**：现状 `idx_biz_date(biz_date)` 已可；但 Prechecker NOT EXISTS 仍要回表。H1 已用 LEFT JOIN 解决。

---

## 五、🟡 低风险（建议但非必须）

| ID | 风险 | 建议 |
|---|---|---|
| L1 | `t_store_daily_stats` 新增 4 列（split_status/split_batch_no/split_at/split_total_amount）写入放大 | 同步 H2 用 gh-ost 在线加；行大小约 +40 字节 |
| L2 | `t_reconcile_diff` 缺 `biz_date` 字段（V2 沿用已有表的 `order_no/transaction_id`，biz_date 通过 detail JSON 存） | 迁移 0026 加列 `biz_date DATE NULL` + `idx_diff_type_biz_date(diff_type, biz_date)`，便于管理端按日查询 |
| L3 | `error_message VARCHAR(1024)` 在 UTF8MB4 下 4 字节/字符，最大 4 KB | 应用层做截断（V2 已规划），确认 max_length |
| L4 | `t_split_daily_execution.duration_ms INT` 上限约 24 天 | 实际分账 < 1 小时，可接受 |
| L5 | `t_split_audit` detail JSON 无大小约束 | 应用层做截断 4 KB |

---

## 六、连接池与 QPS 影响

### 当前架构假设
- `db_conn.master` 单实例 MySQL 8.0，连接池 `db.SetMaxOpenConns(50)`（假设）；
- 收银回调高频 INSERT：平均 100 QPS，峰值 500 QPS；
- 分账请求低频：单商户 5 次/日 = **全平台 1000 次/日 ≈ 0.01 QPS**。

### V2 引入后
- Prechecker 单次 2 个聚合查询 = **增加 0.02 QPS 全平台**，可忽略；
- 但**单次查询耗时**从无 → **50–500 ms**，可能短暂占用连接；
- async 汇总 5 分钟一次 × 商户 = **200 QPS 短时**（单次 50 ms），高峰期可能与收银 INSERT 竞争连接。

**优化建议**：
- `cmd/server/main.go` 中 DB 连接池调至 `MaxOpenConns(100)`、`MaxIdleConns(20)`；
- Prechecker / async 汇总使用**专用读连接池**（`db.SetMaxOpenConns(30)` 的只读账号），与主写池隔离；
- async 汇总**错峰**：从 5 分钟改为 10 分钟 + 随机抖动（避免与 09:00 对账重叠）。

---

## 七、容量规划（1 年）

| 表 | 当前估算 | 1 年增量 | 备注 |
|---|---|---|---|
| `t_order` | 3000 万 | +1500 万（500 万/年 × 1.5 倍增长） | 主索引已覆盖 |
| `t_split_execution` | 4500 万 | +2000 万 | `uk_order_receiver` 足够 |
| `t_split_bill` | 数十万 | +数十万 | 小表 |
| `t_split_bill_biz_date` | 0 | +数百万 | 与 bill 1:N |
| `t_reconcile_diff` | 0 | +数十万（SPLIT_PRECHECK + 已有） | 单条小 |
| `t_split_daily_execution` | 0 | +20 万（200 商户 × 5 规则 × 365 × 0.5 失败率） | 小表 |
| `t_split_audit` | 0 | **+1000 万**（千万级） | 需评估归档策略 |
| `t_store_daily_stats` | 数千 | +数万 | 小表 |

**t_split_audit 归档策略**：
- 保留 90 天在线，超过归档到 `t_split_audit_archive`（压缩表）；
- 按 `biz_type` 分区：`PARTITION BY RANGE (TO_DAYS(created_at))`。

---

## 八、迁移成本与回滚

### 迁移 0027（性能索引）
- `idx_merchant_status_paidat`：online DDL 约 **5–10 分钟**（3000 万行）；
- `idx_merchant_split` / `idx_store_paidat`：各 **2–5 分钟**；
- **总停机/锁等待**：使用 gh-ost 几乎 0 阻塞；如用 ALGORITHM=INPLACE 仍需短暂 table share lock。

### 迁移 0026（业务表）
- `t_store_daily_stats` 加 4 列：瞬间；
- `t_split_daily_execution` 新建：瞬间；
- `t_split_bill_biz_date` 新建：瞬间；
- `t_reconcile_diff` 加索引 `idx_diff_type_date`：瞬间（如已有跳过）；
- `t_split_audit` 新建：瞬间。

**回滚预案**：
- 删除迁移 0027 索引（`ALTER TABLE DROP KEY`）；
- 删除迁移 0026 业务表 / 列；
- 应用层 `if !hasColumn("split_status")` 兼容老逻辑。

---

## 九、验证清单（性能验收）

| 项 | 目标 | 测试方法 |
|---|---|---|
| Prechecker Layer A 单商户单日 | < 50 ms P99 | sysbench + EXPLAIN ANALYZE |
| Prechecker Layer A 单商户 30 日 | < 200 ms P99 | 同上 |
| Prechecker Layer B 单商户 30 日 × 20 门店 | < 300 ms P99 | 同上 |
| Backfill 单商户 30 日 | < 500 ms | 同上 |
| async 汇总 单商户×日 | < 50 ms P99 | 同上 |
| 收银回调 INSERT 耗时 | 增加 < 5% | wrk 压测 |
| 并发 100 个 Prechecker 同时调用 | 总耗时 < 10 s | k6 |
| `t_split_audit` 千万级查询 | < 200 ms P99 | EXPLAIN |

---

## 十、修订后落地的优化清单

| 优先级 | 优化项 | 落地 |
|---|---|---|
| 🔴P0 | 加 `idx_merchant_status_paidat` | 迁移 0027（必须做） |
| 🔴P0 | Prechecker 改 LEFT JOIN 替代 NOT EXISTS | `prechecker.go` 重写 |
| 🔴P0 | `t_split_audit` 加 `idx_biz_time` | 迁移 0026 增列 |
| 🟠P1 | 加 `idx_merchant_split` / `idx_store_paidat` | 迁移 0027 |
| 🟠P1 | async 汇总增量触发 + 队列去重 | `internal/split/recompute/queue.go` |
| 🟠P1 | 连接池拆分（Prechecker / async 用独立读池） | `cmd/server/main.go` |
| 🟠P1 | `t_reconcile_diff` 加 `biz_date` 列 + 索引 | 迁移 0026 增列 |
| 🟠P1 | `uk_merchant_batch` 改 `uk_run_id` | 迁移 0026 |
| 🟡P2 | Prechecker 结果缓存 60 秒 | `internal/split/recon/cache.go` |
| 🟡P2 | `t_split_audit` 月度分区 + 90 天归档 | 迁移 0028（后续迭代） |
| 🟡P2 | `t_split_bill_biz_date` 量大时按月分区 | 后续迭代 |

---

## 十一、最终结论

V2 方案在落地前必须补 3 个改动：

1. **迁移 0027**：3 个 `t_order` 复合索引（在线 DDL）
2. **Prechecker 重写**：NOT EXISTS → LEFT JOIN + 物化聚合
3. **`t_split_audit` 索引**：`idx_biz_time`

其他 7 项优化可在迭代 B/C 同步推进，不阻塞主流程。

**性能 P99 目标（落地后）**：
- Prechecker 双层：单商户×30 日 ≤ 300 ms
- async 汇总：单商户×日 ≤ 50 ms
- 收银回调写入：性能影响 < 5%

**风险评级**：🔴→🟢（落地后）。

---

**下一步建议**：
1. 先在测试环境用 100 万行 mock 数据跑 `EXPLAIN ANALYZE` 验证 H1+H2 优化效果
2. 迁移 0027 准备 gh-ost 脚本并演练回滚
3. Prechecker 重写后跑 chaos 测试（注入慢查询、连接池满）