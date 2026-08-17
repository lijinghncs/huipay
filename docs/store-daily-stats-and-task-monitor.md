# 门店日报定时任务 + 定时任务监测列表 迭代方案

> 目标：
> 1. 新增定时任务「门店订单日报」：T+1 02:00 按门店聚合 T 日订单总笔数、总金额，落 `t_store_daily_stats`；
> 2. 管理端新增「门店按日统计」报表（多日范围汇总 + 明细下钻）；
> 3. 抽定「定时任务运行监测」基础设施（注册表 + 运行日志表 + HTTP 接口 + 管理端页面），覆盖所有现有/未来定时任务的执行情况可视化。
>
> 范围：**不**触动支付侧对账（T+1 04:00 已存在）。仅纳入「门店日报」+「监测基础设施」。

---

## 一、现状速查

| 关注点 | 现状 |
|---|---|
| 调度方式 | 各业务独立 `Start(ctx)` + ticker；`cmd/server/main.go` 直接 `go xxx.Start(...)` 注册 4 个：超时关单(30s)、幂等清理(1h)、每日对账(1min tick 命中 09:00)、分账补偿(30s) |
| 运行记录 | **无任何持久化**；只有 zap 日志；管理端无接口查看 |
| 调度时间配置 | 硬编码（间隔/时刻）；无配置项 |
| `t_order` 字段 | 已有 `merchant_id/store_id/paid_at/paid_amount/status/channel/amount`，索引 `idx_store(store_id)` 与 `idx_split_batch(merchant_id, split_batch_no, store_id)` |
| `t_store` | 已有 ID/MerchantID/Status（迁移 0015） |
| 管理端 API | `admin/merchants*`、`merchant/stores*`、`merchant/codes*`、`merchant/split/*`，无 admin/stores/*、无 admin/scheduler/* |
| 管理端前端 | 4 页：概览/商户/通道/风控；概览页是 mock；无门店/定时任务页面 |
| `prom` 指标 | `SplitSuccessRate / SplitAmountTotal / SplitOrderTotal / SplitFailureReasonTotal / SplitHangingTotal / SplitRetryTotal` |
| 迁移号 | 已到 `0023_add_split_batch_no_to_t_order`；本轮新表使用 `0024_*` |

---

## 二、关键决策（已与用户对齐）

| 决策点 | 取值 |
|---|---|
| 调度时刻 | 门店日报 T+1 02:00；不与现有对账 09:00 冲突；后续可调 |
| 统计口径 | **按门店 × 日** 聚合：订单总笔数（PAID）+ 总金额（`SUM(paid_amount)`，分） |
| 幂等键 | `(store_id, biz_date)` 唯一；T 日重跑覆盖（`ON DUPLICATE KEY UPDATE`） |
| 多日范围汇总 | 报表默认查询 `start_date~end_date` 区间，按门店横向聚合；明细下钻展示每日值 |
| 监测基础设施 | 「调度注册表（启动时一次性记录进程内存在的调度任务）」+「运行日志表（每次执行一条）」；所有调度器改造后接入 |
| 调度时间可配置 | 通过 `t_scheduler_config` 存 cron 表达式（最小：每天几点、间隔秒数），后续可热更；本期先落地**结构 + 默认值**，UI 编辑迭代 2 |
| 商户隔离 | 报表粒度「按商户 + 时间区间 + 门店筛选」，管理员按商户过滤 |

---

## 三、迭代规划（2 个迭代，共 11 个任务）

### 迭代 1：门店日报 + 监测基础设施（核心）

#### 1.1 迁移 `0024_create_t_store_daily_stats.up.sql`（新建）

```sql
CREATE TABLE t_store_daily_stats (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  merchant_id     BIGINT UNSIGNED NOT NULL COMMENT '所属商户',
  store_id        BIGINT UNSIGNED NOT NULL COMMENT '门店 ID',
  biz_date        DATE            NOT NULL COMMENT '业务日期 YYYY-MM-DD',
  order_count     INT             NOT NULL DEFAULT 0 COMMENT '订单总笔数(PAID)',
  paid_amount     BIGINT          NOT NULL DEFAULT 0 COMMENT '订单总金额(分,SUM(paid_amount))',
  channel_breakdown JSON          NULL     COMMENT '各渠道拆分 {WECHAT_NATIVE:{count,amount}, ...}',
  status_breakdown  JSON          NULL     COMMENT '各状态笔数 {PAID:n,REFUNDED:n,CLOSED:n}',
  generated_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '统计生成时间',
  updated_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_store_date (store_id, biz_date),
  KEY idx_merchant_date (merchant_id, biz_date),
  KEY idx_date (biz_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='门店订单日报(T+1聚合)';
```

#### 1.2 仓储 `internal/stats/repository/store_daily_stats_repo.go`（新建）

- `Upsert(ctx, rows []StoreDailyStatsModel)`：`ON DUPLICATE KEY UPDATE` 幂等；`channel_breakdown / status_breakdown` 使用 `gorm.Expr("VALUES(...)")` 或 `json_merge_patch`；
- `ListByMerchantDateRange(ctx, merchantID, start, end, storeID?, page, size)`：分页 + 总数；`start <= biz_date < end`；
- `GetStoreStatsSummary(ctx, merchantID, start, end, storeID?)`：聚合 `SUM(order_count)/SUM(paid_amount)`。

#### 1.3 服务 `internal/stats/service/store_daily_stats_service.go`（新建）

- `GenerateDailyStats(ctx, bizDate)`：
  - 聚合 SQL（单次扫表，禁止 N+1）：
    ```sql
    SELECT store_id, merchant_id,
           SUM(CASE WHEN status='PAID' THEN 1 ELSE 0 END) AS order_count,
           SUM(CASE WHEN status='PAID' THEN paid_amount ELSE 0 END) AS paid_amount,
           JSON_OBJECTAGG(...) AS channel_breakdown,
           JSON_OBJECTAGG(...) AS status_breakdown
    FROM t_order
    WHERE merchant_id IN (SELECT id FROM t_entity WHERE entity_type='MERCHANT' AND status=1)
      AND store_id IS NOT NULL
      AND paid_at >= ? AND paid_at < ?
    GROUP BY store_id, merchant_id
    ```
  - 商户门店缺失过滤（已删除门店不出现在表里，避免长期堆积）；
  - 调 `repo.Upsert` 写入；返回生成条数 + 耗时。

#### 1.4 调度 `internal/stats/scheduler/store_daily_stats_scheduler.go`（新建）

- 沿用现有模式 `Start(ctx)` + ticker + `runOnce`，但**接入监测基础设施**（见 1.6）：
  - 注册任务：`scheduler.Register("store_daily_stats", "门店订单日报", "T+1 02:00 聚合 T 日订单", 24*time.Hour)`；
  - 命中 02:00 窗口（与对账 `reconcile_daily.go` 同一判定模式）→ `runOnce`；
  - 异常 `recover` → 写失败运行日志 → 告警 hook；
  - 入参：`bizDate = now.AddDate(0,0,-1)`；首次上线可加 `forceRun(bizDate)` 工具方法便于补跑历史。

#### 1.5 服务入口接入 `cmd/server/main.go`

- 在「8. 启动定时任务」块新增：
  ```go
  statsRepo := statsrepo.NewStoreDailyStatsRepo(dbConn.Master)
  statsSvc  := statsservice.NewStoreDailyStatsService(statsRepo, dbConn.Master, logger)
  schedulers.Register(NewStoreDailyStatsScheduler(statsSvc, dbConn.Master, logger))
  ```
- 重构为 `schedulers.StartAll(ctx)`：遍历注册表逐个启动，与现有 `go xxx.Start()` 并行（迁移期内兼容）。

#### 1.6 监测基础设施

##### 1.6.1 迁移 `0025_create_t_scheduler_registry_and_run_log.up.sql`

```sql
-- 调度注册表：启动时记录进程内已注册的调度任务（name 唯一）
CREATE TABLE t_scheduler_registry (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name          VARCHAR(64)     NOT NULL COMMENT '调度任务唯一名(store_daily_stats/reconcile_daily/...)',
  display_name  VARCHAR(128)    NOT NULL COMMENT '中文名',
  description   VARCHAR(512)    NULL,
  cron_expr     VARCHAR(64)     NULL COMMENT '人类可读描述(每天02:00)',
  interval_sec  INT             NULL COMMENT '周期(秒)，轮询型调度使用',
  enabled       TINYINT         NOT NULL DEFAULT 1,
  instance_id   VARCHAR(64)     NOT NULL COMMENT '本次进程实例ID(HOSTNAME+PID+启动时间)',
  registered_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='定时任务注册表';

-- 运行日志：每次执行一条
CREATE TABLE t_scheduler_run_log (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name          VARCHAR(64)     NOT NULL COMMENT '调度任务名',
  instance_id   VARCHAR(64)     NOT NULL,
  biz_date      DATE            NULL     COMMENT '业务日期(报表/对账类)',
  run_mode      VARCHAR(16)     NOT NULL COMMENT 'AUTO/MANUAL',
  status        VARCHAR(16)     NOT NULL COMMENT 'RUNNING/SUCCESS/FAILED/TIMEOUT',
  started_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  finished_at   DATETIME(3)     NULL,
  duration_ms   INT             NULL,
  rows_affected BIGINT          NULL     COMMENT '影响行数(统计/对账)',
  error_message TEXT            NULL,
  trace_id      VARCHAR(64)     NULL,
  PRIMARY KEY (id),
  KEY idx_name_started (name, started_at),
  KEY idx_status_started (status, started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='定时任务运行日志';
```

##### 1.6.2 调度基础设施 `internal/scheduler/framework/`（新建）

- `registry.go`：
  - 全局 `var Registry = make(map[string]TaskInfo)`；
  - `Register(name, display, desc, cronExpr string, intervalSec int) *Handle`：写 `t_scheduler_registry`（`ON DUPLICATE KEY UPDATE` 更新 display_name/description/cron/interval/enabled），返回 `*Handle`；
  - `Handle` 暴露 `Start(ctx, runner Runner)`：自带 ticker + 实例ID + 运行日志写入 + panic recover。
- `runner.go`：`type Runner func(ctx, bizDate) (rowsAffected int64, err error)`；传 `bizDate=time.Time{}` 表示无业务日期。
- `runlog_repo.go`：`StartRun / FinishRun / FailRun / ListRuns / LatestByName`。
- 现有调度器迁移（最小集）：
  - `reconcile_daily.go`：保留内部 ticker 逻辑，但 `runOnce` 改为先写 RUNNING、结束写 SUCCESS/FAILED；`name="reconcile_daily"`；
  - `compensate.go`：`name="split_compensate"`，每轮记 SUCCESS（行数 = 重入成功数）；
  - `close_expired.go`：`name="order_close_expired"`，每轮记一次；
  - `cleanup_idem.go`：`name="idem_cleanup"`；
  - 新增 `store_daily_stats.go`：`name="store_daily_stats"`。

> 兼容性：保留原有 ticker 逻辑；运行日志写入**不阻断**主流程（`recover + log.Warn`）。

##### 1.6.3 HTTP 接口 `internal/admin/handler/scheduler.go`（新建）

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET /v1/admin/scheduler/tasks` | 已注册任务列表（含 `display_name/cron/interval_sec/enabled/last_run/last_status/last_run_at/last_duration_ms/last_rows`） |
| `GET /v1/admin/scheduler/runs` | 运行日志列表，支持 `name/status/start/end` + 分页 |
| `GET /v1/admin/scheduler/runs/:id` | 单次运行详情（含 `error_message`） |
| `POST /v1/admin/scheduler/tasks/:name/run` | 手动触发（强制重跑）：异步执行 + 返回 `run_id`；仅 admin 角色可用，幂等键 `(name, biz_date, run_mode=MANUAL)` |
| `GET /v1/admin/scheduler/runs/:id/log` | 可选：拉取该次运行期间的结构化日志（沿用 zap + 简单 file/stdout 抓取，本期先返回 `error_message`，迭代 2 接 ELK） |

实现：handler/service 走 `admin` 路由组，复用 `internal/admin/` 新建的 `service/handler/repository` 三层（参照 `internal/merchant`）。

---

### 迭代 2：管理端报表页 + 定时任务监测页

#### 2.1 报表接口 `internal/admin/handler/store_stats.go`（新建）

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET /v1/admin/store-stats` | 列表：参数 `merchant_id? / store_id? / start_date / end_date / page / page_size`；返回 `items[] + total + summary{sum_order_count,sum_paid_amount}` |
| `GET /v1/admin/store-stats/summary` | 多日范围汇总：参数同上；按 `store_id` 行聚合（`SUM(order_count)/SUM(paid_amount)`），同时返回区间内的日列表 `daily_breakdown[]` |
| `GET /v1/admin/stores/:id/daily-stats` | 单门店按日详情（与 `?start_date=&end_date=`） |

数据来源：`t_store_daily_stats`（已存在的聚合结果）。**不**实时计算 `t_order`，避免大表查询阻塞。

#### 2.2 admin-portal 新增页面

##### 2.2.1 路由 `packages/admin-portal/config/routes.ts`

```ts
{ path: '/store-stats', name: '门店统计', icon: 'BarChartOutlined' },
{ path: '/scheduler', name: '定时任务', icon: 'ClockCircleOutlined' },
```

##### 2.2.2 服务 `packages/admin-portal/src/services/admin.ts` 扩展

- `listStoreStats(params)` / `getStoreStatsSummary(params)` / `getStoreDailyStats(storeId, range)`；
- `listSchedulerTasks()` / `listSchedulerRuns(params)` / `getSchedulerRun(id)` / `triggerSchedulerTask(name, bizDate?)`。

##### 2.2.3 页面

- `pages/StoreStats/index.tsx`（新建）：
  - 顶部筛选条：商户 select、门店 select（联动）、日期范围（默认近 7 天）、汇总 KPI 卡；
  - 主表：商户/门店 × 多日汇总（`SUM`），点击门店展开 `daily_breakdown` 抽屉/嵌套表；
  - 导出 CSV（前端按当前筛选生成，不依赖后端导出）。
- `pages/Scheduler/index.tsx`（新建）：
  - Tab 1「任务列表」：表格列 `name / display_name / cron / interval_sec / enabled / last_run_at / last_status / last_duration_ms / last_rows / 操作[运行]`；操作触发 `POST /v1/admin/scheduler/tasks/:name/run`，成功后 toast + 跳转运行日志 Tab；
  - Tab 2「运行日志」：表格列 `name / run_mode / biz_date / status / duration_ms / rows_affected / started_at / 操作[详情]`；详情抽屉展示 `error_message`；
  - 筛选：`name / status / start / end` + 状态徽章 `RUNNING/SUCCESS/FAILED/TIMEOUT`。

---

## 四、关键 SQL 与聚合逻辑（迭代 1.3 重点）

```sql
-- 单次聚合：避免 N+1
INSERT INTO t_store_daily_stats
  (merchant_id, store_id, biz_date, order_count, paid_amount, channel_breakdown, status_breakdown)
SELECT
  o.merchant_id,
  o.store_id,
  DATE(o.paid_at) AS biz_date,
  SUM(CASE WHEN o.status='PAID' THEN 1 ELSE 0 END) AS order_count,
  SUM(CASE WHEN o.status='PAID' THEN o.paid_amount ELSE 0 END) AS paid_amount,
  JSON_OBJECT(
    'WECHAT', JSON_OBJECT(
      'count', SUM(CASE WHEN o.channel='WECHAT' AND o.status='PAID' THEN 1 ELSE 0 END),
      'amount', SUM(CASE WHEN o.channel='WECHAT' AND o.status='PAID' THEN o.paid_amount ELSE 0 END)
    ),
    'ALIPAY', JSON_OBJECT(
      'count', SUM(CASE WHEN o.channel='ALIPAY' AND o.status='PAID' THEN 1 ELSE 0 END),
      'amount', SUM(CASE WHEN o.channel='ALIPAY' AND o.status='PAID' THEN o.paid_amount ELSE 0 END)
    )
  ) AS channel_breakdown,
  JSON_OBJECT(
    'PAID',     SUM(CASE WHEN o.status='PAID' THEN 1 ELSE 0 END),
    'REFUNDED', SUM(CASE WHEN o.status='REFUNDED' THEN 1 ELSE 0 END),
    'CLOSED',   SUM(CASE WHEN o.status='CLOSED' THEN 1 ELSE 0 END)
  ) AS status_breakdown
FROM t_order o
INNER JOIN t_store s ON s.id = o.store_id AND s.status = 1
WHERE o.paid_at >= ? AND o.paid_at < ?
GROUP BY o.merchant_id, o.store_id, DATE(o.paid_at)
ON DUPLICATE KEY UPDATE
  order_count = VALUES(order_count),
  paid_amount = VALUES(paid_amount),
  channel_breakdown = VALUES(channel_breakdown),
  status_breakdown = VALUES(status_breakdown),
  generated_at = CURRENT_TIMESTAMP(3);
```

注意：
- 仅聚合 `paid_at` 落 T 日的订单（已支付时间维度，与「总笔数/总金额」语义对齐）；
- 关联 `t_store.status=1` 过滤已删除门店（后续若恢复，可手动补跑）；
- 渠道枚举以现有 `vo.ChannelCode` 为准，本期先覆盖 `WECHAT / ALIPAY`，其他归 `OTHER`；
- `t_order` 数据量随时间增长后，建议按 `idx_store(store_id)` + 时间区间扫描；如后续超过 500 万行，加 `(paid_at, store_id)` 复合索引或考虑 ClickHouse 迁移。

---

## 五、运行日志写入契约（监测基础设施核心）

所有调度器统一行为：

1. 命中触发条件 → `runlog.StartRun(name, bizDate, run_mode=AUTO)` 写入 `status='RUNNING'`；
2. 执行主体 → `Runner` 返回 `(rowsAffected, err)`；
3. 成功 → `FinishRun(id, rowsAffected)` 写 `status='SUCCESS' / finished_at / duration_ms`；
4. 失败（含超时，timeout 阈值默认 10 分钟，可配置）→ `FailRun(id, errMsg)`；
5. panic → `recover` 后 `FailRun` 写 `error_message`；主流程不阻断。

```go
// internal/scheduler/framework/runner.go（节选）
type Runner func(ctx context.Context, bizDate time.Time) (int64, error)

func (h *Handle) Start(ctx context.Context, runner Runner) {
    ticker := time.NewTicker(h.interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done(): return
        case <-ticker.C:
            if !h.shouldRun(time.Now()) { continue }
            h.executeOnce(ctx, runner)
        }
    }
}
```

---

## 六、接口清单

| 方法 | 路径 | 鉴权 |
|---|---|---|
| GET | `/v1/admin/store-stats` | admin |
| GET | `/v1/admin/store-stats/summary` | admin |
| GET | `/v1/admin/stores/:id/daily-stats` | admin |
| GET | `/v1/admin/scheduler/tasks` | admin |
| GET | `/v1/admin/scheduler/runs` | admin |
| GET | `/v1/admin/scheduler/runs/:id` | admin |
| POST | `/v1/admin/scheduler/tasks/:name/run` | admin |

**注意**：当前 `cmd/server/main.go` 没有 admin 鉴权中间件，admin 路由组裸跑；本轮沿用现状，仅在路由前缀 `/v1/admin/*` 上加 TODO 注释，下一轮迭代统一接入。

---

## 七、任务清单

| 迭代 | 任务 | 涉及文件 |
|---|---|---|
| 1 | 迁移 `0024_create_t_store_daily_stats` | `infra/migrator/migrations/0024_*.sql` |
| 1 | 仓储 `StoreDailyStatsRepo` | `internal/stats/repository/` |
| 1 | 服务 `StoreDailyStatsService`（含聚合 SQL） | `internal/stats/service/` |
| 1 | 调度 `StoreDailyStatsScheduler`（02:00 窗口） | `internal/stats/scheduler/` |
| 1 | 接入 `cmd/server/main.go` | `cmd/server/main.go` |
| 1 | 迁移 `0025_create_t_scheduler_registry_and_run_log` | `infra/migrator/migrations/0025_*.sql` |
| 1 | 调度框架 `registry/runner/runlog_repo` | `internal/scheduler/framework/` |
| 1 | 4 个旧调度器接入监测（reconcile/compensate/close_expired/cleanup_idem） | `internal/{payment/reconcile,split,order}/scheduler/` |
| 2 | admin handler/service/repo + 路由 | `internal/admin/...`、`cmd/server/main.go` |
| 2 | admin-portal services 扩展 | `packages/admin-portal/src/services/admin.ts` |
| 2 | StoreStats 页 + Scheduler 页（含 Tabs） | `packages/admin-portal/src/pages/StoreStats/...`、`Scheduler/...` |
| 2 | 路由 + 菜单接入 | `packages/admin-portal/config/routes.ts`、`App.tsx`、`BasicLayout.tsx` |

---

## 八、依赖与顺序

- 迭代 1 是迭代 2 的前置：报表数据来自 `t_store_daily_stats`；监测页来自 `t_scheduler_run_log`。
- 监测基础设施不影响各调度器主流程，可与 1.1–1.5 **并行**实施（不同文件）。
- 迭代 2 依赖迭代 1 的接口与表。

---

## 九、风险与缓解

| 风险 | 缓解 |
|---|---|
| 02:00 与对账 09:00 不重叠，但同日 MySQL 多任务并发 | 任务错峰（02:00 / 04:00 / 09:00），并控制单任务 batch_size；运行日志写入失败不阻断主流程 |
| `t_order` 数据量大，全量聚合慢 | 按 `paid_at` 区间扫，索引 `idx_store`；聚合走单 SQL；500 万行级以下无需分表，超过再评估 |
| 手动触发重复执行导致重复入账 | 手动触发也写 `run_log`，通过 `(name, biz_date, run_mode)` 唯一约束防止同日 MANUAL 多次；统计任务本身就是 upsert，可重入 |
| 时区漂移导致 biz_date 偏差 | 调度器使用 `time.Local`，与现有对账一致；文档明示「T 日以本地时区划分」 |
| 监测基础设施自身失败 | `run_log` 写入独立 `defer recover`，DB 短暂不可用时仅丢日志、不影响调度任务；DB 长时间不可用走现有 zap 告警 |
| admin 路由无鉴权 | 本轮沿用现状 + TODO；下轮统一接 admin auth 中间件 |

---

## 十、验收标准

1. 启动后 `t_scheduler_registry` 自动出现 5 条记录（4 个旧 + 1 个新增），`instance_id` 与进程一致。
2. T+1 02:00 跑一次后，`t_store_daily_stats` 出现 `(store_id, biz_date)` 记录，`order_count / paid_amount` 与 `t_order` 聚合结果一致（误差 0）；重复运行同一天，**不**产生重复行（唯一键生效）。
3. 管理端 `/store-stats` 页面：选择商户 + 日期范围（多日），表格展示按门店的多日汇总（`SUM`），点击门店展开每日明细；KPI 卡与表合计一致。
4. 管理端 `/scheduler` 页面：
   - 「任务列表」展示 5 个任务及其最近一次运行状态、耗时、影响行数；
   - 「运行日志」支持按任务/状态/时间筛选，可查看失败原因；
   - 点击「运行」可手动触发门店日报或对账任务，列表 5 秒内刷新看到 `RUNNING` → `SUCCESS`。
5. 旧调度器（关单/补偿/对账/幂等清理）的运行也写入 `t_scheduler_run_log`；关闭 DB 重启后再次写入，新的 `instance_id` 反映。
6. 异常注入：聚合 SQL 故意失败 → 主流程不挂、运行日志 `FAILED` + `error_message` 可见、Prometheus 计数（可选）增加。