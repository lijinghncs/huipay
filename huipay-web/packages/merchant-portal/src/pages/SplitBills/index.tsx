// 分账单页：生成分账单（两步：选规则+时间段→预览明细）→ 审批/驳回/执行
import React from 'react';
import {
  App as AntApp,
  Button,
  Card,
  Col,
  DatePicker,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Steps,
  Table,
  Tag,
  Typography,
} from 'antd';
import {
  BranchesOutlined,
  CheckCircleOutlined,
  FileTextOutlined,
  PlusOutlined,
  StopOutlined,
} from '@ant-design/icons';
import dayjs, { type Dayjs } from 'dayjs';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { KpiCard } from '../../components/KpiCard';
import { SplitPreview } from '../../components/SplitPreview';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import {
  approveSplitBill,
  executeSplitByPeriod,
  generateSplitBill,
  listSplitBills,
  listSplitRules,
  previewSplit,
  rejectSplitBill,
  type SplitBill,
  type SplitPreview as SplitPreviewData,
} from '../../services/user';

const { Text } = Typography;

const MONO = { fontVariantNumeric: 'tabular-nums' as const, fontFamily: 'Fira Code, Consolas, Monaco, monospace' };

/** 状态徽章：圆点 + 文案。 */
const billStatusTag = (v: string) => {
  if (v === 'PENDING') return <Tag color="orange" style={{ borderRadius: 10 }}>待审批</Tag>;
  if (v === 'APPROVED') return <Tag color="blue" style={{ borderRadius: 10 }}>已通过</Tag>;
  if (v === 'REJECTED') return <Tag color="default" style={{ borderRadius: 10 }}>已驳回</Tag>;
  if (v === 'EXECUTED') return <Tag color="success" style={{ borderRadius: 10 }}>已执行</Tag>;
  return <Tag style={{ borderRadius: 10 }}>{v}</Tag>;
};

const fmtPeriod = (s: string, e: string) => {
  const sameYear = dayjs(s).isSame(dayjs(e), 'year');
  const fmt = sameYear ? 'MM-DD HH:mm' : 'YYYY-MM-DD HH:mm';
  return `${dayjs(s).format(fmt)} ~ ${dayjs(e).format(fmt)}`;
};

