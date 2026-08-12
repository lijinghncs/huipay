// 商家用户/交易服务（真实调后端 API）
import { get, post, put, del } from '@huipay/shared/api-client';
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
  store_id?: number;
  start?: string;
  end?: string;
} = {}): Promise<{
  items: Order[];
  total: number;
}> {
  return get<{ items: Order[]; total: number }>('/v1/checkout/list', { params });
}

/** 订单统计单行。 */
export interface OrderStatRow {
  key: string;
  label: string;
  order_count: number;
  amount: number;
  paid: number;
}

/** 订单统计（筛选范围内全部订单）。 */
export interface OrderStats {
  order_count: number;
  paid_order_count: number;
  total_amount: number;
  total_paid: number;
  by_status: OrderStatRow[];
  by_channel: OrderStatRow[];
  by_store: OrderStatRow[];
}

/** 拉取订单统计（聚合当前筛选条件下全部订单）。 */
export async function getOrderStats(params: {
  status?: string;
  channel?: string;
  code_id?: string;
  store_id?: number;
  start?: string;
  end?: string;
} = {}): Promise<OrderStats> {
  return get<OrderStats>('/v1/checkout/stats', { params });
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
  store_id?: number;
  store_name?: string;
  code_id: string;
  status: number; // 1=启用 0=停用
  remark?: string;
  checkout_url: string;
  created_at: string;
  disabled_at?: string | null;
}

/** 分页码牌列表。 */
export async function listCodes(params: { page?: number; size?: number; status?: number; store_id?: number } = {}): Promise<{
  items: PaymentCode[];
  total: number;
  page: number;
  size: number;
}> {
  return get<{ items: PaymentCode[]; total: number; page: number; size: number }>('/v1/merchant/codes', { params });
}

/** 创建收款码牌。 */
export async function createCode(remark: string, storeId?: number): Promise<PaymentCode> {
  return post<PaymentCode>('/v1/merchant/codes', { remark, store_id: storeId ?? 0 });
}

/** 停用收款码牌。 */
export async function disableCode(id: number): Promise<{ id: number; status: number }> {
  return post<{ id: number; status: number }>(`/v1/merchant/codes/${id}/disable`);
}

/** 绑定/解绑收款码牌门店（storeId 为 0 表示解绑）。 */
export async function setCodeStore(id: number, storeId?: number): Promise<PaymentCode> {
  return post<PaymentCode>(`/v1/merchant/codes/${id}/store`, { store_id: storeId ?? 0 });
}

/** 门店。 */
export interface Store {
  id: number;
  store_code: string;
  merchant_id: number;
  name: string;
  store_type?: string;
  contact_phone?: string;
  region?: string;
  address?: string;
  longitude?: number | null;
  latitude?: number | null;
  status: number; // 1=启用 0=停用
  code_count: number;
  order_count: number;
  created_at: string;
  updated_at: string;
}

/** 门店详情（含关联码牌数/订单数）。 */
export interface StoreDetail extends Store {
  code_count: number;
  order_count: number;
}

/** 门店统计（列表 KPI）。 */
export interface StoreStats {
  total: number;
  active: number;
  month_new: number;
}

/** 门店列表（分页 + 名称/状态筛选）。 */
export async function listStores(params: { page?: number; size?: number; keyword?: string; status?: number } = {}): Promise<{
  items: Store[];
  total: number;
  page: number;
  size: number;
}> {
  return get<{ items: Store[]; total: number; page: number; size: number }>('/v1/merchant/stores', { params });
}

/** 门店统计。 */
export async function getStoreStats(): Promise<StoreStats> {
  return get<StoreStats>('/v1/merchant/stores/stats');
}

/** 门店详情。 */
export async function getStore(id: number): Promise<StoreDetail> {
  return get<StoreDetail>(`/v1/merchant/stores/${id}`);
}

/** 创建门店。 */
export async function createStore(data: Partial<Store>): Promise<Store> {
  return post<Store>('/v1/merchant/stores', data);
}

/** 更新门店。 */
export async function updateStore(id: number, data: Partial<Store>): Promise<Store> {
  return put<Store>(`/v1/merchant/stores/${id}`, data);
}

/** 启停门店。 */
export async function setStoreStatus(id: number, status: number): Promise<Store> {
  return post<Store>(`/v1/merchant/stores/${id}/status`, { status });
}

/** 删除门店。 */
export async function deleteStore(id: number): Promise<{ id: number; deleted: boolean }> {
  return del<{ id: number; deleted: boolean }>(`/v1/merchant/stores/${id}`);
}
