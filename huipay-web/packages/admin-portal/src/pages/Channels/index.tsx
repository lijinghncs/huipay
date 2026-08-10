// 通道配置
import React, { useMemo, useState } from 'react';
import { Card, Table, Tag, Switch, Space, Input, Button, Popconfirm, Tooltip, message } from 'antd';
import { SearchOutlined, ReloadOutlined, EditOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import type { ChannelCode } from '@huipay/shared';

interface ChannelRow {
  code: ChannelCode;
  name: string;
  fee_rate: string;
  enabled: boolean;
  mch_id: string;
  status: 'NORMAL' | 'MAINTENANCE' | 'OFFLINE';
}

const mockData: ChannelRow[] = [
  { code: 'WECHAT_H5', name: '微信 H5', fee_rate: '0.6%', enabled: true, mch_id: 'wx_10001', status: 'NORMAL' },
  { code: 'ALIPAY_H5', name: '支付宝 H5', fee_rate: '0.6%', enabled: true, mch_id: 'ali_20002', status: 'NORMAL' },
  { code: 'WECHAT_QR', name: '微信扫码', fee_rate: '0.38%', enabled: false, mch_id: 'wx_30003', status: 'MAINTENANCE' },
  { code: 'ALIPAY_QR', name: '支付宝扫码', fee_rate: '0.5%', enabled: true, mch_id: 'ali_40004', status: 'OFFLINE' },
];

export const Channels: React.FC = () => {
  const [keyword, setKeyword] = useState('');
  const [data, setData] = useState(mockData);

  const filtered = useMemo(() => {
    const kw = keyword.trim().toLowerCase();
    return data.filter((c) => !kw || c.name.toLowerCase().includes(kw) || c.code.toLowerCase().includes(kw));
  }, [keyword, data]);

  const toggleEnabled = (code: ChannelCode, enabled: boolean) => {
    setData((prev) => prev.map((c) => (c.code === code ? { ...c, enabled } : c)));
    message.success(enabled ? '已启用通道' : '已停用通道');
  };

  const columns: ColumnsType<ChannelRow> = [
    {
      title: '通道编码',
      dataIndex: 'code',
      key: 'code',
      render: (v: string) => <span style={{ fontFamily: 'monospace', fontWeight: 600 }}>{v || '-'}</span>,
    },
    { title: '名称', dataIndex: 'name', key: 'name', sorter: (a, b) => a.name.localeCompare(b.name) },
    { title: '费率', dataIndex: 'fee_rate', key: 'fee_rate' },
    { title: '商户号', dataIndex: 'mch_id', key: 'mch_id' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      filters: [
        { text: '正常', value: 'NORMAL' },
        { text: '维护中', value: 'MAINTENANCE' },
        { text: '下线', value: 'OFFLINE' },
      ],
      onFilter: (v, r) => r.status === v,
      render: (v: ChannelRow['status']) =>
        v === 'NORMAL' ? (
          <Tag color="green">正常</Tag>
        ) : v === 'MAINTENANCE' ? (
          <Tag color="orange">维护中</Tag>
        ) : (
          <Tag color="red">下线</Tag>
        ),
    },
    {
      title: '启用',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (v: boolean, r) => (
        <Switch checked={v} onChange={(next) => toggleEnabled(r.code, next)} />
      ),
    },
    {
      title: '操作',
      key: 'actions',
      render: () => (
        <Space size={4}>
          <Tooltip title="编辑">
            <Button type="link" size="small" icon={<EditOutlined />}>
              编辑
            </Button>
          </Tooltip>
        </Space>
      ),
    },
  ];

  return (
    <Card title="支付通道配置">
      <Space style={{ marginBottom: 16 }} wrap>
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="搜索通道名称 / 编码"
          style={{ width: 240 }}
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
        />
        <Button icon={<ReloadOutlined />} onClick={() => setKeyword('')}>
          重置
        </Button>
      </Space>

      <Table<ChannelRow>
        className="hp-zebra"
        rowKey="code"
        columns={columns}
        dataSource={filtered}
        scroll={{ x: 800 }}
        pagination={false}
        locale={{ emptyText: '暂无通道数据' }}
      />
    </Card>
  );
};