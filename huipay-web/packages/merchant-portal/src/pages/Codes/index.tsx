// 收款码页面：创建 / 查看 / 下载二维码 / 复制收款链接 / 停用
import React from 'react';
import { App as AntApp, Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Typography } from 'antd';
import { QRCode } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { formatDateTime } from '@huipay/shared/utils';
import { createCode, disableCode, listCodes, listStores, setCodeStore, type PaymentCode } from '../../services/user';

const { Text } = Typography;

const statusTag = (v: number) =>
  v === 1 ? <Tag color="success" style={{ borderRadius: 10 }}>启用</Tag> : <Tag style={{ borderRadius: 10 }}>停用</Tag>;

export const Codes: React.FC = () => {
  const { message } = AntApp.useApp();
  const queryClient = useQueryClient();
  const nav = useNavigate();
  const [page, setPage] = React.useState(1);
  const [size, setSize] = React.useState(20);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [qrCode, setQrCode] = React.useState<PaymentCode | null>(null);
  const qrRef = React.useRef<{ toDataURL?: () => string } | null>(null);
  const [form] = Form.useForm<{ remark?: string; store_id?: number }>();
  const [storeOpen, setStoreOpen] = React.useState(false);
  const [storeTarget, setStoreTarget] = React.useState<PaymentCode | null>(null);
  const [storeForm] = Form.useForm<{ store_id?: number }>();

  const listQuery = useQuery({
    queryKey: ['codes', page, size],
    queryFn: () => listCodes({ page, size }),
  });

  const storesQuery = useQuery({
    queryKey: ['stores', 'options'],
    queryFn: () => listStores({ page: 1, size: 200, status: 1 }),
  });

  const createMutation = useMutation({
    mutationFn: ({ remark, storeId }: { remark: string; storeId?: number }) => createCode(remark, storeId),
    onSuccess: () => {
      message.success('收款码创建成功');
      setCreateOpen(false);
      form.resetFields();
      queryClient.invalidateQueries({ queryKey: ['codes'] });
    },
    onError: () => message.error('创建失败，请重试'),
  });

  const disableMutation = useMutation({
    mutationFn: (id: number) => disableCode(id),
    onSuccess: () => {
      message.success('已停用');
      queryClient.invalidateQueries({ queryKey: ['codes'] });
    },
    onError: () => message.error('停用失败，请重试'),
  });

  const setStoreMutation = useMutation({
    mutationFn: ({ id, storeId }: { id: number; storeId?: number }) => setCodeStore(id, storeId),
    onSuccess: () => {
      message.success('门店已更新');
      setStoreOpen(false);
      storeForm.resetFields();
      queryClient.invalidateQueries({ queryKey: ['codes'] });
    },
    onError: () => message.error('操作失败，请重试'),
  });

  const openStoreModal = (record: PaymentCode) => {
    setStoreTarget(record);
    storeForm.setFieldsValue({ store_id: record.store_id });
    setStoreOpen(true);
  };

  const copyLink = async (code: PaymentCode) => {
    try {
      await navigator.clipboard.writeText(code.checkout_url);
      message.success('收款链接已复制');
    } catch {
      message.warning('复制失败，请手动复制链接');
    }
  };

  const downloadQR = () => {
    if (!qrCode) return;
    const url = qrRef.current?.toDataURL?.();
    if (!url) {
      message.warning('二维码图片生成失败，请截图保存');
      return;
    }
    const a = document.createElement('a');
    a.download = `收款码-${qrCode.code_id}.png`;
    a.href = url;
    a.click();
  };

  const columns = [
    { title: '短码', dataIndex: 'code_id', key: 'code_id', width: 140 },
    { title: '备注', dataIndex: 'remark', key: 'remark', render: (v?: string) => v || '-' },
    {
      title: '所属门店',
      dataIndex: 'store_name',
      key: 'store_name',
      width: 160,
      render: (v: string | undefined, r: PaymentCode) =>
        v ? (
          <a onClick={() => r.store_id && nav(`/stores/${r.store_id}`)}>{v}</a>
        ) : (
          <Tag color="warning" style={{ borderRadius: 10 }}>未绑定</Tag>
        ),
    },
    { title: '状态', dataIndex: 'status', key: 'status', width: 90, render: statusTag },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 180, render: (v: string) => formatDateTime(v) },
    {
      title: '操作',
      key: 'actions',
      width: 320,
      render: (_: unknown, record: PaymentCode) => (
        <Space size="small">
          <Button size="small" type="link" onClick={() => setQrCode(record)}>
            查看二维码
          </Button>
          <Button size="small" type="link" onClick={() => copyLink(record)}>
            复制链接
          </Button>
          <Button size="small" type="link" onClick={() => openStoreModal(record)}>
            {record.store_id ? '更换门店' : '绑定门店'}
          </Button>
          {record.status === 1 && (
            <Popconfirm title="停用后该码牌将无法收款，确认停用？" onConfirm={() => disableMutation.mutate(record.id)}>
              <Button size="small" type="link" danger loading={disableMutation.isPending}>
                停用
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ];

  return (
    <Card
      title={
        <Space>
          <span>收款码</span>
          <Tag style={{ borderRadius: 10 }} color="blue">{listQuery.data?.total ?? 0}</Tag>
        </Space>
      }
      extra={
        <Button type="primary" onClick={() => setCreateOpen(true)}>
          创建收款码
        </Button>
      }
    >
      <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
        创建收款码后，可下载打印二维码，消费者扫码即可输入金额完成付款。
      </Text>
      <Table<PaymentCode>
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
          onChange: (p, s) => {
            setPage(p);
            setSize(s);
          },
        }}
      />

      <Modal
        title="创建收款码"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => {
          // 直接读取表单值并创建，绕过 Form.onFinish（antd 在该 Modal 场景下 callbacks 未注入，form.submit() 不会触发 onFinish）
          const v = form.getFieldsValue();
          createMutation.mutate({ remark: v.remark?.trim() ?? '', storeId: v.store_id });
        }}
        confirmLoading={createMutation.isPending}
        okText="创建"
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={(v) => createMutation.mutate({ remark: v.remark?.trim() ?? '', storeId: v.store_id })}
        >
          <Form.Item name="store_id" label="所属门店" extra="建议绑定门店，以启用按门店分账">
            <Select
              showSearch
              optionFilterProp="label"
              placeholder="选择门店"
              allowClear
              options={(storesQuery.data?.items ?? []).map((s) => ({ value: s.id, label: s.name }))}
              loading={storesQuery.isLoading}
            />
          </Form.Item>
          <Form.Item name="remark" label="备注" extra="最多 64 字，如门店名/收银位">
            <Input placeholder="如：1 号门店收银台" maxLength={64} allowClear />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`${storeTarget?.store_id ? '更换门店' : '绑定门店'} - ${storeTarget?.code_id ?? ''}`}
        open={storeOpen}
        onCancel={() => setStoreOpen(false)}
        onOk={() => {
          const v = storeForm.getFieldsValue();
          if (!storeTarget) return;
          setStoreMutation.mutate({ id: storeTarget.id, storeId: v.store_id });
        }}
        confirmLoading={setStoreMutation.isPending}
        okText="保存"
      >
        <Form form={storeForm} layout="vertical">
          <Form.Item name="store_id" label="所属门店" extra="清空保存即解绑门店；建议绑定门店以启用按门店分账">
            <Select
              showSearch
              optionFilterProp="label"
              placeholder="选择门店（清空即解绑）"
              allowClear
              options={(storesQuery.data?.items ?? []).map((s) => ({ value: s.id, label: s.name }))}
              loading={storesQuery.isLoading}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`收款码 ${qrCode?.code_id ?? ''}`}
        open={!!qrCode}
        onCancel={() => setQrCode(null)}
        footer={
          <Space>
            <Button onClick={() => qrCode && copyLink(qrCode)}>复制链接</Button>
            <Button type="primary" onClick={downloadQR}>
              下载二维码
            </Button>
          </Space>
        }
      >
        {qrCode && (
          <div style={{ textAlign: 'center', padding: 16 }}>
            <QRCode ref={qrRef as never} value={qrCode.checkout_url} size={240} />
            <div style={{ marginTop: 12 }}>
              <Text type="secondary">消费者扫码后输入金额即可付款</Text>
            </div>
          </div>
        )}
      </Modal>
    </Card>
  );
};
