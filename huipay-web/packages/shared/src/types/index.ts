// 共享类型定义，与后端 OpenAPI 对齐

export type ChannelCode = 'WECHAT' | 'ALIPAY' | 'UNIONPAY' | 'BANK' | 'DCEP';

export type OrderStatus = 'CREATED' | 'PAID' | 'CLOSED' | 'REFUNDED';

export type SplitStatus = 'PENDING' | 'PROCESSING' | 'SUCCESS' | 'FAILED' | 'RETURNED';

/** 后端统一响应 */
export interface ApiResponse<T = unknown> {
  code: string;
  message: string;
  trace_id?: string;
  data?: T;
}

/** 订单 */
export interface Order {
  id: number;
  order_no: string;
  merchant_order_no: string;
  merchant_id: number;
  store_id?: number;
  store_name?: string;
  code_id?: string;
  amount: number;
  paid_amount: number;
  coupon_discount: number;
  channel?: ChannelCode;
  channel_trade_no?: string;
  split_status: SplitStatus;
  status: OrderStatus;
  expire_at?: string;
  paid_at?: string;
  created_at: string;
  updated_at: string;
}

/** 主动查询支付结果（只读，后端不更新本地订单） */
export interface QueryResult {
  order_no: string;
  local_status: string;
  paid: boolean;
  paid_amount: number;
  channel_trade_no?: string;
  channel?: ChannelCode;
  paid_at?: number;
}

/** 预下单请求 */
export interface PrecreateRequest {
  merchant_id: number;
  merchant_order_no: string;
  amount: number;
  subject?: string;
  notify_url?: string;
  expire_seconds?: number;
  coupons?: string[];
  metadata?: Record<string, unknown>;
}

/** 可用通道 */
export interface ChannelAvailable {
  code: ChannelCode;
  fee_rate: string;
  available: boolean;
}

/** 预下单响应 */
export interface PrecreateResponse {
  order_no: string;
  channels: ChannelAvailable[];
  expire_at: string;
  checkout_url: string;
  paid_amount?: number;
  coupon_discount?: number;
}

/** 钱包 */
export interface Wallet {
  id: number;
  wallet_no: string;
  entity_id: number;
  entity_type: string;
  currency: string;
  balance: number;
  frozen: number;
  pre_frozen: number;
  version: number;
  status: number;
  created_at: string;
  updated_at: string;
}

/** 账本流水 */
export interface JournalEntry {
  id: string;
  wallet_id: number;
  direction: 'DEBIT' | 'CREDIT';
  amount: number;
  balance_after: number;
  biz_type: string;
  biz_id: string;
  counterparty_id?: number;
  idempotency_key: string;
  trace_id?: string;
  remark?: string;
  created_at: string;
}

/** 分账分配单元 */
export interface SplitAllocation {
  level: number;
  entity_id: number;
  entity_type: string;
  amount: number;
}

/** 分账执行请求 */
export interface SplitExecuteRequest {
  order_no: string;
  merchant_id: number;
  amount: number;
  rule_code?: string;
  store_id?: number;
  channel?: ChannelCode;
}

/** 分账执行响应 */
export interface SplitExecuteResponse {
  order_no: string;
  rule_code?: string;
  allocations: SplitAllocation[];
}
