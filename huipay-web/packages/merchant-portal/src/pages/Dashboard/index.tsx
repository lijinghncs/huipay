// Dashboard 概览页：欢迎 Hero + 经营 KPI + 账户信息 + 快捷入口
import React from 'react';
import { Card, Col, Row, Skeleton, Typography } from 'antd';
import { useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  ArrowRightOutlined,
  BranchesOutlined,
  CalendarOutlined,
  CreditCardOutlined,
  FileTextOutlined,
  IdcardOutlined,
  LockOutlined,
  OrderedListOutlined,
  PayCircleOutlined,
  QrcodeOutlined,
  SafetyCertificateOutlined,
  ShopOutlined,
  TransactionOutlined,
  WalletOutlined,
} from '@ant-design/icons';
import { getMerchantOverview, getMerchantProfile } from '../../services/user';
import { KpiCard } from '../../components/KpiCard';

const formatCents = (cents: number): string => `¥${(Number(cents) / 100).toFixed(2)}`;

/** 按当前时段生成问候语。 */
const greeting = (): string => {
  const h = new Date().getHours();
  if (h < 6) return '夜深了';
  if (h < 9) return '早上好';
  if (h < 12) return '上午好';
  if (h < 14) return '中午好';
  if (h < 18) return '下午好';
  return '晚上好';
};

/** 今日日期 + 星期。 */
const todayText = (): string => {
  const weeks = ['日', '一', '二', '三', '四', '五', '六'];
  const d = new Date();
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日 星期${weeks[d.getDay()]}`;
};

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

// 区块标题
const SectionTitle: React.FC<{ icon: React.ReactNode; children: React.ReactNode }> = ({ icon, children }) => (
  <div className="hp-split-section-title">
    {icon}
    <span>{children}</span>
  </div>
);

export const Dashboard: React.FC = () => {
  const navigate = useNavigate();
  const profile = useQuery({ queryKey: ['merchant', 'profile'], queryFn: getMerchantProfile });
  const overview = useQuery({ queryKey: ['merchant', 'overview'], queryFn: getMerchantOverview });

  const loading = profile.isLoading || overview.isLoading;
  const p = profile.data;

  type KpiItem = { title: string; value: number; formatter?: (v: number) => string; icon: React.ReactNode; color: string };
  const kpis: KpiItem[] = [
    { title: '累计实付', value: overview.data?.total_paid ?? 0, formatter: formatCents, icon: <CreditCardOutlined />, color: '#06b6a4' },
    { title: '订单总数', value: overview.data?.order_count ?? 0, icon: <OrderedListOutlined />, color: '#8b5cf6' },
    { title: '已支付订单', value: overview.data?.paid_order_count ?? 0, icon: <PayCircleOutlined />, color: '#f59e0b' },
    { title: '可用收款码', value: overview.data?.active_code_count ?? 0, icon: <QrcodeOutlined />, color: '#ec4899' },
  ];

  const entries = [
    { key: '/codes', icon: <QrcodeOutlined />, title: '收款码', desc: '管理收款码与门店绑定', color: '#1e6fff' },
    { key: '/transactions', icon: <TransactionOutlined />, title: '交易明细', desc: '查询订单与支付流水', color: '#06b6a4' },
    { key: '/split-bills', icon: <FileTextOutlined />, title: '分账单', desc: '生成与审批分账批次', color: '#8b5cf6' },
    { key: '/store-stats', icon: <ShopOutlined />, title: '门店统计', desc: '门店每日经营数据', color: '#ec4899' },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
      {/* 欢迎 Hero */}
      <div className="hp-dash-hero">
        <div className="hp-dash-hero-main">
          <div className="hp-dash-hero-hello">
            {greeting()}，{p?.name ?? '商户'}
          </div>
          <div className="hp-dash-hero-meta">
            {p?.entity_code && <span className="hp-dash-hero-code">{p.entity_code}</span>}
            <span className={`hp-dash-hero-status${p?.status === 1 ? ' is-ok' : ' is-off'}`}>
              <span className="hp-dash-hero-status-dot" />
              {p?.status === 1 ? '账户正常' : '已停用'}
            </span>
            <span className="hp-dash-hero-date">{todayText()}</span>
          </div>
        </div>
        <div className="hp-dash-hero-balance">
          <div className="hp-dash-hero-balance-label">钱包余额</div>
          <div className="hp-dash-hero-balance-value">{formatCents(p?.balance ?? overview.data?.balance ?? 0)}</div>
          <div className="hp-dash-hero-balance-sub">冻结 {formatCents(p?.frozen ?? 0)}</div>
        </div>
      </div>

      {/* 经营 KPI */}
      <div>
        <SectionTitle icon={<CreditCardOutlined />}>经营概览</SectionTitle>
        <Row gutter={[16, 16]}>
          {kpis.map((k) => (
            <Col xs={12} sm={12} md={6} key={k.title}>
              <KpiCard title={k.title} value={k.value} icon={k.icon} color={k.color} loading={loading} formatter={k.formatter} />
            </Col>
          ))}
        </Row>
      </div>

      {/* 快捷入口 */}
      <div>
        <SectionTitle icon={<TransactionOutlined />}>快捷入口</SectionTitle>
        <Row gutter={[16, 16]}>
          {entries.map((e) => (
            <Col xs={12} md={6} key={e.key}>
              <div className="hp-dash-entry" onClick={() => navigate(e.key)}>
                <span
                  className="hp-dash-entry-icon"
                  style={{ background: `linear-gradient(135deg, ${e.color}, ${e.color}cc)` }}
                >
                  {e.icon}
                </span>
                <div className="hp-dash-entry-text">
                  <div className="hp-dash-entry-title">{e.title}</div>
                  <div className="hp-dash-entry-desc">{e.desc}</div>
                </div>
                <ArrowRightOutlined className="hp-dash-entry-arrow" />
              </div>
            </Col>
          ))}
        </Row>
      </div>

      {/* 账户信息 */}
      <div>
        <SectionTitle icon={<IdcardOutlined />}>账户信息</SectionTitle>
        <Card className="hp-info-card" size="small">
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
                <Field icon={<LockOutlined />} label="冻结金额">{formatCents(p?.frozen ?? 0)}</Field>
                <Field icon={<BranchesOutlined />} label="分账模式">
                  {p?.split_mode === 'LOCAL_ONLY' ? '仅本地记账' : p?.split_mode === 'CHANNEL_REQUIRED' ? '强制通道' : '自动（通道优先）'}
                </Field>
                <Field icon={<CalendarOutlined />} label="开户时间">
                  {p?.created_at ? p.created_at.slice(0, 10) : '-'}
                </Field>
              </Col>
            </Row>
          )}
        </Card>
      </div>
    </div>
  );
};
