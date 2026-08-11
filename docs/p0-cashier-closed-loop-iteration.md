# P0 基础收银闭环迭代计划

> 目标：把「商户管理 → 微信支付参数配置 → 商户收款码生成 → 消费者扫码付款 → 交易结果查询」全链路补齐到可上线闭环。
> 前置：`wechat-payment-closed-loop-iteration.md` 的迭代 A/B/C 与商户微信配置阶段一（`merchant-wechat-config.md`）已完成；本计划只补剩余断点。

---

## 一、闭环现状速查（断点分析）

| 环节 | 当前状态 | 闭环断点 |
|---|---|---|
| 商户管理 | admin-portal 进件/列表/详情/编辑/状态/概览完整；merchant-portal 自助资料/概览已接真实接口 | 生产鉴权仍是 `X-Merchant-Id` 明文头（开发信任，生产需登录态） |
| 微信支付参数配置 | 阶段一完成：存储 + AES 加密 + API + 管理端界面 | **阶段二未做：支付链路仍是平台级单例 `wxAdapter`，商户配置只存不生效** |
| 商户收款码 | 后端 API 完整（创建/列表/停用、6 位短码、金额范围校验） | **无商户端管理页面；无二维码图片生成/下载；无码牌落地页短链** |
| 消费者扫码付款 | `?code=` 入口 → 金额输入 → `precreate-by-code` → Pay 已通（NATIVE/H5/JSAPI） | **NATIVE 返回的 `qr_code` 未在前端渲染；无支付成功/失败/超时结果页** |
| 交易结果查询 | `GET /v1/checkout/:order_no/query` 已存在；merchant-portal 交易列表页为骨架 | **前端轮询只查本地订单状态，回调延迟时订单会卡住；列表无筛选/无详情** |

---

## 二、迭代规划（5 个迭代，共 17 个任务）

### 实施状态

| 迭代 | 状态 | 落地摘要 |
|---|---|---|
| 1 商户微信配置生效 | ✅ 已完成 | `wechat.Manager`（TTL 缓存 + 平台兜底）、`NewForMerchant` 回调路径带 `:merchant_id`、订单/查单/关单按商户解析、`POST /v1/notify/wechat/:merchant_id` 分流；含 manager/回调分流/配置解密测试 |
| 2 商户收款码闭环 | ✅ 已完成 | 码牌视图补 `checkout_url`；merchant-portal 新增「收款码」页（建码/停用/查看二维码/下载 PNG/复制链接）；落地页复用 `h5?code=` |
| 3 收银台体验补全 | ✅ 已完成 | 修复 `OrderModel` 缺 JSON tag（字段驼峰/下划线不匹配的隐藏 bug）；Checkout 渲染 NATIVE 二维码 + 倒计时 + 失败提示；轮询叠加 `/query` 通道兜底；H5 成功/关闭/失败结果页；码牌场景按端默认 JSAPI/H5 |
| 4 交易结果查询闭环 | ✅ 已完成 | 交易列表支持状态/通道/码牌/时间筛选 + 分页 + 汇总；订单详情抽屉 + 「查询通道状态」；钱包流水点击订单号跳转详情 |
| 5 收银闭环加固 | ✅ 已完成 | 最小商户登录：迁移 0014（login_phone + bcrypt 密码）、`POST /v1/auth/merchant/login`、HMAC token、Bearer 中间件（生产可关明文头）、登录页 + 路由守卫 + 管理端设置登录密码；码牌防枚举埋点；UAT 验收清单 |

### 迭代 1：商户微信支付配置生效（支付链路按商户分流）— 核心断点

> 目标：商户配置的微信参数真正参与下单/查单/回调，未配置商户回退平台通道。

#### 1.1 新增 `WechatClientManager`（按商户懒加载/缓存/失效）

- 文件：`huipay-backend/internal/payment/channel/wechat/manager.go`（新建）
- 设计要点：
  - 按 `merchant_id` 懒加载微信适配器：读取 `t_entity.wechat_config`（AES 解密敏感字段），组装 `config.WeChatConfig`，复用现有 `wechat.New(cfg)` 构造。
  - 进程内缓存（`sync.Map` + TTL 30 秒），配置更新后最迟 30 秒生效；`Manager.Invalidate` 提供主动失效入口。
  - 平台级配置作为默认兜底：商户未启用微信配置时返回平台适配器（与现状一致，保证回归兼容）。
  - provider 查询失败 / 配置损坏 / 适配器构造失败：仅告警并回退平台，不阻断下单。
  - `Manager` 同时实现 `GetAdapter(merchantID, channel)` 与 `Get(merchantID)` 两个入口，分别供订单服务与回调处理器使用。

