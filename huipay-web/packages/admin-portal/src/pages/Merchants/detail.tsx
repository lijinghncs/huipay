// 商户详情子页：经营概览 + 基本信息 + 商户身份认证资料 + 微信支付配置（渐变页头 + KPI 卡片布局）
import React from 'react';
import { useQuery } from '@tanstack/react-query';
import { Card, Descriptions, Skeleton, Result, Button, Space, Row, Col, Statistic, Alert, Tag, Avatar, Divider } from 'antd';
import {
  ArrowLeftOutlined,
  SettingOutlined,
  WalletOutlined,
  AccountBookOutlined,
  CheckCircleOutlined,
  ShoppingCartOutlined,
  QrcodeOutlined,
  LockOutlined,
  EnvironmentOutlined,
  UserOutlined,
  PhoneOutlined,
  BankOutlined,
  IdcardOutlined,
} from '@ant-design/icons';
import { useNavigate, useParams } from 'react-router-dom';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import { getMerchant, getMerchantOverview } from '../../services/admin';
import { typeText, MerchantStatusTag, WechatEnabledTag, ConfiguredTag, Mono } from './shared';

/** 页头信息项（深色渐变背景上的亮色小字） */
const HeadInfo: React.FC<{ label: string; children?: React.ReactNode }> = ({ label, children }) => (
  <div>
    <div style={{ fontSize: 12, color: 'rgba(255,255,255,0.6)', letterSpacing: 0.5, marginBottom: 4 }}>{label}</div>
    <div style={{ fontSize: 14, color: '#fff', fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace' }}>{children}</div>
  </div>
);

/** 经营概览 KPI 卡片（渐变图标） */
const kpiCards = (ov: {
  balance?: number;
  total_paid?: number;
  paid_order_count?: number;
  order_count?: number;
  active_code_count?: number;
  frozen?: number;
}) => [
  { title: '钱包余额', value: ov?.balance ?? 0, money: true, icon: <WalletOutlined />, color: '#1e6fff' },
  { title: '累计实付', value: ov?.total_paid ?? 0, money: true, icon: <AccountBookOutlined />, color: '#06b6a4' },
  { title: '已支付订单', value: ov?.paid_order_count ?? 0, money: false, icon: <CheckCircleOutlined />, color: '#22c55e' },
  { title: '订单总数', value: ov?.order_count ?? 0, money: false, icon: <ShoppingCartOutlined />, color: '#f59e0b' },
  { title: '可用码牌', value: ov?.active_code_count ?? 0, money: false, icon: <QrcodeOutlined />, color: '#8b5cf6' },
  { title: '冻结金额', value: ov?.frozen ?? 0, money: true, icon: <LockOutlined />, color: '#f5222d' },
];

/** 信息区字段：为缺失值提供友好占位 */
const fmt = (v?: string | number | null) => (v === undefined || v === null || v === '' ? '—' : String(v));

/** 信息区小图标字段行 */
const InfoField: React.FC<{ icon: React.ReactNode; label: string; value?: React.ReactNode }> = ({ icon, label, value }) => (
  <div className="hp-info-field">
    <span className="hp-info-icon">{icon}</span>
    <div className="hp-info-body">
      <div className="hp-info-label">{label}</div>
      <div className="hp-info-value">{value ?? '—'}</div>
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
  const kyc = d.kyc_data;

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }} className="hp-page">
      {/* 页头：品牌渐变 + 商户徽标 */}
      <Card
        className="hp-detail-hero"
        bodyStyle={{ padding: '22px 24px' }}
      >
        <Space style={{ display: 'flex', justifyContent: 'space-between', width: '100%' }} align="center" wrap>
          <Space size={14} align="center">
            <Button
              shape="circle"
              type="text"
              icon={<ArrowLeftOutlined style={{ color: '#fff' }} />}
              onClick={() => nav('/merchants')}
            />
            <Avatar
              size={48}
              shape="square"
              style={{
                borderRadius: 12,
                background: 'linear-gradient(135deg, #1e6fff 0%, #06b6a4 100%)',
                color: '#fff',
                fontWeight: 700,
                fontSize: 20,
                boxShadow: '0 6px 14px rgba(30,111,255,0.35)',
              }}
            >
              {d.name?.slice(0, 1) || '商'}
            </Avatar>
            <div>
              <Space size={10} align="center">
                <span style={{ fontSize: 20, fontWeight: 700, color: '#fff', letterSpacing: 0.5 }}>{d.name}</span>
                <Tag style={{ borderRadius: 10, color: '#fff', background: 'rgba(255,255,255,0.15)', borderColor: 'rgba(255,255,255,0.35)' }}>
                  {typeText(d.entity_type)}
                </Tag>
                <MerchantStatusTag status={d.status} />
              </Space>
              <div style={{ marginTop: 6, fontSize: 13, color: 'rgba(255,255,255,0.7)' }}>
                <Mono>{d.entity_code ?? '-'}</Mono>
              </div>
            </div>
          </Space>
          <Button
            type="primary"
            icon={<SettingOutlined />}
            style={{ background: 'rgba(255,255,255,0.16)', borderColor: 'rgba(255,255,255,0.5)', boxShadow: 'none', color: '#fff' }}
            onClick={() => nav(`/merchants/${d.id}/wechat-config`)}
          >
            微信支付配置
          </Button>
        </Space>
        <Divider style={{ margin: '18px 0 16px', borderColor: 'rgba(255,255,255,0.12)' }} />
        <div style={{ display: 'flex', gap: 48, flexWrap: 'wrap' }}>
          <HeadInfo label="钱包号">{d.wallet_no ?? '-'}</HeadInfo>
          <HeadInfo label="创建时间">{d.created_at ? formatDateTime(d.created_at) : '-'}</HeadInfo>
          <HeadInfo label="最近更新">{d.updated_at ? formatDateTime(d.updated_at) : '-'}</HeadInfo>
        </div>
      </Card>

      {/* 经营概览 KPI */}
      <Row gutter={[16, 16]}>
        {kpiCards(ov).map((k) => (
          <Col xs={12} sm={12} md={8} key={k.title}>
            <Card className="hp-kpi" bodyStyle={{ padding: '18px 20px' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <div>
                  <div style={{ color: '#66718b', fontSize: 13, marginBottom: 8 }}>{k.title}</div>
                  <Statistic
                    value={k.value}
                    formatter={(v) => (k.money ? formatCents(Number(v)) : String(v))}
                    valueStyle={{ color: '#1f2a44', fontWeight: 700, fontSize: 22 }}
                  />
                </div>
                <span
                  className="hp-kpi-icon"
                  style={{
                    width: 46,
                    height: 46,
                    borderRadius: 13,
                    display: 'inline-flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: 22,
                    color: '#fff',
                    boxShadow: '0 6px 14px rgba(16,24,40,0.14)',
                    background: `linear-gradient(135deg, ${k.color}, ${k.color}aa)`,
                  }}
                >
                  {k.icon}
                </span>
              </div>
            </Card>
          </Col>
        ))}
      </Row>

      {/* 基本信息 + 身份认证资料 */}
      <Row gutter={16}>
        <Col xs={24} md={12}>
          <Card title="基本信息" size="small" className="hp-info-card">
            <InfoField icon={<IdcardOutlined />} label="商户号" value={<Mono>{fmt(d.entity_code)}</Mono>} />
            <InfoField icon={<EnvironmentOutlined />} label="主体类型" value={typeText(d.entity_type)} />
            <InfoField icon={<UserOutlined />} label="商户名称" value={d.name} />
            <InfoField icon={<CheckCircleOutlined />} label="状态" value={<MerchantStatusTag status={d.status} />} />
          </Card>
        </Col>
        <Col xs={24} md={12}>
          <Card title="商户身份认证资料" size="small" className="hp-info-card">
            <InfoField icon={<UserOutlined />} label="法人 / 经营者" value={fmt(kyc?.legal_name)} />
            <InfoField icon={<IdcardOutlined />} label="证件号" value={fmt(kyc?.license_no)} />
            <InfoField icon={<BankOutlined />} label="结算卡号" value={fmt(kyc?.bank_account)} />
            <InfoField icon={<EnvironmentOutlined />} label="开户行" value={fmt(kyc?.bank_name)} />
            <InfoField icon={<UserOutlined />} label="联系人" value={fmt(kyc?.contact_name)} />
            <InfoField icon={<PhoneOutlined />} label="联系电话" value={fmt(kyc?.contact_phone)} />
          </Card>
        </Col>
      </Row>

      {/* 微信支付配置 */}
      <Card
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
          <Row gutter={16}>
            <Col xs={24} md={12}>
              <Descriptions column={1} bordered size="small">
                <Descriptions.Item label="启用"><WechatEnabledTag enabled={wc.enabled} /></Descriptions.Item>
                <Descriptions.Item label="商户号"><Mono>{wc.mchid || '-'}</Mono></Descriptions.Item>
                <Descriptions.Item label="AppID">{wc.appid || '-'}</Descriptions.Item>
                <Descriptions.Item label="回调地址前缀">{wc.notify_base_url || '-'}</Descriptions.Item>
              </Descriptions>
            </Col>
            <Col xs={24} md={12}>
              <Descriptions column={1} bordered size="small">
                <Descriptions.Item label="AppSecret"><ConfiguredTag configured={wc.app_secret_configured} /></Descriptions.Item>
                <Descriptions.Item label="APIv3 密钥"><ConfiguredTag configured={wc.api_v3_key_configured} /></Descriptions.Item>
                <Descriptions.Item label="商户 API 私钥"><ConfiguredTag configured={wc.merchant_private_key_configured} /></Descriptions.Item>
                <Descriptions.Item label="微信平台公钥"><ConfiguredTag configured={wc.platform_public_key_configured} /></Descriptions.Item>
              </Descriptions>
            </Col>
          </Row>
        )}
      </Card>
    </Space>
  );
};