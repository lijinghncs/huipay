// 商家工作台 RBAC 权限点（（与 AntD Pro 6 access.ts 对齐）
export default function access(initialState?: { currentUser?: { role: string } }) {
  const role = initialState?.currentUser?.role ?? 'merchant';
  return {
    canViewTransactions: true,
    canViewWallets: role !== 'guest',
    canEditSplitRules: role === 'merchant_admin',
    canWithdraw: role === 'merchant_admin' || role === 'merchant_finance',
  };
}