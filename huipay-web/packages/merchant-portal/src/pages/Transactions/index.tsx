// 交易列表页：筛选 / 分页 / 汇总 / 订单详情（含通道侧状态主动查询）
import React from 'react';
import {
  Alert,
  App as AntApp,
  Button,
  Card,
  DatePicker,
  Descriptions,
  Drawer,
  Empty,
  Input,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import {
  AccountBookOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  CreditCardOutlined,
  ExclamationCircleOutlined,
  SearchOutlined,
  WechatOutlined,
  PayCircleOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { StatusTag } from '@huipay/ui-kit';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import type { Order, QueryResult } from '@huipay/shared';
import { getOrder, listOrders, listStores, queryOrder } from '../../services/user';
import { KpiCard } from '../../components/KpiCard';

const { RangePicker } = DatePicker;

const channelLabel: Record<string, string> = { WECHAT: '微信支付', ALIPAY: '支付宝' };

const channelIcon = (v?: string) => {
  if (v === 'WECHAT') return <WechatOutlined style={{ color: '#07c160' }} />;
  if (v === 'ALIPAY') return <CreditCardOutlined style={{ color: '#1677ff' }} />;
  return <PayCircleOutlined style={{ color: '#999' }} />;
};

const statusTag = (v: string) => <StatusTag status={v} />;

export const Transactions: React.FC = () => {
  const { message } = AntApp.useApp();
  const [searchParams] = useSearchParams();
  const [page, setPage] = React.useState(1);
  const [size, setSize] = React.useState(20);
  const [status, setStatus] = React.useState<string | undefined>();
  const [channel, setChannel] = React.useState<string | undefined>();
  const [codeId, setCodeId] = React.useState('');
  const [storeId, setStoreId] = React.useState<number | undefined>();
  const [range, setRange] = React.useState<[string, string] | null>(null);

  // 已提交的筛选条件：仅点击"查询"时更新，避免改下拉/日期就触发自动查询
  const [applied, setApplied] = React.useState<{
    status?: string;
    channel?: string;
    codeId: string;
    storeId?: number;
    range: [string, string] | null;
  }>({ status, channel, codeId, storeId, range });

  const [detailOrderNo, setDetailOrderNo] = React.useState<string | null>(searchParams.get('order_no'));
  const [channelResult, setChannelResult] = React.useState<QueryResult | null>(null);
  const [queryLoading, setQueryLoading] = React.useState(false);
  const [queryError, setQueryError] = React.useState('');

  const listQuery = useQuery({
    queryKey: ['orders', page, size, applied.status, applied.channel, applied.codeId, applied.storeId, applied.range],
    queryFn: () =>
      listOrders({
        page,
        size,
        status: applied.status,
        channel: applied.channel,
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

  const filterLabel = {
    status: '全部状态',
    channel: '全部通道',
    codeId: '全部码牌',
    storeId: '全部门店',
  };

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
      title: '来源',
      key: 'source',
      width: 150,
      render: (_: unknown, r: Order) => (
        <Space direction="vertical" size={0}>
          <span style={{ fontSize: 13 }}>{r.code_id || '-'}</span>
          <span style={{ fontSize: 12, color: '#8a94a6' }}>{r.store_name || '未关联门店'}</span>
        </Space>
      ),
    },
    {
      title: '金额',
      dataIndex: 'amount',
      key: 'amount',
      align: 'right' as const,
      width: 110,
      render: (v: number) => <span style={{ fontWeight: 600 }}>{formatCents(v)}</span>,
    },
    {
      title: '实付',
      dataIndex: 'paid_amount',
      key: 'paid_amount',
      align: 'right' as const,
      width: 110,
      render: (v: number, r: Order) =>
        r.status === 'PAID' ? (
          <span style={{ color: '#06b6a4', fontWeight: 600 }}>{formatCents(v)}</span>
        ) : (
          <span style={{ color: '#b0b8c7' }}>{formatCents(v ?? 0)}</span>
        ),
    },
    {
      title: '通道',
      dataIndex: 'channel',
      key: 'channel',
      width: 90,
      render: (v?: string) => (
        <Space size={4} style={{ color: '#3a4658' }}>
          {channelIcon(v)}
          <span style={{ fontSize: 13 }}>{v ? channelLabel[v] ?? v : '-'}</span>
        </Space>
      ),
    },
    { title: '订单状态', dataIndex: 'status', key: 'status', width: 96, render: statusTag },
    { title: '分账状态', dataIndex: 'split_status', key: 'split_status', width: 96, render: statusTag },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 170,
      render: (v: string) => <span style={{ color: '#5b6b81', fontSize: 13 }}>{formatDateTime(v)}</span>,
    },
  ];

  return (
    <div className="hp-page" style={{ minHeight: 'calc(100vh - 84px)' }}>
      {/* 页面头部 */}
      <div
        style={{
          display: 'flex',
          alignItems: 'flex-end',
          justifyContent: 'space-between',
          marginBottom: 20,
          flexWrap: 'wrap',
          gap: 12,
        }}
      >
        <div>
          <Typography.Title level={4} style={{ margin: 0, fontWeight: 700 }}>
            交易列表
          </Typography.Title>
          <Typography.Text type="secondary" style={{ fontSize: 13 }}>
            查看订单流水、支付状态与分账进度
          </Typography.Text>
        </div>
        <Tag color="blue" style={{ borderRadius: 10, padding: '3px 12px', fontWeight: 600 }}>
          共 {listQuery.data?.total ?? 0} 笔交易
        </Tag>
      </div>

      {/* 汇总 KPI */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 16, marginBottom: 20 }}>
        <KpiCard
          title="本页订单数"
          value={items.length}
          icon={<AccountBookOutlined />}
          color="#1e6fff"
          loading={listQuery.isLoading}
          formatter={(v) => `${v} / ${listQuery.data?.total ?? 0}`}
        />
        <KpiCard
          title="本页金额合计"
          value={pageAmount}
          icon={<CheckCircleOutlined />}
          color="#06b6a4"
          loading={listQuery.isLoading}
          formatter={(v) => formatCents(v)}
        />
        <KpiCard
          title="本页实付合计"
          value={pagePaid}
          icon={<PayCircleOutlined />}
          color="#8b5cf6"
          loading={listQuery.isLoading}
          formatter={(v) => formatCents(v)}
        />
      </div>

      {/* 筛选区 */}
      <Card
        title={<Typography.Text strong style={{ fontSize: 15 }}>交易筛选</Typography.Text>}
        extra={<Typography.Text type="secondary" style={{ fontSize: 12 }}>修改条件后点击「查询」生效</Typography.Text>}
        style={{ boxShadow: 'var(--shadow-card)' }}
      >
        <Space wrap size="large" style={{ rowGap: 16 }}>
          <div>
            <Typography.Text strong style={{ fontSize: 12, color: '#8a94a6', display: 'block', marginBottom: 6 }}>
              订单状态
            </Typography.Text>
            <Select
              allowClear
              placeholder={filterLabel.status}
              style={{ width: 140 }}
              value={status}
              onChange={setStatus}
              suffixIcon={<ClockCircleOutlined />}
              options={[
                { value: 'CREATED', label: '待支付' },
                { value: 'PAID', label: '已支付' },
                { value: 'CLOSED', label: '已关闭' },
              ]}
            />
          </div>
          <div>
            <Typography.Text strong style={{ fontSize: 12, color: '#8a94a6', display: 'block', marginBottom: 6 }}>
              支付通道
            </Typography.Text>
            <Select
              allowClear
              placeholder={filterLabel.channel}
              style={{ width: 140 }}
              value={channel}
              onChange={setChannel}
              options={[
                { value: 'WECHAT', label: '微信支付' },
                { value: 'ALIPAY', label: '支付宝' },
              ]}
            />
          </div>
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
              onClick={() => {
                setApplied({ status, channel, codeId, storeId, range });
                setPage(1);
              }}
            >
              查询
            </Button>
            <Button
              onClick={() => {
                setStatus(undefined);
                setChannel(undefined);
                setCodeId('');
                setStoreId(undefined);
                setRange(null);
                setApplied({ status: undefined, channel: undefined, codeId: '', storeId: undefined, range: null });
                setPage(1);
              }}
            >
              重置
            </Button>
          </div>
        </Space>
      </Card>

      {/* 交易表格 */}
      <Card style={{ marginTop: 20, boxShadow: 'var(--shadow-card)' }}>
        <Table<Order>
          className="hp-zebra hp-tx-table"
          rowKey="order_no"
          columns={columns}
          dataSource={items}
          loading={listQuery.isLoading}
          locale={{
            emptyText: (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={<Typography.Text type="secondary">暂无符合条件的交易</Typography.Text>}
              />
            ),
          }}
          onRow={(record) => ({ onClick: () => openDetail(record), style: { cursor: 'pointer' } })}
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
            <span>订单详情</span>
            {detail && statusTag(detail.status)}
          </Space>
        }
        width={520}
        open={!!detailOrderNo}
        onClose={closeDetail}
      >
        {detail && (
          <>
            <Descriptions column={1} size="middle" bordered>
              <Descriptions.Item label="订单号">
                <Typography.Text code style={{ fontSize: 12 }}>{detail.order_no}</Typography.Text>
              </Descriptions.Item>
              <Descriptions.Item label="商户单号">{detail.merchant_order_no}</Descriptions.Item>
              <Descriptions.Item label="来源码牌">{detail.code_id || '-'}</Descriptions.Item>
              <Descriptions.Item label="所属门店">{detail.store_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="订单金额">{formatCents(detail.amount)}</Descriptions.Item>
              <Descriptions.Item label="实付金额">
                <span style={{ color: '#06b6a4', fontWeight: 600 }}>{formatCents(detail.paid_amount ?? 0)}</span>
              </Descriptions.Item>
              <Descriptions.Item label="支付通道">
                <Space size={4}>
                  {channelIcon(detail.channel)}
                  <span>{detail.channel ? channelLabel[detail.channel] ?? detail.channel : '-'}</span>
                </Space>
              </Descriptions.Item>
              <Descriptions.Item label="渠道交易号">{detail.channel_trade_no || '-'}</Descriptions.Item>
              <Descriptions.Item label="订单状态">{statusTag(detail.status)}</Descriptions.Item>
              <Descriptions.Item label="分账状态">{statusTag(detail.split_status)}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{formatDateTime(detail.created_at)}</Descriptions.Item>
              {detail.paid_at && <Descriptions.Item label="支付时间">{formatDateTime(detail.paid_at)}</Descriptions.Item>}
              {detail.expire_at && <Descriptions.Item label="过期时间">{formatDateTime(detail.expire_at)}</Descriptions.Item>}
            </Descriptions>

            <Space style={{ marginTop: 20 }} wrap>
              <Button loading={queryLoading} onClick={runChannelQuery} disabled={!detail.channel}>
                查询通道状态
              </Button>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                仅查询，不修改本地订单；以回调结果为准
              </Typography.Text>
            </Space>

            {queryError && (
              <Alert
                type="error"
                showIcon
                icon={<ExclamationCircleOutlined />}
                style={{ marginTop: 12 }}
                message={queryError}
              />
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
                        {channelResult.paid_at && (
                          <span>支付时间：{formatDateTime(new Date(channelResult.paid_at * 1000).toISOString())}</span>
                        )}
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
    </div>
  );
};