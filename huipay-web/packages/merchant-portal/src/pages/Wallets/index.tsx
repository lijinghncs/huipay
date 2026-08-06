// 钱包页面
import React from 'react';
import { Card, Input, Space, Table } from 'antd';
import { Money, StatusTag } from '@huipay/ui-kit';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import type { JournalEntry } from '@huipay/shared';

const entryColumns = [
  { title: '流水号', dataIndex: 'id', key: 'id', width: 220 },
  { title: '方向', dataIndex: 'direction', key: 'direction', render: (v: string) => (v === 'CREDIT' ? '入账' : '出账') },
  { title: '金额', dataIndex: 'amount', key: 'amount', render: (v: number) => formatCents(v) },
  { title: '余额', dataIndex: 'balance_after', key: 'balance_after', render: (v: number) => formatCents(v) },
  { title: '业务', dataIndex: 'biz_type', key: 'biz_type' },
  { title: '时间', dataIndex: 'created_at', key: 'created_at', render: (v: string) => formatDateTime(v) },
];

export const Wallets: React.FC = () => {
  const [entityId, setEntityId] = React.useState<string>('');

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card>
        <Space>
          <span>主体 ID：</span>
          <Input value={entityId} onChange={(e) => setEntityId(e.target.value)} placeholder="输入 entity_id" style={{ width: 240 }} />
          <span>示例状态：</span>
          <StatusTag status="SUCCESS" />
        </Space>
      </Card>
      <Card title="钱包余额">
        <Money cents={12345600} />
      </Card>
      <Card title="账本流水">
        <Table<JournalEntry> rowKey="id" columns={entryColumns} dataSource={[]} pagination={{ pageSize: 30 }} />
      </Card>
    </Space>
  );
};