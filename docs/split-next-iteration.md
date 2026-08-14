# 分账功能现状与下一步迭代计划

> 目标：基于现有分账实现（规则引擎 + 执行器 + 分账单审批流 + 执行记录），补齐「支付后自动分账 → 商户自助运营 → 微信资金分账 → 退款回滚与安全」的资金闭环。
> 现状基线：`0e99d8b feat: 实现完整分账功能模块`。

---

## 一、现状速查

| 能力 | 状态 | 说明 |
|---|---|---|
| 分账规则管理 | ✅ 完整 | 后端 CRUD/启停/删除 + 条件（渠道/门店）+ 分配方案（比例/固定/全门店分摊）+ 优先级；前端 SplitRules 页 |
| 规则引擎 | ⚠️ 基础 | 按 `merchant/channel/store_ids` 匹配 + 优先级倒序；`start_at/end_at/tag` 字段已定义但 **Match 未实现**；无多级分账（Level 字段有雏形） |
| 分账执行器 | ✅ 完整 | `t_split_execution` 记录、按 (order_no, receiver) 幂等、通道分账重试（微信未实现时跳过）、内部转账幂等、自动开通门店钱包 |
| 分账服务/接口 | ✅ 完整 | 单笔 execute、按时间段 execute-period、分账单生成/审批/驳回/执行、执行记录/明细查询；全部商户隔离 |
| 分账单审批流 | ✅ 后端完整 / ❌ 前端缺失 | `t_split_bill` 迁移 0020、PENDING→EXECUTED/REJECTED；**前端无分账单页面** |
| 分账记录页 | ✅ 完整 | 前端 Splits 页：按订单聚合列表 + 明细抽屉 |
| 支付回调自动分账 | ❌ 缺失 | 回调只入账商户钱包，不触发分账；`t_order.split_status` 创建后一直是 PENDING，从未更新 |
| 微信真实分账 | ❌ 缺失 | `Adapter.Split/ReturnSplit/FinishSplit` 空实现；无通道时仅本地记账，微信侧资金不会分出 |
| 退款回滚 | ❌ 缺失 | 退款回调仅验签+log，不做反向入账/分账回退 |
| 对账 | ⚠️ 仅支付侧 | T+1 对账只覆盖支付通道，分账对账/差异处理缺失 |
| 风控/可观测 | ⚠️ 基础 | 仅 `SplitSuccessRate` 指标；无分账金额/失败原因分布、无频率与上限风控 |

## 二、关键缺口（按闭环优先级）

1. **分账没有"触发源"**：支付成功事件没有接到分账链路，一切分账都靠手动调 API，前端也没有入口调用单笔 execute。
2. **前端运营页面缺一半**：分账单（生成/审批/驳回/执行）与按时间段分账没有页面，API 已全部就绪。
3. **规则引擎能力与校验不足**：时间/标签条件未生效、无多级分账、后端不校验 store_ids 归属与比例合计、单笔 execute 不预校验余额（可能部分成功）。
4. **微信资金层未打通**：分账只发生在内部账本，微信侧资金未真正分出；接收方管理、分账回退/完结、状态同步均缺失。
5. **退款与资金安全未闭环**：已分账订单退款会造成资金不平；无分账对账与风控阈值。

---

## 三、迭代计划（5 个迭代，共 18 个任务）

### 迭代 1：支付成功自动分账（闭环核心）

> 目标：支付成功 → 自动匹配规则 → 自动分账，订单 `split_status` 全链路联动，失败可重试。

#### 1.1 支付回调后触发分账
- 文件：`internal/payment/notify/handler.go`、`internal/outbox/`、`cmd/server/main.go`
- 设计：回调入账成功后，写入「订单已支付」outbox 事件（复用 `t_outbox_event` + worker）；新增 `SplitCoordinator` 消费事件：查订单（merchant/store/channel/paid_at）→ 规则引擎匹配 → 计算分配 → executor 执行。
- 无命中规则 / 未配置门店等场景：记录 `SPLIT_SKIPPED` 日志与指标，不视为失败。

