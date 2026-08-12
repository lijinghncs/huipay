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

/** 支付场景类型。 */
export type PayType = 'NATIVE' | 'H5' | 'JSAPI';

/** 发起支付响应（镜像后端 PayResponse 字段）。 */
export interface PayResponse {
  order_no: string;
  channel: string;
  pay_type: PayType;
  pay_url?: string;
  qr_code?: string;
  prepay_id?: string;
  /** JSAPI 微信内拉起的调起参数（后端对 prepay_id 二次签名） */
  jsapi?: JSAPIParams;
}

/** JSAPI 前端调起参数（传给 WeixinJSBridge.getBrandWCPayRequest）。 */
export interface JSAPIParams {
  appId: string;
  timeStamp: string;
  nonceStr: string;
  package: string;
  signType: string;
  paySign: string;
}

export interface CheckoutProps {
  /** 后端预下单返回的订单号 */
  orderNo: string;
  /** 已存在的通道列表 */
  channels: Array<{ code: string; fee_rate: string; available: boolean }>;
  /** 订单金额（分） */
  amount: number;
  /** 已应用优惠（分） */
  discount?: number;
  /** JSAPI 场景必填 */
  openId?: string;
  /** 默认支付场景（默认 NATIVE） */
  defaultPayType?: PayType;
  /** 是否显示场景切换（默认 true） */
  showPayTypeSelector?: boolean;
  /** 是否显示支付方式（通道）切换（默认 true） */
  showChannelSelector?: boolean;
  /** 用户选中的支付通道变化 */
  onChannelChange?: (code: string) => void;
  /** 用户选中的支付场景变化 */
  onPayTypeChange?: (payType: PayType) => void;
  /** 支付完成回调 */
  onSuccess?: (result: { orderNo: string; channel: string }) => void;
  /** JSAPI 拉起回调（前端负责 WeixinJSBridge） */
  onJSAPIReady?: (prepayId: string) => void;
  /** 支付失败回调 */
  onError?: (err: Error) => void;
  /** 自定义主题色 */
  primaryColor?: string;
}