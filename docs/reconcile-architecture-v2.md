# 对账模块架构改进方案（V2 · 低耦合高内聚重构）

> 版本：v1.1
> 日期：2026-08-17
> 状态：评审通过，实施中
>
> 本文档为对账域的架构改进方案（不涉及功能需求变更），目标：
> 1. 将散落在 4 处的对账代码收敛为独立限界上下文 `internal/recon`
> 2. 消除同表双写、跨域直查、比对与副作用缠绕三类结构性问题
> 3. 沿用 split 域 P0–P5 重构的同一套打法（纯函数核心 + 端口适配 + 薄调度 + handler 零改动）
>
> 关联文档：
> - [reconcile-center-design.md](./reconcile-center-design.md)（对账中心页面设计，本方案不改其 API 契约）
> - [split-precheck-and-execution-record-v2.md](./split-precheck-and-execution-record-v2.md)（前置对账功能方案，本方案保留其全部口径决策）

---

## 一、背景与现状速查

系统存在三层对账机制，代码分散在 4 个位置，共享同一张差异表 `t_reconcile_diff`：

| 对账层 | 代码位置 | 触发方式 | 差异类型 | 差异写入方 |
|---|---|---|---|---|
| ① 前置对账 | `internal/split/recon/prechecker.go`（245 行） | 同步（分账执行/试算/生成账单） | `SPLIT_TOTAL` / `SPLIT_DETAIL` | `split/repository.ReconcileDiffRepo.WriteSplitPrecheck` |
| ② 渠道对账 | `internal/payment/reconcile/wechat.go` + `wechat_store.go` | 每日 09:00（自有 ticker） | `LONG` / `SHORT` / `MISMATCH` | `payment/reconcile.SaveDiffs`（**独立模型**） |
| ③ 执行后对账 | `internal/split/scheduler/reconcile_daily.go`（209 行） | 每日 02:30（framework 注册） | `SPLIT_POST` / `SPLIT_DEGRADED` | `split/repository.ReconcileDiffRepo.WriteSplitPost` |
| 差错中心查询 | `internal/split/service/reconcile.go`（107 行） | HTTP | —（只读 + 核销） | — |

`ReconcileDiffRepo` 当前有 4 个消费方：split Service（商户端差错中心）、admin SplitManageService（管理端差异列表/核销）、recon.Prechecker、split/scheduler。

---

## 二、现状诊断（耦合点清单）

| # | 严重度 | 问题 | 证据 |
|---|---|---|---|
| 🔴1 | 严重 | **同表双写 + 幂等策略冲突，存在误删数据缺陷**：`payment/reconcile.DiffModel`（`BizDate string`、无 `merchant_id`）与 `split/repository.ReconcileDiffModel`（`BizDate time.Time`、有 `merchant_id`）都映射 `t_reconcile_diff`。渠道对账 `SaveDiffs` 按 `WHERE biz_date = ?` **不区分 diff_type 全量清理**（wechat_store.go:37）——09:00 渠道对账会删掉 02:30 执行后对账刚写入的同业务日 `SPLIT_POST` / `SPLIT_DEGRADED` 差异，以及落在同日的 `SPLIT_TOTAL` / `SPLIT_DETAIL` | wechat_store.go:35-50 vs reconcile_diff_repo.go:118-142 |
| 🔴2 | 严重 | **跨域直查表**：渠道对账直查 order 域 `t_order`；Prechecker 直查 stats 域 `t_store_daily_stats`、order 域 `t_order`；执行后对账直查 `t_journal_entry` / `t_split_execution` / `t_split_order_status`。对账逻辑与四个域的表结构硬耦合 | wechat.go:89-93、prechecker.go:151-209、reconcile_daily.go:91-152 |
| 🔴3 | 严重 | **比对逻辑与副作用缠绕，核心算法不可单测**：`Prechecker.Check` 单方法混合补跑触发 → 4 段 raw SQL 取数 → 比对 → 写差异 → 写审计 → prom 打点 → 构造业务错误；真正纯的只有 `compareRows`，但整体必须起 DB 才能测。执行后对账同样 3 段 raw SQL 直接写在调度器里 | prechecker.go:71-149、reconcile_daily.go:117-206 |
| 🟠4 | 高 | **对账无独立域**：对账是平台级跨域能力（涉及 order/payment/split/stats 四域数据），却寄生在 split 与 payment 两处；差错中心查的是三层共享的差异表，却挂在 `split/service` 下；`DiffLong/Short/Mismatch` 常量在 payment/reconcile 与 split/repository 两处重复定义 | wechat.go:19-23 vs reconcile_diff_repo.go:13-21 |
| 🟠5 | 高 | **调度器承担业务**：`SplitReconcileScheduler` 同时负责 cron 注册 + 手动触发 + 取数 + 比对 + 落库 + 告警；`NewSplitReconcileScheduler` 与 `ReconcileRunnable` 将同一 struct 重复构造两遍。渠道对账调度又是另一套（自有 ticker，仅向 framework 登记元信息），两种调度风格并存 | reconcile_daily.go:29-63、payment/reconcile/scheduler/reconcile_daily.go:18-38 |
| 🟠6 | 高 | **基础设施硬耦合**：Prechecker 直接 import `infra/prom` 打点、直接持有 `*gorm.DB`；`ReconcileDiffRepo.DB()` 暴露底层连接（泄漏抽象）；`WriteSplitPrecheck(diffs any)` 弱类型入参 | prechecker.go:23,45、reconcile_diff_repo.go:52,56 |
| 🟡7 | 中 | **端口反向依赖实现**：`ports.Prechecker` 返回 `*recon.CheckResult`，ports 包 import 了 recon 实现包，依赖倒置不彻底 | ports/ports.go:13,32-34 |
| 🟡8 | 中 | **前置对账无运行日志**（对账中心设计文档 §5.1 已立项，依赖本次重构顺带落地：副作用端口化后才能注入 RunLogger） | reconcile-center-design.md §5.1 |

