// 分账规则配置页：概览统计 / 规则列表 / 创建编辑（生效范围 + 分配方案可视化）
import React from 'react';
import {
  App as AntApp,
  Badge,
  Button,
  Card,
  Col,
  Divider,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Progress,
  Radio,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import {
  BranchesOutlined,
  CheckCircleOutlined,
  CalculatorOutlined,
  DeleteOutlined,
  EnvironmentOutlined,
  MinusCircleOutlined,
  PlusOutlined,
  ShopOutlined,
  StopOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { KpiCard } from '../../components/KpiCard';
import { SplitPreview } from '../../components/SplitPreview';
import { StorePicker } from '../../components/StorePicker';
import {
  createSplitRule,
  deleteSplitRule,
  listSplitRules,
  listStores,
  previewSplit,
  setSplitRuleStatus,
  updateSplitRule,
  type SplitPreview as SplitPreviewData,
  type SplitRule,
  type SplitRuleAllocation,
  type Store,
} from '../../services/user';

const { Text } = Typography;
const ALL_STORES = 'ALL_STORES';

const MONO = { fontVariantNumeric: 'tabular-nums' as const, fontFamily: 'Fira Code, Consolas, Monaco, monospace' };

interface AllocationRow {
  mode?: 'ratio' | 'fixed';
  value?: number;
}

interface RuleForm {
  rule_name: string;
  priority?: number;
  channel?: string;
  scope?: 'ALL' | 'SPECIFIED';
  store_ids?: number[];
  allocations: AllocationRow[];
}

/** 状态徽章：圆点 + 文案，不只靠颜色传达。 */
const StatusBadge: React.FC<{ status: number }> = ({ status }) =>
  status === 1 ? (
    <Badge status="success" text="启用" />
  ) : (
    <Badge status="default" text="停用" />
  );

/** 分配方案摘要：接收方 + 比例/金额，金额等宽对齐。 */
const AllocationSummary: React.FC<{ allocations: SplitRuleAllocation[] }> = ({ allocations }) => {
  if (!allocations?.length) return <Text type="secondary">-</Text>;
  return (
    <Space direction="vertical" size={4} wrap>
      {allocations.map((a, i) => {
        const name = a.receiver_scope === ALL_STORES ? '全部门店' : `#${a.receiver_entity_id}`;
        const isRatio = (a.ratio_bps ?? 0) > 0;
        return (
          <Space key={i} size={6}>
            <Tag style={{ borderRadius: 8, marginInlineEnd: 0 }} color={isRatio ? 'blue' : 'green'}>
              {name}
            </Tag>
            <Text style={{ fontSize: 12, ...MONO }}>
              {isRatio ? `${(((a.ratio_bps ?? 0) / 10000) * 100).toFixed(1)}%` : `¥${((a.fixed_amount ?? 0) / 100).toFixed(2)}`}
            </Text>
          </Space>
        );
      })}
    </Space>
  );
};

export const SplitRules: React.FC = () => {
  const { message } = AntApp.useApp();
  const queryClient = useQueryClient();
  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<SplitRule | null>(null);
  const [configRule, setConfigRule] = React.useState<SplitRule | null>(null);
  const [trialRule, setTrialRule] = React.useState<SplitRule | null>(null);
  const [trialPreview, setTrialPreview] = React.useState<SplitPreviewData | null>(null);
  const [trialOpen, setTrialOpen] = React.useState(false);
  const [trialForm] = Form.useForm<{ amount: number; store_ids?: number[]; channel?: string }>();
  const [form] = Form.useForm<RuleForm>();

  const listQuery = useQuery({ queryKey: ['split-rules'], queryFn: () => listSplitRules() });

  // 门店映射：生效门店列展示 + 表单门店选择 + 配置弹窗停用提示
  const storeMapQuery = useQuery({
    queryKey: ['stores', 'all'],
    queryFn: () => listStores({ page: 1, size: 200 }),
    staleTime: 60_000,
  });
  const storeMap = React.useMemo(() => {
    const m = new Map<number, Store>();
    for (const s of storeMapQuery.data?.items ?? []) m.set(s.id, s);
    return m;
  }, [storeMapQuery.data]);
  const storeOptions = React.useMemo(
    () =>
      [...storeMap.values()].map((s) => ({
        value: s.id,
        label: `${s.name}（${s.store_code}）`,
      })),
    [storeMap],
  );

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['split-rules'] });

  const createMutation = useMutation({
    mutationFn: (data: Partial<SplitRule>) => createSplitRule(data),
    onSuccess: () => {
      message.success('分账规则创建成功');
      setOpen(false);
      form.resetFields();
      invalidate();
    },
    onError: (e: Error) => message.error(e.message || '创建失败，请重试'),
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<SplitRule> }) => updateSplitRule(id, data),
    onSuccess: () => {
      message.success('分账规则已更新');
      setOpen(false);
      form.resetFields();
      invalidate();
    },
    onError: (e: Error) => message.error(e.message || '更新失败，请重试'),
  });

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: number; status: number }) => setSplitRuleStatus(id, status),
    onSuccess: () => {
      message.success('状态已更新');
      invalidate();
    },
    onError: () => message.error('操作失败，请重试'),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteSplitRule(id),
    onSuccess: () => {
      message.success('规则已删除');
      invalidate();
    },
    onError: () => message.error('删除失败，请重试'),
  });

  const trialMutation = useMutation({
    mutationFn: (v: { rule_code: string; amount: number; store_ids?: number[]; channel?: string }) =>
      previewSplit({ rule_code: v.rule_code, amount: v.amount, store_ids: v.store_ids, channel: v.channel }),
    onSuccess: (data) => setTrialPreview(data),
    onError: (e: Error) => message.error(e.message || '试算失败，请重试'),
  });

  const openTrial = (r: SplitRule) => {
    setTrialRule(r);
    setTrialPreview(null);
    trialForm.resetFields();
    setTrialOpen(true);
  };

  const submitTrial = () => {
    trialForm.validateFields().then((v) => {
      if (!trialRule) return;
      trialMutation.mutate({
        rule_code: trialRule.rule_code,
        amount: Math.round(v.amount * 100),
        store_ids: (v.store_ids ?? []).length ? v.store_ids : undefined,
        channel: v.channel || undefined,
      });
    });
  };

  const rules = listQuery.data?.items ?? [];
  const activeCount = rules.filter((r) => r.status === 1).length;
  const inactiveCount = rules.length - activeCount;
  const storeCount = new Set(rules.flatMap((r) => r.conditions?.store_ids ?? [])).size;

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ priority: 0, scope: 'ALL', allocations: [{ mode: 'ratio', value: 100 }] });
    setOpen(true);
  };

  const openEdit = (r: SplitRule) => {
    setEditing(r);
    form.resetFields();
    const ids = r.conditions?.store_ids ?? [];
    form.setFieldsValue({
      rule_name: r.rule_name,
      priority: r.priority,
      channel: r.conditions?.channel ?? undefined,
      scope: ids.length ? 'SPECIFIED' : 'ALL',
      store_ids: ids.length ? ids : undefined,
      allocations: r.allocations.map((a) => ({
        mode: a.ratio_bps ? 'ratio' : 'fixed',
        value: a.ratio_bps ? a.ratio_bps / 100 : (a.fixed_amount ?? 0) / 100,
      })),
    });
    setOpen(true);
  };

  const scope = Form.useWatch('scope', form);
  const allocations = Form.useWatch('allocations', form) ?? [];
  const ratioPercent = allocations
    .filter((a) => a.mode === 'ratio')
    .reduce((s, a) => s + (a.value ?? 0), 0);
  const fixedSum = allocations
    .filter((a) => a.mode === 'fixed')
    .reduce((s, a) => s + Math.round((a.value ?? 0) * 100), 0);
  const overRatio = ratioPercent > 100;

  const handleSubmit = () => {
    const v = form.getFieldsValue();
    if (v.scope === 'SPECIFIED' && !(v.store_ids ?? []).length) {
      message.warning('请选择生效门店');
      return;
    }
    if (overRatio) {
      message.error('分配比例合计超过 100%，请调整');
      return;
    }
    const allocationsPayload: SplitRuleAllocation[] = (v.allocations ?? []).map((a: AllocationRow) => ({
      receiver_scope: ALL_STORES,
      ratio_bps: a.mode === 'ratio' ? Math.round(((a.value ?? 0) / 100) * 10000) : 0,
      fixed_amount: a.mode === 'fixed' ? Math.round((a.value ?? 0) * 100) : 0,
    }));
    const payload: Partial<SplitRule> & { rule_code?: string } = {
      rule_name: v.rule_name?.trim(),
      priority: v.priority ?? 0,
      conditions: {
        channel: v.channel || undefined,
        store_ids: v.scope === 'SPECIFIED' ? v.store_ids : [],
      },
      allocations: allocationsPayload,
    };
    if (editing) {
      updateMutation.mutate({ id: editing.id, data: payload });
    } else {
      createMutation.mutate({ ...payload, rule_code: `R${Date.now()}` });
    }
  };

  const columns = [
    {
      title: '规则',
      key: 'rule',
      width: 240,
      render: (_: unknown, r: SplitRule) => (
        <Space direction="vertical" size={2}>
          <Text strong style={{ fontSize: 14 }}>{r.rule_name}</Text>
          <Text type="secondary" style={{ fontSize: 12, ...MONO }}>{r.rule_code}</Text>
        </Space>
      ),
    },
    {
      title: '优先级',
      dataIndex: 'priority',
      key: 'priority',
      width: 90,
      render: (v: number) => <Tag style={{ borderRadius: 8, ...MONO }}>{v}</Tag>,
    },
    {
      title: '生效门店',
      key: 'stores',
      width: 240,
      render: (_: unknown, r: SplitRule) => {
        const ids = r.conditions?.store_ids ?? [];
        if (!ids.length) {
          return <Tag icon={<ShopOutlined />} style={{ borderRadius: 8 }}>全部门店</Tag>;
        }
        const names = ids.map((id) => storeMap.get(id)?.name || `#${id}`);
        return (
          <Space size={4} wrap>
            {names.slice(0, 3).map((n) => (
              <Tag key={n} style={{ borderRadius: 8 }}>{n}</Tag>
            ))}
            {names.length > 3 && (
              <Tooltip title={names.join('、')}>
                <Text type="secondary" style={{ fontSize: 12 }}>等 {names.length} 家</Text>
              </Tooltip>
            )}
          </Space>
        );
      },
    },
    {
      title: '分配方案',
      key: 'allocations',
      render: (_: unknown, r: SplitRule) => <AllocationSummary allocations={r.allocations ?? []} />,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 90,
      render: (v: number) => <StatusBadge status={v} />,
    },
    {
      title: '操作',
      key: 'actions',
      width: 340,
      render: (_: unknown, r: SplitRule) => (
        <Space size={4}>
          <Switch
            size="small"
            checked={r.status === 1}
            checkedChildren="启用"
            unCheckedChildren="停用"
            loading={statusMutation.isPending}
            onChange={(checked) => statusMutation.mutate({ id: r.id, status: checked ? 1 : 0 })}
          />
          <Button size="small" type="text" icon={<EnvironmentOutlined />} onClick={() => setConfigRule(r)}>
            门店配置
          </Button>
          <Button size="small" onClick={() => openTrial(r)}>
            试算
          </Button>
          <Button size="small" type="text" onClick={() => openEdit(r)}>
            编辑
          </Button>
          <Popconfirm title="删除后将不再按该规则分账，确认删除？" onConfirm={() => deleteMutation.mutate(r.id)}>
            <Button size="small" type="text" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div className="hp-page" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <KpiCard title="规则总数" value={rules.length} icon={<BranchesOutlined />} color="#1e6fff" loading={listQuery.isLoading} />
        </Col>
        <Col xs={12} md={6}>
          <KpiCard title="启用中" value={activeCount} icon={<CheckCircleOutlined />} color="#10b981" loading={listQuery.isLoading} />
        </Col>
        <Col xs={12} md={6}>
          <KpiCard title="停用中" value={inactiveCount} icon={<StopOutlined />} color="#f59e0b" loading={listQuery.isLoading} />
        </Col>
        <Col xs={12} md={6}>
          <KpiCard title="涉及门店" value={storeCount} icon={<ShopOutlined />} color="#8b5cf6" loading={listQuery.isLoading} />
        </Col>
      </Row>

      <Card
        title={
          <Space>
            <span>分账规则</span>
            <Tag style={{ borderRadius: 8 }} color="blue">{rules.length}</Tag>
          </Space>
        }
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新建规则
          </Button>
        }
      >
        <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
          订单支付成功后，按优先级最高的命中规则将部分款项分账给门店，其余归商户；比例模式下末笔自动补齐取整误差。
        </Text>
        <Table<SplitRule>
          className="hp-zebra"
          rowKey="id"
          columns={columns}
          dataSource={rules}
          loading={listQuery.isLoading}
          pagination={false}
          scroll={{ x: 980 }}
          locale={{
            emptyText: (
              <Empty description="还没有分账规则">
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                  创建第一条规则
                </Button>
              </Empty>
            ),
          }}
        />
      </Card>

      <Modal
        title={editing ? `编辑规则 · ${editing.rule_code}` : '新建分账规则'}
        open={open}
        onCancel={() => setOpen(false)}
        onOk={handleSubmit}
        confirmLoading={createMutation.isPending || updateMutation.isPending}
        okText={editing ? '保存' : '创建'}
        width={760}
      >
        <Form form={form} layout="vertical">
          <Divider orientation="left" plain style={{ marginTop: 0 }}>
            基本信息
          </Divider>
          <Row gutter={16}>
            <Col span={16}>
              <Form.Item name="rule_name" label="规则名称" rules={[{ required: true, whitespace: true, message: '请输入规则名称' }]}>
                <Input placeholder="如：门店分成 70/30" maxLength={128} />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="priority" label="优先级" extra="数值越大越优先">
                <InputNumber min={0} max={999} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="channel" label="支付渠道" extra="留空表示不限渠道">
            <Select
              allowClear
              placeholder="不限渠道"
              options={[
                { value: 'WECHAT', label: '微信支付' },
                { value: 'ALIPAY', label: '支付宝' },
              ]}
            />
          </Form.Item>

          <Divider orientation="left" plain>生效范围</Divider>
          <Form.Item name="scope" label="规则适用的门店" initialValue="ALL">
            <Radio.Group
              optionType="button"
              buttonStyle="solid"
              options={[
                { value: 'ALL', label: '全部门店' },
                { value: 'SPECIFIED', label: '指定门店' },
              ]}
            />
          </Form.Item>
          {scope === 'SPECIFIED' && (
            <Form.Item
              name="store_ids"
              label="选择门店"
              rules={[{ required: true, message: '请选择至少一个门店' }]}
              extra="仅命中这些门店的订单会按本规则分账"
            >
              <Select
                mode="multiple"
                showSearch
                optionFilterProp="label"
                placeholder="搜索并选择门店"
                options={storeOptions}
                maxTagCount="responsive"
              />
            </Form.Item>
          )}

          <Divider orientation="left" plain>分配方案</Divider>
          <Form.List
            name="allocations"
            rules={[{ validator: async (_, v) => { if (!v?.length) throw new Error('至少添加一个分配项'); } }]}
          >
            {(fields, { add, remove }) => (
              <>
                {fields.map((field) => (
                  <div
                    key={field.key}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 12,
                      padding: '12px 16px',
                      marginBottom: 12,
                      border: '1px solid #e5e9f2',
                      borderRadius: 10,
                      background: '#fafbfd',
                    }}
                  >
                    <Tag color="blue" style={{ borderRadius: 8, margin: 0 }}>全部门店</Tag>
                    <Form.Item name={[field.name, 'mode']} style={{ marginBottom: 0 }}>
                      <Radio.Group
                        optionType="button"
                        buttonStyle="solid"
                        size="small"
                        options={[
                          { value: 'ratio', label: '按比例' },
                          { value: 'fixed', label: '固定金额' },
                        ]}
                      />
                    </Form.Item>
                    <Form.Item
                      name={[field.name, 'value']}
                      rules={[{ required: true, message: '请输入' }]}
                      style={{ marginBottom: 0 }}
                    >
                      <InputNumber min={0} style={{ width: 160 }} placeholder="数值" />
                    </Form.Item>
                    <Form.Item shouldUpdate style={{ marginBottom: 0 }}>
                      {({ getFieldValue }) => (
                        <Text type="secondary">
                          {getFieldValue(['allocations', field.name, 'mode']) === 'ratio' ? '%' : '元'}
                        </Text>
                      )}
                    </Form.Item>
                    <Tooltip title="删除该分配项">
                      <Button
                        type="text"
                        danger
                        icon={<MinusCircleOutlined />}
                        onClick={() => remove(field.name)}
                      />
                    </Tooltip>
                  </div>
                ))}
                <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add({ mode: 'ratio', value: 0 })}>
                  添加分配项
                </Button>

                {(ratioPercent > 0 || fixedSum > 0) && (
                  <div style={{ marginTop: 16 }}>
                    <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 4 }}>
                      <Text type="secondary" style={{ fontSize: 13 }}>
                        {ratioPercent > 0 ? '比例合计' : '金额合计'}
                      </Text>
                      <Text strong style={{ ...MONO }}>
                        {ratioPercent > 0 ? `${ratioPercent.toFixed(1)}%` : `¥${(fixedSum / 100).toFixed(2)}`}
                      </Text>
                    </Space>
                    {ratioPercent > 0 ? (
                      <Progress
                        percent={Math.min(100, ratioPercent)}
                        status={overRatio ? 'exception' : 'active'}
                        size="small"
                      />
                    ) : (
                      <Progress
                        percent={Math.min(100, (fixedSum / 100000) * 100)}
                        showInfo={false}
                        size="small"
                      />
                    )}
                    <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
                      {overRatio
                        ? '比例合计超过 100%，请调整分配项'
                        : ratioPercent > 0 && ratioPercent < 100
                          ? '未分配部分（剩余比例）归商户'
                          : '每个分配项按各门店实收金额占比分摊；末笔自动补齐取整误差'}
                    </Text>
                  </div>
                )}
              </>
            )}
          </Form.List>
        </Form>
      </Modal>

      {configRule && (
        <StorePicker
          rule={configRule}
          open={!!configRule}
          storeMap={storeMap}
          onClose={() => setConfigRule(null)}
          onSaved={invalidate}
        />
      )}

      <Modal
        title={
          <Space>
            <CalculatorOutlined />
            <span>分账试算</span>
            {trialRule && <Tag style={{ borderRadius: 8 }} color="blue">{trialRule.rule_name}</Tag>}
          </Space>
        }
        open={trialOpen}
        onCancel={() => setTrialOpen(false)}
        width={560}
        footer={
          <Space>
            <Button onClick={() => setTrialOpen(false)}>关闭</Button>
            <Button type="primary" loading={trialMutation.isPending} onClick={submitTrial}>
              试算
            </Button>
          </Space>
        }
      >
        <Form form={trialForm} layout="vertical">
          <Form.Item
            name="amount"
            label="试算金额（元）"
            rules={[
              { required: true, message: '请输入试算金额' },
              {
                validator: (_, v) =>
                  v > 0 && v <= 50000 ? Promise.resolve() : Promise.reject(new Error('金额需在 0.01 ~ 50000 之间')),
              },
            ]}
          >
            <InputNumber min={0} max={50000} precision={2} style={{ width: '100%' }} placeholder="如：100.00" />
          </Form.Item>
          <Form.Item name="store_ids" label="参与门店" extra="留空表示全部门店">
            <Select
              mode="multiple"
              allowClear
              showSearch
              optionFilterProp="label"
              placeholder="全部门店"
              options={storeOptions}
              maxTagCount="responsive"
            />
          </Form.Item>
          <Form.Item name="channel" label="支付渠道" extra="留空表示不限渠道">
            <Select
              allowClear
              placeholder="不限渠道"
              options={[
                { value: 'WECHAT', label: '微信支付' },
                { value: 'ALIPAY', label: '支付宝' },
              ]}
            />
          </Form.Item>
        </Form>
        {trialPreview && (
          <SplitPreview
            items={trialPreview.items}
            totalAmount={trialPreview.total_amount}
            merchantRemain={trialPreview.merchant_remain}
            loading={trialMutation.isPending}
          />
        )}
      </Modal>
    </div>
  );
};
