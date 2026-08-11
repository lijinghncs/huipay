// 收款码页面：创建 / 查看 / 下载二维码 / 复制收款链接 / 停用
import React from 'react';
import { App as AntApp, Button, Card, Form, Input, Modal, Popconfirm, Space, Table, Tag, Typography } from 'antd';
import { QRCode } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { formatDateTime } from '@huipay/shared/utils';
import { createCode, disableCode, listCodes, type PaymentCode } from '../../services/user';

const { Text } = Typography;

const statusTag = (v: number) => (v === 1 ? <Tag color="success">启用</Tag> : <Tag>停用</Tag>);

export const Codes: React.FC = () => {
  const { message } = AntApp.useApp();
  const queryClient = useQueryClient();
  const [page, setPage] = React.useState(1);
  const [size, setSize] = React.useState(20);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [qrCode, setQrCode] = React.useState<PaymentCode | null>(null);
  const qrRef = React.useRef<{ toDataURL?: () => string } | null>(null);
  const [form] = Form.useForm<{ remark?: string }>();

  const listQuery = useQuery({
    queryKey: ['codes', page, size],
    queryFn: () => listCodes({ page, size }),
  });

  const createMutation = useMutation({
    mutationFn: (remark: string) => createCode(remark),
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
    { title: '状态', dataIndex: 'status', key: 'status', width: 90, render: statusTag },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 180, render: (v: string) => formatDateTime(v) },
    {
      title: '操作',
      key: 'actions',
      width: 260,
      render: (_: unknown, record: PaymentCode) => (
        <Space size="small">
          <Button size="small" type="link" onClick={() => setQrCode(record)}>
            查看二维码
          </Button>
          <Button size="small" type="link" onClick={() => copyLink(record)}>
            复制链接
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
      title="收款码"
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
        onOk={() => form.submit()}
        confirmLoading={createMutation.isPending}
        okText="创建"
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={(v) => createMutation.mutate(v.remark?.trim() ?? '')}
        >
          <Form.Item name="remark" label="备注" extra="最多 64 字，如门店名/收银位">
            <Input placeholder="如：1 号门店收银台" maxLength={64} allowClear />
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