---

## 三、关键决策

| 决策点 | 结论 | 理由 |
|---|---|---|
| 域归属 | 新建独立限界上下文 `internal/recon`，对账三层全部收敛于此 | 对账是平台级资金安全机制，不应是 split/payment 的附属；消除 🔴2/🟠4 |
| 差异模型 | **单一模型 + 单一仓储**：`t_reconcile_diff` 只允许 `recon/repository` 一个写入/查询入口；删除 `payment/reconcile.DiffModel` 与 `SaveDiffs` | 根治 🔴1 误删缺陷与双写漂移 |
| 幂等策略 | 统一为「按 `diff_type + merchant_id + biz_date` 清理未核销行 → 重写」，已核销差异保留（沿用 `WriteSplitPost` 现行语义） | 渠道对账现行「按日全清」是缺陷来源，废弃；核销留痕不可被对账重跑覆盖 |
| 比对算法 | 抽 `compare` 纯函数包，仅两种比对器：总额比对（`Totals`）+ 按 key 明细比对（`Rows`）。三层对账的比对全部归约到这两个函数 | 三层比对本质同构；纯函数零依赖、可全覆盖单测；消除 🔴3 |
| 三层对账形态 | 统一为 `engine.Job` 接口的三个实现（precheck / channel / postsplit），Engine 承担通用流程 | 新增对账层（如支付宝渠道对账）只需加一个 Job + 一个 Fetcher，不碰引擎 |
| 取数方式 | 每侧数据定义 `SideFetcher` 端口，适配器由**数据属主域**提供（order 域提供订单聚合、stats 域提供日报、split 域提供账本/执行记录、payment 域提供账单下载） | 终结跨域直查；`scope` 包口径 SQL 随订单侧适配器走，口径一致性约束（V2 评审 🔴2）不变 |
| 副作用 | 写差异、审计、告警、运行日志、metrics 全部经端口注入（`DiffStore` / `AuditRecorder` / `Alerter` / `RunLogger` / `Observer`） | 比对核心无副作用；顺带满足 🟡8 前置对账异常 runlog |
| 调度 | scheduler 只保留 cron 窗口/手动触发，调用 `engine.Run(job, bizDate)`；SQL 与业务判断全部移出；渠道对账改走 framework Runner，消除自有 ticker | 消除 🟠5；三层对账调度风格统一，对账中心任务监测口径一致 |
| 表结构 | **不变**（`t_reconcile_diff` / `t_split_audit` / `t_scheduler_run_log` 均不动） | 纯代码重构，无迁移风险 |
| API 契约 | **不变**：商户端差错中心、管理端差异列表/核销、对账中心全部 HTTP 接口路径与出入参不变，handler 零改动 | 与 split P0–P5 重构同等约束；前端无感 |
| 迁移策略 | P0–P4 分阶段，每步独立可交付、可回滚 | 见 §六 |

