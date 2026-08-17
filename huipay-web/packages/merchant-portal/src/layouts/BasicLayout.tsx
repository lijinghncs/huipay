// 商家工作台布局：深色渐变顶部栏 + 分组侧边栏 + 响应式折叠
import React, { useState } from 'react';
import {
  App as AntApp,
  Avatar,
  Breadcrumb,
  Button,
  Drawer,
  Grid,
  Layout,
  Menu,
  Space,
  Tooltip,
  theme,
} from 'antd';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import {
  AlertOutlined,
  BranchesOutlined,
  ClockCircleOutlined,
  DashboardOutlined,
  FileTextOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuOutlined,
  MenuUnfoldOutlined,
  NodeIndexOutlined,
  QrcodeOutlined,
  ShopOutlined,
  TransactionOutlined,
  WalletOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { clearToken } from '../services/auth';
import { getMerchantProfile } from '../services/user';

const { Sider, Content, Header } = Layout;
const { useBreakpoint } = Grid;

const MONO = { fontVariantNumeric: 'tabular-nums' as const, fontFamily: 'Fira Code, Consolas, Monaco, monospace' };

const menuItems = [
  {
    type: 'group' as const,
    label: '经营',
    children: [
      { key: '/', icon: <DashboardOutlined />, label: '概览' },
      { key: '/transactions', icon: <TransactionOutlined />, label: '交易明细' },
      { key: '/store-stats', icon: <ShopOutlined />, label: '门店交易统计' },
    ],
  },
  {
    type: 'group' as const,
    label: '收银与资金',
    children: [
      { key: '/codes', icon: <QrcodeOutlined />, label: '收款码' },
      { key: '/stores', icon: <ShopOutlined />, label: '门店' },
      { key: '/wallets', icon: <WalletOutlined />, label: '钱包' },
    ],
  },
  {
    type: 'group' as const,
    label: '分账管理',
    children: [
      { key: '/split-rules', icon: <BranchesOutlined />, label: '分账规则' },
      { key: '/split-bills', icon: <FileTextOutlined />, label: '分账单' },
      { key: '/splits', icon: <NodeIndexOutlined />, label: '分账明细' },
      { key: '/split-exceptions', icon: <AlertOutlined />, label: '差错中心' },
    ],
  },
  {
    type: 'group' as const,
    label: '系统',
    children: [{ key: '/scheduler', icon: <ClockCircleOutlined />, label: '定时任务' }],
  },
];

const titleMap: Record<string, string> = {
  '/': '概览',
  '/transactions': '交易明细',
  '/store-stats': '门店交易统计',
  '/scheduler': '定时任务',
  '/codes': '收款码',
  '/stores': '门店',
  '/wallets': '钱包',
  '/split-rules': '分账规则',
  '/split-bills': '分账单',
  '/splits': '分账明细',
  '/split-exceptions': '差错中心',
};

interface MenuPanelProps {
  selectedKey: string;
  collapsed?: boolean;
  merchantName?: string;
  merchantCode?: string;
  onSelect: (key: string) => void;
}

const MenuPanel: React.FC<MenuPanelProps> = ({ selectedKey, collapsed, merchantName, merchantCode, onSelect }) => (
  <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
    {/* 商户信息卡 */}
    <div className={`hp-sider-merchant${collapsed ? ' is-collapsed' : ''}`}>
      <Avatar
        size={36}
        style={{
          background: 'linear-gradient(135deg, #1e6fff, #06b6a4)',
          fontWeight: 700,
          fontSize: 15,
          flexShrink: 0,
        }}
      >
        {(merchantName || '商').slice(0, 1)}
      </Avatar>
      {!collapsed && (
        <div className="hp-sider-merchant-info">
          <div className="hp-sider-merchant-name">{merchantName || '商户'}</div>
          {merchantCode && <div className="hp-sider-merchant-code">{merchantCode}</div>}
        </div>
      )}
    </div>

    <div style={{ flex: 1, overflowY: 'auto', overflowX: 'hidden' }}>
      <Menu
        className="hp-sider"
        mode="inline"
        theme="dark"
        selectedKeys={[selectedKey]}
        items={menuItems}
        onClick={({ key }) => onSelect(key as string)}
        style={{ borderInlineEnd: 'none', padding: '4px 0 12px', background: 'transparent' }}
      />
    </div>

    {!collapsed && (
      <div className="hp-sider-footer">
        <span className="hp-sider-footer-dot" />
        汇聚付商户端 v1.0.0
      </div>
    )}
  </div>
);

export const BasicLayout: React.FC = () => {
  const nav = useNavigate();
  const loc = useLocation();
  const { message } = AntApp.useApp();
  const screens = useBreakpoint();
  const isMobile = !screens.md;
  const [collapsed, setCollapsed] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const { token } = theme.useToken();

  const profile = useQuery({ queryKey: ['merchant', 'profile'], queryFn: getMerchantProfile });

  const currentPage = loc.pathname.startsWith('/stores/') ? '门店详情' : (titleMap[loc.pathname] ?? '概览');

  const logout = () => {
    clearToken();
    message.success('已退出登录');
    nav('/login', { replace: true });
  };

  const toggleNav = () => (isMobile ? setDrawerOpen(true) : setCollapsed((c) => !c));

  return (
    <Layout style={{ minHeight: '100vh' }}>
      {/* 顶部栏 */}
      <Header
        className="hp-header"
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '0 20px',
          background: 'linear-gradient(90deg, #0e1a2b 0%, #152642 100%)',
          boxShadow: '0 2px 14px rgba(14,26,43,0.32)',
          position: 'sticky',
          top: 0,
          zIndex: 100,
        }}
      >
        <Space size={12} align="center">
          <span className="hp-logo" aria-hidden>
            <ShopOutlined />
          </span>
          <span className="hp-brand">
            汇聚付
            <span className="hp-brand-sub">商家工作台</span>
          </span>
        </Space>

        <Space size={14} align="center">
          {profile.data?.name && (
            <span className="hp-header-merchant">
              <span className="hp-header-merchant-name">{profile.data.name}</span>
              {profile.data.entity_code && (
                <span className="hp-header-merchant-code" style={MONO}>
                  {profile.data.entity_code}
                </span>
              )}
            </span>
          )}
          <Tooltip title="退出登录">
            <Button
              className="hp-header-logout"
              ghost
              size="small"
              icon={<LogoutOutlined />}
              onClick={logout}
              style={{ borderColor: 'rgba(255,255,255,0.22)', color: 'rgba(255,255,255,0.82)' }}
            >
              退出
            </Button>
          </Tooltip>
        </Space>
      </Header>

      <Layout>
        {/* 桌面端侧边栏 */}
        {!isMobile && (
          <Sider
            width={216}
            collapsedWidth={72}
            collapsed={collapsed}
            theme="dark"
            className="hp-sider-shell"
            style={{
              background: 'linear-gradient(180deg, #0e1a2b 0%, #12213a 100%)',
              boxShadow: '1px 0 0 rgba(16,24,40,0.06)',
            }}
          >
            <MenuPanel
              selectedKey={loc.pathname}
              collapsed={collapsed}
              merchantName={profile.data?.name}
              merchantCode={profile.data?.entity_code}
              onSelect={(k) => nav(k)}
            />
          </Sider>
        )}

        {/* 移动端抽屉导航 */}
        {isMobile && (
          <Drawer
            placement="left"
            width={232}
            open={drawerOpen}
            onClose={() => setDrawerOpen(false)}
            styles={{ body: { padding: 0, background: 'linear-gradient(180deg, #0e1a2b 0%, #12213a 100%)' } }}
            closable={false}
          >
            <MenuPanel
              selectedKey={loc.pathname}
              merchantName={profile.data?.name}
              merchantCode={profile.data?.entity_code}
              onSelect={(k) => {
                nav(k);
                setDrawerOpen(false);
              }}
            />
          </Drawer>
        )}

        <Layout style={{ background: token.colorBgLayout }}>
          <Content style={{ padding: isMobile ? 12 : 20 }}>
            {/* 页内工具栏：折叠 + 标题 + 面包屑 */}
            <div className="hp-toolbar">
              <Space size={12} align="center">
                <Tooltip title={isMobile ? '打开菜单' : collapsed ? '展开菜单' : '收起菜单'}>
                  <span className="hp-toolbar-toggle" onClick={toggleNav} role="button" aria-label="切换菜单">
                    {isMobile ? <MenuOutlined /> : collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
                  </span>
                </Tooltip>
                <span className="hp-toolbar-title">{currentPage}</span>
              </Space>
              {!isMobile && <Breadcrumb items={[{ title: '商家工作台' }, { title: currentPage }]} />}
            </div>

            <div className="hp-page">
              <Outlet />
            </div>
          </Content>
        </Layout>
      </Layout>
    </Layout>
  );
};
