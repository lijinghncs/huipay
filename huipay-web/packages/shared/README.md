# @huipay/shared

汇聚付前端的共享子包，**不独立部署**，作为其他 package 的依赖被引用。

## 提供能力

| 路径 | 说明 |
|---|---|
| `@huipay/shared/types` | 与后端 OpenAPI 对齐的 DTO 类型 |
| `@huipay/shared/api-client` | Axios 封装：自动注入 `Idempotency-Key` / `X-Trace-Id`、统一错误处理 |
| `@huipay/shared/utils` | `formatCents` / `formatDateTime` / `genIdempotencyKey` / `splitByRatio` |

## 使用

```ts
import type { Order, Wallet } from '@huipay/shared/types';
import { createApi, get, post } from '@huipay/shared/api-client';
import { formatCents } from '@huipay/shared/utils';

// 初始化（通常在 main.tsx 调用一次）
createApi({ baseURL: 'https://api.huipay.cn' });

// 业务代码
const order = await get<Order>(`/v1/checkout/${orderNo}`);
console.log(formatCents(order.amount));
```

## 类型对照（与后端对齐）

| 类型 | 对应后端字段 |
|---|---|
| `Order` | `t_order` |
| `Wallet` | `t_wallet` |
| `JournalEntry` | `t_journal_entry` |
| `ChannelCode` | `ChannelCode`（WECHAT / ALIPAY / UNIONPAY / BANK / DCEP） |
| `SplitExecuteRequest` / `SplitExecuteResponse` | `/v1/split/execute` 入参/出参 |

## 注意事项

- 本包作为源码引用（pnpm workspace `"main": "src/index.ts"`），无需单独构建；
- 任何类型变更必须同步更新后端 `merchant-payment-split-platform/technical-architecture.html` 中的 §6 接口契约。