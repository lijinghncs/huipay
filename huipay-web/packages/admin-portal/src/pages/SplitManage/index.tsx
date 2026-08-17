// 分账管理（admin）：每日执行 + 审计 + 对账差异 + 差错监控 四 Tab
import React, { useState } from 'react';
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query';
import {
  Tabs, Table, Select, DatePicker, Button, Space, Tag, Tooltip, Drawer, Typography, Empty, Card, Popconfirm, message as antdMessage,
} from 'antd';
import {
  ReloadOutlined, SearchOutlined, EyeOutlined, RedoOutlined, CheckOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs, { Dayjs } from 'dayjs';
import {
  listDailyExecutions, listAudits, listReconcileDiffs,
  listSplitExceptions, resolveSplitExecution, reopenSplitExecution, resolveReconcileDiff,
  type DailyExecution, type AuditRecord, type ReconcileDiff, type SplitExceptionItem,
} from '../../services/admin';

const { RangePicker } = DatePicker;
const { Text } = Typography;

// 状态徽章映射
function statusBadge(status: string): { text: string; color: string; bg: string } {
  switch (status) {
    case 'SUCCESS':
      return { text: '成功', color: '#047857', bg: 'rgba(16,185,129,0.14)' };
    case 'PARTIAL':
      return { text: '部分成功', color: '#b45309', bg: 'rgba(245,158,11,0.14)' };
    case 'FAILED':
      return { text: '失败', color: '#b91c1c', bg: 'rgba(239,68,68,0.14)' };
    case 'RUNNING':
      return { text: '运行中', color: '#1d4ed8', bg: 'rgba(59,130,246,0.14)' };
    case 'SPLIT_TOTAL':
      return { text: '分账-总额', color: '#b91c1c', bg: 'rgba(239,68,68,0.14)' };
    case 'SPLIT_DETAIL':
      return { text: '分账-明细', color: '#b45309', bg: 'rgba(245,158,11,0.14)' };
    case 'LONG':
      return { text: '长款', color: '#1d4ed8', bg: 'rgba(59,130,246,0.14)' };
    case 'SHORT':
      return { text: '短款', color: '#b45309', bg: 'rgba(245,158,11,0.14)' };
    case 'MISMATCH':
      return { text: '金额不一致', color: '#b91c1c', bg: 'rgba(239,68,68,0.14)' };
    default:
      return { text: status, color: '#64748b', bg: 'rgba(100,116,139,0.1)' };
  }
}

export const SplitManage: React.FC = () => {
  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(6, 'day'), dayjs()]);
  const startDate = range[0]?.format('YYYY-MM-DD') ?? '';
  const endDate = range[1]?.format('YYYY-MM-DD') ?? '';

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Tabs
        defaultActiveKey="daily"
        items={[
          { key: 'daily', label: '每日执行', children: <DailyExecTab startDate={startDate} endDate={endDate} /> },
          { key: 'audit', label: '审计日志', children: <AuditTab /> },
          { key: 'diffs', label: '对账差异', children: <DiffsTab startDate={startDate} endDate={endDate} /> },
          { key: 'exceptions', label: '差错监控', children: <ExceptionsTab /> },
        ]}
      />
      <Card size="small" style={{ background: '#f8fafc' }}>
        <Space size="middle">
          <Text type="secondary">时间范围：</Text>
          <RangePicker
            value={range}
            onChange={(r) => r && setRange([r[0] as Dayjs, r[1] as Dayjs])}
            disabledDate={(d) => d.isAfter(dayjs(), 'day')}
          />
          <Text type="secondary" style={{ fontSize: 12 }}>
            所有 Tab 共用此时间范围（仅审计 Tab 不强制）
          </Text>
        </Space>
      </Card>
    </div>
  );
};

