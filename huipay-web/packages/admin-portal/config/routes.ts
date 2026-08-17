// 路由配置
import type { MenuDataItem } from '@ant-design/pro-components';

const routes: MenuDataItem[] = [
  { path: '/', name: '概览', icon: 'DashboardOutlined' },
  { path: '/merchants', name: '商户管理', icon: 'ShopOutlined' },
  { path: '/channels', name: '通道配置', icon: 'CreditCardOutlined' },
  { path: '/risk-rules', name: '风控规则', icon: 'SafetyCertificateOutlined' },
  // V2 合并版：门店按日统计（含分账字段）+ 分账管理
  { path: '/store-stats', name: '门店统计', icon: 'BarChartOutlined' },
  { path: '/split-manage', name: '分账管理', icon: 'BranchesOutlined' },
];

export default routes;