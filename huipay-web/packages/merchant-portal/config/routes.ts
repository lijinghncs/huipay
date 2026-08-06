// 路由配置（AntD Pro 风格）
import type { MenuDataItem } from '@ant-design/pro-components';

const routes: MenuDataItem[] = [
  { path: '/', name: '概览', icon: 'DashboardOutlined' },
  { path: '/transactions', name: '交易', icon: 'TransactionOutlined' },
  { path: '/wallets', name: '钱包', icon: 'WalletOutlined' },
  { path: '/split-rules', name: '分账规则', icon: 'BranchesOutlined' },
];

export default routes;