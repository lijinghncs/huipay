// 管理后台登录态：登录 API + token 本地存储
import { post } from '@huipay/shared/api-client';

const TOKEN_KEY = 'huipay_admin_token';

export interface AdminLoginResult {
  token: string;
  admin: { id: number; username: string };
}

export async function adminLogin(username: string, password: string): Promise<AdminLoginResult> {
  return post<AdminLoginResult>('/v1/admin/login', { username, password });
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

export function isLoggedIn(): boolean {
  return !!getToken();
}