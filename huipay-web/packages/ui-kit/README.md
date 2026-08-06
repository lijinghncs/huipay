# @huipay/ui-kit

汇聚付前端共享 UI 组件包，**不独立部署**，基于 Ant Design 5 二次封装。

## 提供组件

| 组件 | 用途 |
|---|---|
| `<Money cents={...} />` | 金额展示（分 → 元，AntD `Statistic` 二次封装） |
| `<StatusTag status="..." />` | 通用状态标签（订单/分账状态自动着色 + 中文） |

## 使用

```tsx
import { Money, StatusTag } from '@huipay/ui-kit';

<Money cents={12345600} prefix="余额 ¥" />
<StatusTag status="PAID" />     // 显示绿色「已支付」
<StatusTag status="FAILED" />    // 显示红色「失败」
```

## 后续计划

随着业务复杂度提升，将按需扩展：
- `<ChannelBadge />` 支付通道标识
- `<SplitProgressBar />` 分账进度条
- `<RiskLevelTag />` 风控等级标识

新增组件请保持：
- 基于 AntD 5；
- 类型导出从 `src/components/<Name>/index.ts` 集中；
- 单测覆盖率 ≥ 80%。