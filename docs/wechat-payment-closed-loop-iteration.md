# 微信支付闭环迭代方案

> 目标：完成「商户进件 → 码牌生成 → 消费者扫码 → 综合收银台 → 支付 → 流水查询」全链路。
> 范围：消费者扫**商户收款码牌**（B 端出码），消费者在收银台输入金额并完成支付。

---

## 一、工程现状速查（来自调研）

| 关注点 | 当前状态 |
|---|---|
| 商户进件模块（`internal/merchant`） | ✅ 已实现：`t_entity + t_wallet` 双写事务，`POST /v1/admin/merchants`、`GET /v1/admin/merchants`，admin-portal 有完整界面 |
| 码牌 / 收款码牌概念 | ❌ 完全缺失（无表、无字段、无接口、无页面、无文档） |
| 收银台三形态（H5/Embed/Component） | ✅ 全部具备，前后端与微信适配器均支持 NATIVE/H5/JSAPI |
| 收银台入口与码牌联动 | ❌ 收银台只通过 `?order=xxx` 进，不知道消费者从哪块码牌来 |
| 订单金额锁定 | ✅ Precreate 时锁定，回调强校验；但**没有「消费者输入金额」路径** |
| 支付回调 | ✅ 微信支付回调完整（验签 + 幂等 + 入账 + 5xx 重试） |
| 支付主动查询 | ⚠️ 微信适配器层有 `QueryPayment`，但**没有 HTTP 接口暴露**给前端 |
| 退款回调 | ⚠️ 仅验签 + log，未做反向入账（不在本轮范围） |
| 流水表 `t_journal_entry` | ✅ 已有完整复式记账，索引 `idx_biz(biz_type, biz_id)`、`idx_wallet_created` |
| 流水查询 HTTP | ⚠️ `GET /v1/wallets/:entity_id/entries?limit`，**无时间/类型/订单号过滤** |
| 流水查询前端 | ❌ merchant-portal Wallets 页是骨架，没有真实调接口 |
| 通道在途资金户 | ✅ `account/bootstrap/seed.go` 启动期初始化（微信 + 支付宝） |
| 文档/部署/中间件/对账 | ✅ 完整（Viper/CORS/MerchantID 中间件/T+1 对账/Outbox） |

---

## 二、关键决策（已采纳）

| 决策点 | 取值 | 理由 |
|---|---|---|
| 码牌与商户关系 | 一商户可创建多个码牌、独立管理 | 适配多门店/多收银场景 |
| 消费者输入金额范围 | `0.01 ≤ amount ≤ 50000.00` 元（后端强校验） | 防异常输入与合规 |
| 码牌场景通道选择 | 默认走微信 `PayType=NATIVE`，由 router 默认策略兜底 | 满足「微信闭环」目标，留扩展 |
| 主动查询接口语义 | 只读不改状态，仍以回调为准 | 避免与回调并发覆盖订单状态 |

---

## 三、迭代规划：3 个迭代、共 13 个任务

### 迭代 A：码牌基础（P0 — 必须先做）

#### A1. 新建迁移：码牌表 `t_payment_code`

- 文件：`huipay-backend/migrations/0011_create_t_payment_code.up.sql`
- 设计要点：
  - 字段：`id` BIGINT PK、`merchant_id` BIGINT、`code_id` VARCHAR(16)（对外短码，唯一）、`status` TINYINT（1=启用 / 0=停用）、`remark` VARCHAR(64)、`created_at`、`disabled_at`、软删除 `deleted_at`
  - 唯一键：`uk_code_id(code_id)`
  - 索引：`idx_merchant(merchant_id, status, deleted_at)`
  - 外键：建议不强约束（参考 `t_entity` 风格）
- 配套 down 文件：删除 `t_payment_code`

#### A2. 实体 / VO：`PaymentCode` 实体 + `CodeStatus` 枚举

- 文件：`huipay-backend/internal/domain/entity/payment_code.go`
- 文件：`huipay-backend/internal/domain/vo/code.go`
- 设计要点：与 `wallet.go` / `channel.go` 同级，提供 `PaymentCode` 实体与 `CodeStatus`（`Active=1`、`Disabled=0`）

#### A3. 仓储层：`PaymentCodeRepo`

- 文件：`huipay-backend/internal/paymentcode/repository/payment_code_repo.go`
- 设计要点：仿 `entity_repo.go`，提供 `Create` / `GetByCodeID` / `ListByMerchant` / `Disable` 四个方法

