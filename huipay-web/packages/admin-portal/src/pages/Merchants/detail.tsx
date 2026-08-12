// 商户详情子页：品牌渐变页头 + 经营概览 KPI + 分栏信息卡（复用全局 hp-* 设计语言）
import React from 'react';
import { useQuery } from '@tanstack/react-query';
import { Card, Skeleton, Result, Button, Space, Row, Col, Alert, Tag } from 'antd';
import {
  ArrowLeftOutlined,
  SettingOutlined,
  ShopOutlined,
  IdcardOutlined,
  ApartmentOutlined,
  WalletOutlined,
  UserOutlined,
  CreditCardOutlined,
  BankOutlined,
  PhoneOutlined,
  SafetyCertificateOutlined,
  GlobalOutlined,
  KeyOutlined,
  FileProtectOutlined,
  OrderedListOutlined,
  QrcodeOutlined,
  LockOutlined,
} from '@ant-design/icons';
import { useNavigate, useParams } from 'react-router-dom';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import { getMerchant, getMerchantOverview } from '../../services/admin';
import { typeText, MerchantStatusTag, WechatEnabledTag, ConfiguredTag, Mono } from './shared';
import { KpiCard } from '../../components/KpiCard';

// 信息区单行：图标 + 标签 + 值
const InfoRow: React.FC<{ icon: React.ReactNode; label: string; children?: React.ReactNode }> = ({
  icon,
  label,
  children,
}) => (
  <div className="hp-info-field">
    <span className="hp-info-icon">{icon}</span>
    <div>
      <div className="hp-info-label">{label}</div>
      <div className="hp-info-value">{children ?? '-'}</div>
    </div>
  </div>
);

