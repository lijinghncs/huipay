// 门店配置弹窗：为分账规则选择生效门店（单选 / 多选，形态复用门店列表）
import React from 'react';
import {
  Alert,
  App as AntApp,
  Badge,
  Button,
  Empty,
  Input,
  Modal,
  Radio,
  Segmented,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import { useMutation, useQuery } from '@tanstack/react-query';
import { listStores, updateSplitRule, type SplitRule, type Store } from '../services/user';
import { formatDateTime } from '@huipay/shared/utils';

const { Text } = Typography;
const MONO = { fontVariantNumeric: 'tabular-nums' as const, fontFamily: 'Fira Code, Consolas, Monaco, monospace' };

const STORE_TYPE_LABEL: Record<string, string> = {
  DIRECT: '直营',
  FRANCHISE: '加盟',
  PARTNER: '合作',
};

const statusTag = (v: number) =>
  v === 1 ? <Badge status="success" text="启用" /> : <Badge status="default" text="停用" />;

type Scope = 'ALL' | 'SPECIFIED';
type Mode = 'multi' | 'single';

export interface StorePickerProps {
  /** 目标分账规则（读取 conditions.store_ids 回显） */
  rule: SplitRule;
  open: boolean;
  /** 门店 id → 门店 映射（父页面提供，用于渲染名称与停用提示） */
  storeMap: Map<number, Store>;
  onClose: () => void;
  /** 保存成功回调（父页面刷新规则列表） */
  onSaved: () => void;
}

/** 门店配置弹窗。 */
export const StorePicker: React.FC<StorePickerProps> = ({ rule, open, storeMap, onClose, onSaved }) => {
  const { message } = AntApp.useApp();
  const initialIds = rule.conditions?.store_ids ?? [];

  const [scope, setScope] = React.useState<Scope>(initialIds.length > 0 ? 'SPECIFIED' : 'ALL');
  const [mode, setMode] = React.useState<Mode>('multi');
  const [selectedKeys, setSelectedKeys] = React.useState<number[]>(initialIds);
  const [keyword, setKeyword] = React.useState('');
  const [status, setStatus] = React.useState<number | undefined>(undefined);
  const [page, setPage] = React.useState(1);
  const [size, setSize] = React.useState(20);

  // 每次打开时按规则当前配置重置
  React.useEffect(() => {
    if (open) {
      setScope(initialIds.length > 0 ? 'SPECIFIED' : 'ALL');
      setMode('multi');
      setSelectedKeys(initialIds);
      setKeyword('');
      setStatus(undefined);
      setPage(1);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, rule.id]);

  const listQuery = useQuery({
    queryKey: ['stores-picker', page, size, keyword, status],
    queryFn: () => listStores({ page, size, keyword, status }),
    enabled: open,
  });

  const saveMutation = useMutation({
    mutationFn: (ids: number[]) =>
      updateSplitRule(rule.id, {
        conditions: { ...rule.conditions, store_ids: ids },
      }),
    onSuccess: () => {
      message.success('门店配置已保存');
      onSaved();
      onClose();
    },
    onError: (e: Error) => message.error(e.message || '保存失败，请重试'),
  });

  const handleSave = () => {
    if (scope === 'ALL') {
      saveMutation.mutate([]);
      return;
    }
    const ids = [...new Set(selectedKeys)];
    if (ids.length === 0) {
      message.warning('请选择至少一个门店');
      return;
    }
    const disabledCount = ids.filter((id) => storeMap.get(id)?.status === 0).length;
    if (disabledCount > 0) {
      message.warning(`已选 ${ids.length} 家中 ${disabledCount} 家已停用，仍将保存`);
    }
    saveMutation.mutate(ids);
  };

  const rowSelection = {
    type: (mode === 'single' ? 'radio' : 'checkbox') as 'checkbox' | 'radio',
    selectedRowKeys: selectedKeys,
    preserveSelectedRowKeys: true,
    onChange: (keys: React.Key[]) => {
      const next = keys as number[];
      setSelectedKeys(mode === 'single' ? (next.length ? [next[next.length - 1]] : []) : next);
    },
  };

  const columns = [
    {
      title: '门店名称',
      dataIndex: 'name',
      key: 'name',
      width: 200,
      ellipsis: true,
      render: (v: string) => v,
    },
    { title: '门店编码', dataIndex: 'store_code', key: 'store_code', width: 180, render: (v: string) => <Text style={MONO}>{v}</Text> },
    {
      title: '类型',
      dataIndex: 'store_type',
      key: 'store_type',
      width: 90,
      render: (v?: string) => (v ? STORE_TYPE_LABEL[v] ?? v : '-'),
    },
    { title: '联系电话', dataIndex: 'contact_phone', key: 'contact_phone', width: 130, render: (v?: string) => v || '-' },
    { title: '码牌数', dataIndex: 'code_count', key: 'code_count', width: 80, render: (v?: number) => v ?? 0 },
    { title: '状态', dataIndex: 'status', key: 'status', width: 80, render: statusTag },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 170, render: (v: string) => formatDateTime(v) },
  ];

  return (
    <Modal
      title={`门店配置 · ${rule.rule_name || rule.rule_code}`}
      open={open}
      onCancel={onClose}
      width={860}
      styles={{ body: { paddingTop: 8 } }}
      footer={
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16 }}>
          <Text type="secondary">
            {scope === 'ALL' ? '当前规则对全部门店生效' : `已选 ${selectedKeys.length} 家门店`}
          </Text>
          <Space>
            <Button onClick={onClose}>取消</Button>
            <Button type="primary" loading={saveMutation.isPending} onClick={handleSave}>
              保存
            </Button>
          </Space>
        </div>
      }
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        <Radio.Group
          value={scope}
          onChange={(e) => setScope(e.target.value as Scope)}
          optionType="button"
          buttonStyle="solid"
          options={[
            { value: 'ALL', label: '全部门店' },
            { value: 'SPECIFIED', label: '指定门店' },
          ]}
        />
        {scope === 'ALL' && (
          <Alert
            type="info"
            showIcon
            message="当前规则对全部门店生效，无需选择门店"
            description="如需仅对部分门店生效，请切换到「指定门店」后再选择。"
          />
        )}

        {scope === 'SPECIFIED' && (
          <>
            <Space wrap>
              <Segmented
                value={mode}
                onChange={(v) => {
                  setMode(v as Mode);
                  if (v === 'single') setSelectedKeys((prev) => (prev.length ? [prev[0]] : []));
                }}
                options={[
                  { label: '多选', value: 'multi' },
                  { label: '单选', value: 'single' },
                ]}
              />
              <Input
                placeholder="按门店名称 / 编码搜索"
                style={{ width: 220 }}
                allowClear
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                onPressEnter={() => setPage(1)}
              />
              <Select
                allowClear
                placeholder="状态"
                style={{ width: 120 }}
                value={status}
                onChange={(v) => {
                  setStatus(v);
                  setPage(1);
                }}
                options={[
                  { value: 1, label: '启用' },
                  { value: 0, label: '停用' },
                ]}
              />
              <Button type="primary" onClick={() => setPage(1)}>
                查询
              </Button>
            </Space>

            <Table<Store>
              rowKey="id"
              columns={columns}
              dataSource={listQuery.data?.items ?? []}
              loading={listQuery.isLoading}
              rowSelection={rowSelection}
              size="small"
              scroll={{ x: 980, y: 320 }}
              locale={{
                emptyText: (
                  <Empty description={keyword || status !== undefined ? '没有符合条件的门店' : '还没有门店，请先在门店管理创建'}>
                    {!keyword && status === undefined && (
                      <Text type="secondary" style={{ fontSize: 12 }}>创建门店后即可在此选择</Text>
                    )}
                  </Empty>
                ),
              }}
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
          </>
        )}
      </div>
    </Modal>
  );
};