#### 1.2 路由与订单服务按商户取通道

- 文件：`huipay-backend/internal/payment/router/router.go`、`internal/order/service/order_service.go`
- 设计要点：
  - 路由决策保持平台级默认策略不变；订单服务在拿到通道编码后，按订单 `merchant_id` 通过 Manager 解析商户适配器（未配置商户回退平台）。
  - `Pay` / `QueryPayment` / 超时关单（`close_expired.go`）按订单 `merchant_id` 获取适配器。
  - 商户级适配器的回调路径为 `/v1/notify/wechat/{merchant_id}`（`wechat.NewForMerchant`），下单时自动注入，供微信回调按商户分流。

#### 1.3 支付回调按商户分流

- 文件：`huipay-backend/internal/payment/notify/handler.go`、`cmd/server/main.go`
- 设计要点：
  - 新增路由 `POST /v1/notify/wechat/:merchant_id`：路径参数定位商户 → Manager 取商户适配器 → 用商户配置（平台证书 + `api_v3_key`）验签/解密。
  - 原 `POST /v1/notify/wechat` 保留为平台级兜底（未配置商户/兼容路径）。
  - 金额校验、幂等入账逻辑不变；验签失败仍返回 4xx 让微信重试。

#### 1.4 测试

- 文件：`internal/payment/channel/wechat/manager_test.go`、`internal/payment/notify/`（新增测试）
- 覆盖：manager 缓存命中/TTL 重取/Invalidate/构造失败回退；`GetRuntimeConfig` 敏感字段解密与未配置返回 nil；回调按 `:merchant_id` 分流（平台适配器不被误用）；未配置商户走平台通道；既有金额校验与幂等回归。

---

### 迭代 2：商户收款码完整闭环（页面 + 二维码 + 落地页）

> 目标：商户能在工作台自助建码、下载打印；消费者扫码直达收银台。

#### 2.1 后端：码牌视图补 `checkout_url`

- 文件：`huipay-backend/internal/paymentcode/handler/handler.go`、`internal/paymentcode/service/service.go`
- 设计要点：
  - 码牌视图补充 `checkout_url`（`https://checkout.huipay.cn/h5?code={code_id}`，扫码直达金额输入页）。
  - 二维码渲染/下载由前端完成（antd `QRCode` + canvas 导出 PNG），后端不生成图片，避免引入图片库。
  - 停用/启用语义保持：停用后扫码提示"该收款码已停用"。

#### 2.2 码牌落地页（复用 H5 码牌入口）

- 文件：`huipay-web/packages/checkout-sdk/src/pages/h5.tsx`（已支持 `?code=`）
- 设计要点：`checkout_url` 直接指向现有 `h5?code={code_id}` 金额输入入口，无需新增页面；部署层可用 `/c/{code_id}` 短链重写到该入口（nginx 配置项，代码零改动）。
- 补充：无效/停用码牌由 `precreate-by-code` 返回明确错误，H5 页展示"该收款码不存在/已停用"。

#### 2.3 merchant-portal 新增「收款码」页面

- 文件：`huipay-web/packages/merchant-portal/src/pages/Codes/index.tsx`（新建）、`config/routes.ts`、`src/App.tsx`、`src/layouts/BasicLayout.tsx`、`src/services/user.ts`
- 设计要点：
  - 码牌列表：短码/备注/状态/创建时间/使用状态；创建（备注）与停用/启用。
  - 每行支持「查看二维码」（antd `QRCode`）「下载 PNG」（canvas 导出）「复制收款链接」。
  - 对接 `POST/GET /v1/merchant/codes`、`POST /v1/merchant/codes/:id/disable`。

#### 2.4 验收

- 商户建码 → 展示/下载二维码 → 扫码 → 进入金额输入 → 建单成功；停用后扫码给出停用提示。

---

### 迭代 3：收银台体验补全（NATIVE 二维码 + 结果页 + 轮询兜底）