---

## 四、目标架构

### 4.1 目录结构

```
internal/recon/
├── domain/                  # 纯领域层：零 DB/框架依赖
│   ├── diff.go              # Diff 实体 + DiffType 常量唯一定义点（7 种类型收敛于此）
│   └── snapshot.go          # Snapshot：一侧对账数据的标准模型（总额 / key→金额 明细）
├── compare/                 # 纯函数比对器（无依赖，全覆盖单测）
│   ├── totals.go            # Totals(local, remote int64) —— Layer A / 总额场景
│   └── rows.go              # Rows(local, remote map[K]int64) []Diff —— Layer B / 订单级 / 渠道逐笔
├── ports/                   # 出站接口，定义在消费侧（Go 隐式实现）
│   └── ports.go             # 见 §五
├── engine/                  # 编排层
│   └── engine.go            # Run(ctx, job, bizDate)：runlog → fetch → compare → persist → audit/alert/metrics
├── job/                     # 三层对账 = 三个 Job 实现
│   ├── precheck/            # 前置对账（Layer A/B，迁自 split/recon；保留 ports.Prechecker 兼容签名）
│   ├── channel/             # 渠道对账（迁自 payment/reconcile 的比对与报告逻辑）
│   └── postsplit/           # 执行后对账（迁自 split/scheduler/reconcile_daily.go）
├── repository/              # t_reconcile_diff 唯一写入/查询入口（迁自 split/repository）
└── query/                   # 差错中心 + 管理端只读查询服务（迁自 split/service/reconcile.go 与 admin SplitManageService 的差异部分）
```

迁移完成后删除：`split/recon/`、`split/scheduler/reconcile_daily.go`、`split/repository/reconcile_diff_repo.go`、`payment/reconcile/wechat_store.go`；`payment/reconcile` 仅保留账单下载与解析（`Downloader` / `parseBill`），作为渠道侧 Fetcher 的实现。

### 4.2 依赖方向

```
                 ┌────────────────────────────────────────┐
  handler 层     │ split handler / admin handler（零改动） │
                 └───────────────┬────────────────────────┘
                                 │ 接口不变
                 ┌───────────────▼────────────┐
  编排层         │ recon/engine  +  recon/query│
                 └───────────────┬────────────┘
                                 │ 依赖 ports（接口）
                 ┌───────────────▼────────────┐
  领域层         │ recon/domain + recon/compare│ ← 纯函数，零依赖
                 └────────────────────────────┘
                                 ▲ Go 隐式实现
        ┌────────────┬───────────┼───────────┬─────────────┐
  适配层 │ order 域    │ stats 域   │ split 域   │ payment 域   │ infra
        │ 订单聚合     │ 日报查询    │ 账本/执行   │ 账单下载      │ 审计/告警/runlog/prom
        └────────────┴───────────┴───────────┴─────────────┘
```

依赖规则：`recon` 只依赖自己的 `ports` 接口与 `infra` 的通用能力（errs/notify）；各业务域实现端口，**recon 不 import 任何业务域包**（现有 `ports.Prechecker` 反向依赖一并修正，🟡7）。

### 4.3 Job 抽象

```go
// engine.Job 一次对账任务的完整定义。
type Job interface {
    Name() string        // 任务名：split_precheck / reconcile_daily / split_daily_reconcile（沿用现名，对账中心 runlog 依赖）
    Fetch(ctx context.Context, req FetchRequest) (local, remote domain.Snapshot, err error)
}

// Engine 通用流程（副作用全部经端口）：
// RunLogged(runlog) → job.Fetch → compare → diffStore.Persist（幂等）
//   → auditRecorder（全量留痕）→ alerter（仅不平）→ observer（metrics）
```

三层对账的比对模式映射：

