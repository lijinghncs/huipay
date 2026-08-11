// 商户详情子页：经营概览 + 基本信息 + 商户身份认证资料 + 微信支付配置（分栏卡片布局）
import React from 'react';
import { useQuery } from '@tanstack/react-query';
import { Card, Descriptions, Skeleton, Result, Button, Space, Row, Col, Statistic, Alert, Tag } from 'antd';
import { ArrowLeftOutlined, SettingOutlined } from '@ant-design/icons';
import { useNavigate, useParams } from 'react-router-dom';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import { getMerchant, getMerchantOverview } from '../../services/admin';
import { typeText, MerchantStatusTag, WechatEnabledTag, ConfiguredTag, Mono } from './shared';

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

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {/* 页头 */}
      <Card>
        <Space style={{ display: 'flex', justifyContent: 'space-between', width: '100%' }} align="center">
          <Space size={12} align="center">
            <Button icon={<ArrowLeftOutlined />} onClick={() => nav('/merchants')}>
              返回
            </Button>
            <Space size={10} align="baseline">
              <span style={{ fontSize: 18, fontWeight: 700 }}>{d.name}</span>
              <Tag style={{ borderRadius: 10 }} color="blue">{typeText(d.entity_type)}</Tag>
              <MerchantStatusTag status={d.status} />
            </Space>
          </Space>
          <Space>
            <Button type="primary" icon={<SettingOutlined />} onClick={() => nav(`/merchants/${d.id}/wechat-config`)}>
              微信支付配置
            </Button>
          </Space>
        </Space>
        <Descriptions column={2} size="small" style={{ marginTop: 16 }}>
          <Descriptions.Item label="商户号"><Mono>{d.entity_code}</Mono></Descriptions.Item>
          <Descriptions.Item label="钱包号"><Mono>{d.wallet_no ?? '-'}</Mono></Descriptions.Item>
          <Descriptions.Item label="创建时间">{d.created_at ? formatDateTime(d.created_at) : '-'}</Descriptions.Item>
          <Descriptions.Item label="最近更新">{d.updated_at ? formatDateTime(d.updated_at) : '-'}</Descriptions.Item>
        </Descriptions>
      </Card>

      {/* 经营概览统计 */}
      <Row gutter={16}>
        <Col span={8}><Card size="small"><Statistic title="钱包余额" value={ov?.balance ?? 0} formatter={(v) => formatCents(Number(v))} /></Card></Col>
        <Col span={8}><Card size="small"><Statistic title="累计实付" value={ov?.total_paid ?? 0} formatter={(v) => formatCents(Number(v))} /></Card></Col>
        <Col span={8}><Card size="small"><Statistic title="已支付订单" value={ov?.paid_order_count ?? 0} /></Card></Col>
        <Col span={8}><Card size="small"><Statistic title="订单总数" value={ov?.order_count ?? 0} /></Card></Col>
        <Col span={8}><Card size="small"><Statistic title="可用码牌" value={ov?.active_code_count ?? 0} /></Card></Col>
        <Col span={8}><Card size="small"><Statistic title="冻结金额" value={ov?.frozen ?? 0} formatter={(v) => formatCents(Number(v))} /></Card></Col>
      </Row>

      {/* 基本信息 + 身份认证资料 */}
      <Row gutter={16}>
        <Col span={12}>
          <Card title="基本信息" size="small">
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="商户号"><Mono>{d.entity_code ?? '-'}</Mono></Descriptions.Item>
              <Descriptions.Item label="商户名称">{d.name ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="主体类型">{typeText(d.entity_type)}</Descriptions.Item>
              <Descriptions.Item label="状态"><MerchantStatusTag status={d.status} /></Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="商户身份认证资料" size="small">
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="法人 / 经营者">{d.kyc_data?.legal_name ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="证件号">{d.kyc_data?.license_no ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="结算卡号">{d.kyc_data?.bank_account ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="开户行">{d.kyc_data?.bank_name ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="联系人">{d.kyc_data?.contact_name ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="联系电话">{d.kyc_data?.contact_phone ?? '-'}</Descriptions.Item>
            </Descriptions>
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
            <Col span={12}>
              <Descriptions column={1} bordered size="small">
                <Descriptions.Item label="启用"><WechatEnabledTag enabled={wc.enabled} /></Descriptions.Item>
                <Descriptions.Item label="商户号"><Mono>{wc.mchid || '-'}</Mono></Descriptions.Item>
                <Descriptions.Item label="AppID">{wc.appid || '-'}</Descriptions.Item>
                <Descriptions.Item label="回调地址前缀">{wc.notify_base_url || '-'}</Descriptions.Item>
              </Descriptions>
            </Col>
            <Col span={12}>
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
