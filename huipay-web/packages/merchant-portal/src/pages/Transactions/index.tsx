// 交易列表页：筛选 / 分页 / 汇总 / 订单详情（含通道侧状态主动查询）
import React from 'react';
import { Alert, Button, Card, DatePicker, Descriptions, Drawer, Input, Select, Space, Statistic, Table, Tag, Typography } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { StatusTag } from '@huipay/ui-kit';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import type { Order, QueryResult } from '@huipay/shared';
import { getOrder, listOrders, queryOrder } from '../../services/user';

const { RangePicker } = DatePicker;

const channelLabel: Record<string, string> = { WECHAT: '微信支付', ALIPAY: '支付宝' };

const statusTag = (v: string) => <StatusTag status={v} />;

export const Transactions: React.FC = () => {
  const [searchParams] = useSearchParams();
  const [page, setPage] = React.useState(1);
  const [size, setSize] = React.useState(20);
  const [status, setStatus] = React.useState<string | undefined>();
  const [channel, setChannel] = React.useState<string | undefined>();
  const [codeId, setCodeId] = React.useState('');
  const [range, setRange] = React.useState<[string, string] | null>(null);

  const [detailOrderNo, setDetailOrderNo] = React.useState<string | null>(searchParams.get('order_no'));
  const [channelResult, setChannelResult] = React.useState<QueryResult | null>(null);
  const [queryLoading, setQueryLoading] = React.useState(false);
  const [queryError, setQueryError] = React.useState('');

  const listQuery = useQuery({
    queryKey: ['orders', page, size, status, channel, codeId, range],
    queryFn: () =>
      listOrders({
        page,
        size,
        status,
        channel,
        code_id: codeId || undefined,
        start: range?.[0],
        end: range?.[1],
      }),
  });

  const detailQuery = useQuery({
    queryKey: ['order-detail', detailOrderNo],
    queryFn: () => getOrder(detailOrderNo!),
    enabled: !!detailOrderNo,
  });

  const detail = detailQuery.data;
  const items = listQuery.data?.items ?? [];

  const pageAmount = items.reduce((s, o) => s + o.amount, 0);
  const pagePaid = items.reduce((s, o) => s + (o.paid_amount ?? 0), 0);

  const openDetail = (order: Order) => {
    setDetailOrderNo(order.order_no);
    setChannelResult(null);
    setQueryError('');
  };

  const closeDetail = () => {
    setDetailOrderNo(null);
    setChannelResult(null);
    setQueryError('');
  };

  const runChannelQuery = async () => {
    if (!detailOrderNo) return;
    setQueryLoading(true);
    setQueryError('');
    setChannelResult(null);
    try {
      setChannelResult(await queryOrder(detailOrderNo));
    } catch (e) {
      setQueryError((e as Error)?.message ?? '通道查询失败');
    } finally {
      setQueryLoading(false);
    }
  };

  const columns = [
    { title: '订单号', dataIndex: 'order_no', key: 'order_no', width: 220 },
    { title: '商户单号', dataIndex: 'merchant_order_no', key: 'merchant_order_no', width: 180 },
    { title: '来源码牌', dataIndex: 'code_id', key: 'code_id', width: 100, render: (v?: string) => v || '-' },
    { title: '金额', dataIndex: 'amount', key: 'amount', render: (v: number) => formatCents(v) },
    { title: '实付', dataIndex: 'paid_amount', key: 'paid_amount', render: (v: number) => formatCents(v) },
    { title: '通道', dataIndex: 'channel', key: 'channel', width: 100, render: (v?: string) => (v ? channelLabel[v] ?? v : '-') },
    { title: '订单状态', dataIndex: 'status', key: 'status', width: 100, render: statusTag },
    { title: '分账状态', dataIndex: 'split_status', key: 'split_status', width: 100, render: statusTag },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 180, render: (v: string) => formatDateTime(v) },
  ];

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card>
        <Space wrap>
          <Select
            allowClear
            placeholder="订单状态"
            style={{ width: 140 }}
            value={status}
            onChange={(v) => {
              setStatus(v);
              setPage(1);
            }}
            options={[
              { value: 'CREATED', label: '待支付' },
              { value: 'PAID', label: '已支付' },
              { value: 'CLOSED', label: '已关闭' },
            ]}
          />
          <Select
            allowClear
            placeholder="支付通道"
            style={{ width: 140 }}
            value={channel}
            onChange={(v) => {
              setChannel(v);
              setPage(1);
            }}
            options={[
              { value: 'WECHAT', label: '微信支付' },
              { value: 'ALIPAY', label: '支付宝' },
            ]}
          />
          <Input
            placeholder="来源码牌短码"
            style={{ width: 160 }}
            allowClear
            value={codeId}
            onChange={(e) => setCodeId(e.target.value)}
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
          <Button
            onClick={() => {
              setStatus(undefined);
              setChannel(undefined);
              setCodeId('');
              setRange(null);
              setPage(1);
            }}
          >
            重置
          </Button>
        </Space>
      </Card>

      <Card title="交易列表">
        <Space size="large" style={{ marginBottom: 16 }}>
          <Statistic title="本页订单数" value={items.length} suffix={`/ ${listQuery.data?.total ?? 0}`} />
          <Statistic title="本页金额合计" value={pageAmount} formatter={(v) => formatCents(Number(v))} />
          <Statistic title="本页实付合计" value={pagePaid} formatter={(v) => formatCents(Number(v))} valueStyle={{ color: '#06b6a4' }} />
        </Space>
        <Table<Order>
          rowKey="order_no"
          columns={columns}
          dataSource={items}
          loading={listQuery.isLoading}
          onRow={(record) => ({ onClick: () => openDetail(record), style: { cursor: 'pointer' } })}
          pagination={{
            current: page,
            pageSize: size,
            total: listQuery.data?.total ?? 0,
            showSizeChanger: true,
            onChange: (p, s) => {
              setPage(p);
              setSize(s);
            },
          }}
        />
      </Card>

      <Drawer
        title={`订单详情 ${detail?.order_no ?? ''}`}
        width={520}
        open={!!detailOrderNo}
        onClose={closeDetail}
      >
        {detail && (
          <>
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="订单号">{detail.order_no}</Descriptions.Item>
              <Descriptions.Item label="商户单号">{detail.merchant_order_no}</Descriptions.Item>
              <Descriptions.Item label="来源码牌">{detail.code_id || '-'}</Descriptions.Item>
              <Descriptions.Item label="订单金额">{formatCents(detail.amount)}</Descriptions.Item>
              <Descriptions.Item label="实付金额">{formatCents(detail.paid_amount ?? 0)}</Descriptions.Item>
              <Descriptions.Item label="支付通道">
                {detail.channel ? channelLabel[detail.channel] ?? detail.channel : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="渠道交易号">{detail.channel_trade_no || '-'}</Descriptions.Item>
              <Descriptions.Item label="订单状态">{statusTag(detail.status)}</Descriptions.Item>
              <Descriptions.Item label="分账状态">{statusTag(detail.split_status)}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{formatDateTime(detail.created_at)}</Descriptions.Item>
              {detail.paid_at && <Descriptions.Item label="支付时间">{formatDateTime(detail.paid_at)}</Descriptions.Item>}
              {detail.expire_at && <Descriptions.Item label="过期时间">{formatDateTime(detail.expire_at)}</Descriptions.Item>}
            </Descriptions>

            <Space style={{ marginTop: 16 }}>
              <Button loading={queryLoading} onClick={runChannelQuery} disabled={!detail.channel}>
                查询通道状态
              </Button>
              <Typography.Text type="secondary">仅查询，不修改本地订单；以回调结果为准</Typography.Text>
            </Space>

            {queryError && (
              <Alert type="error" showIcon style={{ marginTop: 12 }} message={queryError} />
            )}
            {channelResult && (
              <Alert
                type={channelResult.paid ? 'success' : 'info'}
                showIcon
                style={{ marginTop: 12 }}
                message={
                  <Space direction="vertical" size={0}>
                    <span>
                      通道侧状态：
                      {channelResult.paid ? <Tag color="success">已支付</Tag> : <Tag>未支付</Tag>}
                    </span>
                    {channelResult.paid && (
                      <>
                        <span>实付金额：{formatCents(channelResult.paid_amount)}</span>
                        <span>渠道交易号：{channelResult.channel_trade_no || '-'}</span>
                        {channelResult.paid_at ? (
                          <span>支付时间：{formatDateTime(new Date(channelResult.paid_at * 1000).toISOString())}</span>
                        ) : null}
                      </>
                    )}
                    {!channelResult.paid && <span>本地状态：{channelResult.local_status}</span>}
                  </Space>
                }
              />
            )}
          </>
        )}
      </Drawer>
    </Space>
  );
};
