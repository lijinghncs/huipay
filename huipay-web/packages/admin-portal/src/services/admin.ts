// 管理后台商户 API（对接后端 /v1/admin/merchants）
import { get, post, put } from '@huipay/shared/api-client';

export interface Merchant {
  id: number;
  entity_code: string; // 商户号
  entity_type: string; // MERCHANT/STORE/...
  name: string;
  kyc_status: number;
  status: number;
  wallet_no: string;
  created_at: string;
}

export interface MerchantListResp {
  items: Merchant[];
  total: number;
  page: number;
  size: number;
}

export interface ListMerchantsParams {
  page?: number;
  page_size?: number;
  keyword?: string;
  status?: number;
}

export interface OnboardRequest {
  name: string;
  type: string; // 主体类型：MERCHANT
  legal_name?: string;
  license_no?: string;
  bank_account?: string;
  bank_name?: string;
  contact_name?: string;
  contact_phone?: string;
  login_phone?: string; // 登录手机号（可选）
  login_password?: string; // 初始登录密码（可选，>=6 位）
  wechat_config?: MerchantWechatConfigInput; // 微信支付配置（敏感字段加密入库）
}

/** 微信支付配置（提交用，含敏感字段；编辑时敏感字段留空 = 不修改） */
export interface MerchantWechatConfigInput {
  enabled: boolean;
  mchid?: string;
  appid?: string;
  app_secret?: string;
  api_v3_key?: string;
  merchant_serial_no?: string;
  merchant_private_key?: string;
  platform_serial_no?: string;
  platform_public_key?: string;
  notify_base_url?: string;
}

/** 微信支付配置读视图（敏感字段仅回 configured 标记，不回显明文） */
export interface MerchantWechatConfigView {
  enabled: boolean;
  mchid: string;
  appid: string;
  app_secret_configured: boolean;
  api_v3_key_configured: boolean;
  merchant_private_key_configured: boolean;
  platform_public_key_configured: boolean;
  merchant_serial_no: string;
  platform_serial_no: string;
  notify_base_url: string;
}

/** 商户列表（分页 + 名称/商户号模糊 + 状态筛选） */
export async function listMerchants(params: ListMerchantsParams = {}): Promise<MerchantListResp> {
  return get<MerchantListResp>('/v1/admin/merchants', { params });
}

/** 商户进件（创建主体 + 自动开钱包） */
export async function onboardMerchant(data: OnboardRequest): Promise<Merchant> {
  return post<Merchant>('/v1/admin/merchants', data);
}

/** 商户详情（含商户身份认证资料与钱包余额） */
export async function getMerchant(id: number): Promise<MerchantDetail> {
  return get<MerchantDetail>(`/v1/admin/merchants/${id}`);
}

/** 更新商户基础资料 */
export async function updateMerchant(id: number, data: OnboardRequest): Promise<Merchant> {
  return put<Merchant>(`/v1/admin/merchants/${id}`, data);
}

/** 启用 / 停用商户 */
export async function setMerchantStatus(id: number, status: number): Promise<Merchant> {
  return post<Merchant>(`/v1/admin/merchants/${id}/status`, { status });
}

/** 商户经营概览 */
export async function getMerchantOverview(id: number): Promise<MerchantOverview> {
  return get<MerchantOverview>(`/v1/admin/merchants/${id}/overview`);
}

/** 商户微信支付配置（敏感字段仅回 configured 标记） */
export async function getMerchantWechatConfig(id: number): Promise<MerchantWechatConfigView | null> {
  return get<MerchantWechatConfigView | null>(`/v1/admin/merchants/${id}/wechat-config`);
}

/** 更新商户微信支付配置（敏感字段留空 = 不修改，非敏感字段留空 = 清空） */
export async function updateMerchantWechatConfig(
  id: number,
  cfg: MerchantWechatConfigInput,
): Promise<MerchantWechatConfigView | null> {
  return put<MerchantWechatConfigView | null>(`/v1/admin/merchants/${id}/wechat-config`, { wechat_config: cfg });
}

