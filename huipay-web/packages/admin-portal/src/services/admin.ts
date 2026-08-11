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
