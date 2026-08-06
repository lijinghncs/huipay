// 管理后台布局
import React from 'react';
import { Layout, Menu } from 'antd';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import {
  DashboardOutlined,
  ShopOutlined,
  CreditCardOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';

const { Sider, Content, Header } = Layout;

const menuItems = [
  { key: '/', icon: <DashboardOutlined />, label: '概览' },
  { key: '/merchants', icon: <ShopOutlined />, label: '商户管理' },
  { key: '/channels', icon: <CreditCardOutlined />, label: '通道配置' },
  { key: '/risk-rules', icon: <SafetyCertificateOutlined />, label: '风控规则' },
];

export const BasicLayout: React.FC = () => {
  const nav = useNavigate();
  const loc = useLocation();
  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ background: '#142746', color: '#fff', padding: '0 24px', fontWeight: 700 }}>
        汇聚付 · 管理后台
      </Header>
      <Layout>
        <Sider width={220} theme="light">
          <Menu mode="inline" selectedKeys={[loc.pathname]} items={menuItems} onClick={({ key }) => nav(key as string)} />
        </Sider>
        <Content style={{ padding: 24, background: '#f6f7fb' }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
};