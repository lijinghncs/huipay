// 商家用户服务（骨架：硬编码）
import type { Order, Wallet, JournalEntry } from '@huipay/shared';

/** 当前登录用户（骨架：固定值）。 */
export async function getCurrentUser(): Promise<{ id: number; name: string; role: string; merchantId: number }> {
  return { id: 1, name: '演示商户', role: 'merchant_admin', merchantId: 10001 };
}

/** 拉取订单列表（骨架）。 */
export async function listOrders(_params?: { page?: number; size?: number }): Promise<{ items: Order[]; total: number }> {
  return { items: [], total: 0 };
}

/** 拉取钱包（骨架）。 */
export async function getWallet(entityId: number): Promise<Wallet | null> {
  return null;
}

/** 拉取账本流水（骨架）。 */
export async function listEntries(_entityId: number): Promise<JournalEntry[]> {
  return [];
}