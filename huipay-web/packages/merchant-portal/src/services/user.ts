// 商家用户/交易服务（真实调后端 API）
import { get } from '@huipay/shared/api-client';
import type { Order, Wallet, JournalEntry } from '@huipay/shared';

/** 当前登录用户（骨架：固定值；生产由登录态注入）。 */
export async function getCurrentUser(): Promise<{ id: number; name: string; role: string; merchantId: number }> {
  return { id: 1, name: '演示商户', role: 'merchant_admin', merchantId: 10001 };
}

/** 拉取订单列表（分页，可选状态过滤）。 */
export async function listOrders(params: { page?: number; size?: number; status?: string } = {}): Promise<{
  items: Order[];
  total: number;
}> {
  return get<{ items: Order[]; total: number }>('/v1/checkout/list', { params });
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