#### 1.2 订单 split_status 状态机
- 文件：`internal/order/model/order_model.go`、`internal/split/`
- 设计：`PENDING → PROCESSING → SUCCESS / PARTIAL / FAILED`；分账完成后回写 `t_order.split_status`（条件更新防并发）；`REFUNDED` 场景留迭代 5。

#### 1.3 失败重试调度
- 文件：`internal/split/scheduler/retry.go`（新建）、`cmd/server/main.go`
- 设计：扫描 FAILED/PARTIAL 执行记录（未达最大重试次数）→ 重跑缺失接收方；沿用现有 scheduler 模式（30s tick + 幂等）。

#### 1.4 单笔 execute 强校验
- 文件：`internal/split/service/service.go`
- 设计：执行前校验商户钱包余额 ≥ 分账总额（避免部分成功）；校验订单归属、订单状态（仅 PAID 可分）；校验规则 store_ids 属于该商户。

#### 1.5 测试
- 覆盖：回调→自动分账端到端、幂等（重复回调/重复执行不重复入账）、余额不足失败、无规则跳过、重试收敛。

---

### 迭代 2：商户端分账运营页面

> 目标：商户自助完成「生成账单 → 审批 → 执行」与按时间段分账，记录页可运营。

#### 2.1 分账单页面（Bills）
- 文件：`huipay-web/packages/merchant-portal/src/pages/SplitBills/index.tsx`（新建）、`routes.ts`、`App.tsx`、`BasicLayout.tsx`
- 设计：列表（批次号/规则/时间段/总额/状态）+ 生成账单（选规则 + 时间段 + 预览明细）+ 详情抽屉 + 审批/驳回按钮（仅 PENDING）；状态徽章：待审批/已执行/已驳回。

#### 2.2 按时间段分账入口
- 文件：`huipay-web/packages/merchant-portal/src/pages/SplitBills/index.tsx`
- 设计：账单页内提供「立即按时间段分账」动作（调 `executeSplitByPeriod`），或与生成账单二选一引导（推荐走审批流）。

#### 2.3 分账记录页增强
- 文件：`huipay-web/packages/merchant-portal/src/pages/Splits/index.tsx`
- 设计：按状态/时间段筛选；失败/部分失败支持「重试」按钮（调后端重试）；行内展示命中规则；与交易详情互相跳转。

#### 2.4 规则试算（可选）
- 文件：`huipay-backend/internal/split/service/service.go`、`handler.go`、`SplitRules/index.tsx`
- 设计：`POST /v1/merchant/split/rules/:id/preview`（入参金额/门店/渠道，返回分配明细不落库）；前端「试算」按钮弹窗展示。

---

### 迭代 3：规则引擎与校验完善

> 目标：规则能力符合时间/标签/多级分账场景，后端强校验，避免脏数据。

#### 3.1 规则创建/更新后端校验
- 文件：`internal/split/service/service.go`
- 设计：`store_ids` 归属当前商户校验（查 `t_store`）；分配比例合计 ≤ 100%（>100 拒绝）；接收方类型白名单（STORE/MERCHANT）；单条固定金额 ≥ 0.01 元；规则编码格式校验。

#### 3.2 条件与生效时间落地
- 文件：`internal/split/rule/engine.go`、`internal/split/repository/split_rule_repo.go`
- 设计：`Match` 实现 `StartAt/EndAt`（订单支付时间）、`Tag`（订单标签，预留）；`effective_from/to` 接入 `ListByMerchant` 过滤与前端表单（生效起止日期）。

#### 3.3 多级分账
- 文件：`internal/split/rule/engine.go`、`internal/split/service/service.go`、`executor.go`
- 设计：分配项支持 `level`（1..N），执行时按 level 顺序：上级分配从源钱包出、下级分配从上级接收方钱包出（链式）；执行记录 `level` 已具备。

#### 3.4 规则复制与命中统计
- 文件：`internal/split/`、`SplitRules/index.tsx`
- 设计：规则复制（新 code + 同配置）；规则命中次数/金额统计（读 `t_split_execution` 聚合），列表展示。

---

### 迭代 4：微信真实分账（资金层闭环）

> 目标：分账从内部账本延伸到微信侧真实资金，接收方可管理、可回退、可完结。