#### A4. Service：`PaymentCodeService`

- 文件：`huipay-backend/internal/paymentcode/service/service.go`
- 设计要点：
  - 生成 `code_id`：6 位字母+数字，排除歧义字符（0/O、1/I/L）；冲突时重试最多 5 次
  - 创建绑定当前登录商户（通过中间件注入的 `merchant_id`）
  - 权限校验：仅本商户可停用 / 列表自己的码牌
  - 单元测试：`service_test.go` 覆盖冲突重试、权限校验、停用幂等

#### A5. Handler：`/v1/merchant/codes`

- 文件：`huipay-backend/internal/paymentcode/handler/handler.go`
- 注册：`huipay-backend/cmd/server/main.go` 的 v1 路由组
- 路由：
  - `POST /v1/merchant/codes` — 创建
  - `GET  /v1/merchant/codes` — 列表
  - `POST /v1/merchant/codes/:id/disable` — 停用
- 中间件：复用 `MerchantIDFromHeader` + `MerchantID` 注入当前商户

#### A6. 测试：`service_test.go` + `handler_test.go`

- 文件：`huipay-backend/internal/paymentcode/service/service_test.go`
- 文件：`huipay-backend/internal/paymentcode/handler/handler_test.go`
- 覆盖：冲突重试、跨商户访问拒绝、停用幂等、handler 路由 + 入参校验

---

### 迭代 B：收银台与订单联动（P0 — 闭环核心）

#### B1. 改造 Precreate：支持「码牌建单」

- 文件：`huipay-backend/internal/order/service/order_service.go`
- 文件：`huipay-backend/internal/order/handler/order_handler.go`
- 设计要点：
  - `PrecreateRequest` 新增可选 `Source`（`SOURCE_CODE` / `SOURCE_ORDER`）+ `CodeID`
  - 当 `Source=SOURCE_CODE` 时：`merchant_id` 从 `code_id` 反查（替代 `X-Merchant-Id` 头）；`merchant_order_no` 由后端生成（同 `code_id + 时间戳`）
  - `amount` 由前端传入，校验 `0.01 ≤ amount ≤ 50000.00` 元（即 1 ≤ 分 ≤ 5,000,000）
  - `t_order` 增加列 `code_id VARCHAR(16) NULL`（迁移 `0012_add_t_order_code_id.up.sql`）
  - 复用 `uk_merchant_order` 兜底幂等

#### B2. 收银台入口支持 `?code=xxx`

- 文件：`huipay-web/packages/checkout-sdk/src/pages/h5.tsx`
- 文件：`huipay-web/packages/checkout-sdk/src/pages/embed.tsx`
- 设计要点：
  - URL 优先读 `code`，缺则 `order`
  - `code` 模式下：调用 `POST /v1/checkout/precreate-by-code` 拿 `order_no`，再走原支付流程
  - 新增接口：`POST /v1/checkout/precreate-by-code`，body `{ code_id, amount }`，后端调 `OrderService.Precreate(Source=SOURCE_CODE, CodeID, Amount)` 并返回 `order_no + checkout_url`

#### B3. 新增「金额输入」组件

- 文件：`huipay-web/packages/checkout-sdk/src/components/AmountInput.tsx`
- 设计要点：
  - 仅 `code` 模式下展示
  - 校验最低 0.01 元；调 `usePrecreateByCode(code, amount)`
  - 提交后跳转 `?order=xxx`（与现有流程衔接）

#### B4. 收银台按 `PayType` 调起支付（Native 为主）

- 文件：`huipay-web/packages/checkout-sdk/src/components/Checkout.tsx`
- 设计要点：
  - code 场景下 `pay_type=NATIVE`，前端轮询 `useOrder` 命中 `PAID` 即成功
  - 与现有 `usePay` 流程一致，仅传参变化

#### B5. 暴露主动查询接口

- 文件：`huipay-backend/cmd/server/main.go` 新增 `GET /v1/checkout/:order_no/query`
- 调用：`wechat.Adapter.QueryPayment`
- 设计要点：
  - **只读不改**：不更新订单状态，仍以回调为准
  - 返回 `{ order_no, paid, paid_amount, channel_trade_no, paid_at }`
  - 用作：前端长轮询兜底、后台诊断工具

---

### 迭代 C：交易流水查询（P1 — 收尾闭环）

