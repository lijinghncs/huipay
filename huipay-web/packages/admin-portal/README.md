# huipay-admin-portal

汇聚付 · 管理后台（部署：https://admin.huipay.cn）

## 本地开发

```bash
pnpm dev
# 打开 http://localhost:5171
```

## 构建

```bash
pnpm build
```

## 目录结构

```
src/
├── layouts/BasicLayout.tsx
├── pages/
│   ├── Analytics/        # 概览 / BI
│   ├── Merchants/        # 商户管理
│   ├── Channels/         # 通道配置
│   └── RiskRules/        # 风控规则
├── services/admin.ts
├── access/index.ts
└── App.tsx
```