// 商家工作台布局
import React from 'react';
import { App as AntApp, Button, Layout, Menu, Space } from 'antd';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { DashboardOutlined, TransactionOutlined, QrcodeOutlined, WalletOutlined, BranchesOutlined } from '@ant-design/icons';
import { clearToken } from '../services/auth';

const { Sider, Content, Header } = Layout;

const menuItems = [
  { key: '/', icon: <DashboardOutlined />, label: '概览' },
  { key: '/transactions', icon: <TransactionOutlined />, label: '交易' },
  { key: '/codes', icon: <QrcodeOutlined />, label: '收款码' },
  { key: '/wallets', icon: <WalletOutlined />, label: '钱包' },
  { key: '/split-rules', icon: <BranchesOutlined />, label: '分账规则' },
];

export const BasicLayout: React.FC = () => {
  const nav = useNavigate();
  const loc = useLocation();
  const { message } = AntApp.useApp();

  const logout = () => {
    clearToken();
    message.success('已退出登录');
    nav('/login', { replace: true });
  };

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ background: '#0e1a2b', color: '#fff', padding: '0 24px', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <span style={{ fontWeight: 700 }}>汇聚付 · 商家工作台</span>
        <Space>
          <Button ghost size="small" onClick={logout}>
            退出登录
          </Button>
        </Space>
      </Header>
      <Layout>
        <Sider width={220} theme="light">
          <Menu
            mode="inline"
            selectedKeys={[loc.pathname]}
            items={menuItems}
            onClick={({ key }) => nav(key as string)}
          />
        </Sider>
        <Content style={{ padding: 24, background: '#f6f7fb' }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
};
