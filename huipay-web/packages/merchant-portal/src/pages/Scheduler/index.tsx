// 定时任务监测：任务列表 + 运行日志（只读，当前商户可见）
import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Card, Table, Tabs, Tag, Button, Space, Select, Drawer, Descriptions, Empty, Typography, Modal, DatePicker, message } from 'antd';
import { ReloadOutlined, CheckCircleOutlined, CloseCircleOutlined, SyncOutlined, FieldTimeOutlined, ThunderboltOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs, { Dayjs } from 'dayjs';
import { listSchedulerTasks, listSchedulerRuns, getSchedulerRun, triggerSchedulerTask, type SchedulerTask, type SchedulerRun } from '../../services/user';

const { Text } = Typography;

function StatusTag({ status }: { status?: string }) {
  if (!status) return <Tag>—</Tag>;
  const map: Record<string, { color: string; icon: React.ReactNode }> = {
    RUNNING: { color: 'processing', icon: <SyncOutlined spin /> },
    SUCCESS: { color: 'success', icon: <CheckCircleOutlined /> },
    FAILED: { color: 'error', icon: <CloseCircleOutlined /> },
    TIMEOUT: { color: 'warning', icon: <FieldTimeOutlined /> },
  };
  const m = map[status] ?? { color: 'default', icon: null };
  return (
    <Tag color={m.color} icon={m.icon}>
      {status}
    </Tag>
  );
}

function fmtMs(ms?: number): string {
  if (ms === undefined || ms === null) return '-';
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms}ms`;
}

export const Scheduler: React.FC = () => {
  const [activeTab, setActiveTab] = useState('tasks');
  const [runName, setRunName] = useState<string | undefined>();
  const [runStatus, setRunStatus] = useState<string | undefined>();
  const [runPage, setRunPage] = useState(1);
  const [detailId, setDetailId] = useState<number | null>(null);
  const [triggerTask, setTriggerTask] = useState<SchedulerTask | null>(null);
  const [triggerBizDate, setTriggerBizDate] = useState<Dayjs>();
  const [triggering, setTriggering] = useState(false);

  const tasks = useQuery({ queryKey: ['merchant', 'scheduler', 'tasks'], queryFn: listSchedulerTasks });

  const runs = useQuery({
    queryKey: ['merchant', 'scheduler', 'runs', runName, runStatus, runPage],
    queryFn: () => listSchedulerRuns({ name: runName, status: runStatus, page: runPage, page_size: 20 }),
  });

  const detail = useQuery({
    queryKey: ['merchant', 'scheduler', 'runs', detailId],
    queryFn: () => getSchedulerRun(detailId!),
    enabled: detailId !== null,
  });

  const taskCols: ColumnsType<SchedulerTask> = [
    { title: '任务名', dataIndex: 'name', render: (v: string) => <Text code>{v}</Text> },
    { title: '名称', dataIndex: 'display_name' },
    { title: '说明', dataIndex: 'description', ellipsis: true },
    {
      title: '触发',
      key: 'cron',
      width: 130,
      render: (_, r) => (r.cron_expr ? <Tag>{r.cron_expr}</Tag> : r.interval_sec ? <Tag>{r.interval_sec}s 轮询</Tag> : '-'),
    },
    { title: '最近状态', dataIndex: 'last_status', width: 110, render: (v?: string) => <StatusTag status={v} /> },
    { title: '最近运行', dataIndex: 'last_run_at', width: 170, render: (v?: string) => (v ? dayjs(v).format('MM-DD HH:mm:ss') : '-') },
    { title: '耗时', dataIndex: 'last_duration_ms', width: 90, render: (v?: number) => fmtMs(v) },
    { title: '行数', dataIndex: 'last_rows', width: 80, render: (v?: number) => v ?? '-' },
    {
      title: '操作',
      key: 'action',
      width: 110,
      render: (_, r) =>
        r.manual_supported ? (
          <Button size="small" type="link" icon={<ThunderboltOutlined />} onClick={() => { setTriggerTask(r); setTriggerBizDate(undefined); }}>
            手动执行
          </Button>
        ) : (
          <Text type="secondary">-</Text>
        ),
    },
  ];

  // name → 中文任务名称映射
  const nameToDisplay = new Map((tasks.data ?? []).map((t) => [t.name, t.display_name]));

  const doTrigger = async () => {
    if (!triggerTask) return;
    setTriggering(true);
    try {
      await triggerSchedulerTask(triggerTask.name, triggerBizDate ? triggerBizDate.format('YYYY-MM-DD') : undefined);
      message.success('已触发，任务将在后台执行');
      setTriggerTask(null);
      tasks.refetch();
      runs.refetch();
    } catch {
      message.error('触发失败，请稍后重试');
    } finally {
      setTriggering(false);
    }
  };

  const runCols: ColumnsType<SchedulerRun> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    {
      title: '任务名称',
      dataIndex: 'name',
      width: 170,
      render: (v: string) => (
        <Space direction="vertical" size={0}>
          <Text>{nameToDisplay.get(v) ?? v}</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>
            <Text code style={{ fontSize: 12 }}>{v}</Text>
          </Text>
        </Space>
      ),
    },
    { title: '方式', dataIndex: 'run_mode', width: 90, render: (v: string) => <Tag color={v === 'MANUAL' ? 'gold' : 'blue'}>{v}</Tag> },
    { title: '业务日期', dataIndex: 'biz_date', width: 110, render: (v?: string) => v || '-' },
    { title: '状态', dataIndex: 'status', width: 100, render: (v: string) => <StatusTag status={v} /> },
    { title: '耗时', dataIndex: 'duration_ms', width: 90, render: (v?: number) => fmtMs(v) },
    { title: '行数', dataIndex: 'rows_affected', width: 80, render: (v?: number) => v ?? '-' },
    { title: '开始时间', dataIndex: 'started_at', width: 170, render: (v: string) => dayjs(v).format('MM-DD HH:mm:ss') },
    {
      title: '操作',
      key: 'action',
      width: 90,
      render: (_, r) => (
        <Button size="small" type="link" onClick={() => setDetailId(r.id)}>
          详情
        </Button>
      ),
    },
  ];

  return (
    <Card>
      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={[
          {
            key: 'tasks',
            label: '任务列表',
            children: (
              <Table className="hp-zebra hp-tx-table" rowKey="name" columns={taskCols} dataSource={tasks.data ?? []} loading={tasks.isLoading} pagination={false} locale={{ emptyText: '暂无已注册调度任务' }} />
            ),
          },
          {
            key: 'runs',
            label: '运行日志',
            children: (
              <>
                <Space wrap style={{ marginBottom: 16 }}>
                  <Select
                    allowClear
                    placeholder="全部任务"
                    style={{ width: 200 }}
                    value={runName}
                    onChange={setRunName}
                    options={(tasks.data ?? []).map((t) => ({ value: t.name, label: t.display_name }))}
                  />
                  <Select
                    allowClear
                    placeholder="全部状态"
                    style={{ width: 140 }}
                    value={runStatus}
                    onChange={(v) => {
                      setRunStatus(v);
                      setRunPage(1);
                    }}
                    options={['RUNNING', 'SUCCESS', 'FAILED', 'TIMEOUT'].map((s) => ({ value: s, label: s }))}
                  />
                  <Button icon={<ReloadOutlined />} onClick={() => { runs.refetch(); tasks.refetch(); }} loading={runs.isFetching}>
                    刷新
                  </Button>
                </Space>
                <Table
                  className="hp-zebra hp-tx-table"
                  rowKey="id"
                  columns={runCols}
                  dataSource={runs.data?.items ?? []}
                  loading={runs.isLoading}
                  pagination={{ current: runPage, pageSize: 20, total: runs.data?.total ?? 0, onChange: setRunPage, showSizeChanger: false }}
                  locale={{ emptyText: '暂无运行日志' }}
                />
              </>
            ),
          },
        ]}
      />

      <Drawer title="运行日志详情" width={520} open={detailId !== null} onClose={() => setDetailId(null)}>
        {detail.data ? (
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label="ID">{detail.data.id}</Descriptions.Item>
            <Descriptions.Item label="任务名">{detail.data.name}</Descriptions.Item>
            <Descriptions.Item label="运行方式">{detail.data.run_mode}</Descriptions.Item>
            <Descriptions.Item label="业务日期">{detail.data.biz_date || '-'}</Descriptions.Item>
            <Descriptions.Item label="状态"><StatusTag status={detail.data.status} /></Descriptions.Item>
            <Descriptions.Item label="开始时间">{dayjs(detail.data.started_at).format('YYYY-MM-DD HH:mm:ss')}</Descriptions.Item>
            <Descriptions.Item label="结束时间">{detail.data.finished_at ? dayjs(detail.data.finished_at).format('YYYY-MM-DD HH:mm:ss') : '-'}</Descriptions.Item>
            <Descriptions.Item label="耗时">{fmtMs(detail.data.duration_ms)}</Descriptions.Item>
            <Descriptions.Item label="影响行数">{detail.data.rows_affected ?? '-'}</Descriptions.Item>
            <Descriptions.Item label="实例ID"><Text code>{detail.data.instance_id}</Text></Descriptions.Item>
            <Descriptions.Item label="错误信息">
              <Text type="danger" style={{ whiteSpace: 'pre-wrap' }}>{detail.data.error_message || '无'}</Text>
            </Descriptions.Item>
          </Descriptions>
        ) : (
          <Empty />
        )}
      </Drawer>

      <Modal
        title="手动执行任务"
        open={triggerTask !== null}
        onCancel={() => setTriggerTask(null)}
        onOk={doTrigger}
        confirmLoading={triggering}
        okText="执行"
        cancelText="取消"
      >
        {triggerTask && (
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            <div>
              <Text strong>{triggerTask.display_name}</Text>
              <Text type="secondary">（<Text code>{triggerTask.name}</Text>）</Text>
            </div>
            <Text type="secondary">可指定业务日期；不填则使用任务默认日期。</Text>
            <DatePicker
              style={{ width: 200 }}
              placeholder="选择业务日期（可选）"
              value={triggerBizDate}
              onChange={setTriggerBizDate}
              disabledDate={(d) => d.isAfter(dayjs(), 'day')}
              format="YYYY-MM-DD"
            />
          </Space>
        )}
      </Modal>
    </Card>
  );
};