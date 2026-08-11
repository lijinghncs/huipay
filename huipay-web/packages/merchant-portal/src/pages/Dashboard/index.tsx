// Dashboard 概览页：当前商户资料 + 经营概览
import React from 'react';
import { Card, Col, Descriptions, Row, Skeleton, Statistic, Space, Tag, Typography } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { getMerchantOverview, getMerchantProfile } from '../../services/user';

const formatCents = (cents: number): string => `¥${(Number(cents) / 100).toFixed(2)}`;

export const Dashboard: React.FC = () => {
  const profile = useQuery({ queryKey: ['merchant', 'profile'], queryFn: getMerchantProfile });
  const overview = useQuery({ queryKey: ['merchant', 'overview'], queryFn: getMerchantOverview });

  const loading = profile.isLoading || overview.isLoading;

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card title="商户资料">
        {profile.isLoading ? (
          <Skeleton active paragraph={{ rows: 3 }} />
        ) : profile.isError ? (
          <Typography.Text type="danger">加载失败，请稍后重试</Typography.Text>
        ) : (
          <Descriptions column={{ xs: 1, sm: 2, md: 3 }} size="small">
            <Descriptions.Item label="商户号">{profile.data?.entity_code}</Descriptions.Item>
            <Descriptions.Item label="商户名称">{profile.data?.name}</Descriptions.Item>
            <Descriptions.Item label="钱包号">{profile.data?.wallet_no}</Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color={profile.data?.status === 1 ? 'success' : 'default'}>
                {profile.data?.status === 1 ? '正常' : '停用'}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="钱包余额">{formatCents(profile.data?.balance ?? 0)}</Descriptions.Item>
            <Descriptions.Item label="冻结金额">{formatCents(profile.data?.frozen ?? 0)}</Descriptions.Item>
          </Descriptions>
        )}
      </Card>

      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <Statistic title="钱包余额" value={overview.data?.balance ?? 0} formatter={(v) => formatCents(Number(v))} valueStyle={{ color: '#1e6fff' }} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="累计实付" value={overview.data?.total_paid ?? 0} formatter={(v) => formatCents(Number(v))} valueStyle={{ color: '#06b6a4' }} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="订单总数" value={overview.data?.order_count ?? 0} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="可用收款码" value={overview.data?.active_code_count ?? 0} />
          </Card>
        </Col>
      </Row>

      {loading && <Skeleton active paragraph={{ rows: 2 }} />}
    </Space>
  );
};