/** 设置 / 重置商户登录手机号与密码 */
export async function setMerchantLoginPassword(id: number, loginPhone: string, password: string): Promise<Merchant> {
  return post<Merchant>(`/v1/admin/merchants/${id}/login-password`, { login_phone: loginPhone, password });
}

export interface MerchantDetail {
  id: number;
  entity_code: string;
  entity_type: string;
  name: string;
  kyc_status: number;
  kyc_data: Record<string, string>;
  status: number;
  wallet_no: string;
  balance: number;
  frozen: number;
  pre_frozen: number;
  wechat_config?: MerchantWechatConfigView | null;
  created_at: string;
  updated_at: string;
}

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

export async function getCurrentAdmin(): Promise<{ id: number; name: string; role: string }> {
  return { id: 1, name: '平台管理员', role: 'admin' };
}

export async function listChannels(): Promise<unknown[]> {
  return [];
}

export async function listRiskRules(): Promise<unknown[]> {
  return [];
}

// ---- V2 合并版：门店按日统计（含分账字段）+ 每日执行 + 审计 + 对账差异 ----

export interface StoreStatItem {
  id: number;
  merchant_id: number;
  store_id: number;
  biz_date: string;
  order_count: number;
  paid_amount: number;
  channel_breakdown?: string;
  status_breakdown?: string;
  split_status?: 'PENDING' | 'SUCCESS' | 'PARTIAL' | 'FAILED';
  split_batch_no?: string;
  split_at?: string;
  split_total_amount?: number;
}

export interface StoreStatListResp {
  items: StoreStatItem[];
  total: number;
}

export interface StoreSumRow {
  store_id: number;
  store_name: string;
  order_count: number;
  paid_amount: number;
}

export interface StoreStatSummaryResp {
  summary: { order_count: number; paid_amount: number };
  items: StoreSumRow[];
}

export interface StoreStatParams {
  merchant_id?: number;
  store_id?: number;
  start_date: string;
  end_date: string;
  page?: number;
  page_size?: number;
}

export async function listAdminStoreStats(params: StoreStatParams): Promise<StoreStatListResp> {
  return get<StoreStatListResp>('/v1/admin/store-stats', { params });
}

export async function getAdminStoreStatSummary(params: StoreStatParams): Promise<StoreStatSummaryResp> {
  return get<StoreStatSummaryResp>('/v1/admin/store-stats/summary', { params });
}

export async function getAdminStoreDailyStats(storeId: number, startDate: string, endDate: string): Promise<StoreStatItem[]> {
  return get<StoreStatItem[]>(`/v1/admin/stores/${storeId}/daily-stats`, {
    params: { start_date: startDate, end_date: endDate },
  });
}

export async function recomputeStoreStats(merchantId: number, bizDate: string): Promise<{ updated_stores: number }> {
  return post<{ updated_stores: number }>('/v1/admin/store-stats/recompute', {}, {
    params: { merchant_id: merchantId, biz_date: bizDate },
  });
}

export async function resetStoreSplitStatus(merchantId: number, storeId: number, bizDate: string): Promise<{ reset: boolean }> {
  return post<{ reset: boolean }>('/v1/admin/store-stats/reset-split-status', {}, {
    params: { merchant_id: merchantId, store_id: storeId, biz_date: bizDate },
  });
}

// ---- 每日执行 ----
export interface DailyExecution {
  id: number;
  run_id: string;
  merchant_id: number;
  biz_date: string;
  batch_no: string;
  status: 'RUNNING' | 'SUCCESS' | 'PARTIAL' | 'FAILED';
  started_at: string;
  finished_at?: string;
  duration_ms?: number;
  error_code?: string;
  error_message?: string;
  reconcile_diff_id?: number;
  operator_type: string;
  operator_id: number;
}

export interface DailyExecListResp {
  items: DailyExecution[];
  total: number;
}

