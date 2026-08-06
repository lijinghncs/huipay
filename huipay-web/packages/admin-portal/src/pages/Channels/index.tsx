// 通道配置
import React from 'react';
import { Card, Table, Tag, Switch, Space } from 'antd';
import type { ChannelCode } from '@huipay/shared';

interface ChannelRow {
  code: ChannelCode;
  name: string;
  fee_rate: string;
  enabled: boolean;
  mch_id: string;
  status: 'NORMAL' | 'MAINTENANCE' | 'OFFLINE';
}

const columns = [
  { title: '通道编码', dataIndex: 'code', key: 'code' },
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: '费率', dataIndex: 'fee_rate', key: 'fee_rate' },
  { title: '商户号', dataIndex: 'mch_id', key: 'mch_id' },
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
    render: (v: ChannelRow['status']) =>
      v === 'NORMAL' ? <Tag color="green">正常</Tag> : v === 'MAINTENANCE' ? <Tag color="orange">维护中</Tag> : <Tag color="red">下线</Tag>,
  },
  {
    title: '启用',
    dataIndex: 'enabled',
    key: 'enabled',
    render: (v: boolean, _r: ChannelRow) => <Switch checked={v} />,
  },
];

export const Channels: React.FC = () => {
  return (
    <Card title="支付通道配置">
      <Space direction="vertical" style={{ width: '100%' }}>
        <Table<ChannelRow> rowKey="code" columns={columns} dataSource={[]} pagination={false} />
      </Space>
    </Card>
  );
};