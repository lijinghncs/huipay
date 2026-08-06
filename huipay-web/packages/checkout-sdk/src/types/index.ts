// 收银台对外暴露的类型（从 shared 透传）
export type {
  ChannelCode,
  Order,
  Wallet,
  JournalEntry,
  SplitAllocation,
  ChannelAvailable,
  PrecreateRequest,
  PrecreateResponse,
} from '@huipay/shared';

export interface CheckoutProps {
  /** 后端预下单返回的订单号 */
  orderNo: string;
  /** 已存在的通道列表 */
  channels: Array<{ code: string; fee_rate: string; available: boolean }>;
  /** 订单金额（分） */
  amount: number;
  /** 已应用优惠（分） */
  discount?: number;
  /** 用户选中的支付通道变化 */
  onChannelChange?: (code: string) => void;
  /** 支付完成回调 */
  onSuccess?: (result: { orderNo: string; channel: string }) => void;
  /** 支付失败回调 */
  onError?: (err: Error) => void;
  /** 自定义主题色 */
  primaryColor?: string;
}