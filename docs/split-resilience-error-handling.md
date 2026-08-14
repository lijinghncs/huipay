# 分账差错处理与容错机制 · 优化方案

> 目标：围绕「幂等与一致性、补偿与重试、降级与闸门、对账与审计、可观测与告警、回滚」六层，补齐分账链路的容错能力，杜绝资金重复分、悬挂分、静默不一致。
> 前置：分账执行器/规则/账单审批已实现；本方案基于 `0e99d8b` 现状。

---

## 一、现状容错能力评估

| 环节 | 已有机制 | 薄弱点 |
|---|---|---|
| 本地转账 | ✅ `ledger.Transfer` 复式记账 + `BizType:BizID:from:to` 幂等键 | 无 |
| 执行记录 | ✅ `t_split_execution` 按 (order_no, receiver) upsert，重入跳过已成功 | 无订单级状态记录；无悬挂检测 |
| 通道调用 | ✅ 有限重试（3 次） | **无通道幂等单号**：通道成功后本地失败，重试会再次调通道 → 微信侧重复分账风险 |
| 失败处理 | ⚠️ 单接收方失败即中止，记录 FAILED | **无后台补偿**：半途失败后靠人工/未来手动重试，无自动重入 |
| 通道未配置 | ⚠️ `splitWithRetry` 无适配器时**静默跳过**，仅本地入账 | 资金「本地记了、微信没分」，无人感知 |
| 余额校验 | ⚠️ 仅 period/bill 校验 | 单笔 execute 无预校验 → 可能部分成功 |
| 账单审批 | ⚠️ 直接 `Updates` | **无乐观锁**：并发审批可能双执行（依赖执行器幂等兜底，但有窗口） |
| 规则一致性 | ⚠️ 执行时读当前规则 | 规则被改后重试结果可能与首次不一致（无快照） |
| 对账 | ❌ 仅支付侧 | 分账无对账、无差异处理 |
| 审计 | ❌ 无 | 审批/重试/回退无操作记录 |
| 可观测 | ⚠️ 仅 `SplitSuccessRate` | 无金额/失败原因/悬挂/降级指标与告警 |
| 回滚 | ❌ 无 | 已分账订单退款资金不平 |

---

## 二、问题清单与风险分级

| 风险 | 级别 | 场景 |
|---|---|---|
| 通道重复分账 | 🔴 严重 | 通道分账成功 → 本地转账/落库失败 → 重试再次调通道（无幂等单号） |
| 半途失败无补偿 | 🔴 严重 | 多接收方分账中第 3 家失败，第 1–2 家已分，无后台补齐与告警 |
| 进程中断悬挂 | 🟠 高 | executor 执行中进程崩溃，记录停留 PROCESSING/部分 SUCCESS |
| 通道未配置静默降级 | 🟠 高 | 本地入账但微信未分，T+1 前无人察觉 |
| 单笔 execute 部分成功 | 🟠 高 | 余额不足时前面接收方已转、后面失败 |
| 并发审批双执行 | 🟠 高 | 两个请求同时 approve，均通过校验 |
| 规则变更导致重试不一致 | 🟡 中 | 分账失败后规则被改，重试按新规则执行 |
| 无对账/审计/告警 | 🟡 中 | 差错发现依赖人工，无留痕 |

---

## 三、优化设计（六层）

### A 层 · 幂等与一致性（防重复）

**A1. 通道分账幂等单号**
- `t_split_execution` 增加 `channel_req_no VARCHAR(64)`：生成规则 `SP{order_no 后 12}{receiver_id 补零}`（确定性）。
- 执行前按 (order_no, receiver) 读取/分配并随记录持久化；重试**复用同一 channel_req_no** 调通道（微信按 `out_order_no` 幂等），杜绝通道侧重复分账。
- 文件：`internal/split/executor/executor.go`、迁移 0021。

**A2. 订单级分账状态表（新增 `t_split_order_status`）**
- 字段：`order_no`(PK)、`merchant_id`、`rule_id`、`rule_snapshot JSON`（规则快照，A4）、`total_amount`、`receiver_count`、`success_count`、`status`（PENDING/PROCESSING/SUCCESS/PARTIAL/FAILED/DEAD）、`attempt_count`、`next_retry_at`、`degraded TINYINT`、`last_error`、时间戳。
- 作用：悬挂检测、重试调度、列表/明细查询、状态回写 `t_order.split_status` 都基于它。
- 文件：迁移 0021、`internal/split/executor/`、`internal/split/repository/`。

**A3. 账单审批乐观锁**
- `ApproveBill` / `RejectBill` 改为 `UPDATE ... WHERE id=? AND status='PENDING'`，`RowsAffected==0` 返回「账单已被处理」。
- 文件：`internal/split/repository/bill_repo.go`、`internal/split/service/service.go`。

