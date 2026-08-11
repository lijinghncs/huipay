// 商户登录态：登录 API + token 本地存储 + merchant_id 解码
import { post } from '@huipay/shared/api-client';

const TOKEN_KEY = 'huipay_merchant_token';

export interface MerchantLoginResult {
  token: string;
  merchant: {
    id: number;
    entity_code: string;
    name: string;
    wallet_no?: string;
  };
}

export async function merchantLogin(phone: string, password: string): Promise<MerchantLoginResult> {
  return post<MerchantLoginResult>('/v1/auth/merchant/login', { phone, password });
}

export function saveToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? '';
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

/** 从 JWT payload 本地解码 merchant_id（仅读取，不校验签名）。 */
export function merchantIdFromToken(): number {
  const token = getToken();
  if (!token) return 0;
  try {
    const raw = token.split('.')[1] ?? '';
    const json = atob(raw.replace(/-/g, '+').replace(/_/g, '/'));
    const payload = JSON.parse(json) as { merchant_id?: number };
    return Number(payload.merchant_id ?? 0);
  } catch {
    return 0;
  }
}
