// 差错中心：异常订单（失败/悬挂/死单/降级）+ 对账差异核销
import React, { useState } from 'react';
import {
  App as AntApp,
  Button,
  DatePicker,
  Drawer,
  Popconfirm,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Timeline,
  Tooltip,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  AlertOutlined,
  AuditOutlined,
  CheckCircleOutlined,
  RedoOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import dayjs, { Dayjs } from 'dayjs';
import {
  diffTypeText,
  exceptionStatusBadge,
  listSplitAudits,
  listSplitDiffs,
  listSplitExceptions,
  reopenSplitExecution,
  resolveSplitDiff,
  retrySplitExecution,
  type SplitAuditItem,
  type SplitDiffItem,
  type SplitExceptionItem,
} from '../../services/user';

const MONO: React.CSSProperties = { fontVariantNumeric: 'tabular-nums', fontFamily: 'Fira Code, Consolas, Monaco, monospace' };
const formatCents = (cents?: number): string => `¥${(Number(cents ?? 0) / 100).toFixed(2)}`;
const fmtTime = (t?: string): string => (t ? dayjs(t).format('MM-DD HH:mm:ss') : '-');

/** 状态徽章。 */
const Badge: React.FC<{ status?: string }> = ({ status }) => {
  const b = exceptionStatusBadge(status);
  return (
    <span
      style={{
        display: 'inline-block',
        padding: '2px 10px',
        borderRadius: 999,
        fontSize: 12,
        lineHeight: '20px',
        color: b.color,
        background: b.bg,
        fontWeight: 600,
      }}
    >
      {b.text}
    </span>
  );
};

/** 审计动作文案。 */
const auditActionText = (a: string): string => {
  const map: Record<string, string> = {
    EXECUTE: '执行分账',
    EXECUTE_FAILED: '执行失败',
    RETRY: '手动重试',
    REOPEN: '死单复位重开',
    RESOLVE: '人工核销',
    APPROVE: '审批通过',
    REJECT: '驳回',
    RECONCILE_PASSED: '对账通过',
    RECONCILE_FAILED: '对账失败',
    RESET: '状态重置',
    MANUAL_OVERRIDE: '人工干预',
  };
  return map[a] || a;
};

/** 异常订单 Tab。 */
const ExceptionsTab: React.FC = () => {
  const { message } = AntApp.useApp();
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<string>();
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(20);
  const [auditOrderNo, setAuditOrderNo] = useState<string>();

  const query = useQuery({
    queryKey: ['split-exceptions', status, page, size],
    queryFn: () => listSplitExceptions({ page, size, status }),
  });

  const audits = useQuery({
    queryKey: ['split-audits', auditOrderNo],
    queryFn: () => listSplitAudits(auditOrderNo!),
    enabled: !!auditOrderNo,
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['split-exceptions'] });

  const retryMut = useMutation({
    mutationFn: retrySplitExecution,
    onSuccess: (r) => {
      message.success(`重试完成：成功 ${r.success} / 失败 ${r.failed}`);
      invalidate();
    },
    onError: (e: Error) => message.error(e.message || '重试失败'),
  });

  const reopenMut = useMutation({
    mutationFn: reopenSplitExecution,
    onSuccess: () => {
      message.success('已复位重开，补偿调度将在 30 秒内自动重试');
      invalidate();
    },
    onError: (e: Error) => message.error(e.message || '复位失败'),
  });

  const columns: ColumnsType<SplitExceptionItem> = [
    {
      title: '订单号 / 批次号',
      dataIndex: 'order_no',
      width: 210,
      render: (v: string) => <span style={{ ...MONO, fontSize: 12 }}>{v}</span>,
    },
    {
      title: '分账金额',
      dataIndex: 'total_amount',
      width: 110,
      align: 'right',
      render: (v: number) => <span style={{ ...MONO, fontWeight: 600 }}>{formatCents(v)}</span>,
    },
    {
      title: '接收方',
      width: 90,
      align: 'center',
      render: (_, r) => (
        <span style={MONO}>
          {r.success_count}/{r.receiver_count}
        </span>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (v: string, r) => (
        <Space size={4}>
          <Badge status={v} />
          {r.degraded === 1 && (
            <Tooltip title="降级订单：本地已入账，通道未分">
              <Tag color="orange" style={{ marginInlineEnd: 0 }}>降级</Tag>
            </Tooltip>
          )}
        </Space>
      ),
    },
    {
      title: '重试',
      dataIndex: 'attempt_count',
      width: 70,
      align: 'center',
      render: (v: number) => <span style={MONO}>{v}/5</span>,
    },
    {
      title: '下次重试',
      dataIndex: 'next_retry_at',
      width: 130,
      render: (v?: string) => <span style={{ ...MONO, fontSize: 12 }}>{fmtTime(v)}</span>,
    },
    {
      title: '最近错误',
      dataIndex: 'last_error',
      ellipsis: { showTitle: false },
      render: (v: string) =>
        v ? (
          <Tooltip title={v} placement="topLeft">
            <Typography.Text type="danger" style={{ fontSize: 12 }} ellipsis>
              {v}
            </Typography.Text>
          </Tooltip>
        ) : (
          '-'
        ),
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      width: 130,
      render: (v: string) => <span style={{ ...MONO, fontSize: 12 }}>{fmtTime(v)}</span>,
    },
    {
      title: '操作',
      width: 200,
      render: (_, r) => (
        <Space size={8}>
          {(r.status === 'FAILED' || r.status === 'PARTIAL' || r.status === 'SUSPENDED') && (
            <Button
              size="small"
              icon={<RedoOutlined />}
              loading={retryMut.isPending && retryMut.variables === r.order_no}
              onClick={() => retryMut.mutate(r.order_no)}
            >
              重试
            </Button>
          )}
          {r.status === 'DEAD' && (
            <Popconfirm
              title="复位重开"
              description="将清零重试计数并重新进入自动补偿，确认？"
              onConfirm={() => reopenMut.mutate(r.order_no)}
            >
              <Button size="small" type="primary" danger icon={<ReloadOutlined />}>
                复位重开
              </Button>
            </Popconfirm>
          )}
          <Button size="small" type="text" icon={<AuditOutlined />} onClick={() => setAuditOrderNo(r.order_no)}>
            审计
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <>
      <Space style={{ marginBottom: 12 }}>
        <Select
          allowClear
          placeholder="状态筛选"
          style={{ width: 160 }}
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          options={[
            { value: 'FAILED', label: '分账失败' },
            { value: 'PARTIAL', label: '部分分账' },
            { value: 'SUSPENDED', label: '悬挂中' },
            { value: 'DEAD', label: '死单' },
            { value: 'RESOLVED', label: '已核销' },
          ]}
        />
        <Button icon={<ReloadOutlined />} onClick={invalidate}>
          刷新
        </Button>
      </Space>
      <Table<SplitExceptionItem>
        className="hp-tx-table"
        rowKey="order_no"
        size="middle"
        loading={query.isLoading}
        columns={columns}
        dataSource={query.data?.items ?? []}
        pagination={{
          current: page,
          pageSize: size,
          total: query.data?.total ?? 0,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (p, s) => {
            setPage(p);
            setSize(s);
          },
        }}
      />
      <Drawer
        title={
          <span>
            审计时间线 <span style={{ ...MONO, fontSize: 12, color: '#8a94a6' }}>{auditOrderNo}</span>
          </span>
        }
        open={!!auditOrderNo}
        onClose={() => setAuditOrderNo(undefined)}
        width={480}
      >
        {audits.isLoading ? (
          <Typography.Text type="secondary">加载中...</Typography.Text>
        ) : (audits.data?.items?.length ?? 0) === 0 ? (
          <Typography.Text type="secondary">暂无审计记录</Typography.Text>
        ) : (
          <Timeline
            items={(audits.data!.items as SplitAuditItem[]).map((a) => ({
              key: a.id,
              color: a.action.includes('FAIL') ? 'red' : a.action === 'RESOLVE' || a.action === 'APPROVE' ? 'green' : 'blue',
              children: (
                <div>
                  <div style={{ fontWeight: 600 }}>
                    {auditActionText(a.action)}
                    <span style={{ marginLeft: 8, fontSize: 12, color: '#8a94a6', fontWeight: 400 }}>
                      {a.operator_type === 'MERCHANT' ? '商户' : a.operator_type === 'ADMIN' ? '管理员' : '系统'}
                    </span>
                  </div>
                  <div style={{ ...MONO, fontSize: 12, color: '#8a94a6', marginTop: 2 }}>
                    {dayjs(a.created_at).format('YYYY-MM-DD HH:mm:ss')}
                  </div>
                  {a.detail && (
                    <div style={{ ...MONO, fontSize: 12, color: '#66718b', marginTop: 4, wordBreak: 'break-all' }}>
                      {a.detail}
                    </div>
                  )}
                </div>
              ),
            }))}
          />
        )}
      </Drawer>
    </>
  );
};

/** 对账差异 Tab。 */
const DiffsTab: React.FC = () => {
  const { message } = AntApp.useApp();
  const queryClient = useQueryClient();
  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(7, 'day'), dayjs()]);
  const [diffType, setDiffType] = useState<string>();
  const [resolved, setResolved] = useState<boolean>(false);
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(20);

  const query = useQuery({
    queryKey: ['split-diffs', range[0].format('YYYY-MM-DD'), range[1].format('YYYY-MM-DD'), diffType, resolved, page, size],
    queryFn: () =>
      listSplitDiffs({
        page,
        size,
        start_date: range[0].format('YYYY-MM-DD'),
        end_date: range[1].format('YYYY-MM-DD'),
        diff_type: diffType,
        resolved,
      }),
  });

  const resolveMut = useMutation({
    mutationFn: resolveSplitDiff,
    onSuccess: () => {
      message.success('已核销');
      queryClient.invalidateQueries({ queryKey: ['split-diffs'] });
    },
    onError: (e: Error) => message.error(e.message || '核销失败'),
  });

  const columns: ColumnsType<SplitDiffItem> = [
    {
      title: '业务日',
      dataIndex: 'biz_date',
      width: 110,
      render: (v: string) => <span style={{ ...MONO, fontSize: 12 }}>{dayjs(v).format('YYYY-MM-DD')}</span>,
    },
    {
      title: '差异类型',
      dataIndex: 'diff_type',
      width: 160,
      render: (v: string) => (
        <Tag color={v === 'SPLIT_POST' ? 'red' : v === 'SPLIT_DEGRADED' ? 'orange' : 'blue'}>{diffTypeText(v)}</Tag>
      ),
    },
    {
      title: '订单号',
      dataIndex: 'order_no',
      width: 200,
      render: (v?: string) => (v ? <span style={{ ...MONO, fontSize: 12 }}>{v}</span> : '-'),
    },
    {
      title: '账本金额',
      dataIndex: 'local_amount',
      width: 110,
      align: 'right',
      render: (v?: number) => <span style={MONO}>{formatCents(v)}</span>,
    },
    {
      title: '执行金额',
      dataIndex: 'channel_amount',
      width: 110,
      align: 'right',
      render: (v?: number) => <span style={MONO}>{formatCents(v)}</span>,
    },
    {
      title: '详情',
      dataIndex: 'detail',
      ellipsis: { showTitle: false },
      render: (v?: string) =>
        v ? (
          <Tooltip title={v} placement="topLeft">
            <span style={{ ...MONO, fontSize: 12, color: '#66718b' }}>{v}</span>
          </Tooltip>
        ) : (
          '-'
        ),
    },
    {
      title: '状态',
      width: 130,
      render: (_, r) =>
        r.resolved_at ? (
          <Tooltip title={`核销于 ${dayjs(r.resolved_at).format('YYYY-MM-DD HH:mm:ss')}`}>
            <Tag icon={<CheckCircleOutlined />} color="success">
              已核销
            </Tag>
          </Tooltip>
        ) : (
          <Tag color="warning">未核销</Tag>
        ),
    },
    {
      title: '操作',
      width: 100,
      render: (_, r) =>
        !r.resolved_at && (
          <Popconfirm title="确认核销该差异？" description="核销表示已人工核对处理完毕。" onConfirm={() => resolveMut.mutate(r.id)}>
            <Button size="small" type="primary" ghost>
              核销
            </Button>
          </Popconfirm>
        ),
    },
  ];

  return (
    <>
      <Space style={{ marginBottom: 12 }} wrap>
        <DatePicker.RangePicker
          value={range}
          allowClear={false}
          onChange={(v) => {
            if (v?.[0] && v?.[1]) {
              setRange([v[0], v[1]]);
              setPage(1);
            }
          }}
        />
        <Select
          allowClear
          placeholder="差异类型"
          style={{ width: 180 }}
          value={diffType}
          onChange={(v) => {
            setDiffType(v);
            setPage(1);
          }}
          options={[
            { value: 'SPLIT_POST', label: '执行后·账本不平' },
            { value: 'SPLIT_DEGRADED', label: '降级订单' },
            { value: 'SPLIT_TOTAL', label: '前置对账·总额不平' },
            { value: 'SPLIT_DETAIL', label: '前置对账·门店日不平' },
          ]}
        />
        <Select
          style={{ width: 120 }}
          value={resolved}
          onChange={(v) => {
            setResolved(v);
            setPage(1);
          }}
          options={[
            { value: false, label: '未核销' },
            { value: true, label: '已核销' },
          ]}
        />
      </Space>
      <Table<SplitDiffItem>
        className="hp-tx-table"
        rowKey="id"
        size="middle"
        loading={query.isLoading}
        columns={columns}
        dataSource={query.data?.items ?? []}
        pagination={{
          current: page,
          pageSize: size,
          total: query.data?.total ?? 0,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (p, s) => {
            setPage(p);
            setSize(s);
          },
        }}
      />
    </>
  );
};

export const SplitExceptions: React.FC = () => (
  <div>
    <div className="hp-split-head">
      <div>
        <div className="hp-split-head-title">
          <AlertOutlined style={{ marginRight: 8, color: '#dc2626' }} />
          差错中心
        </div>
        <div className="hp-split-head-sub">分账异常订单处理（重试 / 死单复位 / 审计追溯）与对账差异核销</div>
      </div>
    </div>
    <Tabs
      defaultActiveKey="exceptions"
      items={[
        { key: 'exceptions', label: '异常订单', children: <ExceptionsTab /> },
        { key: 'diffs', label: '对账差异', children: <DiffsTab /> },
      ]}
    />
  </div>
);
