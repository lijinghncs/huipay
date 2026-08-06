// 商户管理
import React from 'react';
import { Card, Table, Tag, Button, Space } from 'antd';

interface MerchantRow {
  id: number;
  code: string;
  name: string;
  type: string;
  kyc_status: number;
  status: number;
  created_at: string;
}

const kycLabels: Record<number, { color: string; label: string }> = {
  0: { color: 'default', label: '未提交' },
  1: { color: 'orange', label: '审核中' },
  2: { color: 'green', label: '已通过' },
  3: { color: 'red', label: '已拒绝' },
};

const columns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '主体号', dataIndex: 'code', key: 'code', width: 200 },
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: '类型', dataIndex: 'type', key: 'type' },
  {
    title: 'KYC',
    dataIndex: 'kyc_status',
    key: 'kyc_status',
    render: (v: number) => <Tag color={kycLabels[v]?.color ?? 'default'}>{kycLabels[v]?.label ?? '-'}</Tag>,
  },
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
    render: (v: number) => (v === 1 ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>),
  },
  {
    title: '操作',
    key: 'actions',
    render: () => (
      <Space>
        <Button type="link" size="small">
          详情
        </Button>
        <Button type="link" size="small">
          KYC 审核
        </Button>
      </Space>
    ),
  },
];

export const Merchants: React.FC = () => {
  return (
    <Card title="商户管理">
      <Table<MerchantRow> rowKey="id" columns={columns} dataSource={[]} pagination={{ pageSize: 20 }} />
    </Card>
  );
};