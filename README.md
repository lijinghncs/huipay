# 汇聚付 HuiPay · Monorepo

> 商户收款 + 分账系统的基础工程仓库（后端 + 前端 monorepo）。
> 依据：[产品方案](./../merchant-payment-split-platform/merchant-payment-split-platform.html) · [技术方案](./../merchant-payment-split-platform/technical-architecture.html)

## 仓库结构

```
huipay-monorepo/
├── huipay-backend/         Go 1.22 后端单体（Gin + GORM + MySQL 8.0）
├── huipay-web/             前端 pnpm workspace（Vite + React 18 + AntD Pro）
│   └── packages/
│       ├── shared/         @huipay/shared 共享类型 / API Client / 工具
│       ├── checkout-sdk/   @huipay/checkout-sdk 收银台 SDK
│       ├── ui-kit/         @huipay/ui-kit 共享 UI 组件
│       ├── merchant-portal/   商家工作台（merchant.huipay.cn）
│       └── admin-portal/      管理后台（admin.huipay.cn）
└── README.md（本文件）
```

## 快速开始

### 1. 后端

```bash
cd huipay-backend
go mod tidy
make migrate-up        # 跑数据库迁移
make run               # 本地启动
```

启动后访问：
- 健康检查：<http://localhost:8080/healthz>
- Prometheus 指标：<http://localhost:8080/metrics>
- 预下单：`POST /v1/checkout/precreate`

### 2. 前端

```bash
cd huipay-web
pnpm install
pnpm dev:merchant      # 商家工作台 http://localhost:5170
pnpm dev:admin         # 管理后台    http://localhost:5171
pnpm dev:sdk           # 收银台 SDK   http://localhost:5173
```

## 三大前端工程 & 部署域名

| 工程 | 部署域名 |
|---|---|
| `huipay-merchant-portal` | <https://merchant.huipay.cn> |
| `huipay-admin-portal` | <https://admin.huipay.cn> |
| `huipay-checkout-sdk` | <https://checkout.huipay.cn> |

后端 API：`https://api.huipay.cn/v1/...`

## 提交规范

- 使用 Conventional Commits：`feat:` / `fix:` / `docs:` / `refactor:` / `chore:` / `test:`
- 示例：`feat(order): add precreate idempotent key`

## 版本与里程碑

| 阶段 | 时间 | 关键交付 |
|---|---|---|
| P0 基础收银 | M0–M2 | 聚合收银 MVP（微信 + 支付宝） |
| P1 一级分账 | M3–M4 | 账户 /账本 /分账 /提现 /T+0 短账 |
| P2 多级分账 | M3–M6 | 规则 DSL /灵工代征 /T+1 长账 /退款回滚 |
| P3 权益叠加 | M7–M8 | 券 /储值卡 /营销引擎 |
| P4 风控 + 可观测 | M9–M11 | 风控引擎 /分账引擎拆微服务 |
| P5 开放平台 | M12+ | OAuth 2.0 /开放 API /SDK /ISV |

## 关联文档

- 产品方案：[merchant-payment-split-platform.html](../merchant-payment-split-platform/merchant-payment-split-platform.html)
- 技术方案：[technical-architecture.html](../merchant-payment-split-platform/technical-architecture.html)
- 品牌方案：[huipay-brand-book.html](../merchant-payment-split-platform/huipay-brand-book.html)