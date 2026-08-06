# huipay-web

汇聚付 HuiPay 前端 monorepo（pnpm workspace）。

## 工程划分

| 包名 | 路径 | 形态 | 部署域名 |
|---|---|---|---|
| `huipay-merchant-portal` | packages/merchant-portal | SPA | https://merchant.huipay.cn |
| `huipay-admin-portal` | packages/admin-portal | SPA | https://admin.huipay.cn |
| `huipay-checkout-sdk` | packages/checkout-sdk | 类库 + 页面 | https://checkout.huipay.cn |

共享子包（不独立部署）：
- `@huipay/shared` 共享类型 / API Client / 工具
- `@huipay/ui-kit` 基于 AntD 二次封装

## 快速开始

```bash
pnpm install
pnpm dev:merchant   # 商家工作台
pnpm dev:admin      # 管理后台
pnpm dev:sdk        # 收银台 SDK
```

## 构建

```bash
pnpm build:all
```

## 目录结构

```
huipay-web/
├── packages/
│   ├── shared/              # @huipay/shared
│   ├── checkout-sdk/        # @huipay/checkout-sdk
│   ├── ui-kit/              # @huipay/ui-kit
│   ├── merchant-portal/     # 商家工作台
│   └── admin-portal/        # 管理后台
├── pnpm-workspace.yaml
├── tsconfig.base.json
└── package.json
```