// ---- Tab 1: 每日执行 ----
const DailyExecTab: React.FC<{ startDate: string; endDate: string }> = ({ startDate, endDate }) => {
  const [statusFilter, setStatusFilter] = useState<string | undefined>();
  const [detail, setDetail] = useState<DailyExecution | null>(null);
  const q = useQuery({
    queryKey: ['admin', 'split-daily', startDate, endDate, statusFilter],
    queryFn: () => listDailyExecutions({
      start_date: startDate, end_date: endDate, status: statusFilter, page_size: 50,
    }),
    enabled: !!startDate && !!endDate,
  });

  const columns: ColumnsType<DailyExecution> = [
    { title: '执行 ID', dataIndex: 'id', width: 80 },
    { title: '业务日期', dataIndex: 'biz_date', width: 120, render: (v: string) => dayjs(v).format('YYYY-MM-DD') },
    { title: '商户', dataIndex: 'merchant_id', width: 100 },
    { title: '批次号', dataIndex: 'batch_no', render: (v: string) => <Text code style={{ fontSize: 12 }}>{v}</Text> },
    {
      title: '状态', dataIndex: 'status', width: 110,
      render: (v: string) => {
        const b = statusBadge(v);
        return <span style={{ color: b.color, background: b.bg, padding: '2px 10px', borderRadius: 10, fontSize: 12, fontWeight: 500 }}>{b.text}</span>;
      },
    },
    { title: '耗时(ms)', dataIndex: 'duration_ms', width: 100, render: (v?: number) => v ?? '-' },
    { title: '开始时间', dataIndex: 'started_at', width: 160, render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss') },
    { title: '结束时间', dataIndex: 'finished_at', width: 160, render: (v?: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm:ss') : '-') },
    {
      title: '操作', key: 'op', width: 80, fixed: 'right',
      render: (_, row) => (
        <Button size="small" icon={<EyeOutlined />} onClick={() => setDetail(row)}>详情</Button>
      ),
    },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 12 }}>
        <Select
          allowClear placeholder="全部状态" style={{ width: 160 }}
          value={statusFilter} onChange={setStatusFilter}
          options={[
            { value: 'RUNNING', label: '运行中' },
            { value: 'SUCCESS', label: '成功' },
            { value: 'PARTIAL', label: '部分成功' },
            { value: 'FAILED', label: '失败' },
          ]}
        />
        <Button icon={<ReloadOutlined />} onClick={() => q.refetch()} loading={q.isFetching}>刷新</Button>
      </Space>
      <Table<DailyExecution>
        columns={columns}
        dataSource={q.data?.items ?? []}
        rowKey="id"
        loading={q.isFetching}
        pagination={{ pageSize: 20, showSizeChanger: false }}
        scroll={{ x: 1200 }}
        locale={{ emptyText: <Empty description="该区间暂无执行记录" /> }}
      />
      <Drawer
        title="每日执行详情"
        open={!!detail}
        width={560}
        onClose={() => setDetail(null)}
      >
        {detail && (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Item label="执行 ID" value={detail.id} />
            <Item label="run_id" value={<Text code style={{ fontSize: 12 }}>{detail.run_id}</Text>} />
            <Item label="商户 ID" value={detail.merchant_id} />
            <Item label="业务日期" value={dayjs(detail.biz_date).format('YYYY-MM-DD')} />
            <Item label="批次号" value={<Text code>{detail.batch_no}</Text>} />
            <Item label="状态" value={<Tag color={detail.status === 'SUCCESS' ? 'success' : detail.status === 'FAILED' ? 'error' : 'warning'}>{statusBadge(detail.status).text}</Tag>} />
            <Item label="耗时" value={detail.duration_ms ? `${detail.duration_ms} ms` : '-'} />
            <Item label="开始时间" value={dayjs(detail.started_at).format('YYYY-MM-DD HH:mm:ss')} />
            <Item label="结束时间" value={detail.finished_at ? dayjs(detail.finished_at).format('YYYY-MM-DD HH:mm:ss') : '-'} />
            <Item label="操作者" value={`${detail.operator_type}#${detail.operator_id}`} />
            {detail.error_code && <Item label="错误码" value={<Tag color="error">{detail.error_code}</Tag>} />}
            {detail.error_message && <Item label="错误信息" value={<Text type="danger" style={{ whiteSpace: 'pre-wrap' }}>{detail.error_message}</Text>} />}
            {detail.reconcile_diff_id && <Item label="关联对账差异" value={`#${detail.reconcile_diff_id}`} />}
          </Space>
        )}
      </Drawer>
    </div>
  );
};

const Item: React.FC<{ label: string; value: React.ReactNode }> = ({ label, value }) => (
  <div>
    <div style={{ color: '#64748b', fontSize: 12, marginBottom: 4 }}>{label}</div>
    <div style={{ fontSize: 14 }}>{value}</div>
  </div>
);

// ---- Tab 2: 审计 ----
const AuditTab: React.FC = () => {
  const [bizType, setBizType] = useState<string | undefined>();
  const [action, setAction] = useState<string | undefined>();
  const [detail, setDetail] = useState<AuditRecord | null>(null);
  const q = useQuery({
    queryKey: ['admin', 'split-audit', bizType, action],
    queryFn: () => listAudits({ biz_type: bizType, action, page_size: 50 }),
  });

  const columns: ColumnsType<AuditRecord> = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: '业务类型', dataIndex: 'biz_type', width: 130, render: (v: string) => <Tag>{v}</Tag> },
    { title: 'biz_id', dataIndex: 'biz_id', width: 200, render: (v: string) => <Text code style={{ fontSize: 12 }}>{v}</Text> },
    {
      title: '动作', dataIndex: 'action', width: 160,
      render: (v: string) => <Tag color={v.includes('FAIL') ? 'error' : v.includes('PASSED') || v === 'EXECUTE' ? 'success' : 'default'}>{v}</Tag>,
    },
    { title: '操作者', dataIndex: 'operator_type', width: 100, render: (v: string, r) => `${v}#${r.operator_id}` },
    { title: '时间', dataIndex: 'created_at', width: 160, render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss') },
    {
      title: '操作', key: 'op', width: 80,
      render: (_, row) => <Button size="small" icon={<EyeOutlined />} onClick={() => setDetail(row)}>详情</Button>,
    },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 12 }}>
        <Select allowClear placeholder="全部业务类型" style={{ width: 160 }} value={bizType} onChange={setBizType}
          options={[{ value: 'DAILY_SPLIT', label: 'DAILY_SPLIT' }, { value: 'SPLIT_BILL', label: 'SPLIT_BILL' }, { value: 'SPLIT_EXEC', label: 'SPLIT_EXEC' }]} />
        <Select allowClear placeholder="全部动作" style={{ width: 200 }} value={action} onChange={setAction}
          options={[
            { value: 'EXECUTE', label: 'EXECUTE' },
            { value: 'EXECUTE_FAILED', label: 'EXECUTE_FAILED' },
            { value: 'RECONCILE_PASSED', label: 'RECONCILE_PASSED' },
            { value: 'RECONCILE_FAILED', label: 'RECONCILE_FAILED' },
            { value: 'RESET', label: 'RESET' },
            { value: 'RECOMPUTE', label: 'RECOMPUTE' },
            { value: 'MANUAL_OVERRIDE', label: 'MANUAL_OVERRIDE' },
            { value: 'APPROVE', label: 'APPROVE' },
            { value: 'REJECT', label: 'REJECT' },
          ]} />
        <Button icon={<ReloadOutlined />} onClick={() => q.refetch()} loading={q.isFetching}>刷新</Button>
      </Space>
      <Table<AuditRecord>
        columns={columns}
        dataSource={q.data?.items ?? []}
        rowKey="id"
        loading={q.isFetching}
        pagination={{ pageSize: 20 }}
        locale={{ emptyText: <Empty description="暂无审计记录" /> }}
      />
      <Drawer title="审计详情" open={!!detail} width={560} onClose={() => setDetail(null)}>
        {detail && (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Item label="ID" value={detail.id} />
            <Item label="业务类型" value={<Tag>{detail.biz_type}</Tag>} />
            <Item label="biz_id" value={<Text code>{detail.biz_id}</Text>} />
            <Item label="动作" value={<Tag color="blue">{detail.action}</Tag>} />
            <Item label="操作者" value={`${detail.operator_type}#${detail.operator_id}`} />
            <Item label="时间" value={dayjs(detail.created_at).format('YYYY-MM-DD HH:mm:ss')} />
            {detail.detail && <Item label="详情 JSON" value={<pre style={{ background: '#f1f5f9', padding: 12, borderRadius: 6, fontSize: 12, overflow: 'auto' }}>{JSON.stringify(safeParseJSON(detail.detail), null, 2)}</pre>} />}
          </Space>
        )}
      </Drawer>
    </div>
  );
};

