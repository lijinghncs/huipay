// 管理后台 RBAC
export default function access(initialState?: { currentUser?: { role: string } }) {
  const role = initialState?.currentUser?.role ?? 'admin';
  return {
    canViewMerchants: ['admin', 'operator'].includes(role),
    canEditChannels: role === 'admin',
    canEditRiskRules: ['admin', 'risk'].includes(role),
    canViewAnalytics: true,
  };
}