export async function listDailyExecutions(params: {
  merchant_id?: number;
  start_date: string;
  end_date: string;
  status?: string;
  page?: number;
  page_size?: number;
}): Promise<DailyExecListResp> {
  return get<DailyExecListResp>('/v1/admin/split/daily-executions', { params });
}

export async function getDailyExecution(id: number): Promise<DailyExecution> {
  return get<DailyExecution>(`/v1/admin/split/daily-executions/${id}`);
}

// ---- 审计 ----
export interface AuditRecord {
  id: number;
  biz_type: string;
  biz_id: string;
  action: string;
  operator_type: string;
  operator_id: number;
  detail?: string;
  created_at: string;
}

export interface AuditListResp {
  items: AuditRecord[];
  total: number;
}

export async function listAudits(params: {
  biz_type?: string;
  biz_id?: string;
  action?: string;
  page?: number;
  page_size?: number;
}): Promise<AuditListResp> {
  return get<AuditListResp>('/v1/admin/split/audit', { params });
}

// ---- 对账差异 ----
export interface ReconcileDiff {
  id: number;
  biz_date: string;
  merchant_id?: number;
  diff_type: 'LONG' | 'SHORT' | 'MISMATCH' | 'SPLIT_TOTAL' | 'SPLIT_DETAIL';
  order_no?: string;
  transaction_id?: string;
  local_amount?: number;
  channel_amount?: number;
  detail?: string;
  resolved_at?: string;
  created_at: string;
}

export interface ReconcileDiffListResp {
  items: ReconcileDiff[];
  total: number;
}

export async function listReconcileDiffs(params: {
  merchant_id?: number;
  diff_type?: string;
  start_date: string;
  end_date: string;
  page?: number;
  page_size?: number;
}): Promise<ReconcileDiffListResp> {
  return get<ReconcileDiffListResp>('/v1/admin/reconcile-diffs', { params });
}

/** 分账状态徽章辅助（与 merchant-portal 一致）。 */
export function splitStatusBadge(status?: string): { text: string; color: string; bg: string } {
  switch (status) {
    case 'SUCCESS':
      return { text: '已分账', color: '#047857', bg: 'rgba(16,185,129,0.14)' };
    case 'PARTIAL':
      return { text: '部分分账', color: '#b45309', bg: 'rgba(245,158,11,0.14)' };
    case 'FAILED':
      return { text: '分账失败', color: '#b91c1c', bg: 'rgba(239,68,68,0.14)' };
    default:
      return { text: '未分账', color: '#64748b', bg: 'rgba(100,116,139,0.14)' };
  }
}

// ---- 差错监控（管理端跨商户）----
export interface SplitExceptionItem {
  order_no: string;
  merchant_id: number;
  total_amount: number;
  receiver_count: number;
  success_count: number;
  status: string;
  attempt_count: number;
  next_retry_at?: string;
  degraded: number;
  last_error?: string;
  created_at: string;
  updated_at: string;
}

export interface SplitExceptionPage {
  items: SplitExceptionItem[];
  total: number;
  page: number;
  size: number;
}

export async function listSplitExceptions(params: {
  status?: string;
  degraded?: number;
  page?: number;
  page_size?: number;
}): Promise<SplitExceptionPage> {
  return get<SplitExceptionPage>('/v1/admin/split/exceptions', { params });
}

export async function resolveSplitExecution(orderNo: string, note?: string): Promise<{ order_no: string; resolved: boolean }> {
  return post<{ order_no: string; resolved: boolean }>(`/v1/admin/split/executions/${orderNo}/resolve`, { note });
}

export async function reopenSplitExecution(orderNo: string): Promise<{ order_no: string; reopened: boolean }> {
  return post<{ order_no: string; reopened: boolean }>(`/v1/admin/split/executions/${orderNo}/reopen`);
}

export async function resolveReconcileDiff(id: number): Promise<{ id: number; resolved: boolean }> {
  return post<{ id: number; resolved: boolean }>(`/v1/admin/reconcile-diffs/${id}/resolve`);
}
