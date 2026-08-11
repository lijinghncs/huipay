// 商户管理列表：进件 + 搜索筛选 + 启用/停用 + 登录密码 + 子页入口（详情 / 微信配置）
import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Card,
  Table,
  Tag,
  Button,
  Space,
  Tooltip,
  Input,
  Select,
  Form,
  Modal,
  Dropdown,
  App,
  Alert,
  Skeleton,
  Typography,
} from 'antd';
import {
  PlusOutlined,
  SearchOutlined,
  ReloadOutlined,
  EditOutlined,
  SettingOutlined, LockOutlined,
  LockOutlined,
  ArrowRightOutlined,
  DownOutlined,
  StopOutlined,
  CheckCircleOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { useNavigate } from 'react-router-dom';
import {
  listMerchants,
  onboardMerchant,
  getMerchant,
  updateMerchant,
  setMerchantStatus,
  setMerchantLoginPassword,
  type Merchant,
  type OnboardRequest,
} from '../../services/admin';
import { typeText, MerchantStatusTag, Mono, WechatConfigFormFields } from './shared';

const { Text } = Typography;

interface MerchantRow {
  id: number;
  code: string; // 商户号
  type: string;
  name: string;
  status: number;
  wallet_no: string;
  created_at: string;
}

function toRow(m: Merchant): MerchantRow {
  return {
    id: m.id,
    code: m.entity_code,
    type: typeText(m.entity_type),
    name: m.name,
    status: m.status,
    wallet_no: m.wallet_no,
    created_at: m.created_at,
  };
}

// 进件 / 编辑共用的基础资料字段
const MerchantFormFields: React.FC = () => (
  <>
    <Form.Item name="name" label="商户名称" rules={[{ required: true, message: '请输入商户名称' }]}>
      <Input placeholder="请输入商户名称" />
    </Form.Item>
    <Form.Item name="type" label="主体类型" initialValue="MERCHANT" rules={[{ required: true }]}>
      <Select disabled options={[{ value: 'MERCHANT', label: '企业 / 个体户' }]} />
    </Form.Item>
    <Form.Item name="legal_name" label="法人 / 经营者姓名">
      <Input placeholder="请输入法人 / 经营者姓名" />
    </Form.Item>
    <Form.Item name="license_no" label="营业执照 / 证件号">
      <Input placeholder="请输入营业执照号 / 身份证号" />
    </Form.Item>
    <Form.Item name="bank_account" label="结算银行卡号">
      <Input placeholder="用于后续结算打款" />
    </Form.Item>
    <Form.Item name="bank_name" label="开户行">
      <Input placeholder="请输入开户行" />
    </Form.Item>
    <Form.Item name="contact_name" label="联系人">
      <Input placeholder="请输入联系人" />
    </Form.Item>
    <Form.Item name="contact_phone" label="联系电话">
      <Input placeholder="请输入联系电话" />
    </Form.Item>
  </>
);

export const Merchants: React.FC = () => {
  const { message, modal } = App.useApp();
  const queryClient = useQueryClient();
  const nav = useNavigate();
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<number | undefined>();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [modalOpen, setModalOpen] = useState(false);
  const [form] = Form.useForm();
  const [editId, setEditId] = useState<number | null>(null);
  const [editForm] = Form.useForm();
  const [pwId, setPwId] = useState<number | null>(null);
  const [pwForm] = Form.useForm();
  const [pwId, setPwId] = useState<number | null>(null);
  const [pwForm] = Form.useForm();

  const setPassword = useMutation({
    mutationFn: ({ id, phone, password }: { id: number; phone: string; password: string }) =>
      setMerchantLoginPassword(id, phone, password),
    onSuccess: () => {
      message.success('登录手机号/密码已设置');
      setPwId(null);
      pwForm.resetFields();
    },
    onError: (err) => message.error(`设置失败：${err.message}`),
  });

  const setPassword = useMutation({
    mutationFn: ({ id, phone, password }: { id: number; phone: string; password: string }) =>
      setMerchantLoginPassword(id, phone, password),
    onSuccess: () => {
      message.success('登录手机号/密码已设置');
      setPwId(null);
      pwForm.resetFields();
    },
    onError: (err) => message.error(`设置失败：${err.message}`),
  });

  const { data, isLoading, isFetching, isError, refetch } = useQuery({
    queryKey: ['merchants', { keyword, status, page, pageSize }],
    queryFn: () =>
      listMerchants({
        page,
        page_size: pageSize,
        keyword: keyword || undefined,
        status,
      }),
    placeholderData: (prev) => prev,
  });

  const onboard = useMutation({
    mutationFn: (values: OnboardRequest) => onboardMerchant(values),
    onSuccess: () => {
      message.success('商户进件成功（已自动开通钱包）');
      setModalOpen(false);
      form.resetFields();
      queryClient.invalidateQueries({ queryKey: ['merchants'] });
    },
    onError: (err) => message.error(`进件失败：${err.message}`),
  });

  const update = useMutation({
    mutationFn: ({ id, values }: { id: number; values: OnboardRequest }) => updateMerchant(id, values),
    onSuccess: () => {
      message.success('商户资料已更新');
      setEditId(null);
      editForm.resetFields();
      queryClient.invalidateQueries({ queryKey: ['merchants'] });
    },
    onError: (err) => message.error(`更新失败：${err.message}`),
  });

  const toggleStatus = useMutation({
    mutationFn: ({ id, status }: { id: number; status: number }) => setMerchantStatus(id, status),
    onSuccess: () => {
      message.success('商户状态已更新');
      queryClient.invalidateQueries({ queryKey: ['merchants'] });
    },
    onError: (err) => message.error(`操作失败：${err.message}`),
  });

  // 编辑预填数据
  const editDetailQuery = useQuery({
    queryKey: ['merchant-edit', editId],
    queryFn: () => getMerchant(editId!),
    enabled: !!editId,
  });
  React.useEffect(() => {
    if (editDetailQuery.data) {
      const k = editDetailQuery.data.kyc_data ?? {};
      editForm.setFieldsValue({
        name: editDetailQuery.data.name,
        type: editDetailQuery.data.entity_type,
        legal_name: k.legal_name ?? '',
        license_no: k.license_no ?? '',
        bank_account: k.bank_account ?? '',
        bank_name: k.bank_name ?? '',
        contact_name: k.contact_name ?? '',
        contact_phone: k.contact_phone ?? '',
      });
    }
  }, [editDetailQuery.data, editForm]);

  const columns: ColumnsType<MerchantRow> = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 70 },
    {
      title: '商户号',
      dataIndex: 'code',
      key: 'code',
      width: 190,
      render: (v: string) => <Mono>{v || '-'}</Mono>,
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 180,
      ellipsis: true,
      sorter: (a, b) => a.name.localeCompare(b.name),
      render: (v: string, r) => (
        <Space size={6}>
          <Text strong>{v}</Text>
          <Tag style={{ borderRadius: 10, fontSize: 12 }}>{r.type}</Tag>
        </Space>
      ),
    },
    { title: '类型', dataIndex: 'type', key: 'type', width: 90, hidden: true },
    {
      title: '钱包号',
      dataIndex: 'wallet_no',
      key: 'wallet_no',
      width: 170,
      render: (v: string) => <Mono>{v || '-'}</Mono>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 90,
      filters: [{ text: '启用', value: 1 }, { text: '停用', value: 0 }],
      onFilter: (v, r) => r.status === v,
      render: (v: number) => <MerchantStatusTag status={v} />,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 165,
      render: (v: string) => (v ? v.replace('T', ' ').slice(0, 19) : '-'),
    },
    {
      title: '操作',
      key: 'actions',
      fixed: 'right',
      width: 130,
      render: (_, r) => (
        <Space size={4}>
          <Tooltip title="查看商户详情">
            <Button type="link" size="small" onClick={() => nav(`/merchants/${r.id}`)}>
              详情 <ArrowRightOutlined style={{ fontSize: 10 }} />
            </Button>
          </Tooltip>
          <Dropdown
            menu={{
              items: [
                { key: 'edit', icon: <EditOutlined />, label: '编辑' },
                { key: 'wechat', icon: <SettingOutlined />, label: '微信配置' },
                { key: 'password', icon: <LockOutlined />, label: '登录密码' },
                {
                  key: 'toggle',
                  icon: r.status === 1 ? <StopOutlined /> : <CheckCircleOutlined />,
                  label: r.status === 1 ? '停用' : '启用',
                  danger: r.status === 1,
                },
              ],
              onClick: ({ key }) => {
                if (key === 'edit') {
                  setEditId(r.id);
                } else if (key === 'wechat') {
                  nav(`/merchants/${r.id}/wechat-config`);
                } else if (key === 'password') {
                  setPwId(r.id);
                  pwForm.resetFields();
                } else if (key === 'toggle') {
                  const next = r.status === 1 ? 0 : 1;
                  modal.confirm({
                    title: r.status === 1 ? '确认停用该商户？' : '确认启用该商户？',
                    okText: r.status === 1 ? '停用' : '启用',
                    okButtonProps: { danger: r.status === 1 },
                    onOk: () => toggleStatus.mutate({ id: r.id, status: next }),
                  });
                }
              },
            }}
          >
            <Button type="link" size="small">
              更多 <DownOutlined style={{ fontSize: 10 }} />
            </Button>
          </Dropdown>
          <Tooltip title="设置登录手机号与密码（商户工作台登录用）">
            <Button type="link" size="small" icon={<LockOutlined />} onClick={() => { setPwId(r.id); pwForm.resetFields(); }}>
              登录密码
            </Button>
          </Tooltip>
        </Space>
      ),
    },
  ];

  return (
    <Card
      title={
        <Space>
          <span>商户管理</span>
          <Tag style={{ borderRadius: 10 }} color="blue">{data?.total ?? 0}</Tag>
        </Space>
      }
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>
          新建商户
        </Button>
      }
    >
      <Space style={{ marginBottom: 16 }} wrap>
        <Input
          allowClear
          prefix={<SearchOutlined style={{ color: 'rgba(0,0,0,0.35)' }} />}
          placeholder="搜索商户名称 / 商户号"
          style={{ width: 240 }}
          value={keyword}
          onChange={(e) => { setKeyword(e.target.value); setPage(1); }}
        />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 120 }}
          value={status}
          onChange={(v) => { setStatus(v); setPage(1); }}
          options={[
            { value: 1, label: '启用' },
            { value: 0, label: '停用' },
          ]}
        />
        <Button icon={<ReloadOutlined />} onClick={() => { setKeyword(''); setStatus(undefined); setPage(1); }}>
          重置
        </Button>
      </Space>

      {isError ? (
        <Alert
          type="error"
          showIcon
          message="商户列表加载失败"
          description="请确认后端服务已启动（localhost:8080），或检查网络后重试。"
          action={
            <Button size="small" danger onClick={() => refetch()}>
              重试
            </Button>
          }
          style={{ marginBottom: 16 }}
        />
      ) : isLoading ? (
        <Skeleton active paragraph={{ rows: 6 }} />
      ) : (
        <Table<MerchantRow>
          className="hp-zebra"
          rowKey="id"
          columns={columns}
          dataSource={(data?.items ?? []).map(toRow)}
          loading={isFetching}
          scroll={{ x: 1000 }}
          pagination={{
            current: page,
            pageSize,
            total: data?.total ?? 0,
            showSizeChanger: true,
            pageSizeOptions: [10, 20, 50],
            showTotal: (t) => `共 ${t} 条`,
            onChange: (p, s) => { setPage(p); setPageSize(s); },
          }}
          locale={{ emptyText: '暂无商户数据，点击右上角「新建商户」进件' }}
        />
      )}

      {/* 进件 */}
      <Modal
        title="商户进件"
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => {
          form
            .validateFields()
            .then((values: OnboardRequest) => onboard.mutate(values))
            .catch(() => undefined);
        }}
        confirmLoading={onboard.isPending}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <MerchantFormFields />
          <WechatConfigFormFields />
        </Form>
      </Modal>

      {/* 编辑 */}
      <Modal
        title={`编辑商户 ${editId ?? ''}`}
        open={!!editId}
        onCancel={() => { setEditId(null); editForm.resetFields(); }}
        onOk={() => {
          editForm
            .validateFields()
            .then((values: OnboardRequest) => editId && update.mutate({ id: editId, values }))
            .catch(() => undefined);
        }}
        confirmLoading={update.isPending}
        destroyOnHidden
      >
        <Form form={editForm} layout="vertical" style={{ marginTop: 16 }}>
          <MerchantFormFields />
        </Form>
      </Modal>

      {/* 设置登录手机号与密码（商户工作台登录） */}
      <Modal
        title={`设置登录密码${pwId ? ` · 商户 ${pwId}` : ''}`}
        open={!!pwId}
        onCancel={() => { setPwId(null); pwForm.resetFields(); }}
        onOk={() => {
          pwForm
            .validateFields()
            .then((values: { phone: string; password: string }) =>
              pwId && setPassword.mutate({ id: pwId, phone: values.phone, password: values.password }),
            )
            .catch(() => undefined);
        }}
        confirmLoading={setPassword.isPending}
        destroyOnClose
      >
        <Form form={pwForm} layout="vertical" style={{ marginTop: 16 }}>
          <Alert
            type="info"
            showIcon
            message="商户使用该手机号 + 密码登录商家工作台；密码至少 6 位。"
            style={{ marginBottom: 8 }}
          />
          <Form.Item name="phone" label="登录手机号" rules={[{ required: true, message: '请输入登录手机号' }]}>
            <Input placeholder="如 13800000000" maxLength={32} />
          </Form.Item>
          <Form.Item name="password" label="登录密码" rules={[
            { required: true, message: '请输入登录密码' },
            { min: 6, message: '密码至少 6 位' },
          ]}>
            <Input.Password placeholder="至少 6 位" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 设置登录手机号与密码（商户工作台登录） */}
      <Modal
        title={`设置登录密码${pwId ? ` · 商户 ${pwId}` : ''}`}
        open={!!pwId}
        onCancel={() => { setPwId(null); pwForm.resetFields(); }}
        onOk={() => {
          pwForm
            .validateFields()
            .then((values: { phone: string; password: string }) =>
              pwId && setPassword.mutate({ id: pwId, phone: values.phone, password: values.password }),
            )
            .catch(() => undefined);
        }}
        confirmLoading={setPassword.isPending}
        destroyOnClose
      >
        <Form form={pwForm} layout="vertical" style={{ marginTop: 16 }}>
          <Alert
            type="info"
            showIcon
            message="商户使用该手机号 + 密码登录商家工作台；密码至少 6 位。"
            style={{ marginBottom: 8 }}
          />
          <Form.Item name="phone" label="登录手机号" rules={[{ required: true, message: '请输入登录手机号' }]}>
            <Input placeholder="如 13800000000" maxLength={32} />
          </Form.Item>
          <Form.Item name="password" label="登录密码" rules={[
            { required: true, message: '请输入登录密码' },
            { min: 6, message: '密码至少 6 位' },
          ]}>
            <Input.Password placeholder="至少 6 位" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 设置登录手机号与密码（商户工作台登录） */}
      <Modal
        title={`设置登录密码${pwId ? ` · 商户 ${pwId}` : ''}`}
        open={!!pwId}
        onCancel={() => { setPwId(null); pwForm.resetFields(); }}
        onOk={() => {
          pwForm
            .validateFields()
            .then((values: { phone: string; password: string }) =>
              pwId && setPassword.mutate({ id: pwId, phone: values.phone, password: values.password }),
            )
            .catch(() => undefined);
        }}
        confirmLoading={setPassword.isPending}
        destroyOnHidden
      >
        <Form form={pwForm} layout="vertical" style={{ marginTop: 16 }}>
          <Alert
            type="info"
            showIcon
            message="商户使用该手机号 + 密码登录商家工作台；密码至少 6 位。"
            style={{ marginBottom: 8 }}
          />
          <Form.Item name="phone" label="登录手机号" rules={[{ required: true, message: '请输入登录手机号' }]}>
            <Input placeholder="如 13800000000" maxLength={32} />
          </Form.Item>
          <Form.Item name="password" label="登录密码" rules={[
            { required: true, message: '请输入登录密码' },
            { min: 6, message: '密码至少 6 位' },
          ]}>
            <Input.Password placeholder="至少 6 位" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

