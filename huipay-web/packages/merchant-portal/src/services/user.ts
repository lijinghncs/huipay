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

/** 拉取账本流水。 */
export async function listEntries(entityId: number): Promise<{ items: JournalEntry[] }> {
  return get<{ items: JournalEntry[] }>(`/v1/wallets/${entityId}/entries`);
}