#### C1. 流水仓储增强：按时间/类型/订单号过滤

- 文件：`huipay-backend/internal/account/repository/wallet_repo.go`
- 文件：`huipay-backend/internal/account/service/service.go`
- 设计要点：新增 `ListByFilter(walletID, bizType, bizID, start, end, limit)`；复用 `idx_biz` 与 `idx_wallet_created`

#### C2. 改造 `/v1/wallets/:entity_id/entries`

- 文件：`huipay-backend/internal/account/handler/handler.go`
- query 参数：`biz_type` / `biz_id` / `start` / `end` / `limit` / `page`
- 保持向后兼容（无过滤参数时维持原行为）

#### C3. merchant-portal Wallets 页面真实化

- 文件：`huipay-monorepo/huipay-web/packages/merchant-portal/src/pages/Wallets/index.tsx`
- 设计要点：
  - 接入 `listEntries`（已有 `services/user.ts` 适配器，补全参数）
  - 展示余额卡 + 流水表（含 `biz_type` / `amount` / `direction` / `created_at` / `biz_id`）
  - 增加按订单号搜索框、按类型筛选、按时间区间

---

## 四、任务清单（待执行确认）

| 迭代 | 任务 | 涉及文件 |
|---|---|---|
| A1 | 码牌迁移 SQL（up/down） | `migrations/0011_create_t_payment_code.up.sql`、`.down.sql` |
| A2 | `PaymentCode` 实体 + `CodeStatus` VO | `internal/domain/entity/payment_code.go`、`internal/domain/vo/code.go` |
| A3 | `PaymentCodeRepo` | `internal/paymentcode/repository/payment_code_repo.go` |
| A4 | `PaymentCodeService` | `internal/paymentcode/service/service.go` |
| A5 | `PaymentCodeHandler` + 路由注册 | `internal/paymentcode/handler/handler.go`、`cmd/server/main.go` |
| A6 | service/handler 测试 | `internal/paymentcode/..._test.go` |
| B1 | Precreate 支持码牌建单 + 迁移 `0012_add_t_order_code_id` | `internal/order/service/order_service.go`、`internal/order/handler/order_handler.go`、`internal/order/model/order_model.go`、`migrations/0012_add_t_order_code_id.up.sql` |
| B2 | 收银台入口支持 `?code=` | `huipay-web/packages/checkout-sdk/src/pages/h5.tsx`、`embed.tsx` |
| B3 | 金额输入组件 | `huipay-web/packages/checkout-sdk/src/components/AmountInput.tsx` |
| B4 | 收银台 Native 调起（沿用） | `huipay-web/packages/checkout-sdk/src/components/Checkout.tsx` |
| B5 | 主动查询 HTTP 接口 | `cmd/server/main.go` |
| C1 | 流水仓储过滤能力 | `internal/account/repository/wallet_repo.go`、`internal/account/service/service.go` |
| C2 | `/v1/wallets/:id/entries` 改造 | `internal/account/handler/handler.go` |
| C3 | merchant-portal Wallets 真实化 | `merchant-portal/src/pages/Wallets/index.tsx`、`merchant-portal/src/services/user.ts` |

---

## 五、不在本轮范围（待后续迭代）

- 退款账务闭环（`HandleWechatRefund` 仅 log）
- 微信真实分账（`Split/ReturnSplit/FinishSplit` 空实现）
- 平台证书自动轮换（`StaticCertProvider`）
- 商户自助进件（当前仅 admin-portal 入口）
- 支付宝通道（骨架 TODO）
- 码牌图片直出（二维码渲染 + 短链跳转）

---

## 六、关键风险与缓解

| 风险 | 缓解 |
|---|---|
| `code_id` 暴力枚举 | 6 位字母数字空间约 32^6 ≈ 10 亿；命中 `merchant_id` 仅暴露「存在性」，但建议加上「枚举失败次数」日志告警 |
| 消费者输入金额被篡改 | 后端校验 `0.01 ≤ amount ≤ 50000.00`，与 `code_id` 反查的商户一致 |
| 回调与主动查询并发 | 主动查询不改状态；CAS `MarkPaid` 兜底 |
| 码牌软删除影响在途订单 | 软删除保留 `code_id`，订单挂历史关联不影响入账 |
| 收银台多入口路由冲突 | 优先 `code` 模式，无 `order` 时才走码牌建单；预留 `&order=` 显式优先级 |