**A4. 规则快照**
- 执行/生成账单时，将 `matched.conditions + allocations` 序列化存入 `t_split_order_status.rule_snapshot`（账单则存 `t_split_bill.detail`，已有）。
- 重试/补偿一律按快照重建分配，保证结果一致。

### B 层 · 补偿与重试（防悬挂、防半途）

**B1. 补偿调度器（新建 `internal/split/scheduler/compensate.go`）**
- 30s tick，扫描 `t_split_order_status`：
  - `status IN (FAILED, PARTIAL) AND attempt_count < 5 AND next_retry_at <= now` → 重入
  - 重入逻辑：按 `rule_snapshot` + 未 SUCCESS 接收方重建分配 → executor 重跑（幂等跳过已成功）
  - 指数退避：30s → 1m → 2m → 4m → 8m（封顶）；达 5 次置 `DEAD` 并告警
- 启动注册：`cmd/server/main.go`。

**B2. 悬挂检测**
- 扫描 `status='PROCESSING' AND updated_at < now-10min` → 置 `SUSPENDED`（并入 B1 重入范围）。
- 重入前按执行记录 `hasSuccess` 补齐，已成功接收方不动。

**B3. 手动重试接口**
- `POST /v1/merchant/split/executions/:order_no/retry`：校验商户归属 → 复用 B1 重入逻辑 → 返回「重试成功 N / 失败 M」。
- 文件：`internal/split/handler/handler.go`、`service/service.go`（与迭代 2 的 B3 合并）。

### C 层 · 降级与闸门（防静默不一致、防部分成功）

**C1. 通道降级显式化**
- executor 支持分账模式（配置/商户级）：
  - `AUTO`（默认）：有通道走通道；无通道降级本地入账并标记 `degraded=1` + 告警
  - `LOCAL_ONLY`：仅本地记账（标记 degraded）
  - `CHANNEL_REQUIRED`：通道不可用即 FAILED，**不**本地入账
- 对账时 `degraded=1` 的订单强制纳入差异清单（D1）。

**C2. 余额预校验**
- 单笔 execute、period、bill approve、补偿重入前：校验商户钱包余额 ≥ 待分金额（未成功接收方金额和）。
- 避免「前面转出、后面失败」的部分成功；校验后余额不足直接失败并进入重试队列。

**C3. 自动分账熔断**
- 自动分账连续失败 ≥ N 次（默认 3）→ 自动将自动分账开关置 OFF，后续订单转入「账单待审批」人工处理；触发时告警。
- 配置存 DB（`t_system_config` 或复用 env），管理端可查看/复位。

### D 层 · 对账与审计（防差错滞留）

**D1. 分账对账（T+1）**
- 对账任务比较三处金额（按订单聚合）：
  1. `t_journal_entry` 中 `biz_type='SPLIT'` 的入账合计
  2. `t_split_order_status` SUCCESS 金额合计
  3. 通道侧分账成功金额（迭代 4 接入微信账单后启用）
- 不一致 → 写入 `t_reconcile_diff`（扩展 `diff_type='SPLIT'`），订单挂起 + 告警；管理端人工核销。
- 文件：`internal/payment/reconcile/`、`internal/split/repository/`。

**D2. 审计日志（新增 `t_split_audit`）**
- 记录：账单审批/驳回、手动重试、人工补偿、回退、规则变更快照。
- 字段：`id/biz_type/biz_id/action/operator_type/operator_id/detail JSON/created_at`。
- 查询接口：`GET /v1/merchant/split/audit?biz_type=&biz_id=`（管理端可查全量）。

### E 层 · 可观测与告警

- 新增 Prometheus 指标（`infra/prom/prom.go`）：
  - `split_amount_total`（分账金额）
  - `split_order_total{status}`（SUCCESS/PARTIAL/FAILED/DEAD）
  - `split_failure_reason_total{reason}`（insufficient_balance / channel_fail / transfer_fail / rule_missing / degraded）
  - `split_hanging_total`（悬挂数）、`split_retry_total`、`split_auto_disabled_total`
- 告警规则（Grafana/日志聚合）：连续失败、悬挂 > 0、降级订单、自动分账熔断触发。
- 保留 `SplitSuccessRate`（改为订单级成功率 gauge，定时上报）。

### F 层 · 回滚（退款联动）

- 退款成功回调 → 若订单已分账：
  1. 微信侧回退（迭代 4 后）；未接通道则本地回退并标记 degraded
  2. 本地反向入账（门店钱包 → 商户钱包，`biz_type='SPLIT_RETURN'`）
- 幂等：新增 `t_split_return(order_no, receiver_entity_id, refund_no, amount, status, channel_return_no)`，`uk(order_no, receiver, refund_no)`；CAS 防重。
- 回退失败 → 进 B1 补偿队列；累计回退 ≤ 已分金额校验。

---

## 四、表结构变更（迁移 0021）

