// 分账明细页：输入分账批次号，查询该批次下的门店明细；点击门店查看该门店在本批次的订单交易明细
import React from 'react';
import { App as AntApp, Button, Card, Drawer, Empty, Input, Space, Table, Typography } from 'antd';
import { BranchesOutlined, ReloadOutlined, SearchOutlined, ShopOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import { getBillStoreOrders, getBillStores, type BillStoreItem, type BillStoreOrder } from '../../services/user';

const { Text } = Typography;

/** 状态徽章：圆点 + 文案。 */
const BillStatusBadge: React.FC<{ status: string }> = ({ status }) => {
  const map: Record<string, { text: string; cls: string }> = {
    PENDING: { text: '待审批', cls: 'hp-split-status--pending' },
    APPROVED: { text: '已通过', cls: 'hp-split-status--approved' },
    REJECTED: { text: '已驳回', cls: 'hp-split-status--rejected' },
    EXECUTED: { text: '已执行', cls: 'hp-split-status--executed' },
  };
  const item = map[status] ?? { text: status, cls: 'hp-split-status--other' };
  return (
    <span className={`hp-split-status ${item.cls}`}>
      <span className="hp-split-status-dot" />
      {item.text}
    </span>
  );
};

/** 订单状态标签。 */
const OrderStatusLabel: React.FC<{ status: string }> = ({ status }) => {
  const map: Record<string, { text: string; color: string; bg: string }> = {
    PAID: { text: '已支付', color: '#047857', bg: 'rgba(16,185,129,0.14)' },
    CREATED: { text: '待支付', color: '#1d4ed8', bg: 'rgba(59,130,246,0.14)' },
    CLOSED: { text: '已关闭', color: '#64748b', bg: 'rgba(100,116,139,0.14)' },
    REFUNDED: { text: '已退款', color: '#b91c1c', bg: 'rgba(239,68,68,0.14)' },
  };
  const item = map[status] ?? { text: status, color: '#475569', bg: 'rgba(100,116,139,0.1)' };
  return (
    <span
      className="hp-split-status hp-split-status--other"
      style={{ color: item.color, background: item.bg }}
    >
      {item.text}
    </span>
  );
};

const fmtPeriod = (s: string, e: string) => {
  const sameYear = dayjs(s).isSame(dayjs(e), 'year');
  const fmt = sameYear ? 'MM-DD HH:mm' : 'YYYY-MM-DD HH:mm';
  return `${dayjs(s).format(fmt)} ~ ${dayjs(e).format(fmt)}`;
};

export const Splits: React.FC = () => {
  const { message } = AntApp.useApp();
  const [searchParams, setSearchParams] = useSearchParams();
  const urlBatch = (searchParams.get('batch_no') ?? '').trim();
  const [batchNo, setBatchNo] = React.useState(urlBatch);
  const [orderStore, setOrderStore] = React.useState<{ batchNo: string; storeId: number } | null>(null);

  // URL 携带批次号（从分账单详情跳转）时，同步输入框并自动查询
  React.useEffect(() => {
    setBatchNo(urlBatch);
  }, [urlBatch]);

  const searchedBatch = urlBatch || null;

  const summaryQuery = useQuery({
    queryKey: ['split-bill-stores', searchedBatch],
    queryFn: () => getBillStores(searchedBatch!),
    enabled: !!searchedBatch,
  });

  const ordersQuery = useQuery({
    queryKey: ['split-bill-store-orders', orderStore?.batchNo, orderStore?.storeId],
    queryFn: () => getBillStoreOrders(orderStore!.batchNo, orderStore!.storeId),
    enabled: !!orderStore,
  });

  const summary = summaryQuery.data;

  const handleSearch = () => {
    const v = batchNo.trim();
    if (!v) {
      message.warning('请输入分账批次号');
      return;
    }
    setOrderStore(null);
    setSearchParams({ batch_no: v }, { replace: true });
  };

  const handleReset = () => {
    setBatchNo('');
    setOrderStore(null);
    setSearchParams({}, { replace: true });
  };

  const openOrders = (s: BillStoreItem) => {
    if (!searchedBatch) return;
    setOrderStore({ batchNo: searchedBatch, storeId: s.store_id });
  };
  const closeOrders = () => setOrderStore(null);

  const orders = ordersQuery.data;

  return (
    <div className="hp-page">
      {/* 页头 */}
      <div className="hp-split-head">
        <div>
          <div className="hp-split-head-title">分账明细</div>
          <div className="hp-split-head-sub">按分账批次号查看门店分成，追踪对应订单交易</div>
        </div>
      </div>

      {/* 查询区 */}
      <div className="hp-split-search">
        <Input
          className="hp-split-search-input"
          prefix={<SearchOutlined style={{ color: '#8a94a6' }} />}
          placeholder="请输入分账批次号，如 SP5-1786636800-1786693800"
          value={batchNo}
          onChange={(e) => setBatchNo(e.target.value)}
          onPressEnter={handleSearch}
          allowClear
        />
        <Button
          className="hp-split-search-btn"
          type="primary"
          icon={<SearchOutlined />}
          onClick={handleSearch}
          loading={summaryQuery.isFetching}
        >
          查询
        </Button>
        <Button icon={<ReloadOutlined />} onClick={handleReset}>
          重置
        </Button>
      </div>

      {summaryQuery.isLoading ? (
        <Card>
          <div style={{ textAlign: 'center', padding: 48 }}>
            <BranchesOutlined spin style={{ fontSize: 28, color: '#1e6fff' }} />
            <div style={{ marginTop: 12, color: '#66718b' }}>正在查询批次门店明细…</div>
          </div>
        </Card>
      ) : summaryQuery.isError ? (
        <Card>
          <div className="hp-split-empty">
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="查询失败，请核对批次号后重试" />
          </div>
        </Card>
      ) : summary ? (
        <>
          {/* 批次概览 Hero */}
          <div className="hp-split-hero">
            <div className="hp-split-hero-main">
              <div className="hp-split-hero-batch">
                <span className="hp-split-hero-batch-label">批次号</span>
                <span className="hp-split-hero-title">{summary.batch_no}</span>
                <BillStatusBadge status={summary.status} />
              </div>
              <div className="hp-split-hero-rule">
                {summary.rule_name} · {summary.rule_code}
              </div>
              <div className="hp-split-hero-meta">
                <div className="hp-split-hero-meta-item">
                  <span className="hp-split-hero-meta-label">时间段</span>
                  <span className="hp-split-hero-meta-value">{fmtPeriod(summary.start_time, summary.end_time)}</span>
                </div>
                <div className="hp-split-hero-meta-item">
                  <span className="hp-split-hero-meta-label">参与门店</span>
                  <span className="hp-split-hero-meta-value">{summary.stores.length} 家</span>
                </div>
              </div>
            </div>
            <div className="hp-split-hero-amount">
              <div className="hp-split-hero-amount-label">分账总额</div>
              <div className="hp-split-hero-amount-value">{formatCents(summary.total_amount)}</div>
              <div className="hp-split-hero-amount-sub">按规则分配给各门店</div>
            </div>
          </div>

          {/* 门店明细 */}
          <Card style={{ borderRadius: 14 }}>
            <div className="hp-split-section-title">
              <ShopOutlined />
              <span>门店明细</span>
            </div>
            {summary.stores.length === 0 ? (
              <div className="hp-split-empty">
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该批次无门店明细" />
              </div>
            ) : (
              <Table<BillStoreItem>
                className="hp-zebra"
                rowKey="store_id"
                dataSource={summary.stores}
                pagination={false}
                columns={[
                  {
                    title: '门店',
                    key: 'store',
                    render: (_: unknown, r: BillStoreItem) => (
                      <div className="hp-split-store-cell">
                        <div className="hp-split-store-avatar">{(r.store_name || '门').slice(0, 1)}</div>
                        <div>
                          <div className="hp-split-store-name">{r.store_name || `门店 #${r.store_id}`}</div>
                          <div className="hp-split-store-id">门店 #{r.store_id}</div>
                        </div>
                      </div>
                    ),
                  },
                  {
                    title: '可分金额',
                    dataIndex: 'amount',
                    key: 'amount',
                    align: 'right' as const,
                    render: (v: number) => <span className="hp-split-amount">{formatCents(v)}</span>,
                  },
                  {
                    title: '占比',
                    key: 'ratio',
                    align: 'right' as const,
                    width: 200,
                    render: (_: unknown, r: BillStoreItem) => {
                      const pct = r.ratio ? Number(r.ratio) : 0;
                      return (
                        <div className="hp-split-ratio-cell">
                          <div className="hp-split-ratio-track">
                            <div className="hp-split-ratio-fill" style={{ width: `${Math.min(pct, 100)}%` }} />
                          </div>
                          <span className="hp-split-ratio-text">{r.ratio ? `${r.ratio}%` : '-'}</span>
                        </div>
                      );
                    },
                  },
                  {
                    title: '操作',
                    key: 'actions',
                    width: 110,
                    align: 'right' as const,
                    render: (_: unknown, r: BillStoreItem) => (
                      <Button
                        className="hp-split-order-btn"
                        size="small"
                        type="link"
                        icon={<SearchOutlined />}
                        onClick={() => openOrders(r)}
                      >
                        订单明细
                      </Button>
                    ),
                  },
                ]}
              />
            )}
          </Card>
        </>
      ) : (
        <Card>
          <div className="hp-split-empty">
            <Empty
              image="https://trae-api-cn.mchost.guru/api/ide/v1/text_to_image?prompt=minimal%20flat%20illustration%20of%20two%20shopping%20storefronts%20with%20coins%20and%20split%20arrows%2C%20soft%20blue%20and%20teal%20palette%2C%20light%20background&image_size=square"
              description="输入分账批次号，查询该批次下的门店分成明细"
            />
          </div>
        </Card>
      )}

      {/* 订单明细抽屉 */}
      <Drawer
        title={
          <Space>
            <BranchesOutlined />
            <span>{orders?.store_name ?? '门店'} · 批次订单明细</span>
            {orderStore && <Typography.Text code style={{ fontSize: 12 }}>{orderStore.batchNo}</Typography.Text>}
          </Space>
        }
        width={600}
        open={!!orderStore}
        onClose={closeOrders}
      >
        {ordersQuery.isLoading ? (
          <div style={{ textAlign: 'center', padding: 48 }}>
            <BranchesOutlined spin style={{ fontSize: 24, color: '#1e6fff' }} />
            <div style={{ marginTop: 12, color: '#66718b' }}>加载订单明细…</div>
          </div>
        ) : orders && orders.orders.length === 0 ? (
          <Empty description="该批次下该门店暂无订单" />
        ) : orders ? (
          <Table<BillStoreOrder>
            size="small"
            rowKey="order_no"
            dataSource={orders.orders}
            pagination={false}
            columns={[
              {
                title: '订单号',
                dataIndex: 'order_no',
                key: 'order_no',
                render: (v: string) => <Typography.Text code className="hp-split-order-no">{v}</Typography.Text>,
              },
              {
                title: '金额',
                dataIndex: 'amount',
                key: 'amount',
                align: 'right' as const,
                render: (v: number) => <span className="hp-split-amount">{formatCents(v)}</span>,
              },
              {
                title: '状态',
                dataIndex: 'status',
                key: 'status',
                width: 90,
                render: (v: string) => <OrderStatusLabel status={v} />,
              },
              {
                title: '支付时间',
                dataIndex: 'paid_at',
                key: 'paid_at',
                width: 170,
                render: (v?: string | null) =>
                  v ? (
                    <Text type="secondary" style={{ fontSize: 12 }}>{formatDateTime(v)}</Text>
                  ) : (
                    <Text type="secondary" style={{ fontSize: 12 }}>-</Text>
                  ),
              },
            ]}
          />
        ) : null}
      </Drawer>
    </div>
  );
};