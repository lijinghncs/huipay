// 风控规则管理
import React, { useMemo, useState } from 'react';
import { Card, Table, Tag, Button, Input, Select, Space, Form, Modal, message } from 'antd';
import { PlusOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';

interface RiskRuleRow {
  id: number;
  name: string;
  type: string;
  decision: string;
  priority: number;
  enabled: boolean;
}

const decisionColor: Record<string, string> = { BLOCK: 'red', REVIEW: 'orange', PASS: 'green' };
const decisionLabel: Record<string, string> = { BLOCK: '拦截', REVIEW: '人工审核', PASS: '放行' };

const mockData: RiskRuleRow[] = [
  { id: 1, name: '单笔金额超限拦截', type: '金额', decision: 'BLOCK', priority: 100, enabled: true },
  { id: 2, name: '高频交易风控', type: '频次', decision: 'REVIEW', priority: 80, enabled: true },
  { id: 3, name: '新商户限额', type: '商户', decision: 'REVIEW', priority: 60, enabled: false },
  { id: 4, name: '黑名单校验', type: '名单', decision: 'BLOCK', priority: 120, enabled: true },
];

export const RiskRules: React.FC = () => {
  const [keyword, setKeyword] = useState('');
  const [enabled, setEnabled] = useState<boolean | undefined>();
  const [modalOpen, setModalOpen] = useState(false);
  const [form] = Form.useForm();

  const filtered = useMemo(() => {
    const kw = keyword.trim().toLowerCase();
    return mockData.filter((r) => {
      const matchKw = !kw || r.name.toLowerCase().includes(kw) || r.type.toLowerCase().includes(kw);
      const matchEnabled = enabled === undefined || r.enabled === enabled;
      return matchKw && matchEnabled;
    });
  }, [keyword, enabled]);

  const columns: ColumnsType<RiskRuleRow> = [
    {
      title: '规则名',
      dataIndex: 'name',
      key: 'name',
      render: (v: string) => <span style={{ fontWeight: 600 }}>{v || '-'}</span>,
    },
    { title: '类型', dataIndex: 'type', key: 'type', filters: [{ text: '金额', value: '金额' }, { text: '频次', value: '频次' }, { text: '商户', value: '商户' }, { text: '名单', value: '名单' }], onFilter: (v, r) => r.type === v },
    {
      title: '决策',
      dataIndex: 'decision',
      key: 'decision',
      filters: [
        { text: '拦截', value: 'BLOCK' },
        { text: '人工审核', value: 'REVIEW' },
        { text: '放行', value: 'PASS' },
      ],
      onFilter: (v, r) => r.decision === v,
      render: (v: string) => <Tag color={decisionColor[v] ?? 'default'}>{decisionLabel[v] ?? v}</Tag>,
    },
    { title: '优先级', dataIndex: 'priority', key: 'priority', sorter: (a, b) => a.priority - b.priority },
    {
      title: '启用',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (v: boolean) =>
        v ? <Tag color="green">是</Tag> : <Tag>否</Tag>,
    },
  ];

  return (
    <Card
      title="风控规则"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>
          新建规则
        </Button>
      }
    >
      <Space style={{ marginBottom: 16 }} wrap>
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="搜索规则名 / 类型"
          style={{ width: 220 }}
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
        />
        <Select
          allowClear
          placeholder="启用状态"
          style={{ width: 120 }}
          value={enabled}
          onChange={setEnabled}
          options={[
            { value: true, label: '启用' },
            { value: false, label: '停用' },
          ]}
        />
        <Button icon={<ReloadOutlined />} onClick={() => { setKeyword(''); setEnabled(undefined); }}>
          重置
        </Button>
      </Space>

      <Table<RiskRuleRow>
        className="hp-zebra"
        rowKey="id"
        columns={columns}
        dataSource={filtered}
        scroll={{ x: 700 }}
        pagination={false}
        locale={{ emptyText: '暂无风控规则' }}
      />

      <Modal
        title="新建风控规则"
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => {
          form.validateFields().then(() => {
            message.success('规则创建成功（演示）');
            setModalOpen(false);
            form.resetFields();
          });
        }}
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label="规则名" rules={[{ required: true, message: '请输入规则名' }]}>
            <Input placeholder="请输入规则名" />
          </Form.Item>
          <Form.Item name="type" label="类型" rules={[{ required: true, message: '请选择类型' }]}>
            <Select options={[{ value: '金额', label: '金额' }, { value: '频次', label: '频次' }, { value: '商户', label: '商户' }, { value: '名单', label: '名单' }]} />
          </Form.Item>
          <Form.Item name="decision" label="决策" rules={[{ required: true, message: '请选择决策' }]}>
            <Select options={[{ value: 'BLOCK', label: '拦截' }, { value: 'REVIEW', label: '人工审核' }, { value: 'PASS', label: '放行' }]} />
          </Form.Item>
          <Form.Item name="priority" label="优先级" rules={[{ required: true, message: '请输入优先级' }]}>
            <Input type="number" placeholder="数值越大优先级越高" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};