> 目标：消费者在任意端（组件/H5）完成支付并看到明确结果。

#### 3.1 Checkout 组件渲染 NATIVE 二维码

- 文件：`huipay-web/packages/checkout-sdk/src/components/Checkout.tsx`
- 设计要点：`Pay` 返回 `qr_code` 时渲染二维码（调用方决定展示方式，组件内置 fallback 弹层）；修复当前 `qr_code` 被忽略的断点；H5 页选择 NATIVE 时同样可扫码支付。

#### 3.2 支付结果页与倒计时

- 文件：`huipay-web/packages/checkout-sdk/src/components/PaymentResult.tsx`（新建）、`src/pages/h5.tsx`、`src/pages/embed.tsx`
- 设计要点：成功页展示金额/单号/时间；失败页展示原因与重试；订单超时（`expire_at`）展示"已超时"并回到码牌重新建单；倒计时提示剩余时间。

#### 3.3 状态轮询叠加通道查询兜底

- 文件：`huipay-web/packages/checkout-sdk/src/hooks/useCheckout.ts`
- 设计要点：本地轮询 `GET /v1/checkout/:order_no` 之外，间隔调 `GET /v1/checkout/:order_no/query` 兜底；通道侧已支付而本地未收到回调时提示"支付处理中"并继续等待，避免用户重复支付。

#### 3.4 码牌场景默认体验优化

- 文件：`huipay-web/packages/checkout-sdk/src/pages/h5.tsx`
- 设计要点：码牌模式微信内默认 JSAPI、非微信内默认 H5/NATIVE，隐藏不适用场景选项；仍保留手动切换能力。

---

### 迭代 4：交易结果查询与商户后台闭环

> 目标：商户能查询、筛选、定位每一笔扫码收款的结果。

#### 4.1 交易列表增强

- 文件：`huipay-web/packages/merchant-portal/src/pages/Transactions/index.tsx`、`src/services/user.ts`；后端 `internal/order/handler/order_handler.go`、`internal/order/repository/order_repo.go`
- 设计要点：状态/时间/码牌/通道筛选、正确的分页（当前前端硬编码 page=1）、金额汇总；后端 `List` 补充过滤参数。

#### 4.2 订单详情（抽屉/页）

- 文件：`huipay-web/packages/merchant-portal/src/pages/Transactions/index.tsx`
- 设计要点：订单金额/实付/通道/支付时间/渠道单号/来源码牌；「查询通道状态」按钮调 `/query` 展示通道侧结果；入口跳转钱包流水。

#### 4.3 流水与订单联动

- 文件：`huipay-web/packages/merchant-portal/src/pages/Wallets/index.tsx`
- 设计要点：流水 `biz_id`（订单号）可点击跳转订单详情；订单详情可查看对应入账流水。

---

### 迭代 5：收银闭环加固与端到端验收（上线前）

> 目标：把开发态信任替换为可上线安全模型，并完成全链路验收。

#### 5.1 商户登录态

- 文件：`huipay-backend/internal/middleware/auth.go`、merchant-portal 登录相关
- 设计要点：将 `X-Merchant-Id` 明文头替换为真实登录（会话/JWT），或生产环境禁用明文头信任并接入登录页；与 P5 OAuth 规划对齐（可先做最小登录，完整 OAuth 留 P5）。

#### 5.2 码牌防枚举与金额防篡改监控

- 文件：`huipay-backend/internal/paymentcode/`、`infra/prom/`
- 设计要点：短码枚举失败/异常频次埋点告警（沿用现有 Prometheus 体系）；金额范围校验已有，补异常输入日志聚合。

#### 5.3 端到端联调脚本与验收清单

- 文件：`docs/p0-cashier-uat-checklist.md`（新建）
- 覆盖全链路：进件 → 配置微信参数 → 建码 → 下载二维码 → 消费者扫码 → 金额输入 → 支付 → 回调入账 → 查单 → 流水/交易查询 → 停用码牌。

---

## 三、任务清单