function safeParseJSON(s: string): unknown {
  try { return JSON.parse(s); } catch { return s; }
}

// ---- Tab 3: 对账差异 ----
const DiffsTab: React.FC<{ startDate: string; endDate: string }> = ({ startDate, endDate }) => {
  const [diffType, setDiffType] = useState<string | undefined>();
  const qc = useQueryClient();
  const { message } = antdMessage;
  const q = useQuery({
    queryKey: ['admin', 'split-diffs', startDate, endDate, diffType],
    queryFn: () => listReconcileDiffs({
      start_date: startDate, end_date: endDate, diff_type: diffType, page_size: 50,
    }),
    enabled: !!startDate && !!endDate,
  });

  const resolveMut = useMutation({
    mutationFn: resolveReconcileDiff,
    onSuccess: () => {
      message.success('已核销差异');
      qc.invalidateQueries({ queryKey: ['admin', 'split-diffs'] });
    },
    onError: (e: Error) => message.error(e.message || '核销失败'),
  });

  const columns: ColumnsType<ReconcileDiff> = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: '业务日期', dataIndex: 'biz_date', width: 120, render: (v: string) => dayjs(v).format('YYYY-MM-DD') },
    {
      title: '差异类型', dataIndex: 'diff_type', width: 120,
      render: (v: string) => {
        const b = statusBadge(v);
        return <span style={{ color: b.color, background: b.bg, padding: '2px 10px', borderRadius: 10, fontSize: 12 }}>{b.text}</span>;
      },
    },
    { title: '商户', dataIndex: 'merchant_id', width: 100 },
    { title: '订单号', dataIndex: 'order_no', render: (v?: string) => (v ? <Text code style={{ fontSize: 12 }}>{v}</Text> : '-') },
    {
      title: '本地金额(分)', dataIndex: 'local_amount', width: 120,
      render: (v?: number) => v != null ? `¥${(v / 100).toFixed(2)}` : '-',
    },
    {
      title: '通道金额(分)', dataIndex: 'channel_amount', width: 120,
      render: (v?: number) => v != null ? `¥${(v / 100).toFixed(2)}` : '-',
    },
    {
      title: '已解决', dataIndex: 'resolved_at', width: 160,
      render: (v?: string) => v ? <Tag color="success">{dayjs(v).format('YYYY-MM-DD HH:mm')}</Tag> : <Tag>未解决</Tag>,
    },
    { title: '创建时间', dataIndex: 'created_at', width: 160, render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss') },
    {
      title: '操作', key: 'op', width: 100, fixed: 'right',
      render: (_, r) => (
        <Popconfirm
          title="核销该对账差异？"
          description="确认该差异已线下处理完毕"
          disabled={!!r.resolved_at}
          onConfirm={() => resolveMut.mutate(r.id)}
        >
          <Button size="small" icon={<CheckOutlined />} disabled={!!r.resolved_at}>核销</Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 12 }}>
        <Select allowClear placeholder="全部类型" style={{ width: 180 }} value={diffType} onChange={setDiffType}
          options={[
            { value: 'SPLIT_TOTAL', label: '分账-总额' },
            { value: 'SPLIT_DETAIL', label: '分账-明细' },
            { value: 'SPLIT_POST', label: '分账-执行后' },
            { value: 'SPLIT_DEGRADED', label: '分账-降级' },
            { value: 'LONG', label: '长款' },
            { value: 'SHORT', label: '短款' },
            { value: 'MISMATCH', label: '金额不一致' },
          ]} />
        <Button icon={<ReloadOutlined />} onClick={() => q.refetch()} loading={q.isFetching}>刷新</Button>
      </Space>
      <Table<ReconcileDiff>
        columns={columns}
        dataSource={q.data?.items ?? []}
        rowKey="id"
        loading={q.isFetching}
        pagination={{ pageSize: 20 }}
        scroll={{ x: 1400 }}
        locale={{ emptyText: <Empty description="该区间暂无差异" /> }}
      />
    </div>
  );
};

