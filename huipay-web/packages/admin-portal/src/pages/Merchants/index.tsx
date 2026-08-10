// 商户管理
import React, { useMemo, useState } from 'react';
import {
  Card,
  Table,
  Tag,
  Button,
  Space,
  Tooltip,
  Input,
  Select,
  Form,
  Modal,
  message,
} from 'antd';
import { EyeOutlined, SafetyOutlined, PlusOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';

interface MerchantRow {
  id: number;
  code: string;
  name: string;
  type: string;
  kyc_status: number;
  status: number;
  created_at: string;
}

const kycLabels: Record<number, { color: string; label: string }> = {
  0: { color: 'default', label: '未提交' },
  1: { color: 'orange', label: '审核中' },
  2: { color: 'green', label: '已通过' },
  3: { color: 'red', label: '已拒绝' },
};

const mockData: MerchantRow[] = [
  { id: 1, code: 'M10001', name: '示例科技有限公司', type: '企业', kyc_status: 2, status: 1, created_at: '2026-01-12' },
  { id: 2, code: 'M10002', name: '示例个体工商户', type: '个体户', kyc_status: 1, status: 1, created_at: '2026-02-03' },
  { id: 3, code: 'M10003', name: '示例贸易有限公司', type: '企业', kyc_status: 0, status: 0, created_at: '2026-03-08' },
  { id: 4, code: 'M10004', name: '示例日用百货店', type: '个体户', kyc_status: 3, status: 1, created_at: '2026-04-21' },
];

export const Merchants: React.FC = () => {
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<number | undefined>();
  const [modalOpen, setModalOpen] = useState(false);
  const [form] = Form.useForm();

  const filtered = useMemo(() => {
    return mockData.filter((m) => {
      const kw = keyword.trim().toLowerCase();
      const matchKw = !kw || m.name.toLowerCase().includes(kw) || m.code.toLowerCase().includes(kw);
      const matchStatus = status === undefined || m.status === status;
      return matchKw && matchStatus;
    });
  }, [keyword, status]);

  const columns: ColumnsType<MerchantRow> = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
    {
      title: '主体号',
      dataIndex: 'code',
      key: 'code',
      width: 200,
      render: (v: string) => <span style={{ fontFamily: 'monospace', fontWeight: 600 }}>{v || '-'}</span>,
    },
    { title: '名称', dataIndex: 'name', key: 'name', sorter: (a, b) => a.name.localeCompare(b.name) },
    { title: '类型', dataIndex: 'type', key: 'type', filters: [{ text: '企业', value: '企业' }, { text: '个体户', value: '个体户' }], onFilter: (v, r) => r.type === v },
    {
      title: 'KYC',
      dataIndex: 'kyc_status',
      key: 'kyc_status',
      render: (v: number) => <Tag color={kycLabels[v]?.color ?? 'default'}>{kycLabels[v]?.label ?? '-'}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      filters: [{ text: '启用', value: 1 }, { text: '停用', value: 0 }],
      onFilter: (v, r) => r.status === v,
      render: (v: number) => (v === 1 ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>),
    },
    {
      title: '操作',
      key: 'actions',
      fixed: 'right',
      width: 150,
      render: () => (
        <Space size={4}>
          <Tooltip title="详情">
            <Button type="link" size="small" icon={<EyeOutlined />}>
              详情
            </Button>
          </Tooltip>
          <Tooltip title="KYC 审核">
            <Button type="link" size="small" icon={<SafetyOutlined />}>
              KYC 审核
            </Button>
          </Tooltip>
        </Space>
      ),
    },
  ];

  return (
    <Card
      title="商户管理"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>
          新建商户
        </Button>
      }
    >
      <Space style={{ marginBottom: 16 }} wrap>
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="搜索商户名称 / 主体号"
          style={{ width: 240 }}
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
        />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 120 }}
          value={status}
          onChange={setStatus}
          options={[
            { value: 1, label: '启用' },
            { value: 0, label: '停用' },
          ]}
        />
        <Button icon={<ReloadOutlined />} onClick={() => { setKeyword(''); setStatus(undefined); }}>
          重置
        </Button>
      </Space>

      <Table<MerchantRow>
        className="hp-zebra"
        rowKey="id"
        columns={columns}
        dataSource={filtered}
        scroll={{ x: 900 }}
        pagination={{ pageSize: 20, showTotal: (t) => `共 ${t} 条` }}
        locale={{ emptyText: '暂无商户数据' }}
      />

      <Modal
        title="新建商户"
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => {
          form.validateFields().then(() => {
            message.success('商户创建成功（演示）');
            setModalOpen(false);
            form.resetFields();
          });
        }}
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="code" label="主体号" rules={[{ required: true, message: '请输入主体号' }]}>
            <Input placeholder="如 M10005" />
          </Form.Item>
          <Form.Item name="name" label="商户名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="请输入商户名称" />
          </Form.Item>
          <Form.Item name="type" label="商户类型" rules={[{ required: true, message: '请选择类型' }]}>
            <Select options={[{ value: '企业', label: '企业' }, { value: '个体户', label: '个体户' }]} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};