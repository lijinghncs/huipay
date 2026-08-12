// 钱包页面（真实对接后台：余额 + 账本流水，支持类型/单号/时间过滤）
import React from 'react';
import { Button, Card, DatePicker, Input, Select, Space, Table } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { Money } from '@huipay/ui-kit';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import type { JournalEntry } from '@huipay/shared';
import { getCurrentUser, getWallet, listEntries } from '../../services/user';

const { RangePicker } = DatePicker;

const entryColumns = [
  { title: '流水号', dataIndex: 'id', key: 'id', width: 220 },
  { title: '方向', dataIndex: 'direction', key: 'direction', render: (v: string) => (v === 'CREDIT' ? '入账' : '出账') },
  { title: '金额', dataIndex: 'amount', key: 'amount', render: (v: number) => formatCents(v) },
  { title: '余额', dataIndex: 'balance_after', key: 'balance_after', render: (v: number) => formatCents(v) },
  { title: '业务类型', dataIndex: 'biz_type', key: 'biz_type' },
  {
    title: '业务单号',
    dataIndex: 'biz_id',
    key: 'biz_id',
    width: 180,
    render: (v: string, r: JournalEntry) =>
      r.biz_type === 'PAYMENT' ? <Link to={`/transactions?order_no=${encodeURIComponent(v)}`}>{v}</Link> : v,
  },
  { title: '时间', dataIndex: 'created_at', key: 'created_at', render: (v: string) => formatDateTime(v) },
];

export const Wallets: React.FC = () => {
  const [entityId, setEntityId] = React.useState<number | null>(null);
  const [bizType, setBizType] = React.useState<string | undefined>(undefined);
  const [bizId, setBizId] = React.useState<string>('');
  const [range, setRange] = React.useState<[string, string] | null>(null);
  const [page, setPage] = React.useState(1);
  const [size, setSize] = React.useState(20);

  // 默认取当前登录商户的 entity_id，可手动覆盖
  React.useEffect(() => {
    getCurrentUser()
      .then((u) => setEntityId(u.merchantId))
      .catch(() => undefined);
  }, []);

  const walletQuery = useQuery({
    queryKey: ['wallet', entityId],
    queryFn: () => getWallet(entityId!),
    enabled: !!entityId,
  });

  const entriesQuery = useQuery({
    queryKey: ['wallet-entries', entityId, page, size, bizType, bizId, range],
    queryFn: () =>
      listEntries(entityId!, {
        page,
        size,
        biz_type: bizType,
        biz_id: bizId || undefined,
        start: range?.[0],
        end: range?.[1],
      }),
    enabled: !!entityId,
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card title="钱包余额" loading={walletQuery.isLoading}>
        <Money cents={walletQuery.data?.balance ?? 0} />
      </Card>
      <Card title="账本流水">
        <Space wrap style={{ marginBottom: 16 }}>
          <Select
            allowClear
            placeholder="业务类型"
            style={{ width: 160 }}
            value={bizType}
            onChange={(v) => {
              setBizType(v);
              setPage(1);
            }}
            options={[
              { value: 'PAYMENT', label: '支付' },
              { value: 'SPLIT', label: '分账' },
              { value: 'REFUND', label: '退款' },
              { value: 'WITHDRAW', label: '提现' },
            ]}
          />
          <Input
            placeholder="按业务单号搜索"
            style={{ width: 200 }}
            allowClear
            value={bizId}
            onChange={(e) => setBizId(e.target.value)}
            onPressEnter={() => setPage(1)}
          />
          <RangePicker
            onChange={(v) => {
              if (v && v[0] && v[1]) {
                setRange([v[0].startOf('day').toISOString(), v[1].endOf('day').toISOString()]);
              } else {
                setRange(null);
              }
              setPage(1);
            }}
          />
          <Button type="primary" onClick={() => setPage(1)}>
            查询
          </Button>
        </Space>
        <Table<JournalEntry>
          className="hp-zebra"
          rowKey="id"
          columns={entryColumns}
          dataSource={entriesQuery.data?.items ?? []}
          loading={entriesQuery.isLoading}
          pagination={{
            current: page,
            pageSize: size,
            total: entriesQuery.data?.total ?? 0,
            showSizeChanger: true,
            onChange: (p, s) => {
              setPage(p);
              setSize(s);
            },
          }}
        />
      </Card>
    </div>
  );
};