// ---- Tab 4: 差错监控（跨商户异常订单）----
const ExceptionsTab: React.FC = () => {
  const [status, setStatus] = useState<string>();
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(20);
  const qc = useQueryClient();
  const { message } = antdMessage;

  const q = useQuery({
    queryKey: ['admin', 'split-exceptions', status, page, size],
    queryFn: () => listSplitExceptions({ page, size, status }),
  });
  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin', 'split-exceptions'] });

  const resolveMut = useMutation({
    mutationFn: resolveSplitExecution,
    onSuccess: () => { message.success('已人工核销'); invalidate(); },
    onError: (e: Error) => message.error(e.message || '核销失败'),
  });
  const reopenMut = useMutation({
    mutationFn: reopenSplitExecution,
    onSuccess: () => { message.success('已复位重开，补偿调度将自动重试'); invalidate(); },
    onError: (e: Error) => message.error(e.message || '复位失败'),
  });

  const columns: ColumnsType<SplitExceptionItem> = [
    { title: '订单号', dataIndex: 'order_no', width: 200, render: (v: string) => <Text code style={{ fontSize: 12 }}>{v}</Text> },
    { title: '商户', dataIndex: 'merchant_id', width: 90 },
    { title: '状态', dataIndex: 'status', width: 100, render: (v: string) => <Tag color={v === 'SUCCESS' ? 'success' : v === 'FAILED' || v === 'DEAD' ? 'error' : v === 'PARTIAL' ? 'warning' : 'default'}>{v}</Tag> },
    {
      title: '金额', dataIndex: 'total_amount', width: 110, align: 'right',
      render: (v: number) => `¥${(v / 100).toFixed(2)}`,
    },
    { title: '接收方', width: 90, align: 'center', render: (_, r) => <Text code>{r.success_count}/{r.receiver_count}</Text> },
    { title: '重试', dataIndex: 'attempt_count', width: 70, align: 'center', render: (v: number) => <Text code>{v}/5</Text> },
    {
      title: '最近错误', dataIndex: 'last_error', ellipsis: { showTitle: false },
      render: (v: string) => v ? <Tooltip title={v}><Typography.Text type="danger" style={{ fontSize: 12 }} ellipsis>{v}</Typography.Text></Tooltip> : '-',
    },
    { title: '更新时间', dataIndex: 'updated_at', width: 150, render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss') },
    {
      title: '操作', key: 'op', width: 180, fixed: 'right',
      render: (_, r) => (
        <Space size={8}>
          {r.status === 'DEAD' && (
            <Popconfirm title="复位重开" description="清零重试计数并重新进入补偿调度，确认？" onConfirm={() => reopenMut.mutate(r.order_no)}>
              <Button size="small" icon={<RedoOutlined />}>复位重开</Button>
            </Popconfirm>
          )}
          {(r.status === 'DEAD' || r.status === 'FAILED' || r.status === 'PARTIAL' || r.status === 'SUSPENDED') && (
            <Popconfirm title="人工核销" description="标记该订单差错已线下闭环，确认？" onConfirm={() => resolveMut.mutate(r.order_no)}>
              <Button size="small" type="primary" danger icon={<CheckOutlined />}>核销</Button>
            </Popconfirm>
          )}
          {r.status === 'RESOLVED' && <Tag color="green">已闭环</Tag>}
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 12 }}>
        <Select allowClear placeholder="状态筛选" style={{ width: 160 }} value={status}
          onChange={(v) => { setStatus(v); setPage(1); }}
          options={[
            { value: 'FAILED', label: '分账失败' },
            { value: 'PARTIAL', label: '部分分账' },
            { value: 'SUSPENDED', label: '悬挂中' },
            { value: 'DEAD', label: '死单' },
            { value: 'RESOLVED', label: '已核销' },
          ]} />
        <Button icon={<ReloadOutlined />} onClick={invalidate}>刷新</Button>
      </Space>
      <Table<SplitExceptionItem>
        columns={columns}
        dataSource={q.data?.items ?? []}
        rowKey="order_no"
        loading={q.isFetching}
        scroll={{ x: 1200 }}
        pagination={{
          current: page, pageSize: size, total: q.data?.total ?? 0,
          showSizeChanger: true, showTotal: (t) => `共 ${t} 条`,
          onChange: (p, s) => { setPage(p); setSize(s); },
        }}
        locale={{ emptyText: <Empty description="暂无异常订单" /> }}
      />
    </div>
  );
};

export default SplitManage;