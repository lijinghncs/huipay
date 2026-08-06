// 商家工作台布局
import React from 'react';
import { Layout, Menu } from 'antd';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { DashboardOutlined, TransactionOutlined, WalletOutlined, BranchesOutlined } from '@ant-design/icons';

const { Sider, Content, Header } = Layout;

const menuItems = [
  { key: '/', icon: <DashboardOutlined />, label: '概览' },
  { key: '/transactions', icon: <TransactionOutlined />, label: '交易' },
  { key: '/wallets', icon: <WalletOutlined />, label: '钱包' },
  { key: '/split-rules', icon: <BranchesOutlined />, label: '分账规则' },
];

export const BasicLayout: React.FC = () => {
  const nav = useNavigate();
  const loc = useLocation();
  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ background: '#0e1a2b', color: '#fff', padding: '0 24px', fontWeight: 700 }}>
        汇聚付 · 商家工作台
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