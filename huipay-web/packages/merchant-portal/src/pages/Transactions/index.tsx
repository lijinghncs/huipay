// 交易列表页
import React from 'react';
import { Card, Table } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { StatusTag } from '@huipay/ui-kit';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import type { Order } from '@huipay/shared';
import { listOrders } from '../../services/user';

const columns = [
  { title: '订单号', dataIndex: 'order_no', key: 'order_no', width: 200 },
  { title: '商户单号', dataIndex: 'merchant_order_no', key: 'merchant_order_no', width: 160 },
  { title: '金额', dataIndex: 'amount', key: 'amount', render: (v: number) => formatCents(v) },
  { title: '实付', dataIndex: 'paid_amount', key: 'paid_amount', render: (v: number) => formatCents(v) },
  { title: '通道', dataIndex: 'channel', key: 'channel' },
  { title: '订单状态', dataIndex: 'status', key: 'status', render: (v: string) => <StatusTag status={v} /> },
  { title: '分账状态', dataIndex: 'split_status', key: 'split_status', render: (v: string) => <StatusTag status={v} /> },
  { title: '创建时间', dataIndex: 'created_at', key: 'created_at', render: (v: string) => formatDateTime(v) },
];

export const Transactions: React.FC = () => {
  const { data, isLoading } = useQuery({
    queryKey: ['orders'],
    queryFn: () => listOrders({ page: 1, size: 20 }),
  });

  return (
    <Card title="交易列表">
      <Table<Order>
        rowKey="order_no"
        columns={columns}
        dataSource={data?.items ?? []}
        loading={isLoading}
        pagination={{ pageSize: 20, showSizeChanger: true, total: data?.total ?? 0 }}
      />
    </Card>
  );
};