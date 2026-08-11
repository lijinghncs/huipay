// 商家用户/交易服务（真实调后端 API）
import { get } from '@huipay/shared/api-client';
import type { Order, Wallet, JournalEntry, QueryResult } from '@huipay/shared';
import { merchantIdFromToken } from './auth';

/** 当前登录用户（从登录态 token 解码商户号）。 */
export async function getCurrentUser(): Promise<{ id: number; name: string; role: string; merchantId: number }> {
  const id = merchantIdFromToken();
  if (!id) {
    throw new Error('未登录');
  }
  return { id, name: '商户', role: 'merchant_admin', merchantId: id };
}

/** 拉取订单列表（分页，可选状态过滤）。 */
export async function listOrders(params: {
  page?: number;
  size?: number;
  status?: string;
  code_id?: string;
  channel?: string;
  start?: string;
  end?: string;
} = {}): Promise<{
  items: Order[];
  total: number;
}> {
  return get<{ items: Order[]; total: number }>('/v1/checkout/list', { params });
}

/** 单笔订单查询。 */
export async function getOrder(orderNo: string): Promise<Order> {
  return get<Order>(`/v1/checkout/${orderNo}`);
}

/** 主动查询通道侧支付结果（只读）。 */
export async function queryOrder(orderNo: string): Promise<QueryResult> {
  return get<QueryResult>(`/v1/checkout/${orderNo}/query`);
}

/** 拉取钱包。 */
export async function getWallet(entityId: number): Promise<Wallet> {
  return get<Wallet>(`/v1/wallets/${entityId}`);
}

/** 拉取账本流水（分页 + 过滤）。 */
export async function listEntries(
  entityId: number,
  params: { page?: number; size?: number; biz_type?: string; biz_id?: string; start?: string; end?: string } = {},
): Promise<{ items: JournalEntry[]; total: number; page: number; size: number }> {
  return get<{ items: JournalEntry[]; total: number; page: number; size: number }>(
    `/v1/wallets/${entityId}/entries`,
    { params },
  );
}

/** 当前商户自助资料（商户号 / 名称 / 钱包 / 余额）。 */
export interface MerchantProfile {
  id: number;
  entity_code: string;
  entity_type: string;
  name: string;
  kyc_status: number;
  kyc_data?: Record<string, unknown>;
  status: number;
  wallet_no: string;
  balance: number;
  frozen: number;
  pre_frozen: number;
  created_at: string;
  updated_at: string;
}

/** 拉取当前商户资料。 */
export async function getMerchantProfile(): Promise<MerchantProfile> {
  return get<MerchantProfile>('/v1/merchant/profile');
}

/** 当前商户经营概览。 */
export interface MerchantOverview {
  merchant_id: number;
  entity_code: string;
  name: string;
  balance: number;
  frozen: number;
  total_paid: number;
  order_count: number;
  paid_order_count: number;
  active_code_count: number;
}

/** 拉取当前商户经营概览。 */
export async function getMerchantOverview(): Promise<MerchantOverview> {
  return get<MerchantOverview>('/v1/merchant/overview');
}

/** 收款码牌。 */
export interface PaymentCode {
  id: number;
  merchant_id: number;
  code_id: string;
  status: number; // 1=启用 0=停用
  remark?: string;
  checkout_url: string;
  created_at: string;
  disabled_at?: string | null;
}

/** 分页码牌列表。 */
export async function listCodes(params: { page?: number; size?: number; status?: number } = {}): Promise<{
  items: PaymentCode[];
  total: number;
  page: number;
  size: number;
}> {
  return get<{ items: PaymentCode[]; total: number; page: number; size: number }>('/v1/merchant/codes', { params });
}

/** 创建收款码牌。 */
export async function createCode(remark: string): Promise<PaymentCode> {
  return post<PaymentCode>('/v1/merchant/codes', { remark });
}

/** 停用收款码牌。 */
export async function disableCode(id: number): Promise<{ id: number; status: number }> {
  return post<{ id: number; status: number }>(`/v1/merchant/codes/${id}/disable`);
}
