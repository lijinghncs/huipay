// 管理后台布局
import React, { useState } from 'react';
import { Layout, Menu, Breadcrumb, Avatar, Dropdown, Space, theme, Drawer, Grid, Button } from 'antd';
import {
  DashboardOutlined,
  ShopOutlined,
  CreditCardOutlined,
  SafetyCertificateOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MenuOutlined,
  UserOutlined,
  LogoutOutlined,
  DownOutlined,
  BarChartOutlined,
  BranchesOutlined,
} from '@ant-design/icons';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { clearToken } from '../services/auth';

const { Sider, Content, Header } = Layout;

const menuItems = [
  { key: '/', icon: <DashboardOutlined />, label: '概览' },
  { key: '/merchants', icon: <ShopOutlined />, label: '商户管理' },
  { key: '/channels', icon: <CreditCardOutlined />, label: '通道配置' },
  { key: '/risk-rules', icon: <SafetyCertificateOutlined />, label: '风控规则' },
  // V2 合并版：门店按日统计 + 分账管理
  { key: '/store-stats', icon: <BarChartOutlined />, label: '门店统计' },
  { key: '/split-manage', icon: <BranchesOutlined />, label: '分账管理' },
];

const breadcrumbMap: Record<string, string> = {
  '/': '概览',
  '/merchants': '商户管理',
  '/channels': '通道配置',
  '/risk-rules': '风控规则',
  '/store-stats': '门店统计',
  '/split-manage': '分账管理',
};

// 由路径解析「当前菜单 key」与「页面标题」（子页面归入所属一级菜单）
function resolvePage(pathname: string): { menuKey: string; title: string } {
  if (pathname.startsWith('/merchants/')) {
    const sub = pathname.split('/')[2]; // :id 或 ':id'
    if (pathname.endsWith('/wechat-config')) {
      return { menuKey: '/merchants', title: '微信支付配置' };
    }
    if (sub && /^\d+$/.test(sub)) {
      return { menuKey: '/merchants', title: '商户详情' };
    }
  }
  return { menuKey: breadcrumbMap[pathname] ? pathname : '/', title: breadcrumbMap[pathname] ?? '概览' };
}

const { useBreakpoint } = Grid;

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
  const screens = useBreakpoint();
  const isMobile = !screens.md;
  const [collapsed, setCollapsed] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const { token } = theme.useToken();

  const { menuKey, title: currentPage } = resolvePage(loc.pathname);

  const logout = () => {
    clearToken();
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
          background: '#0e1a33',
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
              fontWeight: 800,
              fontSize: 16,
              color: '#fff',
            }}
          >
            汇
          </span>
          <span style={{ fontSize: 16, fontWeight: 700, letterSpacing: 0.5 }}>
            汇聚付
            <span style={{ marginLeft: 8, fontSize: 12, fontWeight: 400, color: 'rgba(255,255,255,0.55)' }}>管理后台</span>
          </span>
        </Space>
        <Dropdown
          menu={{
            items: [
              { key: 'profile', icon: <UserOutlined />, label: '个人中心' },
              { type: 'divider' },
              { key: 'logout', icon: <LogoutOutlined />, label: '退出登录' },
            ],
            onClick: ({ key }) => {
              if (key === 'logout') logout();
            },
          }}
        >
          <Space style={{ cursor: 'pointer', color: 'rgba(255,255,255,0.9)' }} size={8}>
            <Avatar size={30} style={{ background: '#1e6fff' }} icon={<UserOutlined />} />
            <span>平台管理员</span>
            <DownOutlined style={{ fontSize: 11 }} />
          </Space>
        </Dropdown>
      </Header>

      <Layout>
        {!isMobile && (
          <Sider
            width={220}
            collapsedWidth={64}
            collapsed={collapsed}
            theme="dark"
            style={{
              background: '#0e1a33',
              boxShadow: '1px 0 0 rgba(16,24,40,0.06)',
              display: 'flex',
              flexDirection: 'column',
            }}
          >
            <div style={{ flex: 1, overflowY: 'auto', overflowX: 'hidden' }}>
              <MenuPanel selectedKey={menuKey} onSelect={(k) => nav(k)} />
            </div>
            {!collapsed && <div className="hp-sider-footer">汇聚付 v1.0.0</div>}
          </Sider>
        )}

        {isMobile && (
          <Drawer
            placement="left"
            width={220}
            open={drawerOpen}
            onClose={() => setDrawerOpen(false)}
            styles={{ body: { padding: 0, background: '#0e1a33' } }}
            closable={false}
          >
            <MenuPanel
              selectedKey={menuKey}
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
                  style={{
                    cursor: 'pointer',
                    fontSize: 16,
                    color: token.colorTextSecondary,
                    display: 'inline-flex',
                  }}
                >
                  {isMobile ? <MenuOutlined /> : collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
                </span>
                <span style={{ fontSize: 18, fontWeight: 700 }}>{currentPage}</span>
              </Space>
              {!isMobile && (
                <Breadcrumb
                  items={[
                    { title: '管理后台' },
                    ...(menuKey !== '/' && menuKey !== loc.pathname
                      ? [{ title: breadcrumbMap[menuKey] ?? menuKey, href: menuKey }]
                      : []),
                    { title: currentPage },
                  ]}
                />
              )}
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