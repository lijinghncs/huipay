// 门店按日统计（admin）：跨商户只读监控（日常操作入口在商家工作台）
import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  Card, Table, Select, DatePicker, Button, Space, Empty, Typography, Tag, Tooltip,
} from 'antd';
import { ShopOutlined, SearchOutlined, DownloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs, { Dayjs } from 'dayjs';
import {
  listAdminStoreStats, getAdminStoreStatSummary,
  splitStatusBadge, type StoreStatItem,
} from '../../services/admin';
import { get } from '@huipay/shared/api-client';

const { RangePicker } = DatePicker;
const { Text } = Typography;

function fmtYuan(cents: number): string {
  return (cents / 100).toFixed(2);
}

function fmtChannel(raw?: string): string {
  if (!raw) return '-';
  try {
    const obj = JSON.parse(raw) as Record<string, { count: number; amount: number }>;
    const parts = Object.entries(obj).filter(([, v]) => v.count > 0).map(([k, v]) => `${k}×${v.count}`);
    return parts.length ? parts.join(' ') : '-';
  } catch {
    return '-';
  }
}

// 拉取商户列表（admin 端）
async function listAllMerchants(): Promise<{ items: { id: number; name: string }[] }> {
  return get<{ items: { id: number; name: string }[] }>('/v1/admin/merchants', { params: { page: 1, page_size: 500 } });
}

export const StoreStats: React.FC = () => {
  const [merchantId, setMerchantId] = useState<number | undefined>();
  const [storeId, setStoreId] = useState<number | undefined>();
  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(6, 'day'), dayjs()]);
  const startDate = range[0]?.format('YYYY-MM-DD') ?? '';
  const endDate = range[1]?.format('YYYY-MM-DD') ?? '';

  const merchants = useQuery({ queryKey: ['admin', 'merchants', 'all'], queryFn: () => listAllMerchants() });
  const stats = useQuery({
    queryKey: ['admin', 'store-stats', 'list', merchantId, storeId, startDate, endDate],
    queryFn: () => listAdminStoreStats({
      merchant_id: merchantId, store_id: storeId,
      start_date: startDate, end_date: endDate, page_size: 500,
    }),
    enabled: !!startDate && !!endDate,
  });
  const summary = useQuery({
    queryKey: ['admin', 'store-stats', 'summary', merchantId, storeId, startDate, endDate],
    queryFn: () => getAdminStoreStatSummary({
      merchant_id: merchantId, store_id: storeId, start_date: startDate, end_date: endDate,
    }),
    enabled: !!startDate && !!endDate,
  });

  const rows = stats.data?.items ?? [];
  const merchantNameMap = new Map((merchants.data?.items ?? []).map((m) => [m.id, m.name]));

  const exportCsv = () => {
    const header = ['业务日期', '商户', '门店ID', '订单笔数', '实收金额(元)', '是否分账', '分账批次号', '分账时间'];
    const lines = rows.map((r) => {
      const b = splitStatusBadge(r.split_status);
      return [
        dayjs(r.biz_date).format('YYYY-MM-DD'),
        merchantNameMap.get(r.merchant_id) ?? r.merchant_id,
        r.store_id,
        r.order_count,
        fmtYuan(r.paid_amount),
        b.text,
        r.split_batch_no ?? '',
        r.split_at ? dayjs(r.split_at).format('YYYY-MM-DD HH:mm:ss') : '',
      ];
    });
    const csv = [header, ...lines].map((row) => row.join(',')).join('\n');
    const blob = new Blob(['\ufeff' + csv], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `门店每日统计_${startDate}_${endDate}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const columns: ColumnsType<StoreStatItem> = [
    { title: '业务日期', dataIndex: 'biz_date', width: 120, render: (v: string) => dayjs(v).format('YYYY-MM-DD') },
    {
      title: '商户',
      dataIndex: 'merchant_id',
      width: 140,
      render: (v: number) => <Text strong>{merchantNameMap.get(v) ?? `商户 #${v}`}</Text>,
    },
    {
      title: '门店',
      dataIndex: 'store_id',
      width: 90,
      render: (v: number) => (
        <Space>
          <span className="hp-split-store-avatar" style={{ width: 28, height: 28, fontSize: 13 }}>
            <ShopOutlined />
          </span>
          <Text strong>#{v}</Text>
        </Space>
      ),
    },
    { title: '订单笔数', dataIndex: 'order_count', align: 'right', render: (v: number) => <Text strong>{v}</Text> },
    {
      title: '实收金额',
      dataIndex: 'paid_amount',
      align: 'right',
      render: (v: number) => (
        <Text strong style={{ color: '#e11d48' }}>¥{fmtYuan(v)}</Text>
      ),
    },
    { title: '渠道', dataIndex: 'channel_breakdown', width: 120, render: (v?: string) => <Tag>{fmtChannel(v)}</Tag> },
    {
      title: '是否分账',
      dataIndex: 'split_status',
      width: 120,
      render: (v: string | undefined, row: StoreStatItem) => {
        const b = splitStatusBadge(v);
        const tip = row.split_status === 'PARTIAL'
          ? `已分账 ¥${fmtYuan(row.split_total_amount ?? 0)}，剩余订单待分账`
          : row.split_status === 'SUCCESS'
            ? `已分账 ¥${fmtYuan(row.split_total_amount ?? 0)}`
            : row.split_status === 'FAILED' ? '分账失败，请联系平台' : '订单尚未分账';
        return (
          <Tooltip title={tip}>
            <span style={{ color: b.color, background: b.bg, padding: '2px 10px', borderRadius: 10, fontSize: 12, fontWeight: 500 }}>
              {b.text}
            </span>
          </Tooltip>
        );
      },
    },
    {
      title: '分账时间',
      dataIndex: 'split_at',
      width: 160,
      render: (v?: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm') : '-'),
    },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div className="hp-split-search">
        <Select
          allowClear
          placeholder="全部商户"
          style={{ width: 200 }}
          value={merchantId}
          onChange={setMerchantId}
          options={(merchants.data?.items ?? []).map((m) => ({ value: m.id, label: m.name }))}
        />
        <Select
          allowClear
          placeholder="全部门店"
          style={{ width: 160 }}
          value={storeId}
          onChange={setStoreId}
          disabled={!merchantId}
          options={merchantId ? Array.from(new Set(rows.filter((r) => r.merchant_id === merchantId).map((r) => r.store_id)))
            .map((sid) => ({ value: sid, label: `#${sid}` })) : []}
        />
        <RangePicker
          allowClear={false}
          value={range}
          onChange={(r) => r && setRange([r[0] as Dayjs, r[1] as Dayjs])}
          disabledDate={(d) => d.isAfter(dayjs(), 'day')}
        />
        <Button className="hp-split-search-btn" type="primary" icon={<SearchOutlined />} onClick={() => stats.refetch()} loading={stats.isFetching}>
          查询
        </Button>
        <Button icon={<DownloadOutlined />} onClick={exportCsv} disabled={rows.length === 0}>
          导出 CSV
        </Button>
      </div>

      {summary.data && (
        <Card size="small">
          <Space size="large" wrap>
            <span>订单总笔数：<Text strong>{summary.data.summary.order_count}</Text></span>
            <span>实收总金额：<Text strong style={{ color: '#e11d48' }}>¥{fmtYuan(summary.data.summary.paid_amount)}</Text></span>
            <span>覆盖门店数：<Text strong>{summary.data.items.length}</Text></span>
          </Space>
        </Card>
      )}

      <Table<StoreStatItem>
        columns={columns}
        dataSource={rows}
        rowKey={(r) => `${r.store_id}-${r.biz_date}`}
        loading={stats.isFetching}
        pagination={{ pageSize: 20, showSizeChanger: false }}
        scroll={{ x: 1200 }}
        locale={{
          emptyText: rows.length === 0 && !stats.isFetching ? <Empty description="该区间暂无门店日报数据" /> : '暂无数据',
        }}
      />
    </div>
  );
};

export default StoreStats;