export const MerchantDetailPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const merchantId = Number(id);
  const nav = useNavigate();

  const detailQuery = useQuery({
    queryKey: ['merchant-detail', merchantId],
    queryFn: () => getMerchant(merchantId),
    enabled: !!merchantId,
  });
  const overviewQuery = useQuery({
    queryKey: ['merchant-overview', merchantId],
    queryFn: () => getMerchantOverview(merchantId),
    enabled: !!merchantId,
  });

  if (detailQuery.isLoading || overviewQuery.isLoading) {
    return <Card><Skeleton active paragraph={{ rows: 8 }} /></Card>;
  }
  if (detailQuery.isError || !detailQuery.data) {
    return (
      <Result
        status="404"
        title="商户不存在"
        subTitle={detailQuery.error?.message ?? '未找到该商户，可能已被删除'}
        extra={
          <Button type="primary" icon={<ArrowLeftOutlined />} onClick={() => nav('/merchants')}>
            返回商户列表
          </Button>
        }
      />
    );
  }

  const d = detailQuery.data;
  const ov = overviewQuery.data;
  const wc = d.wechat_config;

  type KpiItem = { title: string; value: number; formatter?: (v: number) => string; icon: React.ReactNode; color: string };
  const kpis: KpiItem[] = [
    { title: '钱包余额', value: ov?.balance ?? 0, formatter: (v: number) => formatCents(v), icon: <WalletOutlined />, color: '#1e6fff' },
    { title: '累计实付', value: ov?.total_paid ?? 0, formatter: (v: number) => formatCents(v), icon: <CreditCardOutlined />, color: '#06b6a4' },
    { title: '已支付订单', value: ov?.paid_order_count ?? 0, icon: <SafetyCertificateOutlined />, color: '#f59e0b' },
    { title: '订单总数', value: ov?.order_count ?? 0, icon: <OrderedListOutlined />, color: '#8b5cf6' },
    { title: '可用码牌', value: ov?.active_code_count ?? 0, icon: <QrcodeOutlined />, color: '#ec4899' },
    { title: '冻结金额', value: ov?.frozen ?? 0, formatter: (v: number) => formatCents(v), icon: <LockOutlined />, color: '#64748b' },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* 品牌渐变页头 */}
      <Card className="hp-detail-hero">
        <Space style={{ display: 'flex', justifyContent: 'space-between', width: '100%' }} align="center" wrap>
          <Space size={12} align="center" wrap>
            <Button icon={<ArrowLeftOutlined />} onClick={() => nav('/merchants')} style={{ background: 'rgba(255,255,255,0.12)', border: '1px solid rgba(255,255,255,0.2)', color: '#fff' }}>
              返回
            </Button>
            <Space size={10} align="baseline" wrap>
              <span style={{ fontSize: 20, fontWeight: 800, color: '#fff' }}>{d.name}</span>
              <Tag style={{ borderRadius: 10, border: '1px solid rgba(255,255,255,0.25)', color: '#fff', background: 'rgba(255,255,255,0.12)' }}>
                {typeText(d.entity_type)}
              </Tag>
              {d.status === 1 ? (
                <Tag color="success" style={{ borderRadius: 10 }}>启用</Tag>
              ) : (
                <Tag color="error" style={{ borderRadius: 10 }}>停用</Tag>
              )}
            </Space>
          </Space>
          <Button type="primary" icon={<SettingOutlined />} onClick={() => nav(`/merchants/${d.id}/wechat-config`)}>
            微信支付配置
          </Button>
        </Space>
        <Row gutter={[24, 8]} style={{ marginTop: 20 }}>
          <Col xs={12} md={6}>
            <div style={{ color: 'rgba(255,255,255,0.6)', fontSize: 12 }}>商户号</div>
            <div style={{ color: '#fff', fontFamily: 'monospace', fontWeight: 600, marginTop: 4 }}>{d.entity_code}</div>
          </Col>
          <Col xs={12} md={6}>
            <div style={{ color: 'rgba(255,255,255,0.6)', fontSize: 12 }}>钱包号</div>
            <div style={{ color: '#fff', fontFamily: 'monospace', fontWeight: 600, marginTop: 4 }}>{d.wallet_no ?? '-'}</div>
          </Col>
          <Col xs={12} md={6}>
            <div style={{ color: 'rgba(255,255,255,0.6)', fontSize: 12 }}>创建时间</div>
            <div style={{ color: '#fff', marginTop: 4 }}>{d.created_at ? formatDateTime(d.created_at) : '-'}</div>
          </Col>
          <Col xs={12} md={6}>
            <div style={{ color: 'rgba(255,255,255,0.6)', fontSize: 12 }}>最近更新</div>
            <div style={{ color: '#fff', marginTop: 4 }}>{d.updated_at ? formatDateTime(d.updated_at) : '-'}</div>
          </Col>
        </Row>
      </Card>

      {/* 经营概览 KPI */}
      <Row gutter={[16, 16]}>
        {kpis.map((k) => (
          <Col xs={12} sm={12} md={8} key={k.title}>
            <KpiCard title={k.title} value={k.value} icon={k.icon} color={k.color} formatter={k.formatter} />
          </Col>
        ))}
      </Row>

      {/* 基本信息 + 身份认证资料 */}
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <Card className="hp-info-card" title="基本信息" size="small">
            <InfoRow icon={<ShopOutlined />} label="商户名称">{d.name}</InfoRow>
            <InfoRow icon={<ApartmentOutlined />} label="主体类型">{typeText(d.entity_type)}</InfoRow>
            <InfoRow icon={<IdcardOutlined />} label="商户号"><Mono>{d.entity_code}</Mono></InfoRow>
            <InfoRow icon={<WalletOutlined />} label="钱包号"><Mono>{d.wallet_no ?? '-'}</Mono></InfoRow>
            <InfoRow icon={<SafetyCertificateOutlined />} label="状态"><MerchantStatusTag status={d.status} /></InfoRow>
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card className="hp-info-card" title="商户身份认证资料" size="small">
            <InfoRow icon={<UserOutlined />} label="法人 / 经营者">{d.kyc_data?.legal_name}</InfoRow>
            <InfoRow icon={<IdcardOutlined />} label="证件号">{d.kyc_data?.license_no}</InfoRow>
            <InfoRow icon={<CreditCardOutlined />} label="结算卡号">{d.kyc_data?.bank_account}</InfoRow>
            <InfoRow icon={<BankOutlined />} label="开户行">{d.kyc_data?.bank_name}</InfoRow>
            <InfoRow icon={<UserOutlined />} label="联系人">{d.kyc_data?.contact_name}</InfoRow>
            <InfoRow icon={<PhoneOutlined />} label="联系电话">{d.kyc_data?.contact_phone}</InfoRow>
          </Card>
        </Col>
      </Row>

      {/* 微信支付配置 */}
      <Card
        className="hp-info-card"
        title="微信支付配置"
        size="small"
        extra={
          <Button type="link" size="small" icon={<SettingOutlined />} onClick={() => nav(`/merchants/${d.id}/wechat-config`)}>
            前往配置
          </Button>
        }
      >
        {!wc ? (
          <Alert type="info" showIcon message="该商户尚未配置微信支付，点击右上角「前往配置」设置。" />
        ) : (
          <Row gutter={[24, 8]}>
            <Col xs={24} md={12}>
              <InfoRow icon={<SafetyCertificateOutlined />} label="启用"><WechatEnabledTag enabled={wc.enabled} /></InfoRow>
              <InfoRow icon={<WalletOutlined />} label="商户号"><Mono>{wc.mchid || '-'}</Mono></InfoRow>
              <InfoRow icon={<ApartmentOutlined />} label="AppID">{wc.appid || '-'}</InfoRow>
              <InfoRow icon={<GlobalOutlined />} label="回调地址前缀">{wc.notify_base_url || '-'}</InfoRow>
            </Col>
            <Col xs={24} md={12}>
              <InfoRow icon={<KeyOutlined />} label="AppSecret"><ConfiguredTag configured={wc.app_secret_configured} /></InfoRow>
              <InfoRow icon={<FileProtectOutlined />} label="APIv3 密钥"><ConfiguredTag configured={wc.api_v3_key_configured} /></InfoRow>
              <InfoRow icon={<KeyOutlined />} label="商户 API 私钥"><ConfiguredTag configured={wc.merchant_private_key_configured} /></InfoRow>
              <InfoRow icon={<FileProtectOutlined />} label="微信平台公钥"><ConfiguredTag configured={wc.platform_public_key_configured} /></InfoRow>
            </Col>
          </Row>
        )}
      </Card>
    </div>
  );
};