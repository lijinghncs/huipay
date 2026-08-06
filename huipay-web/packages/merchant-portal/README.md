# huipay-merchant-portal

汇聚付 · 商家工作台（部署：https://merchant.huipay.cn）

## 本地开发

```bash
pnpm dev
# 打开 http://localhost:5170
```

## 构建

```bash
pnpm build
# 产物在 dist/
```

## 目录结构

```
src/
├── layouts/BasicLayout.tsx       # 主布局（含菜单）
├── pages/
│   ├── Dashboard/                # 概览
│   ├── Transactions/             # 交易列表
│   ├── Wallets/                  # 钱包与流水
│   └── SplitRules/               # 分账规则配置
├── services/user.ts              # 用户/订单/钱包 API（骨架）
├── access/index.ts               # RBAC 权限点
└── App.tsx                       # 路由入口
```

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `VITE_API_BASE` | https://api.huipay.cn | 后端 API 地址 |