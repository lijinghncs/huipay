// 分账规则配置页
import React from 'react';
import { Card, Button, Space, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';

const { Text } = Typography;

export const SplitRules: React.FC = () => {
  return (
    <Card
      title="分账规则"
      extra={
        <Button type="primary" icon={<PlusOutlined />}>
          新建规则
        </Button>
      }
    >
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Text type="secondary">
          分账规则支持按渠道 / 商户类型 / 门店 / 活动 / 时间段 / 客户标签等条件灵活配置。
        </Text>
        <Text type="secondary">
          骨架阶段：规则配置表单与编辑器将随 P2 阶段一并实现。
        </Text>
      </Space>
    </Card>
  );
};