```sql
-- t_split_order_status：订单级分账状态（悬挂检测/重试调度/列表查询）
CREATE TABLE t_split_order_status (
  order_no        CHAR(32)     NOT NULL,
  merchant_id     BIGINT UNSIGNED NOT NULL,
  rule_id         BIGINT UNSIGNED NULL,
  rule_snapshot   JSON         NULL COMMENT '执行时规则快照',
  total_amount    BIGINT       NOT NULL,
  receiver_count  INT          NOT NULL DEFAULT 0,
  success_count   INT          NOT NULL DEFAULT 0,
  status          VARCHAR(16)  NOT NULL DEFAULT 'PENDING',
  attempt_count   INT          NOT NULL DEFAULT 0,
  next_retry_at   DATETIME(3)  NULL,
  degraded        TINYINT      NOT NULL DEFAULT 0,
  last_error      VARCHAR(512) NULL,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (order_no),
  KEY idx_status_retry (status, next_retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分账订单级状态';

-- t_split_execution 增列
ALTER TABLE t_split_execution
  ADD COLUMN channel_req_no VARCHAR(64) NULL COMMENT '通道分账幂等单号' AFTER order_no,
  ADD COLUMN degraded      TINYINT     NOT NULL DEFAULT 0 COMMENT '降级模式(仅本地入账)',
  ADD KEY idx_channel_req (channel_req_no);

-- t_split_audit：分账审计
CREATE TABLE t_split_audit (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  biz_type      VARCHAR(32)     NOT NULL COMMENT 'BILL/EXECUTION/RETURN',
  biz_id        VARCHAR(64)     NOT NULL,
  action        VARCHAR(32)     NOT NULL COMMENT 'APPROVE/REJECT/RETRY/RETURN/MANUAL',
  operator_type VARCHAR(16)     NOT NULL COMMENT 'MERCHANT/ADMIN/SYSTEM',
  operator_id   BIGINT UNSIGNED NOT NULL DEFAULT 0,
  detail        JSON            NULL,
  created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_biz (biz_type, biz_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分账审计日志';

-- t_split_return：分账回退（迭代 5 启用）
CREATE TABLE t_split_return (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  order_no          CHAR(32)     NOT NULL,
  receiver_entity_id BIGINT UNSIGNED NOT NULL,
  refund_no         CHAR(64)     NOT NULL,
  amount            BIGINT       NOT NULL,
  status            VARCHAR(16)  NOT NULL DEFAULT 'PENDING',
  channel_return_no VARCHAR(64)  NULL,
  last_error        VARCHAR(512) NULL,
  created_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_order_receiver_refund (order_no, receiver_entity_id, refund_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分账回退记录';
```

---

## 五、接口与组件变更

| 变更 | 说明 |
|---|---|
| `POST /v1/merchant/split/executions/:order_no/retry` | 手动重试（B3） |
| `GET /v1/merchant/split/audit` | 审计查询（D2，商户隔离；管理端可全量） |
| `POST /v1/admin/split/reconcile/run`（可选） | 管理端触发分账对账（D1） |
| 补偿调度器 | `internal/split/scheduler/compensate.go`（B1/B2），`main.go` 注册 |
| 执行器改造 | 通道幂等单号（A1）、快照（A4）、余额预校验（C2）、degraded 标记（C1） |
| 账单审批 | 乐观锁（A3） |
| 指标 | `infra/prom/prom.go`（E） |

## 六、涉及文件

- 迁移：`infra/migrator/migrations/0021_*.sql`（新建）
- 后端：`internal/split/executor/executor.go`、`internal/split/service/service.go`、`internal/split/handler/handler.go`、`internal/split/repository/`（order_status/audit/return repo）、`internal/split/scheduler/compensate.go`（新建）、`internal/payment/reconcile/`、`infra/prom/prom.go`、`cmd/server/main.go`
- 前端（可选最小化）：Splits 页重试按钮已含于迭代 2；审计/对账为管理端后续

## 七、验收标准（故障注入）

1. **通道重复防护**：模拟「通道成功 → 本地失败 → 重试」，微信侧只产生一次分账（channel_req_no 复用）。
2. **半途恢复**：3 接收方分账中第 2 家失败，补偿调度自动补齐第 2/3 家，最终 SUCCESS；重复调度不重复入账。
3. **悬挂恢复**：执行中进程中断，10 分钟后自动置 SUSPENDED 并重入补齐。
4. **并发审批**：同一账单两个 approve 并发，仅一个成功，资金只分一次。
5. **余额不足**：单笔 execute 余额不足时整体失败（无部分成功），订单进入重试队列，充值后自动/手动重试成功。
6. **规则变更一致性**：分账失败后修改规则，重试按快照执行，结果与首次一致。
7. **通道未配置**：degraded=1 且 T+1 对账产生差异记录，可人工核销。
8. **审计留痕**：审批/驳回/重试/回退均有 `t_split_audit` 记录。
