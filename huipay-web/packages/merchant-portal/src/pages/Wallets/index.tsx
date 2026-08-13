// 钱包页面（真实对接后台：余额 + 账本流水，支持类型/单号/时间过滤）
import React from 'react';
import { App as AntApp, Button, Card, DatePicker, Descriptions, Form, Input, Modal, Select, Space, Table, Tag } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { Dayjs } from 'dayjs';
import { Link } from 'react-router-dom';
import { Money } from '@huipay/ui-kit';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import type { JournalEntry } from '@huipay/shared';
import {
  approveSplitBill,
  generateSplitBill,
  getCurrentUser,
  getWallet,
  listEntries,
  listSplitRules,
  rejectSplitBill,
  type SplitBill,
} from '../../services/user';

const { RangePicker } = DatePicker;

const billStatusTag = (v: string) => {
  if (v === 'PENDING') return <Tag color="orange" style={{ borderRadius: 10 }}>待审批</Tag>;
  if (v === 'APPROVED') return <Tag color="blue" style={{ borderRadius: 10 }}>已通过</Tag>;
  if (v === 'REJECTED') return <Tag color="default" style={{ borderRadius: 10 }}>已驳回</Tag>;
  if (v === 'EXECUTED') return <Tag color="success" style={{ borderRadius: 10 }}>已执行</Tag>;
  return <Tag style={{ borderRadius: 10 }}>{v}</Tag>;
};

const entryColumns = [
  { title: '流水号', dataIndex: 'id', key: 'id', width: 220 },
  { title: '方向', dataIndex: 'direction', key: 'direction', render: (v: string) => (v === 'CREDIT' ? '入账' : '出账') },
  { title: '金额', dataIndex: 'amount', key: 'amount', render: (v: number) => formatCents(v) },
  { title: '余额', dataIndex: 'balance_after', key: 'balance_after', render: (v: number) => formatCents(v) },
  { title: '业务类型', dataIndex: 'biz_type', key: 'biz_type' },
  {
    title: '业务单号',
    dataIndex: 'biz_id',
    key: 'biz_id',
    width: 180,
    render: (v: string, r: JournalEntry) =>
      r.biz_type === 'PAYMENT' ? <Link to={`/transactions?order_no=${encodeURIComponent(v)}`}>{v}</Link> : v,
  },
  { title: '时间', dataIndex: 'created_at', key: 'created_at', render: (v: string) => formatDateTime(v) },
];

