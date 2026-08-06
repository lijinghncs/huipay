// Dashboard 概览页
import React from 'react';
import { Card, Col, Row, Statistic, Space } from 'antd';
import { Money, StatusTag } from '@huipay/ui-kit';
import { formatDateTime } from '@huipay/shared/utils';

export const Dashboard: React.FC = () => {
  // 骨架：硬编码示例数据；真实项目从 useQuery 拉取
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <Statistic title="今日交易额" value={12345.67} prefix="¥" precision={2} valueStyle={{ color: '#1e6fff' }} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="今日订单数" value={156} valueStyle={{ color: '#06b6a4' }} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Money cents={98765400} prefix="余额 ¥" />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="分账成功率" value={99.95} suffix="%" valueStyle={{ color: '#06b6a4' }} />
          </Card>
        </Col>
      </Row>
      <Card title="最近交易">
        <p>暂无数据 - {formatDateTime(new Date().toISOString())}</p>
        <p>示例状态：<StatusTag status="PAID" /></p>
      </Card>
    </Space>
  );
};