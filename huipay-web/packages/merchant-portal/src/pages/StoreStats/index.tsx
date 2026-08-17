// 门店统计：按日期范围平铺每日日报 + CSV 导出（当前商户）
import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Card, Table, Select, DatePicker, Button, Space, Alert, Empty, Typography, Tag } from 'antd';
import { ShopOutlined, DownloadOutlined, SearchOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs, { Dayjs } from 'dayjs';
import { listStores, listStoreStats, splitStatusBadge, type StoreStatItem } from '../../services/user';

const { RangePicker } = DatePicker;
const { Text } = Typography;

function fmtYuan(cents: number): string {
  return (cents / 100).toFixed(2);
}

// 解析渠道 JSON（如 {"WECHAT":{"count":2,"amount":100}} -> WECHAT×2）
function fmtChannel(raw?: string): string {
  if (!raw) return '-';
  try {
    const obj = JSON.parse(raw) as Record<string, { count: number; amount: number }>;
    const parts = Object.entries(obj)
      .filter(([, v]) => v.count > 0)
      .map(([k, v]) => `${k}×${v.count}`);
    return parts.length ? parts.join(' ') : '-';
  } catch {
    return '-';
  }
}

export const StoreStats: React.FC = () => {
  const [storeId, setStoreId] = useState<number | undefined>();
  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(6, 'day'), dayjs()]);

  const startDate = range[0]?.format('YYYY-MM-DD') ?? '';
  const endDate = range[1]?.format('YYYY-MM-DD') ?? '';

  const stores = useQuery({ queryKey: ['merchant', 'stores'], queryFn: () => listStores({ size: 200 }) });
  const storeNameMap = new Map((stores.data?.items ?? []).map((s) => [s.id, s.name]));
  const storeOptions = (stores.data?.items ?? []).map((s) => ({ value: s.id, label: s.name }));

  const stats = useQuery({
    queryKey: ['merchant', 'store-stats', 'list', storeId, startDate, endDate],
    queryFn: () => listStoreStats({ store_id: storeId, start_date: startDate, end_date: endDate, page_size: 500 }),
    enabled: !!startDate && !!endDate,
  });

  const rows = stats.data?.items ?? [];
  const excess = dayjs(endDate).endOf('day').isAfter(dayjs());

  const exportCsv = () => {
    const header = ['业务日期', '门店ID', '门店名称', '订单笔数', '实收金额(元)', '是否分账', '分账批次号', '分账时间'];
    const lines = rows.map((r) => {
      const badge = splitStatusBadge(r.split_status);
      return [
        dayjs(r.biz_date).format('YYYY-MM-DD'),
        r.store_id,
        storeNameMap.get(r.store_id) ?? r.store_id,
        r.order_count,
        fmtYuan(r.paid_amount),
        badge.text,
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
    { title: '门店ID', dataIndex: 'store_id', width: 90 },
    {
      title: '门店名称',
      dataIndex: 'store_id',
      render: (v: number) => (
        <Space>
          <span className="hp-split-store-avatar" style={{ width: 30, height: 30, fontSize: 14 }}>
            <ShopOutlined />
          </span>
          <Text strong>{storeNameMap.get(v) ?? `门店 #${v}`}</Text>
        </Space>
      ),
    },
    { title: '订单笔数', dataIndex: 'order_count', align: 'right', render: (v: number) => <Text strong>{v}</Text> },
    {
      title: '实收金额',
      dataIndex: 'paid_amount',
      align: 'right',
      render: (v: number) => (
        <Text strong style={{ color: '#e11d48' }}>
          ¥{fmtYuan(v)}
        </Text>
      ),
    },
    { title: '渠道', dataIndex: 'channel_breakdown', render: (v?: string) => <Tag>{fmtChannel(v)}</Tag> },
    {
      title: '已分账',
      dataIndex: 'split_status',
      width: 110,
      render: (v?: string) => {
        const b = splitStatusBadge(v);
        return (
          <span
            style={{
              display: 'inline-block',
              padding: '2px 10px',
              borderRadius: 999,
              fontSize: 12,
              lineHeight: '20px',
              color: b.color,
              background: b.bg,
            }}
          >
            {b.text}
          </span>
        );
      },
    },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div className="hp-split-search">
        <Select
          allowClear
          placeholder="全部门店"
          style={{ width: 200 }}
          value={storeId}
          onChange={setStoreId}
          options={storeOptions}
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

      {excess && <Alert type="info" showIcon message="日期范围包含今天，今天数据为已生成部分（今日 02:00 后才会生成完整日报）。" />}

      <Card title="门店每日汇总" extra={<Text type="secondary">共计 {rows.length} 条记录</Text>}>
        {stats.isLoading ? (
          <Empty description="加载中..." />
        ) : rows.length === 0 ? (
          <Empty className="hp-split-empty" description="该条件下暂无门店日报数据，请等待每日 02:00 自动生成。" />
        ) : (
          <Table className="hp-zebra hp-tx-table" rowKey={(r) => `${r.id}-${r.store_id}-${r.biz_date}`} columns={columns} dataSource={rows} pagination={{ pageSize: 20 }} />
        )}
      </Card>
    </div>
  );
};