| 迭代 | 任务 | 主要涉及文件 |
|---|---|---|
| 1 | WechatClientManager 按商户懒加载/缓存 + 存储配置→运行时转换 | `internal/payment/channel/wechat/manager.go`（新）、`internal/merchant/service/service.go` |
| 1 | 路由/订单服务按商户取通道 | `internal/payment/router/router.go`、`internal/order/service/order_service.go`、`internal/order/scheduler/close_expired.go` |
| 1 | 回调按 serial 分流 | `internal/payment/notify/handler.go`、`cmd/server/main.go` |
| 1 | manager/回调分流测试 | `internal/payment/channel/wechat/manager_test.go`（新）、`internal/payment/notify/` |
| 2 | 码牌 checkout_url + 停用提示 | `internal/paymentcode/service/service.go` |
| 2 | 落地页复用 `h5?code=` + 部署层短链（可选） | `packages/checkout-sdk/src/pages/h5.tsx`（已支持）、部署 nginx 配置 |
| 2 | 商户收款码页面 | `packages/merchant-portal/src/pages/Codes/index.tsx`（新）、`routes.ts`、`App.tsx`、`BasicLayout.tsx`、`services/user.ts` |
| 2 | 码牌闭环验收 | 联调验证 |
| 3 | NATIVE 二维码渲染 | `packages/checkout-sdk/src/components/Checkout.tsx` |
| 3 | 结果页 + 倒计时 | `packages/checkout-sdk/src/components/PaymentResult.tsx`（新）、`h5.tsx`、`embed.tsx` |
| 3 | 轮询叠加通道查询兜底 | `packages/checkout-sdk/src/hooks/useCheckout.ts` |
| 3 | 码牌场景默认体验 | `packages/checkout-sdk/src/pages/h5.tsx` |
| 4 | 交易列表筛选/分页/汇总 | `packages/merchant-portal/src/pages/Transactions/index.tsx`、`internal/order/` |
| 4 | 订单详情 + 通道查询 | `packages/merchant-portal/src/pages/Transactions/index.tsx` |
| 4 | 流水与订单联动 | `packages/merchant-portal/src/pages/Wallets/index.tsx` |
| 5 | 商户登录态（最小可上线版） | `internal/middleware/auth.go`、merchant-portal |
| 5 | 防枚举/防篡改监控 | `internal/paymentcode/`、`infra/prom/` |
| 5 | UAT 验收清单 | `docs/p0-cashier-uat-checklist.md`（新） |

---

## 四、依赖与顺序

- 迭代 1 依赖商户微信配置阶段一（已完成）；是其余迭代的前置（支付链路按商户生效后才能验证码牌/收银台的真实支付）。
- 迭代 2 与迭代 3 相互独立，可在迭代 1 完成后并行推进。
- 迭代 4 依赖迭代 3 的查询兜底与结果页（订单详情需展示通道侧状态）。
- 迭代 5 可穿插进行，但商户登录态建议在对外可用前完成。

## 五、风险与缓解

| 风险 | 缓解 |
|---|---|
| 商户微信证书轮换（当前静态证书） | P0 上线前至少完成人工轮换流程文档；优先落地 `/v3/certificates` 自动拉取 |
| 回调分流设计复杂导致验签错配 | 按 `Wechatpay-Serial` 索引匹配，绝不"先平台验再商户验"；单测覆盖 |
| 真实微信商户号/公网回调域名缺失，无法真实验证 | 联调用 sandbox 或 mock 适配器先行；UAT 阶段用真实商户号 |
| NATIVE 与 H5/JSAPI 场景切换易乱 | 码牌模式按端默认（微信内 JSAPI、非微信内 H5/NATIVE），隐藏不适用项 |
| 回调延迟导致用户重复支付 | 轮询叠加 `/query` 通道兜底 + "支付处理中"提示，禁止重复建单支付 |

## 六、上线验收标准（全链路）

1. 管理端进件商户并配置微信支付参数（敏感字段密文入库、只回 `configured`）。
2. 商户端登录后创建收款码，可查看/下载二维码、复制收款链接。
3. 消费者扫码进入落地页，输入金额 0.01–50000 元，完成微信支付（NATIVE/H5/JSAPI 按端适配）。
4. 回调入账商户钱包（通道在途资金户 → 商户备付金），幂等重复回调不重复入账。
5. 消费者与商户都能看到一致的支付结果；交易列表可按状态/时间/码牌筛选，订单详情可主动查询通道状态。
6. 停用码牌后扫码明确提示；超时订单自动关单；钱包流水与订单可互相定位。
