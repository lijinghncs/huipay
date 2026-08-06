// 路由配置
import type { MenuDataItem } from '@ant-design/pro-components';

const routes: MenuDataItem[] = [
  { path: '/', name: '概览', icon: 'DashboardOutlined' },
  { path: '/merchants', name: '商户管理', icon: 'ShopOutlined' },
  { path: '/channels', name: '通道配置', icon: 'CreditCardOutlined' },
  { path: '/risk-rules', name: '风控规则', icon: 'SafetyCertificateOutlined' },
];

export default routes;