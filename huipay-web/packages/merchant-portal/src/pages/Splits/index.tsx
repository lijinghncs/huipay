// 分账记录页：分页查看每次分账（按订单聚合），查看明细展示各接收方分成金额
import React from 'react';
import { App as AntApp, Button, Card, Drawer, Descriptions, Empty, Space, Table, Tag, Typography } from 'antd';
import { BranchesOutlined, EyeOutlined } from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { StatusTag } from '@huipay/ui-kit';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import {
  getSplitExecutionDetail,
  listSplitExecutions,
  type SplitExecutionDetail,
  type SplitExecutionSummary,
} from '../../services/user';

const { Text } = Typography;

const channelLabel: Record<string, string> = { WECHAT: '微信支付', ALIPAY: '支付宝' };

const receiverTypeLabel: Record<string, string> = {
  STORE: '门店',
  MERCHANT: '商户',
  PROMOTER: '推广员',
  PLATFORM: '平台',
  ISV: '服务商',
};

/** 分账记录聚合状态标签（成功 / 部分失败 / 失败）。 */
const splitStatusTag = (v: string) => {
  if (v === 'SUCCESS') return <Tag color="success" style={{ borderRadius: 10 }}>成功</Tag>;
  if (v === 'PARTIAL') return <Tag color="warning" style={{ borderRadius: 10 }}>部分失败</Tag>;
  if (v === 'FAILED') return <Tag color="error" style={{ borderRadius: 10 }}>失败</Tag>;
  return <Tag style={{ borderRadius: 10 }}>{v}</Tag>;
};

export const Splits: React.FC = () => {
  const { message } = AntApp.useApp();
  const [page, setPage] = React.useState(1);
  const [size, setSize] = React.useState(20);
  const [detailOrderNo, setDetailOrderNo] = React.useState<string | null>(null);

  const listQuery = useQuery({
    queryKey: ['split-executions', page, size],
    queryFn: () => listSplitExecutions({ page, size }),
  });

  const detailQuery = useQuery({
    queryKey: ['split-execution-detail', detailOrderNo],
    queryFn: () => getSplitExecutionDetail(detailOrderNo!),
    enabled: !!detailOrderNo,
  });

  const openDetail = (orderNo: string) => setDetailOrderNo(orderNo);
  const closeDetail = () => setDetailOrderNo(null);

  const detailItems = detailQuery.data?.items ?? [];
  const detailTotal = detailItems.reduce((s, d) => s + d.amount, 0);

  const columns = [
    {
      title: '订单号',
      dataIndex: 'order_no',
      key: 'order_no',
      width: 200,
      render: (v: string) => (
        <Typography.Text code style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', fontSize: 12 }}>
          {v}
        </Typography.Text>
      ),
    },
    {
      title: '商户',
      dataIndex: 'merchant_name',
      key: 'merchant_name',
      width: 140,
      render: (v?: string) => (v ? <span style={{ fontSize: 13 }}>{v}</span> : <Text type="secondary">-</Text>),
    },
    {
      title: '分账总额',
      dataIndex: 'total_amount',
      key: 'total_amount',
      align: 'right' as const,
      width: 120,
      render: (v: number) => <span style={{ fontWeight: 600 }}>{formatCents(v)}</span>,
    },
    {
      title: '接收方数',
      dataIndex: 'receiver_count',
      key: 'receiver_count',
      width: 90,
      render: (v: number) => <span style={{ fontSize: 13 }}>{v}</span>,
    },
    {
      title: '通道',
      dataIndex: 'channel',
      key: 'channel',
      width: 100,
      render: (v?: string) => (v ? channelLabel[v] ?? v : '-'),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: splitStatusTag,
    },
    {
      title: '执行时间',
      dataIndex: 'executed_at',
      key: 'executed_at',
      width: 170,
      render: (v?: string) => (v ? <span style={{ color: '#5b6b81', fontSize: 13 }}>{formatDateTime(v)}</span> : '-'),
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_: unknown, r: SplitExecutionSummary) => (
        <Button size="small" type="link" icon={<EyeOutlined />} onClick={() => openDetail(r.order_no)}>
          查看明细
        </Button>
      ),
    },
  ];

  return (
    <div className="hp-page" style={{ maxWidth: 1180 }}>
      <Card
        title={
          <Space>
            <span>分账记录</span>
            <Tag style={{ borderRadius: 10 }} color="blue">{listQuery.data?.total ?? 0}</Tag>
          </Space>
        }
      >
        <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
          仅展示已执行分账的订单；点击「查看明细」可查看该次分账给各门店/接收方分成的金额。
        </Text>
        <Table<SplitExecutionSummary>
          className="hp-zebra"
          rowKey="order_no"
          columns={columns}
          dataSource={listQuery.data?.items ?? []}
          loading={listQuery.isLoading}
          locale={{
            emptyText: (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={<Typography.Text type="secondary">暂无分账记录</Typography.Text>}
              />
            ),
          }}
          pagination={{
            current: page,
            pageSize: size,
            total: listQuery.data?.total ?? 0,
            showSizeChanger: true,
            showTotal: (t) => <span style={{ color: '#8a94a6' }}>共 {t} 条</span>,
            onChange: (p, s) => {
              setPage(p);
              setSize(s);
            },
          }}
        />
      </Card>

      <Drawer
        title={
          <Space>
            <BranchesOutlined />
            <span>分账明细</span>
            {detailOrderNo && (
              <Typography.Text code style={{ fontSize: 12 }}>{detailOrderNo}</Typography.Text>
            )}
          </Space>
        }
        width={560}
        open={!!detailOrderNo}
        onClose={closeDetail}
      >
        {detailQuery.isLoading ? (
          <div style={{ textAlign: 'center', padding: 32 }}>加载中…</div>
        ) : detailItems.length === 0 ? (
          <Empty description="无分账明细" />
        ) : (
          <>
            <Descriptions column={1} size="small" bordered style={{ marginBottom: 16 }}>
              <Descriptions.Item label="分账总额">
                <span style={{ fontWeight: 600 }}>{formatCents(detailTotal)}</span>
              </Descriptions.Item>
              <Descriptions.Item label="接收方数">{detailItems.length}</Descriptions.Item>
            </Descriptions>
            <Table<SplitExecutionDetail>
              rowKey={(r) => `${r.receiver_entity_id}-${r.level}`}
              size="small"
              pagination={false}
              dataSource={detailItems}
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
                  title: '金额',
                  dataIndex: 'amount',
                  key: 'amount',
                  align: 'right' as const,
                  render: (v: number) => <span style={{ fontWeight: 600 }}>{formatCents(v)}</span>,
                },
                {
                  title: '状态',
                  dataIndex: 'status',
                  key: 'status',
                  width: 90,
                  render: (v: string) => <StatusTag status={v} />,
                },
              ]}
            />
            {detailItems.some((d) => d.last_error) && (
              <div style={{ marginTop: 16 }}>
                {detailItems
                  .filter((d) => d.last_error)
                  .map((d) => (
                    <div key={`${d.receiver_entity_id}-${d.level}`} style={{ marginBottom: 8 }}>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        {d.receiver_name}：{d.last_error}
                      </Text>
                    </div>
                  ))}
              </div>
            )}
          </>
        )}
      </Drawer>
    </div>
  );
};