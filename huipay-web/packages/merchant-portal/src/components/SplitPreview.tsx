// 分账试算/账单预览明细组件（账单、按时间段、规则试算共用）
import React from 'react';
import { Empty, Space, Table, Typography } from 'antd';
import { formatCents } from '@huipay/shared/utils';
import type { SplitPreviewItem } from '../services/user';

const { Text } = Typography;

interface SplitPreviewProps {
  items: SplitPreviewItem[];
  totalAmount: number; // 分账基数总额（分）
  merchantRemain: number; // 未分配归商户金额（分）
  loading?: boolean;
}

const receiverTypeLabel: Record<string, string> = {
  STORE: '门店',
  MERCHANT: '商户',
  PROMOTER: '推广员',
  PLATFORM: '平台',
  ISV: '服务商',
};

const MONO = { fontVariantNumeric: 'tabular-nums' as const, fontFamily: 'Fira Code, Consolas, Monaco, monospace' };

/** 分账预览明细表：接收方 / 金额 / 占比，底部合计 + 剩余归商户。 */
export const SplitPreview: React.FC<SplitPreviewProps> = ({ items, totalAmount, merchantRemain, loading }) => {
  if (loading) {
    return <div style={{ textAlign: 'center', padding: 24 }}>试算中…</div>;
  }
  if (!items.length) {
    return <Empty description="无可分配明细（请确认时间段内有实收或门店有实收）" />;
  }
  const assigned = items.reduce((s, it) => s + it.amount, 0);
  return (
    <div>
      <Table<SplitPreviewItem>
        size="small"
        rowKey={(r) => `${r.receiver_entity_id}-${r.receiver_type}`}
        pagination={false}
        dataSource={items}
        columns={[
          {
            title: '接收方',
            dataIndex: 'receiver_name',
            key: 'receiver_name',
            render: (v: string, r) => (
              <Space direction="vertical" size={0}>
                <span style={{ fontWeight: 600 }}>{v}</span>
                <span style={{ fontSize: 12, color: '#8a94a6' }}>
                  {receiverTypeLabel[r.receiver_type] ?? r.receiver_type} #{r.receiver_entity_id}
                </span>
              </Space>
            ),
          },
          {
            title: '可分金额',
            dataIndex: 'amount',
            key: 'amount',
            align: 'right' as const,
            render: (v: number) => <span style={{ fontWeight: 600, ...MONO }}>{formatCents(v)}</span>,
          },
          {
            title: '占比',
            dataIndex: 'ratio',
            key: 'ratio',
            align: 'right' as const,
            width: 110,
            render: (v: number) => (
              <span style={{ color: '#5b6b81', ...MONO }}>{totalAmount > 0 ? `${((v / 10000) * 100).toFixed(2)}%` : '-'}</span>
            ),
          },
        ]}
      />
      <div
        style={{
          marginTop: 12,
          padding: '12px 16px',
          border: '1px solid #e5e9f2',
          borderRadius: 10,
          background: '#fafbfd',
        }}
      >
        <Space style={{ width: '100%', justifyContent: 'space-between' }}>
          <Text type="secondary">已分配合计</Text>
          <Text strong style={MONO}>{formatCents(assigned)}</Text>
        </Space>
        <Space style={{ width: '100%', justifyContent: 'space-between', marginTop: 6 }}>
          <Text type="secondary">剩余归商户</Text>
          <Text strong style={{ ...MONO, color: merchantRemain > 0 ? '#1e6fff' : '#8a94a6' }}>
            {formatCents(merchantRemain)}
          </Text>
        </Space>
      </div>
    </div>
  );
};