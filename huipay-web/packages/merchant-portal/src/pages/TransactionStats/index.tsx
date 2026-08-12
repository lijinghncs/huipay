// 交易统计页：筛选条件 → 按门店分组统计（含实付占比）
import React from 'react';
import {
  Button,
  Card,
  DatePicker,
  Empty,
  Input,
  Select,
  Space,
  Table,
  Typography,
} from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { formatCents } from '@huipay/shared/utils';
import { getOrderStats, listStores } from '../../services/user';
import type { OrderStatRow } from '../../services/user';

// 仅统计已支付订单
const FIXED_STATUS = 'PAID';

const { RangePicker } = DatePicker;

// 按门店统计表：门店 / 订单数 / 金额合计 / 实付合计
const StoreStatTable: React.FC<{
  rows: OrderStatRow[];
  loading?: boolean;
}> = ({ rows, loading }) => (
  <Table<OrderStatRow>
    className="hp-zebra hp-tx-table"
    rowKey="key"
    pagination={false}
    size="small"
    loading={loading}
    dataSource={rows}
    locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无数据" /> }}
    columns={[
      {
        title: '门店',
        dataIndex: 'label',
        render: (l: string) => (
          <span className="hp-stat-chip">
            <span className="hp-stat-dot" style={{ background: '#1e6fff' }} />
            <span>{l}</span>
          </span>
        ),
      },
      { title: '订单数', dataIndex: 'order_count', align: 'right' },
      { title: '金额合计', dataIndex: 'amount', align: 'right', render: (v: number) => formatCents(v) },
      { title: '实付合计', dataIndex: 'paid', align: 'right', render: (v: number) => <span style={{ color: '#06b6a4', fontWeight: 600 }}>{formatCents(v)}</span> },
    ]}
  />
);

export const TransactionStats: React.FC = () => {
  const [codeId, setCodeId] = React.useState('');
  const [storeId, setStoreId] = React.useState<number | undefined>();
  const [range, setRange] = React.useState<[string, string] | null>(null);

  // 已提交的筛选条件：仅点击「查询」时更新
  const [applied, setApplied] = React.useState<{
    codeId: string;
    storeId?: number;
    range: [string, string] | null;
  }>({ codeId, storeId, range });

  const statsQuery = useQuery({
    queryKey: ['order-stats', FIXED_STATUS, applied.codeId, applied.storeId, applied.range],
    queryFn: () =>
      getOrderStats({
        status: FIXED_STATUS,
        code_id: applied.codeId || undefined,
        store_id: applied.storeId,
        start: applied.range?.[0],
        end: applied.range?.[1],
      }),
  });

  const storesQuery = useQuery({
    queryKey: ['stores', 'options'],
    queryFn: () => listStores({ page: 1, size: 200 }),
  });

  const stats = statsQuery.data;
  const filterLabel = { codeId: '全部码牌', storeId: '全部门店' };

  return (
    <div className="hp-page">
      {/* 筛选区 */}
      <Card
        title={<Typography.Text strong style={{ fontSize: 15 }}>统计筛选</Typography.Text>}
        extra={<Typography.Text type="secondary" style={{ fontSize: 12 }}>修改条件后点击「查询」生效</Typography.Text>}
        style={{ boxShadow: 'var(--shadow-card)' }}
      >
        <Space wrap size="large" style={{ rowGap: 16 }}>
          <div>
            <Typography.Text strong style={{ fontSize: 12, color: '#8a94a6', display: 'block', marginBottom: 6 }}>
              来源码牌
            </Typography.Text>
            <Input
              placeholder="短码"
              style={{ width: 150 }}
              value={codeId}
              onChange={(e) => setCodeId(e.target.value)}
              prefix={<SearchOutlined style={{ color: '#b0b8c7' }} />}
            />
          </div>
          <div>
            <Typography.Text strong style={{ fontSize: 12, color: '#8a94a6', display: 'block', marginBottom: 6 }}>
              所属门店
            </Typography.Text>
            <Select
              allowClear
              showSearch
              placeholder={filterLabel.storeId}
              style={{ width: 160 }}
              value={storeId}
              onChange={setStoreId}
              options={(storesQuery.data?.items ?? []).map((s) => ({ value: s.id, label: s.name }))}
              loading={storesQuery.isLoading}
            />
          </div>
          <div>
            <Typography.Text strong style={{ fontSize: 12, color: '#8a94a6', display: 'block', marginBottom: 6 }}>
              日期范围
            </Typography.Text>
            <RangePicker
              onChange={(v) => {
                if (v && v[0] && v[1]) {
                  setRange([v[0].startOf('day').toISOString(), v[1].endOf('day').toISOString()]);
                } else {
                  setRange(null);
                }
              }}
            />
          </div>
          <div style={{ display: 'flex', gap: 12, alignItems: 'flex-end', marginLeft: 'auto' }}>
            <Button
              type="primary"
              icon={<SearchOutlined />}
              onClick={() => setApplied({ codeId, storeId, range })}
            >
              查询
            </Button>
            <Button
              onClick={() => {
                setCodeId('');
                setStoreId(undefined);
                setRange(null);
                setApplied({ codeId: '', storeId: undefined, range: null });
              }}
            >
              重置
            </Button>
          </div>
        </Space>
      </Card>

      {/* 按门店统计表 */}
      <Card
        title={<Typography.Text strong style={{ fontSize: 15 }}>按门店统计</Typography.Text>}
        extra={
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            覆盖 {stats?.order_count ?? 0} 笔交易
          </Typography.Text>
        }
        style={{ marginTop: 20, boxShadow: 'var(--shadow-card)' }}
      >
        <StoreStatTable
          rows={stats?.by_store ?? []}
          loading={statsQuery.isLoading}
        />
      </Card>
    </div>
  );
};