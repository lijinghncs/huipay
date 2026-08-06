// 风控规则管理
import React from 'react';
import { Card, Table, Tag, Button } from 'antd';
import { PlusOutlined } from '@ant-design/icons';

interface RiskRuleRow {
  id: number;
  name: string;
  type: string;
  decision: string;
  priority: number;
  enabled: boolean;
}

const columns = [
  { title: '规则名', dataIndex: 'name', key: 'name' },
  { title: '类型', dataIndex: 'type', key: 'type' },
  {
    title: '决策',
    dataIndex: 'decision',
    key: 'decision',
    render: (v: string) => <Tag color={v === 'BLOCK' ? 'red' : v === 'REVIEW' ? 'orange' : 'green'}>{v}</Tag>,
  },
  { title: '优先级', dataIndex: 'priority', key: 'priority' },
  { title: '启用', dataIndex: 'enabled', key: 'enabled', render: (v: boolean) => (v ? '是' : '否') },
];

export const RiskRules: React.FC = () => {
  return (
    <Card
      title="风控规则"
      extra={
        <Button type="primary" icon={<PlusOutlined />}>
          新建规则
        </Button>
      }
    >
      <Table<RiskRuleRow> rowKey="id" columns={columns} dataSource={[]} pagination={false} />
    </Card>
  );
};