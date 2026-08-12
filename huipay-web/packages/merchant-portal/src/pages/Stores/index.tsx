// 门店列表页：搜索 / 新建 / 编辑 / 启停 / 删除 / 详情
import React from 'react';
import { App as AntApp, Button, Card, Col, Form, Input, InputNumber, Modal, Popconfirm, Row, Select, Space, Table, Tag, Tooltip, Typography } from 'antd';
import { PlusOutlined, ShopOutlined, EnvironmentOutlined, CheckCircleOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { formatDateTime } from '@huipay/shared/utils';
import { KpiCard } from '../../components/KpiCard';
import { createStore, updateStore, setStoreStatus, deleteStore, listStores, getStoreStats, type Store } from '../../services/user';

const { Text } = Typography;

const STATUS_OPTIONS = [
  { value: 'DIRECT', label: '直营' },
  { value: 'FRANCHISE', label: '加盟' },
  { value: 'PARTNER', label: '合作' },
];

const statusTag = (v: number) =>
  v === 1 ? <Tag color="success" style={{ borderRadius: 10 }}>启用</Tag> : <Tag style={{ borderRadius: 10 }}>停用</Tag>;

export const Stores: React.FC = () => {
  const { message } = AntApp.useApp();
  const queryClient = useQueryClient();
  const nav = useNavigate();
  const [page, setPage] = React.useState(1);
  const [size, setSize] = React.useState(20);
  const [keyword, setKeyword] = React.useState('');
  const [status, setStatus] = React.useState<number | undefined>();
  const [modalOpen, setModalOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<Store | null>(null);
  const [form] = Form.useForm<Partial<Store>>();

  const listQuery = useQuery({
    queryKey: ['stores', page, size, keyword, status],
    queryFn: () => listStores({ page, size, keyword, status }),
  });

  const statsQuery = useQuery({
    queryKey: ['stores', 'stats'],
    queryFn: getStoreStats,
    staleTime: 60_000,
  });

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['stores'] });
    queryClient.invalidateQueries({ queryKey: ['stores', 'stats'] });
  };

  const createMutation = useMutation({
    mutationFn: (data: Partial<Store>) => createStore(data),
    onSuccess: () => {
      message.success('门店创建成功');
      setModalOpen(false);
      form.resetFields();
      invalidate();
    },
    onError: () => message.error('创建失败，请重试'),
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Store> }) => updateStore(id, data),
    onSuccess: () => {
      message.success('门店已更新');
      setModalOpen(false);
      form.resetFields();
      invalidate();
    },
    onError: () => message.error('更新失败，请重试'),
  });

  const statusMutation = useMutation({
    mutationFn: ({ id, status: st }: { id: number; status: number }) => setStoreStatus(id, st),
    onSuccess: (_: Store, vars) => {
      message.success(vars.status === 1 ? '已启用' : '已停用');
      invalidate();
    },
    onError: () => message.error('操作失败，请重试'),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteStore(id),
    onSuccess: () => {
      message.success('门店已删除');
      invalidate();
    },
    onError: (e: Error) => message.error(e.message || '删除失败，请重试'),
  });

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    setModalOpen(true);
  };

  const openEdit = (record: Store) => {
    setEditing(record);
    form.setFieldsValue({
      name: record.name,
      store_type: record.store_type,
      contact_phone: record.contact_phone,
      region: record.region,
      address: record.address,
      longitude: record.longitude ?? undefined,
      latitude: record.latitude ?? undefined,
    });
    setModalOpen(true);
  };

  const handleSubmit = () => {
    const v = form.getFieldsValue();
    if (editing) {
      updateMutation.mutate({ id: editing.id, data: v });
    } else {
      createMutation.mutate(v);
    }
  };

  const columns = [
    { title: '门店名称', dataIndex: 'name', key: 'name', width: 200, ellipsis: true, render: (v: string, r: Store) => <a onClick={() => nav(`/stores/${r.id}`)}>{v}</a> },
    { title: '门店编码', dataIndex: 'store_code', key: 'store_code', width: 220, copyable: true },
    { title: '类型', dataIndex: 'store_type', key: 'store_type', width: 90, render: (v?: string) => (v ? STATUS_OPTIONS.find((o) => o.value === v)?.label ?? v : '-') },
    { title: '联系电话', dataIndex: 'contact_phone', key: 'contact_phone', width: 140, render: (v?: string) => v || '-' },
    { title: '码牌数', dataIndex: 'code_count', key: 'code_count', width: 90, render: (v?: number) => v ?? 0 },
    { title: '订单数', dataIndex: 'order_count', key: 'order_count', width: 90, render: (v?: number) => v ?? 0 },
    { title: '地址', dataIndex: 'address', key: 'address', ellipsis: true, render: (v?: string) => v || '-' },
    { title: '状态', dataIndex: 'status', key: 'status', width: 90, render: statusTag },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 180, render: (v: string) => formatDateTime(v) },
    {
      title: '操作',
      key: 'actions',
      width: 220,
      render: (_: unknown, record: Store) => (
        <Space size="small">
          <Button size="small" type="link" onClick={() => nav(`/stores/${record.id}`)}>详情</Button>
          <Button size="small" type="link" onClick={() => openEdit(record)}>编辑</Button>
          {record.status === 1 ? (
            <Popconfirm title="停用该门店？" onConfirm={() => statusMutation.mutate({ id: record.id, status: 0 })}>
              <Button size="small" type="link" danger>停用</Button>
            </Popconfirm>
          ) : (
            <Button size="small" type="link" onClick={() => statusMutation.mutate({ id: record.id, status: 1 })}>启用</Button>
          )}
          <Popconfirm
            title="确认删除该门店？"
            onConfirm={() => deleteMutation.mutate(record.id)}
            disabled={(record.code_count ?? 0) > 0 || (record.order_count ?? 0) > 0}
          >
            <Tooltip title={(record.code_count ?? 0) > 0 || (record.order_count ?? 0) > 0 ? '该门店有关联码牌/订单，不可删除' : ''}>
              <Button
                size="small"
                type="link"
                danger
                loading={deleteMutation.isPending}
                disabled={(record.code_count ?? 0) > 0 || (record.order_count ?? 0) > 0}
              >
                删除
              </Button>
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}><KpiCard title="门店总数" value={statsQuery.data?.total ?? 0} icon={<ShopOutlined />} color="#1e6fff" loading={statsQuery.isLoading} /></Col>
        <Col xs={12} md={6}><KpiCard title="启用中" value={statsQuery.data?.active ?? 0} icon={<CheckCircleOutlined />} color="#06b6a4" loading={statsQuery.isLoading} /></Col>
        <Col xs={12} md={6}><KpiCard title="本月新增" value={statsQuery.data?.month_new ?? 0} icon={<PlusOutlined />} color="#8b5cf6" loading={statsQuery.isLoading} /></Col>
        <Col xs={12} md={6}><KpiCard title="覆盖地区" value={0} icon={<EnvironmentOutlined />} color="#ec4899" loading={statsQuery.isLoading} /></Col>
      </Row>

      <Card
        title="门店管理"
        extra={<Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新建门店</Button>}
      >
        <Space wrap style={{ marginBottom: 16 }}>
          <Input
            placeholder="按门店名称 / 编码搜索"
            style={{ width: 220 }}
            allowClear
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            onPressEnter={() => setPage(1)}
          />
          <Select
            allowClear
            placeholder="状态"
            style={{ width: 120 }}
            value={status}
            onChange={(v) => { setStatus(v); setPage(1); }}
            options={[
              { value: 1, label: '启用' },
              { value: 0, label: '停用' },
            ]}
          />
          <Button type="primary" onClick={() => setPage(1)}>查询</Button>
        </Space>
        <Table<Store>
          className="hp-zebra"
          rowKey="id"
          columns={columns}
          dataSource={listQuery.data?.items ?? []}
          loading={listQuery.isLoading}
          pagination={{
            current: page,
            pageSize: size,
            total: listQuery.data?.total ?? 0,
            showSizeChanger: true,
            onChange: (p, s) => { setPage(p); setSize(s); },
          }}
        />
      </Card>

      <Modal
        title={editing ? '编辑门店' : '新建门店'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={handleSubmit}
        confirmLoading={createMutation.isPending || updateMutation.isPending}
        okText={editing ? '保存' : '创建'}
        width={560}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="门店名称" rules={[{ required: true, whitespace: true, min: 2, max: 40, message: '请输入 2-40 字符的门店名称' }]}>
            <Input placeholder="如：杭州西湖一号店" maxLength={40} />
          </Form.Item>
          <Form.Item name="store_type" label="门店类型">
            <Select allowClear placeholder="直营 / 加盟 / 合作" options={STATUS_OPTIONS} />
          </Form.Item>
          <Form.Item
            name="contact_phone"
            label="联系电话"
            rules={[
              {
                pattern: /^1[3-9]\d{9}$/,
                message: '请输入正确的 11 位手机号',
              },
            ]}
          >
            <Input placeholder="11 位手机号" maxLength={11} />
          </Form.Item>
          <Form.Item name="region" label="所在地区">
            <Input placeholder="省 / 市 / 区" maxLength={128} />
          </Form.Item>
          <Form.Item name="address" label="详细地址">
            <Input placeholder="门店详细地址" maxLength={256} />
          </Form.Item>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name="longitude"
                label="经度"
                rules={[
                  {
                    type: 'number',
                    min: -180,
                    max: 180,
                    message: '经度需在 -180 ~ 180 之间',
                  },
                  {
                    validator: (_, v) =>
                      v == null ||
                      Number.isInteger(v * 1000000)
                        ? Promise.resolve()
                        : Promise.reject(new Error('经度最多 6 位小数')),
                  },
                ]}
              >
                <InputNumber style={{ width: '100%' }} placeholder="经度" step={0.000001} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name="latitude"
                label="纬度"
                rules={[
                  {
                    type: 'number',
                    min: -90,
                    max: 90,
                    message: '纬度需在 -90 ~ 90 之间',
                  },
                  {
                    validator: (_, v) =>
                      v == null ||
                      Number.isInteger(v * 1000000)
                        ? Promise.resolve()
                        : Promise.reject(new Error('纬度最多 6 位小数')),
                  },
                ]}
              >
                <InputNumber style={{ width: '100%' }} placeholder="纬度" step={0.000001} />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
    </div>
  );
};