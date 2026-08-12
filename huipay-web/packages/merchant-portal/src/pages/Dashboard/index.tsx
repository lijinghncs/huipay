// Dashboard 概览页：当前商户资料 + 经营概览 KPI
import React from 'react';
import { Card, Col, Row, Skeleton, Typography } from 'antd';
import { useQuery } from '@tanstack/react-query';
import {
  ShopOutlined,
  IdcardOutlined,
  WalletOutlined,
  SafetyCertificateOutlined,
  CreditCardOutlined,
  LockOutlined,
  OrderedListOutlined,
  QrcodeOutlined,
} from '@ant-design/icons';
import { getMerchantOverview, getMerchantProfile } from '../../services/user';
import { KpiCard } from '../../components/KpiCard';

const formatCents = (cents: number): string => `¥${(Number(cents) / 100).toFixed(2)}`;

// 资料字段单行
const Field: React.FC<{ icon: React.ReactNode; label: string; children?: React.ReactNode }> = ({ icon, label, children }) => (
  <div className="hp-info-field">
    <span className="hp-info-icon">{icon}</span>
    <div>
      <div className="hp-info-label">{label}</div>
      <div className="hp-info-value">{children ?? '-'}</div>
    </div>
  </div>
);

export const Dashboard: React.FC = () => {
  const profile = useQuery({ queryKey: ['merchant', 'profile'], queryFn: getMerchantProfile });
  const overview = useQuery({ queryKey: ['merchant', 'overview'], queryFn: getMerchantOverview });

  const loading = profile.isLoading || overview.isLoading;
  const p = profile.data;

  type KpiItem = { title: string; value: number; formatter?: (v: number) => string; icon: React.ReactNode; color: string };
  const kpis: KpiItem[] = [
    { title: '钱包余额', value: overview.data?.balance ?? 0, formatter: formatCents, icon: <WalletOutlined />, color: '#1e6fff' },
    { title: '累计实付', value: overview.data?.total_paid ?? 0, formatter: formatCents, icon: <CreditCardOutlined />, color: '#06b6a4' },
    { title: '订单总数', value: overview.data?.order_count ?? 0, icon: <OrderedListOutlined />, color: '#8b5cf6' },
    { title: '可用收款码', value: overview.data?.active_code_count ?? 0, icon: <QrcodeOutlined />, color: '#ec4899' },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card className="hp-info-card" title="商户资料" size="small">
        {profile.isLoading ? (
          <Skeleton active paragraph={{ rows: 3 }} />
        ) : profile.isError ? (
          <Typography.Text type="danger">加载失败，请稍后重试</Typography.Text>
        ) : (
          <Row gutter={[24, 8]}>
            <Col xs={24} md={12}>
              <Field icon={<ShopOutlined />} label="商户名称">{p?.name}</Field>
              <Field icon={<IdcardOutlined />} label="商户号">{p?.entity_code}</Field>
              <Field icon={<WalletOutlined />} label="钱包号">{p?.wallet_no}</Field>
            </Col>
            <Col xs={24} md={12}>
              <Field icon={<SafetyCertificateOutlined />} label="状态">
                {p?.status === 1 ? (
                  <span style={{ color: '#06b6a4' }}>正常</span>
                ) : (
                  <span style={{ color: '#f5222d' }}>停用</span>
                )}
              </Field>
              <Field icon={<CreditCardOutlined />} label="钱包余额">{formatCents(p?.balance ?? 0)}</Field>
              <Field icon={<LockOutlined />} label="冻结金额">{formatCents(p?.frozen ?? 0)}</Field>
            </Col>
          </Row>
        )}
      </Card>

      <Row gutter={[16, 16]}>
        {kpis.map((k) => (
          <Col xs={12} sm={12} md={6} key={k.title}>
            <KpiCard title={k.title} value={k.value} icon={k.icon} color={k.color} loading={loading} formatter={k.formatter} />
          </Col>
        ))}
      </Row>
    </div>
  );
};