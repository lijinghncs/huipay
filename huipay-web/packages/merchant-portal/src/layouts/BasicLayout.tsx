// 商家工作台布局
import React, { useState } from 'react';
import { App as AntApp, Avatar, Breadcrumb, Button, Drawer, Grid, Layout, Menu, Space, theme } from 'antd';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { DashboardOutlined, TransactionOutlined, QrcodeOutlined, WalletOutlined, BranchesOutlined, ShopOutlined, MenuFoldOutlined, MenuUnfoldOutlined, MenuOutlined, LogoutOutlined, BarChartOutlined } from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { clearToken } from '../services/auth';
import { getMerchantProfile } from '../services/user';

const { Sider, Content, Header } = Layout;
const { useBreakpoint } = Grid;

const menuItems = [
  { key: '/', icon: <DashboardOutlined />, label: '概览' },
  { key: '/transactions', icon: <TransactionOutlined />, label: '交易' },
  { key: '/transaction-stats', icon: <BarChartOutlined />, label: '交易统计' },
  { key: '/codes', icon: <QrcodeOutlined />, label: '收款码' },
  { key: '/stores', icon: <ShopOutlined />, label: '门店' },
  { key: '/wallets', icon: <WalletOutlined />, label: '钱包' },
  { key: '/split-rules', icon: <BranchesOutlined />, label: '分账规则' },
];

const titleMap: Record<string, string> = {
  '/': '概览',
  '/transactions': '交易',
  '/transaction-stats': '交易统计',
  '/codes': '收款码',
  '/stores': '门店',
  '/wallets': '钱包',
  '/split-rules': '分账规则',
};

const MenuPanel = ({ selectedKey, onSelect }: { selectedKey: string; onSelect: (key: string) => void }) => (
  <Menu
    className="hp-sider"
    mode="inline"
    theme="dark"
    selectedKeys={[selectedKey]}
    items={menuItems}
    onClick={({ key }) => onSelect(key as string)}
    style={{ borderInlineEnd: 'none', paddingTop: 8, background: 'transparent' }}
  />
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

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '0 16px',
          background: '#0e1a2b',
          color: '#fff',
          boxShadow: '0 1px 4px rgba(16,24,40,0.2)',
          position: 'sticky',
          top: 0,
          zIndex: 10,
        }}
      >
        <Space size={12} style={{ alignItems: 'center' }}>
          <span
            style={{
              width: 30,
              height: 30,
              borderRadius: 9,
              background: 'linear-gradient(135deg,#1e6fff,#06b6a4)',
              boxShadow: '0 2px 8px rgba(30,111,255,0.45)',
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#fff',
            }}
          >
            <ShopOutlined style={{ fontSize: 16 }} />
          </span>
          <span style={{ fontSize: 16, fontWeight: 700, letterSpacing: 0.5 }}>
            汇聚付
            <span style={{ marginLeft: 8, fontSize: 12, fontWeight: 400, color: 'rgba(255,255,255,0.55)' }}>商家工作台</span>
          </span>
        </Space>
        <Space size={12} align="center">
          {profile.data?.name && (
            <span style={{ color: 'rgba(255,255,255,0.85)', fontSize: 13 }}>{profile.data.name}</span>
          )}
          <Avatar size={30} style={{ background: '#1e6fff' }} icon={<ShopOutlined />} />
          <Button ghost size="small" icon={<LogoutOutlined />} onClick={logout} style={{ borderColor: 'rgba(255,255,255,0.3)' }}>
            退出
          </Button>
        </Space>
      </Header>

      <Layout>
        {!isMobile && (
          <Sider
            width={200}
            collapsedWidth={64}
            collapsed={collapsed}
            theme="dark"
            style={{
              background: '#0e1a2b',
              boxShadow: '1px 0 0 rgba(16,24,40,0.06)',
              display: 'flex',
              flexDirection: 'column',
            }}
          >
            <div style={{ flex: 1, overflowY: 'auto', overflowX: 'hidden' }}>
              <MenuPanel selectedKey={loc.pathname} onSelect={(k) => nav(k)} />
            </div>
            {!collapsed && <div className="hp-sider-footer">汇聚付商户端 v1.0.0</div>}
          </Sider>
        )}

        {isMobile && (
          <Drawer
            placement="left"
            width={200}
            open={drawerOpen}
            onClose={() => setDrawerOpen(false)}
            styles={{ body: { padding: 0, background: '#0e1a2b' } }}
            closable={false}
          >
            <MenuPanel
              selectedKey={loc.pathname}
              onSelect={(k) => {
                nav(k);
                setDrawerOpen(false);
              }}
            />
          </Drawer>
        )}

        <Layout style={{ background: token.colorBgLayout }}>
          <Content style={{ padding: isMobile ? 12 : 20 }}>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                marginBottom: 16,
              }}
            >
              <Space size={12} align="center">
                <span
                  onClick={() => (isMobile ? setDrawerOpen(true) : setCollapsed((c) => !c))}
                  style={{ cursor: 'pointer', fontSize: 16, color: token.colorTextSecondary, display: 'inline-flex' }}
                >
                  {isMobile ? <MenuOutlined /> : collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
                </span>
                <span style={{ fontSize: 18, fontWeight: 700 }}>{currentPage}</span>
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