export const SplitBills: React.FC = () => {
  const { message } = AntApp.useApp();
  const queryClient = useQueryClient();
  const nav = useNavigate();
  const [page, setPage] = React.useState(1);
  const [size, setSize] = React.useState(20);
  const [genOpen, setGenOpen] = React.useState(false);
  const [step, setStep] = React.useState<1 | 2>(1);
  const [preview, setPreview] = React.useState<SplitPreviewData | null>(null);
  const [genParams, setGenParams] = React.useState<{ rule_code: string; start: string; end: string } | null>(null);
  const [detailBatchNo, setDetailBatchNo] = React.useState<string | null>(null);
  const [genForm] = Form.useForm<{ rule_code: string; start: Dayjs; end: Dayjs }>();
  const startVal = Form.useWatch('start', genForm);
  const endVal = Form.useWatch('end', genForm);

  const listQuery = useQuery({
    queryKey: ['split-bills', page, size],
    queryFn: () => listSplitBills({ page, size }),
  });

  const rulesQuery = useQuery({ queryKey: ['split-rules'], queryFn: () => listSplitRules() });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['split-bills'] });

  const previewMutation = useMutation({
    mutationFn: (v: { rule_code: string; start: string; end: string }) =>
      previewSplit({ rule_code: v.rule_code, start: v.start, end: v.end }),
    onSuccess: (data) => {
      setPreview(data);
      setStep(2);
    },
    onError: (e: Error) => message.error(e.message || '试算失败，请重试'),
  });

  const generateMutation = useMutation({
    mutationFn: (v: { rule_code: string; start: string; end: string }) =>
      generateSplitBill({ rule_code: v.rule_code, start: v.start, end: v.end }),
    onSuccess: (bill) => {
      message.success('分账单已生成，待审批');
      setGenOpen(false);
      setStep(1);
      setPreview(null);
      setGenParams(null);
      genForm.resetFields();
      invalidate();
      setDetailBatchNo(bill.batch_no);
    },
    onError: (e: Error) => message.error(e.message || '生成分账单失败，请重试'),
  });

  const execPeriodMutation = useMutation({
    mutationFn: (v: { rule_code: string; start: string; end: string }) =>
      executeSplitByPeriod({ rule_code: v.rule_code, start: v.start, end: v.end }),
    onSuccess: (resp) => {
      message.success('分账已执行完成');
      setGenOpen(false);
      setStep(1);
      setPreview(null);
      setGenParams(null);
      genForm.resetFields();
      invalidate();
    },
    onError: (e: Error) => message.error(e.message || '分账执行失败，请重试'),
  });

  const approveMutation = useMutation({
    mutationFn: (batchNo: string) => approveSplitBill(batchNo),
    onSuccess: () => {
      message.success('审批通过，分账已执行');
      invalidate();
      setDetailBatchNo(null);
    },
    onError: (e: Error) => message.error(e.message || '审批执行失败，请重试'),
  });

  const rejectMutation = useMutation({
    mutationFn: (batchNo: string) => rejectSplitBill(batchNo),
    onSuccess: () => {
      message.success('分账单已驳回');
      invalidate();
      setDetailBatchNo(null);
    },
    onError: (e: Error) => message.error(e.message || '驳回失败，请重试'),
  });

  const enabledRules = (rulesQuery.data?.items ?? []).filter((r) => r.status === 1);
  const bills = listQuery.data?.items ?? [];
  const nowMonth = dayjs().format('YYYY-MM');
  const pendingCount = bills.filter((b) => b.status === 'PENDING').length;
  const monthExecuted = bills
    .filter((b) => b.status === 'EXECUTED' && dayjs(b.created_at).format('YYYY-MM') === nowMonth)
    .reduce((s, b) => s + b.total_amount, 0);
  const monthRejected = bills.filter(
    (b) => b.status === 'REJECTED' && dayjs(b.created_at).format('YYYY-MM') === nowMonth,
  ).length;

  const openGenerate = () => {
    setStep(1);
    setPreview(null);
    setGenParams(null);
    genForm.resetFields();
    setGenOpen(true);
  };

  const submitStep1 = () => {
    genForm.validateFields().then((v) => {
      const params = {
        rule_code: v.rule_code,
        start: v.start.toISOString(),
        end: v.end.toISOString(),
      };
      setGenParams(params);
      previewMutation.mutate(params);
    });
  };

  const submitGenerate = () => {
    if (!genParams) return;
    generateMutation.mutate(genParams);
  };

  const submitExecPeriod = () => {
    if (!genParams) return;
    execPeriodMutation.mutate(genParams);
  };

  const detailBill = detailBatchNo ? bills.find((b) => b.batch_no === detailBatchNo) ?? null : null;

  const columns = [
    {
      title: '批次号',
      dataIndex: 'batch_no',
      key: 'batch_no',
      width: 220,
      render: (v: string) => (
        <Typography.Text
          code
          style={{ ...MONO, fontSize: 12, cursor: 'pointer' }}
          onClick={() => {
            navigator.clipboard?.writeText(v).then(() => message.success('批次号已复制')).catch(() => undefined);
          }}
        >
          {v}
        </Typography.Text>
      ),
    },
    {
      title: '规则',
      key: 'rule',
      width: 200,
      render: (_: unknown, r: SplitBill) => (
        <Space direction="vertical" size={0}>
          <Text strong style={{ fontSize: 13 }}>{r.rule_name}</Text>
          <Text type="secondary" style={{ fontSize: 12, ...MONO }}>{r.rule_code}</Text>
        </Space>
      ),
    },
    {
      title: '时间段',
      key: 'period',
      width: 240,
      render: (_: unknown, r: SplitBill) => (
        <span style={{ fontSize: 13 }}>{fmtPeriod(r.start_time, r.end_time)}</span>
      ),
    },
    {
      title: '分账总额',
      dataIndex: 'total_amount',
      key: 'total_amount',
      align: 'right' as const,
      width: 130,
      render: (v: number) => <span style={{ fontWeight: 600, ...MONO }}>{formatCents(v)}</span>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: billStatusTag,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 170,
      render: (v: string) => <span style={{ color: '#5b6b81', fontSize: 13 }}>{formatDateTime(v)}</span>,
    },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      render: (_: unknown, r: SplitBill) => (
        <Button size="small" type="link" onClick={() => setDetailBatchNo(r.batch_no)}>
          详情
        </Button>
      ),
    },
  ];

  return (
    <div className="hp-page" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Row gutter={[16, 16]}>
        <Col xs={12} md={8}>
          <KpiCard title="待审批" value={pendingCount} icon={<FileTextOutlined />} color="#f59e0b" loading={listQuery.isLoading} />
        </Col>
        <Col xs={12} md={8}>
          <KpiCard
            title="本月执行"
            value={monthExecuted}
            icon={<CheckCircleOutlined />}
            color="#10b981"
            loading={listQuery.isLoading}
            formatter={(v) => formatCents(v)}
          />
        </Col>
        <Col xs={12} md={8}>
          <KpiCard title="本月驳回" value={monthRejected} icon={<StopOutlined />} color="#f43f5e" loading={listQuery.isLoading} />
        </Col>
      </Row>

      <Card
        title={
          <Space>
            <span>分账单</span>
            <Tag style={{ borderRadius: 8 }} color="blue">{listQuery.data?.total ?? 0}</Tag>
          </Space>
        }
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={openGenerate}>
            生成账单
          </Button>
        }
      >
        <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
          按时间段汇总商户实收并生成待审批分账单；审批通过后执行分账，或直接「生成并立即执行」。
        </Text>
        <Table<SplitBill>
          className="hp-zebra"
          rowKey="batch_no"
          columns={columns}
          dataSource={bills}
          loading={listQuery.isLoading}
          scroll={{ x: 1080 }}
          locale={{
            emptyText: (
              <Empty description="暂无分账单">
                <Button type="primary" icon={<PlusOutlined />} onClick={openGenerate}>
                  生成第一张账单
                </Button>
              </Empty>
            ),
          }}
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

      <Modal
        title="生成分账单"
        open={genOpen}
        onCancel={() => setGenOpen(false)}
        width={720}
        footer={
          step === 1 ? (
            <Space>
              <Button onClick={() => setGenOpen(false)}>取消</Button>
              <Button type="primary" loading={previewMutation.isPending} onClick={submitStep1}>
                下一步
              </Button>
            </Space>
          ) : (
            <Space>
              <Button onClick={() => setStep(1)}>上一步</Button>
              <Popconfirm
                title="跳过审批、直接扣款执行？"
                description="将按该时间段立即分账，不可撤回。"
                onConfirm={submitExecPeriod}
              >
                <Button loading={execPeriodMutation.isPending}>生成并立即执行</Button>
              </Popconfirm>
              <Button type="primary" loading={generateMutation.isPending} onClick={submitGenerate}>
                生成账单（走审批）
              </Button>
            </Space>
          )
        }
      >
        <Steps
          size="small"
          current={step - 1}
          style={{ marginBottom: 20 }}
          items={[{ title: '选择规则与时间段' }, { title: '预览明细' }]}
        />
        {step === 1 ? (
          <Form form={genForm} layout="vertical">
            <Form.Item
              name="rule_code"
              label="分账规则"
              rules={[{ required: true, message: '请选择分账规则' }]}
              extra="仅列出启用中的规则"
            >
              <Select
                placeholder="选择分账规则"
                loading={rulesQuery.isLoading}
                options={enabledRules.map((r) => ({
                  value: r.rule_code,
                  label: r.rule_name,
                }))}
              />
            </Form.Item>
            <Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>
              以该时间段内商户实收总额为分账基数；跨度不超过 31 天，结束时间不晚于当前。
            </Text>
            <Row gutter={12}>
              <Col span={12}>
                <Form.Item
                  name="start"
                  label="开始日期"
                  rules={[{ required: true, message: '请选择开始日期' }]}
                >
                  <DatePicker
                    showTime
                    style={{ width: '100%' }}
                    placeholder="选择开始日期"
                    format="YYYY-MM-DD HH:mm"
                    disabledDate={(d) => d.isAfter(dayjs()) || (endVal ? d.isAfter(endVal) : false)}
                  />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  name="end"
                  label="结束日期"
                  dependencies={['start']}
                  rules={[
                    { required: true, message: '请选择结束日期' },
                    ({ getFieldValue }) => ({
                      validator(_, value) {
                        if (!value) return Promise.resolve();
                        const start = getFieldValue('start');
                        if (start && value.isBefore(start)) {
                          return Promise.reject(new Error('结束日期不能早于开始日期'));
                        }
                        if (start && value.diff(start, 'day') > 31) {
                          return Promise.reject(new Error('时间段跨度不能超过 31 天'));
                        }
                        return Promise.resolve();
                      },
                    }),
                  ]}
                >
                  <DatePicker
                    showTime
                    style={{ width: '100%' }}
                    placeholder="选择结束日期"
                    format="YYYY-MM-DD HH:mm"
                    disabledDate={(d) => d.isAfter(dayjs()) || (startVal ? d.isBefore(startVal) : false)}
                  />
                </Form.Item>
              </Col>
            </Row>
          </Form>
        ) : (
          <SplitPreview
            items={preview?.items ?? []}
            totalAmount={preview?.total_amount ?? 0}
            merchantRemain={preview?.merchant_remain ?? 0}
            loading={previewMutation.isPending}
          />
        )}
      </Modal>

      <Drawer
        title={
          <Space>
            <BranchesOutlined />
            <span>分账单详情</span>
            {detailBatchNo && <Typography.Text code style={{ fontSize: 12 }}>{detailBatchNo}</Typography.Text>}
          </Space>
        }
        width={560}
        open={!!detailBill}
        onClose={() => setDetailBatchNo(null)}
      >
        {detailBill && (
          <>
            <Descriptions column={1} size="small" bordered style={{ marginBottom: 16 }}>
              <Descriptions.Item label="批次号">
                <Text style={MONO}>{detailBill.batch_no}</Text>
              </Descriptions.Item>
              <Descriptions.Item label="规则">
                {detailBill.rule_name}（{detailBill.rule_code}）
              </Descriptions.Item>
              <Descriptions.Item label="时间段">{fmtPeriod(detailBill.start_time, detailBill.end_time)}</Descriptions.Item>
              <Descriptions.Item label="分账总额">
                <span style={{ fontWeight: 700, color: '#e11d48', ...MONO }}>{formatCents(detailBill.total_amount)}</span>
              </Descriptions.Item>
              <Descriptions.Item label="状态">{billStatusTag(detailBill.status)}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{formatDateTime(detailBill.created_at)}</Descriptions.Item>
              {detailBill.approved_at && (
                <Descriptions.Item label="审批时间">{formatDateTime(detailBill.approved_at)}</Descriptions.Item>
              )}
              {detailBill.executed_at && (
                <Descriptions.Item label="执行时间">{formatDateTime(detailBill.executed_at)}</Descriptions.Item>
              )}
            </Descriptions>

            <div style={{ marginTop: 4, textAlign: 'right' }}>
              <Button
                type="link"
                onClick={() => nav(`/splits?batch_no=${encodeURIComponent(detailBill.batch_no)}`)}
              >
                查看门店分账明细 →
              </Button>
            </div>

            {detailBill.status === 'PENDING' && (
              <div style={{ marginTop: 20, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                <Popconfirm title="确认驳回该分账单？" onConfirm={() => rejectMutation.mutate(detailBill.batch_no)}>
                  <Button danger loading={rejectMutation.isPending}>
                    驳回
                  </Button>
                </Popconfirm>
                <Button
                  type="primary"
                  loading={approveMutation.isPending}
                  onClick={() => approveMutation.mutate(detailBill.batch_no)}
                >
                  审批通过并执行
                </Button>
              </div>
            )}
          </>
        )}
      </Drawer>
    </div>
  );
};