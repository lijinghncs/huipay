# @huipay/checkout-sdk

汇聚付收银台 SDK + 独立收银台页面。**独立部署**：`https://checkout.huipay.cn`

## 三种集成形态

### 1. 组件式（Component）

适用于商家自有 React/Vue 项目。

```包
pnpm i @huipay/checkout-sdk
```

```tsx
import { HuiPayCheckout, createApi } from '@huipay/checkout-sdk';

createApi({ baseURL: 'https://api.huipay.cn' });

export default function PayPage() {
  return (
    <HuiPayCheckout
      orderNo="HP202608060001"
      channels={[
        { code: 'WECHAT', fee_rate: '0.60%', available: true },
        { code: 'ALIPAY', fee_rate: '0.55%', available: true },
      ]}
      amount={10000}
      discount={1000}
      onSuccess={(r) => console.log('paid', r)}
      onError={(e) => console.error(e)}
    />
  );
}
```

### 2. 嵌入式（Embed）

通过 iframe 加载，适用于商家 Web / H5。

```tsx
import { HuiPayEmbedded } from '@huipay/checkout-sdk';

<HuiPayEmbedded token={sdkToken} height={640} />
```

后端通过 `/v1/checkout/embed-token?order_no=xxx` 签发短时 token。

### 3. H5 收银台（独立 URL）

适用于扫码支付、短信链接。

```
https://checkout.huipay.cn/h5?order=HP202608060001
```

## 本地开发

```bash
pnpm dev
# 默认端口 5173
```

## 构建与部署

```bash
pnpm build       # 输出 dist/
# 部署：dist/embed.html → /embed、dist/h5.html → /h5、其余 → /
```

## 目录结构

```
src/
├── components/
│   ├── Checkout.tsx        组件式入口
│   └── Embedded.tsx        嵌入式 iframe 加载器
├── hooks/
│   ├── useCheckout.ts      业务 hook（订单轮询 / 预下单）
│   └── useCheckoutUI.ts    UI 状态（Zustand）
├── pages/
│   ├── embed.tsx           /embed 页面骨架
│   └── h5.tsx              /h5 页面骨架
├── types/index.ts
└── styles/global.css
```

## 与后端契约

| 接口 | 方法 | 用途 |
|---|---|---|
| `/v1/checkout/precreate` | POST | 预下单，返回 order_no + channels |
| `/v1/checkout/:order_no` | GET | 轮询订单状态 |
| `/v1/checkout/:order_no/refund` | POST | 退款 |

## 后续 P3 阶段

- 多权益方叠加（券 + 储值卡 + 营销）
- 计价最优解算法集成
- 小程序 / App SDK 形态