| Job | 比对器 | local 侧（Fetcher） | remote 侧（Fetcher） |
|---|---|---|---|
| precheck Layer A | `compare.Totals` | OrderSideFetcher（scope.LayerAQuery 口径） | StatsSideFetcher（日报合计） |
| precheck Layer B | `compare.Rows` | OrderSideFetcher（scope.LayerBQuery 口径） | StatsSideFetcher（日报明细） |
| channel | `compare.Rows`（按 transaction_id / order_no 双键匹配） | OrderSideFetcher（当日 PAID 订单） | ChannelBillFetcher（微信账单，payment 域实现） |
| postsplit | `compare.Rows`（按 order_no） | JournalSideFetcher（SPLIT CREDIT 流水聚合） | ExecutionSideFetcher（SUCCESS 执行聚合 + 降级标记） |

> 注：渠道对账的 LONG/SHORT/MISMATCH 三分类是 `compare.Rows` 的差异原因标注（本地独有/远端独有/金额不一致），不需要第三个比对器。降级订单（`SPLIT_DEGRADED`）是 postsplit Job 在 Fetch 阶段标记、Persist 阶段分型落库，不进比对器。

---

## 五、端口定义（草案）

```go
package ports // internal/recon/ports

// DiffStore 差异持久化（唯一写入 t_reconcile_diff 的端口）。
type DiffStore interface {
    // Persist 幂等写入：按 diff_type + merchant_id + biz_date 清理未核销旧差异后重写。
    Persist(ctx context.Context, w DiffWrite) error
    Resolve(ctx context.Context, id, merchantID uint64) (bool, error)      // 商户端核销
    ResolveByID(ctx context.Context, id uint64) (bool, error)             // 管理端核销
}

// AuditRecorder 审计留痕（实现：t_split_audit）。
type AuditRecorder interface {
    Record(ctx context.Context, bizType, bizID, action string, operatorID uint64, detail any)
}

// Alerter 告警（实现：infra/notify，空配置为 Noop）。
type Alerter interface {
    Alert(ctx context.Context, title, body string)
}

// RunLogger 运行日志（实现：framework.RunLogged；前置对账仅异常写入）。
type RunLogger interface {
    Log(ctx context.Context, name string, bizDate time.Time, fn func() (int64, error)) (int64, error)
}

// Observer 指标上报（实现：infra/prom 适配；消除 recon 直 import prom）。
type Observer interface {
    ObserveDiff(job, level string)   // TOTAL / DETAIL / PASS 等标签沿用
}

// —— 取数端口（按数据属主域实现）——
type OrderSideFetcher interface {
    SumForSplit(ctx context.Context, merchantID uint64, start, end time.Time) (int64, error)
    SumByStoreAndDate(ctx context.Context, merchantID uint64, start, end time.Time) (map[string]int64, error)
    ListPaidForChannel(ctx context.Context, start, end time.Time) ([]domain.LocalOrder, error)
}
type StatsSideFetcher interface {
    HasMissing(ctx context.Context, merchantID uint64, start, end time.Time) (bool, error)
    Backfill(ctx context.Context, start, end time.Time) (int64, error)
    Sum(ctx context.Context, merchantID uint64, start, end time.Time) (int64, error)
    Rows(ctx context.Context, merchantID uint64, start, end time.Time) (map[string]int64, error)
}
type JournalSideFetcher interface {
    SumSplitCreditByOrders(ctx context.Context, orderNos []string) (map[string]int64, error)
}
type ExecutionSideFetcher interface {
    SumSuccessByOrders(ctx context.Context, start, end time.Time, merchantID uint64) ([]domain.OrderExecSum, error)
    ListMerchantsWithExecution(ctx context.Context, start, end time.Time) ([]uint64, error)
}
type ChannelBillFetcher interface {
    FetchBill(ctx context.Context, bizDate string) ([]domain.BillEntry, error)  // payment 域 Downloader 实现
}
```

接口按消费侧最小面定义，各域适配器是薄包装（现有 SQL 原样搬入，不改口径）。

---

## 六、分阶段迁移路径

每步独立可交付、可回滚；除 P0 修复缺陷外，各阶段对外行为完全不变。

### P0 · 统一差异模型与仓储（修复 🔴1，最高优先级）

- `t_reconcile_diff` 写入收敛到 `split/repository.ReconcileDiffRepo`（暂不搬目录）：
  - `payment/reconcile.SaveDiffs` 改为调用统一仓储的新方法 `WriteChannelDiffs`（幂等策略：按 `diff_type + biz_date` 清理未核销行）；渠道差异补 `merchant_id` 归属（经 `t_order` 关联，关联不上的 SHORT 单保持 NULL）
  - 删除 `payment/reconcile.DiffModel`；`DiffLong/Short/Mismatch` 常量改为引用 `split/repository` 定义
