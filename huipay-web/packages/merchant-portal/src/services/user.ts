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

/** 分账规则触发条件。 */
export interface SplitRuleCondition {
  channel?: string;
  store_ids?: number[];
  start_at?: string;
  end_at?: string;
  tag?: string;
}

/** 分账规则分配单元（接收方为全部门店，按实收占比分摊）。 */
export interface SplitRuleAllocation {
  receiver_entity_id?: number;
  receiver_type?: string;
  receiver_scope?: string; // "ALL_STORES" 表示全门店分摊
  ratio_bps?: number; // 万分比（10000 = 100%），与 fixed_amount 互斥
  fixed_amount?: number; // 固定金额（分），与 ratio_bps 互斥
}

/** 分账规则。 */
export interface SplitRule {
  id: number;
  rule_code: string;
  rule_name: string;
  merchant_id: number;
  priority: number;
  conditions: SplitRuleCondition;
  allocations: SplitRuleAllocation[];
  status: number; // 1=启用 0=停用
}

/** 分账规则列表。 */
export async function listSplitRules(): Promise<{ items: SplitRule[] }> {
  return get<{ items: SplitRule[] }>('/v1/merchant/split/rules');
}

/** 创建分账规则。 */
export async function createSplitRule(data: Partial<SplitRule>): Promise<SplitRule> {
  return post<SplitRule>('/v1/merchant/split/rules', data);
}

/** 更新分账规则。 */
export async function updateSplitRule(id: number, data: Partial<SplitRule>): Promise<SplitRule> {
  return put<SplitRule>(`/v1/merchant/split/rules/${id}`, data);
}

/** 启停分账规则。 */
export async function setSplitRuleStatus(id: number, status: number): Promise<{ id: number; status: number }> {
  return post<{ id: number; status: number }>(`/v1/merchant/split/rules/${id}/status`, { status });
}

/** 删除分账规则。 */
export async function deleteSplitRule(id: number): Promise<{ id: number; deleted: boolean }> {
  return del<{ id: number; deleted: boolean }>(`/v1/merchant/split/rules/${id}`);
}

/** 分账执行结果分配单元。 */
export interface SplitAllocationResult {
  entity_id: number;
  entity_type: string;
  amount: number;
}

/** 手动执行订单分账（仅限商户自己的订单，merchant_id 由后端从登录上下文填充）。 */
export async function executeSplit(
  orderNo: string,
  amount: number,
  opts: { storeId?: number; channel?: string; paidAt?: string; ruleCode?: string } = {},
): Promise<{ order_no: string; rule_code: string; allocations: SplitAllocationResult[] }> {
  return post('/v1/split/execute', {
    order_no: orderNo,
    amount,
    store_id: opts.storeId ?? 0,
    channel: opts.channel,
    paid_at: opts.paidAt,
    rule_code: opts.ruleCode,
  });
}

/** 分账记录列表行（按订单聚合）。 */
export interface SplitExecutionSummary {
  order_no: string;
  merchant_name?: string;
  total_amount: number; // 分账总额（分）
  receiver_count: number;
  status: 'SUCCESS' | 'PARTIAL' | 'FAILED';
  channel: string;
  executed_at?: string;
}

/** 分账记录分页结果。 */
export interface SplitExecutionPage {
  items: SplitExecutionSummary[];
  total: number;
}

/** 分账明细行（含接收方名称）。 */
export interface SplitExecutionDetail {
  receiver_entity_id: number;
  receiver_type: string;
  receiver_name: string;
  amount: number;
  level: number;
  status: string;
  channel_split_no: string;
  retry_count: number;
  last_error: string;
  executed_at?: string;
}

/** 分页查询当前商户分账记录。 */
export async function listSplitExecutions(opts: { page: number; size: number }): Promise<SplitExecutionPage> {
  return get<SplitExecutionPage>('/v1/merchant/split/executions', { params: opts });
}

/** 查询某订单分账明细。 */
export async function getSplitExecutionDetail(orderNo: string): Promise<{ order_no: string; items: SplitExecutionDetail[] }> {
  return get<{ order_no: string; items: SplitExecutionDetail[] }>(`/v1/merchant/split/executions/${orderNo}`);
}

/** 按时间段分账请求参数。 */
export interface ExecuteByPeriodRequest {
  start: string;
  end: string;
  rule_code: string;
}

/** 按时间段分账响应。 */
export interface ExecuteByPeriodResponse {
  batch_no: string;
  total_amount: number;
  rule_code: string;
  allocations: SplitAllocationResult[];
}

/** 按时间段执行分账：选定规则，以时间段内商户实收总额为基数，按门店实收占比分配。 */
export async function executeSplitByPeriod(data: ExecuteByPeriodRequest): Promise<ExecuteByPeriodResponse> {
  return post<ExecuteByPeriodResponse>('/v1/merchant/split/execute-period', data);
}

/** 分账单明细（各门店可分金额）。 */
export interface SplitBillItem {
  receiver_entity_id: number;
  receiver_type: string;
  receiver_name: string;
  amount: number;
}

/** 分账单。 */
export interface SplitBill {
  id: number;
  batch_no: string;
  rule_code: string;
  rule_name: string;
  start_time: string;
  end_time: string;
  total_amount: number;
  status: 'PENDING' | 'APPROVED' | 'REJECTED' | 'EXECUTED';
  items: SplitBillItem[];
  created_at: string;
  approved_at?: string;
  executed_at?: string;
}

/** 生成分账单（待审批，不扣款）。 */
export async function generateSplitBill(data: ExecuteByPeriodRequest): Promise<SplitBill> {
  return post<SplitBill>('/v1/merchant/split/bills', data);
}

/** 分页查询分账单。 */
export async function listSplitBills(opts: { page: number; size: number }): Promise<{ items: SplitBill[]; total: number }> {
  return get<{ items: SplitBill[]; total: number }>('/v1/merchant/split/bills', { params: opts });
}

/** 查询分账单详情。 */
export async function getSplitBill(batchNo: string): Promise<SplitBill> {
  return get<SplitBill>(`/v1/merchant/split/bills/${batchNo}`);
}

/** 审批通过分账单并执行。 */
export async function approveSplitBill(batchNo: string): Promise<SplitBill> {
  return post<SplitBill>(`/v1/merchant/split/bills/${batchNo}/approve`);
}

/** 驳回分账单。 */
export async function rejectSplitBill(batchNo: string): Promise<SplitBill> {
  return post<SplitBill>(`/v1/merchant/split/bills/${batchNo}/reject`);
}
