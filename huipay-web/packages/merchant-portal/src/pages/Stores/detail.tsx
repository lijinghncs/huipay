// 门店详情页：基础信息 + 统计 + 关联码牌 + 近期交易
import React from 'react';
import { Button, Card, Col, Descriptions, Row, Space, Table, Tag, Typography } from 'antd';
import { ShopOutlined, QrcodeOutlined, OrderedListOutlined, WalletOutlined, ArrowLeftOutlined } from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';
import { formatDateTime } from '@huipay/shared/utils';
import { KpiCard } from '../../components/KpiCard';
import { getStore, listCodes, listOrders, type StoreDetail } from '../../services/user';

const { Text } = Typography;

const TYPE_LABEL: Record<string, string> = { DIRECT: '直营', FRANCHISE: '加盟', PARTNER: '合作' };

const codeStatusTag = (v: number) =>
  v === 1 ? <Tag color="success" style={{ borderRadius: 10 }}>启用</Tag> : <Tag style={{ borderRadius: 10 }}>停用</Tag>;

export const StoreDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const storeId = Number(id);
  const nav = useNavigate();

  const detailQuery = useQuery({
    queryKey: ['stores', 'detail', storeId],
    queryFn: () => getStore(storeId),
    enabled: !!storeId,
  });

  const codesQuery = useQuery({
    queryKey: ['stores', 'detail', storeId, 'codes'],
    queryFn: () => listCodes({ page: 1, size: 20, store_id: storeId }),
    enabled: !!storeId,
  });

  const ordersQuery = useQuery({
    queryKey: ['stores', 'detail', storeId, 'orders'],
    queryFn: () => listOrders({ page: 1, size: 10, store_id: storeId }),
    enabled: !!storeId,
  });

  const d = detailQuery.data as StoreDetail | undefined;

  const codeColumns = [
    { title: '短码', dataIndex: 'code_id', key: 'code_id', width: 120 },
    { title: '备注', dataIndex: 'remark', key: 'remark', render: (v?: string) => v || '-' },
    { title: '状态', dataIndex: 'status', key: 'status', width: 90, render: codeStatusTag },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 180, render: (v: string) => formatDateTime(v) },
  ];

  const orderColumns = [
    { title: '订单号', dataIndex: 'order_no', key: 'order_no', width: 220 },
    { title: '金额', dataIndex: 'amount', key: 'amount', width: 120, render: (v: number) => `¥${(v / 100).toFixed(2)}` },
    { title: '状态', dataIndex: 'status', key: 'status', width: 100 },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 180, render: (v: string) => formatDateTime(v) },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Space>
        <Button icon={<ArrowLeftOutlined />} onClick={() => nav('/stores')}>返回</Button>
        <Text strong style={{ fontSize: 16 }}>{d?.name ?? '门店详情'}</Text>
        {d && (d.status === 1 ? <Tag color="success" style={{ borderRadius: 10 }}>启用</Tag> : <Tag style={{ borderRadius: 10 }}>停用</Tag>)}
      </Space>

      <Card title="基础信息" loading={detailQuery.isLoading}>
        {d && (
          <Descriptions column={{ xs: 1, md: 2, lg: 3 }} size="small">
            <Descriptions.Item label="门店名称">{d.name}</Descriptions.Item>
            <Descriptions.Item label="门店编码">{d.store_code}</Descriptions.Item>
            <Descriptions.Item label="门店类型">{d.store_type ? TYPE_LABEL[d.store_type] ?? d.store_type : '-'}</Descriptions.Item>
            <Descriptions.Item label="联系电话">{d.contact_phone || '-'}</Descriptions.Item>
            <Descriptions.Item label="所在地区">{d.region || '-'}</Descriptions.Item>
            <Descriptions.Item label="详细地址">{d.address || '-'}</Descriptions.Item>
            <Descriptions.Item label="经度">{d.longitude ?? '-'}</Descriptions.Item>
            <Descriptions.Item label="纬度">{d.latitude ?? '-'}</Descriptions.Item>
            <Descriptions.Item label="创建时间">{formatDateTime(d.created_at)}</Descriptions.Item>
          </Descriptions>
        )}
      </Card>

      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}><KpiCard title="关联码牌" value={d?.code_count ?? 0} icon={<QrcodeOutlined />} color="#1e6fff" loading={detailQuery.isLoading} /></Col>
        <Col xs={12} md={6}><KpiCard title="累计订单" value={d?.order_count ?? 0} icon={<OrderedListOutlined />} color="#06b6a4" loading={detailQuery.isLoading} /></Col>
        <Col xs={12} md={6}><KpiCard title="本月交易额" value={0} icon={<WalletOutlined />} color="#8b5cf6" loading={detailQuery.isLoading} formatter={(v) => `¥${(v / 100).toFixed(2)}`} /></Col>
        <Col xs={12} md={6}><KpiCard title="门店状态" value={d?.status ?? 0} icon={<ShopOutlined />} color="#ec4899" loading={detailQuery.isLoading} formatter={(v) => (v === 1 ? '启用' : '停用')} /></Col>
      </Row>

      <Card title="关联收款码" loading={codesQuery.isLoading}>
        <Table
          className="hp-zebra"
          rowKey="id"
          columns={codeColumns}
          dataSource={codesQuery.data?.items ?? []}
          pagination={false}
        />
      </Card>

      <Card title="近期交易" loading={ordersQuery.isLoading}>
        <Table
          className="hp-zebra"
          rowKey="order_no"
          columns={orderColumns}
          dataSource={ordersQuery.data?.items ?? []}
          pagination={false}
        />
      </Card>
    </div>
  );
};