#### 4.1 微信分账 API 实现
- 文件：`internal/payment/channel/wechat/`（`client.go`、`wechat_adapter.go`）
- 设计：实现 `/v3/profitsharing/orders`（请求分账）、`/v3/profitsharing/orders/{transaction_id}`（查询）、`/v3/profitsharing/return`（回退）、`/v3/profitsharing/orders/{...}/finish`（完结）；商户级配置按 `merchant_id` 取适配器（复用 `Manager`）。

#### 4.2 分账接收方管理
- 文件：`internal/split/receiver/`（新建）、迁移新增 `t_split_receiver`
- 设计：接收方（门店）绑定微信 OpenAPI 商户号/APPID/姓名/关系；新增/解绑接口；执行分账前校验接收方已绑定。

#### 4.3 分账状态同步
- 文件：`internal/split/service/service.go`、`notify/`（分账回调）
- 设计：请求分账后记录 `PROCESSING`；微信回调/主动查询 → 更新 `channel_split_no` 与执行状态；回调验签复用商户证书。

#### 4.4 本地账本与微信侧一致性校验
- 文件：`internal/payment/reconcile/`
- 设计：对账时校验「本地 SPLIT 流水合计 == 微信分账成功金额」；差异写入 `t_reconcile_diff`（扩展场景类型）。

#### 4.5 分账对账（T+1）
- 文件：`internal/payment/reconcile/scheduler/reconcile_daily.go`
- 设计：对账单下载扩展分账账单；差异告警与人工处理入口（管理端）。

---

### 迭代 5：退款回滚与资金安全

> 目标：已分账订单退款可回滚，资金封闭不出错，异常可观测。

#### 5.1 退款触发分账回退
- 文件：`internal/payment/notify/handler.go`、`internal/split/service/service.go`
- 设计：退款成功回调 → 若订单已分账：先微信回退（迭代 4 后）再本地反向入账（门店钱包 → 商户钱包，`SPLIT_RETURN` 流水）；未接微信时仅本地回退并告警。

#### 5.2 回退幂等与失败重试
- 设计：按 (order_no, receiver, refund_no) 幂等；回退失败进重试队列；金额校验：累计回退 ≤ 已分金额。

#### 5.3 风控与可观测
- 文件：`infra/prom/prom.go`、`internal/split/`
- 设计：指标：分账金额/笔数、失败原因分布（余额不足/通道失败/规则缺失）、自动分账延迟；风控：单笔分账上限、接收方数量上限（如 ≤50）、单日自动分账次数阈值；告警接入日志聚合。

---

## 四、依赖与顺序

- 迭代 1 依赖现有规则引擎/执行器（已完成），是其余迭代前置（自动分账落地后才能验证真实链路）。
- 迭代 2 与迭代 1 可并行（前端页面基于现有 API）。
- 迭代 3 建议在迭代 4 前完成（接收方绑定依赖规则校验正确）。
- 迭代 4 依赖商户微信配置生效（已完成）+ 真实微信商户号验证。
- 迭代 5 依赖迭代 4（微信回退）与退款账务闭环。

## 五、风险与缓解

| 风险 | 缓解 |
|---|---|
| 微信分账真实资金操作，验收依赖真实商户号 | 先以 mock/沙箱适配器联调本地链路，真实验证放 UAT |
| 自动分账引入资金自动流出 | 默认开启前可配「自动分账开关 + 账单审批兜底」；余额不足/异常自动降级为账单待审批 |
| 多级分账链式转账复杂度高 | 先单级落地（迭代 1），多级（3.3）单独评审后实施 |
| 退款回滚与微信回退并发 | 全部走幂等键 + 状态机，禁止反向重复入账 |

## 六、验收标准（迭代 1–2 先行）

1. 支付成功回调后，命中规则的订单自动生成分账执行记录，`split_status` 正确流转到 SUCCESS。
2. 重复回调/重复执行不重复入账（复式流水数量正确）。
3. 余额不足时自动分账失败且不产生部分入账，订单标记待处理可重试。
4. 商户端可生成分账单、查看明细、审批/驳回、查看执行结果；分账记录页可筛选与重试。
5. 规则创建时后端拒绝越权门店与超 100% 分配。
