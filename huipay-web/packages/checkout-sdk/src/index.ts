// 收银台 SDK 主入口
export { HuiPayCheckout } from './components/Checkout';
export { HuiPayEmbedded } from './components/Embedded';
export { useCheckout } from './hooks/useCheckout';
export { useCheckoutUI } from './hooks/useCheckoutUI';
export { createApi } from '@huipay/shared/api-client';

export type {
  CheckoutProps,
  ChannelCode,
  Order,
  Wallet,
  JournalEntry,
  SplitAllocation,
  ChannelAvailable,
} from './types';

import './styles/global.css';