- 移除 `ReconcileDiffRepo.DB()`；`WriteSplitPrecheck` 入参由 `any` 改为强类型
- **验收**：09:00 渠道对账重跑不再清除 `SPLIT_*` 差异（回归用例覆盖）

### P1 · 纯函数核心 + 端口 + Prechecker 重构

- 新建 `internal/recon/{domain,compare,ports}`；`compare` 包全覆盖单测（0 容差、单侧缺失、空集、key 解析）
- Prechecker 重构为「StatsSideFetcher 补跑 → OrderSide/StatsSide 取数 → compare 纯函数比对 → DiffStore/AuditRecorder/Observer 副作用」；`scope` 包 SQL 移入 OrderSideFetcher/StatsSideFetcher 适配器
- `split/ports.Prechecker` 接口签名保持（返回类型改为 recon/domain 的结果模型，split Service 调用点零改动）；顺带落地前置对账异常 runlog（🟡8）
- **验收**：Prechecker 比对路径单测不依赖 DB；分账阻断行为回归不变

### P2 · 执行后对账迁移 + 调度瘦身

- `reconcile_daily.go` 比对/落库逻辑迁入 `recon/job/postsplit`；3 段 raw SQL 移入 Journal/ExecutionSideFetcher 适配器
- scheduler 只保留 framework 注册与窗口判断，调 `engine.Run`；删除 `ReconcileRunnable` 重复构造
- **验收**：任务名 `split_daily_reconcile`、02:30 窗口、手动触发行为不变；对账中心 runlog 连续

### P3 · 渠道对账迁移

- `wechat.go` 比对逻辑迁入 `recon/job/channel`；`Downloader`/`parseBill` 留在 payment 域作为 ChannelBillFetcher 实现
- 渠道对账调度改走 framework Runner（与 postsplit 一致），废弃自有 ticker
- **验收**：任务名 `reconcile_daily`、09:00 窗口、LONG/SHORT/MISMATCH 产出与告警不变

### P4 · 查询侧收敛

- 差错中心 4 方法（ListExceptions / ListReconcileDiffs / ResolveReconcileDiff / ListAudits）与 admin 差异查询迁入 `recon/query`；split Service 与 admin SplitManageService 改为委托
- **验收**：商户端/管理端全部接口出入参不变

> 排期建议：P0–P1 收益最大风险最低，必做；P2 紧随；**P3 若渠道对账近期有功能迭代可后置**（避免与功能改动互相干扰）；P4 最后。

---

## 七、兼容性与风险

| 项 | 说明 |
|---|---|
| 不变量 | 表结构、HTTP API 路径与出入参、调度任务名与窗口、对账口径（scope SQL）、0 容差语义、核销权限边界（商户自查自核）均不变 |
| 主要风险 | P3 调度方式切换（自有 ticker → framework Runner）需观察一个完整运行周期；缓解：保留手动触发入口，切换次日人工核对 runlog 与差异产出 |
| 回滚策略 | 每阶段独立提交；P0 为缺陷修复不可回滚外，P1–P4 均可按提交整体 revert |
| 性能 | 取数改走端口不改变 SQL 与索引使用；engine 引入的额外开销为每任务一次 runlog 写入，可忽略 |

## 八、验收标准（整体）

1. `compare` 包单测覆盖率 ≥ 95%，全部用例零 DB 依赖
2. 三个 Job 可在测试中以 fake Fetcher + fake DiffStore 端到端驱动（不落库）
3. 回归清单：前置对账阻断/通过路径、02:30 与 09:00 定时任务各跑一个周期、差错中心查询/核销、管理端差异列表/核销、对账中心三个任务 runlog 可查
4. `grep -r "t_reconcile_diff" internal/` 仅命中 `recon/repository` 与迁移文件

## 九、评审结论（2026-08-17 已确认）

| # | 问题 | 结论 |
|---|---|---|
| Q1 | 重构力度 | **A. 完整版（P0–P4，独立 recon 域）** |
| Q2 | P3 时机 | **立即** |
| Q3 | 渠道差异 SHORT 单的 `merchant_id` 归属 | **经 `t_order` 关联回填**，关联不上保持 NULL |