export const Wallets: React.FC = () => {
  const { message } = AntApp.useApp();
  const queryClient = useQueryClient();
  const [entityId, setEntityId] = React.useState<number | null>(null);
  const [bizType, setBizType] = React.useState<string | undefined>(undefined);
  const [bizId, setBizId] = React.useState<string>('');
  const [range, setRange] = React.useState<[string, string] | null>(null);
  const [page, setPage] = React.useState(1);
  const [size, setSize] = React.useState(20);
  const [splitOpen, setSplitOpen] = React.useState(false);
  const [generatedBill, setGeneratedBill] = React.useState<SplitBill | null>(null);
  const [splitForm] = Form.useForm<{ period: unknown; ruleCode: string }>();

  // 默认取当前登录商户的 entity_id，可手动覆盖
  React.useEffect(() => {
    getCurrentUser()
      .then((u) => setEntityId(u.merchantId))
      .catch(() => undefined);
  }, []);

  const walletQuery = useQuery({
    queryKey: ['wallet', entityId],
    queryFn: () => getWallet(entityId!),
    enabled: !!entityId,
  });

  const rulesQuery = useQuery({ queryKey: ['split-rules'], queryFn: () => listSplitRules() });

  const entriesQuery = useQuery({
    queryKey: ['wallet-entries', entityId, page, size, bizType, bizId, range],
    queryFn: () =>
      listEntries(entityId!, {
        page,
        size,
        biz_type: bizType,
        biz_id: bizId || undefined,
        start: range?.[0],
        end: range?.[1],
      }),
    enabled: !!entityId,
  });

  const splitMutation = useMutation({
    mutationFn: (v: { start: string; end: string; ruleCode: string }) =>
      generateSplitBill({ start: v.start, end: v.end, rule_code: v.ruleCode }),
    onSuccess: (bill) => {
      message.success('分账单已生成');
      setGeneratedBill(bill);
      queryClient.invalidateQueries({ queryKey: ['wallet', entityId] });
      queryClient.invalidateQueries({ queryKey: ['wallet-entries', entityId] });
    },
    onError: (e: Error) => message.error(e.message || '生成分账单失败，请重试'),
  });

  const approveMutation = useMutation({
    mutationFn: (batchNo: string) => approveSplitBill(batchNo),
    onSuccess: (bill) => {
      message.success('审批通过，分账已执行');
      setGeneratedBill(bill);
      queryClient.invalidateQueries({ queryKey: ['wallet', entityId] });
      queryClient.invalidateQueries({ queryKey: ['wallet-entries', entityId] });
    },
    onError: (e: Error) => message.error(e.message || '审批执行失败，请重试'),
  });

  const rejectMutation = useMutation({
    mutationFn: (batchNo: string) => rejectSplitBill(batchNo),
    onSuccess: (bill) => {
      message.success('分账单已驳回');
      setGeneratedBill(bill);
    },
    onError: (e: Error) => message.error(e.message || '驳回失败，请重试'),
  });

  const openSplit = () => {
    splitForm.resetFields();
    setGeneratedBill(null);
    setSplitOpen(true);
  };

  const submitSplit = async () => {
    const v = await splitForm.validateFields();
    const period = v.period as unknown as [Dayjs, Dayjs] | null;
    if (!period || !period[0] || !period[1]) {
      message.warning('请选择起止日期');
      return;
    }
    splitMutation.mutate({
      start: period[0].startOf('day').toISOString(),
      end: period[1].endOf('day').toISOString(),
      ruleCode: v.ruleCode,
    });
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card
        title="钱包余额"
        loading={walletQuery.isLoading}
        extra={
          <Button type="primary" onClick={openSplit}>
            分账
          </Button>
        }
      >
        <Money cents={walletQuery.data?.balance ?? 0} />
      </Card>
      <Card title="账本流水">
        <Space wrap style={{ marginBottom: 16 }}>
          <Select
            allowClear
            placeholder="业务类型"
            style={{ width: 160 }}
            value={bizType}
            onChange={(v) => {
              setBizType(v);
              setPage(1);
            }}
            options={[
              { value: 'PAYMENT', label: '支付' },
              { value: 'SPLIT', label: '分账' },
              { value: 'REFUND', label: '退款' },
              { value: 'WITHDRAW', label: '提现' },
            ]}
          />
          <Input
            placeholder="按业务单号搜索"
            style={{ width: 200 }}
            allowClear
            value={bizId}
            onChange={(e) => setBizId(e.target.value)}
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
        </Space>
        <Table<JournalEntry>
          className="hp-zebra"
          rowKey="id"
          columns={entryColumns}
          dataSource={entriesQuery.data?.items ?? []}
          loading={entriesQuery.isLoading}
          pagination={{
            current: page,
            pageSize: size,
            total: entriesQuery.data?.total ?? 0,
            showSizeChanger: true,
            onChange: (p, s) => {
              setPage(p);
              setSize(s);
            },
          }}
        />
      </Card>

      <Modal
        title={generatedBill ? '分账单' : '按时间段分账'}
        open={splitOpen}
        onCancel={() => setSplitOpen(false)}
        footer={
          generatedBill ? (
            generatedBill.status === 'PENDING' ? (
              <Space>
                <Button loading={rejectMutation.isPending} onClick={() => rejectMutation.mutate(generatedBill.batch_no)}>
                  驳回
                </Button>
                <Button
                  type="primary"
                  loading={approveMutation.isPending}
                  onClick={() => approveMutation.mutate(generatedBill.batch_no)}
                >
                  审批通过并执行
                </Button>
              </Space>
            ) : (
              <Button onClick={() => setSplitOpen(false)}>关闭</Button>
            )
          ) : (
            <Space>
              <Button onClick={() => setSplitOpen(false)}>取消</Button>
              <Button type="primary" loading={splitMutation.isPending} onClick={submitSplit}>
                生成账单
              </Button>
            </Space>
          )
        }
        width={640}
      >
        {generatedBill ? (
          <>
            <Descriptions column={1} size="small" bordered style={{ marginBottom: 16 }}>
              <Descriptions.Item label="批次号">
                <span style={{ fontFamily: 'ui-monospace, Consolas, monospace' }}>{generatedBill.batch_no}</span>
              </Descriptions.Item>
              <Descriptions.Item label="分账规则">{generatedBill.rule_name}</Descriptions.Item>
              <Descriptions.Item label="时间段">
                {formatDateTime(generatedBill.start_time)} ~ {formatDateTime(generatedBill.end_time)}
              </Descriptions.Item>
              <Descriptions.Item label="分账总额">
                <span style={{ fontWeight: 700, color: '#e11d48' }}>{formatCents(generatedBill.total_amount)}</span>
              </Descriptions.Item>
              <Descriptions.Item label="状态">{billStatusTag(generatedBill.status)}</Descriptions.Item>
            </Descriptions>
            <Table
              size="small"
              rowKey="receiver_entity_id"
              pagination={false}
              dataSource={generatedBill.items}
              columns={[
                {
                  title: '接收方',
                  dataIndex: 'receiver_name',
                  key: 'receiver_name',
                  render: (v: string) => <span style={{ fontWeight: 600 }}>{v}</span>,
                },
                {
                  title: '可分金额',
                  dataIndex: 'amount',
                  key: 'amount',
                  align: 'right' as const,
                  render: (v: number) => <span style={{ fontWeight: 600 }}>{formatCents(v)}</span>,
                },
              ]}
            />
          </>
        ) : (
          <Form form={splitForm} layout="vertical">
            <Form.Item
              name="period"
              label="分账时间段"
              rules={[{ required: true, message: '请选择起止日期' }]}
              extra="以该时间段内商户实收总额为分账基数"
            >
              <RangePicker style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="ruleCode" label="分账规则" rules={[{ required: true, message: '请选择分账规则' }]}>
              <Select
                placeholder="选择分账规则"
                loading={rulesQuery.isLoading}
                options={(rulesQuery.data?.items ?? []).map((r) => ({ value: r.rule_code, label: r.rule_name }))}
              />
            </Form.Item>
          </Form>
        )}
      </Modal>
    </div>
  );
};
