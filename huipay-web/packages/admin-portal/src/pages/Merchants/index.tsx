// 商户管理：进件 + 列表 + 详情 + 编辑 + 启用/停用 + 经营概览（对接后端 /v1/admin/merchants）
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
  Drawer,
  Descriptions,
  Statistic,
  Row,
  Col,
  Popconfirm,
  App,
  Alert,
  Skeleton,
  Collapse,
  Switch,
} from 'antd';
import { EyeOutlined, PlusOutlined, SearchOutlined, ReloadOutlined, EditOutlined, SettingOutlined, LockOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { formatCents, formatDateTime } from '@huipay/shared/utils';
import {
  listMerchants,
  onboardMerchant,
  getMerchant,
  getMerchantOverview,
  getMerchantWechatConfig,
  updateMerchantWechatConfig,
  updateMerchant,
  setMerchantStatus,
  setMerchantLoginPassword,
  type Merchant,
  type MerchantDetail,
  type OnboardRequest,
  type MerchantWechatConfigInput,
  type MerchantWechatConfigView,
} from '../../services/admin';

const TYPE_TEXT: Record<string, string> = { MERCHANT: '企业', STORE: '门店', PROMOTER: '推广员', PLATFORM: '平台', ISV: '服务商' };

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
    type: TYPE_TEXT[m.entity_type] ?? m.entity_type,
    name: m.name,
    status: m.status,
    wallet_no: m.wallet_no,
    created_at: m.created_at,
  };
}

// 进件 / 编辑共用的表单字段
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

// 微信支付配置表单字段（进件 / 独立配置共用）。
// edit=true 时敏感字段留空 = 不修改（不覆盖后端既有密文）。
const WechatConfigFormFields: React.FC<{ edit?: boolean }> = ({ edit }) => {
  const keep = edit ? '（留空则不修改）' : '';
  return (
    <Collapse
      ghost
      items={[
        {
          key: 'wechat',
          label: '微信支付配置',
          children: (
            <>
              <Form.Item name={['wechat_config', 'enabled']} label="启用微信支付" valuePropName="checked" initialValue={false}>
                <Switch />
              </Form.Item>
              <Form.Item name={['wechat_config', 'mchid']} label="微信支付商户号">
                <Input placeholder="微信支付商户号 mchid" />
              </Form.Item>
              <Form.Item name={['wechat_config', 'appid']} label="公众号 / 小程序 AppID">
                <Input placeholder="AppID" />
              </Form.Item>
              <Form.Item name={['wechat_config', 'app_secret']} label={`AppSecret${keep}`}>
                <Input.Password placeholder={edit ? '留空则不修改' : '公众号 AppSecret'} />
              </Form.Item>
              <Form.Item name={['wechat_config', 'api_v3_key']} label={`APIv3 密钥${keep}`}>
                <Input.Password placeholder={edit ? '留空则不修改' : 'APIv3 密钥（回调解密）'} />
              </Form.Item>
              <Form.Item name={['wechat_config', 'merchant_serial_no']} label="商户证书序列号">
                <Input placeholder="商户 API 证书 serial_no" />
              </Form.Item>
              <Form.Item name={['wechat_config', 'merchant_private_key']} label={`商户 API 私钥 PEM${keep}`}>
                <Input.TextArea rows={4} placeholder={edit ? '留空则不修改' : '商户 API 私钥 PEM 内容'} />
              </Form.Item>
              <Form.Item name={['wechat_config', 'platform_serial_no']} label="平台证书序列号">
                <Input placeholder="微信平台证书 serial_no" />
              </Form.Item>
              <Form.Item name={['wechat_config', 'platform_public_key']} label={`微信平台公钥 PEM${keep}`}>
                <Input.TextArea rows={4} placeholder={edit ? '留空则不修改' : '微信平台公钥 PEM 内容'} />
              </Form.Item>
              <Form.Item name={['wechat_config', 'notify_base_url']} label="回调地址前缀">
                <Input placeholder="如 https://checkout.huipay.cn" />
              </Form.Item>
            </>
          ),
        },
      ]}
    />
  );
};

// 敏感字段 configured 标记展示（已配置 / 未配置）。
const ConfiguredTag: React.FC<{ configured?: boolean }> = ({ configured }) =>
  configured ? <Tag color="green">已配置</Tag> : <Tag color="default">未配置</Tag>;

export const Merchants: React.FC = () => {
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<number | undefined>();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [modalOpen, setModalOpen] = useState(false);
  const [form] = Form.useForm();
  const [detailId, setDetailId] = useState<number | null>(null);
  const [editId, setEditId] = useState<number | null>(null);
  const [editForm] = Form.useForm();
  const [wcId, setWcId] = useState<number | null>(null);
  const [wcForm] = Form.useForm();
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

  // 详情抽屉数据
  const detailQuery = useQuery({
    queryKey: ['merchant-detail', detailId],
    queryFn: () => getMerchant(detailId!),
    enabled: !!detailId,
  });
  const overviewQuery = useQuery({
    queryKey: ['merchant-overview', detailId],
    queryFn: () => getMerchantOverview(detailId!),
    enabled: !!detailId,
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

  // 微信支付配置：加载既有非敏感字段用于预填（敏感字段留空 = 不修改）
  const wcQuery = useQuery({
    queryKey: ['merchant-wechat', wcId],
    queryFn: () => getMerchantWechatConfig(wcId!),
    enabled: !!wcId,
  });
  React.useEffect(() => {
    if (wcQuery.data) {
      wcForm.setFieldsValue({
        wechat_config: {
          enabled: wcQuery.data.enabled ?? false,
          mchid: wcQuery.data.mchid ?? '',
          appid: wcQuery.data.appid ?? '',
          merchant_serial_no: wcQuery.data.merchant_serial_no ?? '',
          platform_serial_no: wcQuery.data.platform_serial_no ?? '',
          notify_base_url: wcQuery.data.notify_base_url ?? '',
          // 敏感字段不预填明文，留空 = 不修改
        },
      });
    }
  }, [wcQuery.data, wcForm]);

  const saveWechat = useMutation({
    mutationFn: ({ id, cfg }: { id: number; cfg: MerchantWechatConfigInput }) =>
      updateMerchantWechatConfig(id, cfg),
    onSuccess: () => {
      message.success('微信支付配置已保存');
      setWcId(null);
      wcForm.resetFields();
      queryClient.invalidateQueries({ queryKey: ['merchant-detail', wcId] });
    },
    onError: (err) => message.error(`保存失败：${err.message}`),
  });

  const columns: ColumnsType<MerchantRow> = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
    {
      title: '商户号',
      dataIndex: 'code',
      key: 'code',
      width: 200,
      render: (v: string) => <span style={{ fontFamily: 'monospace', fontWeight: 600 }}>{v || '-'}</span>,
    },
    { title: '名称', dataIndex: 'name', key: 'name', sorter: (a, b) => a.name.localeCompare(b.name) },
    { title: '类型', dataIndex: 'type', key: 'type', width: 90 },
    {
      title: '钱包号',
      dataIndex: 'wallet_no',
      key: 'wallet_no',
      width: 180,
      render: (v: string) => <span style={{ fontFamily: 'monospace' }}>{v || '-'}</span>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 90,
      filters: [{ text: '启用', value: 1 }, { text: '停用', value: 0 }],
      onFilter: (v, r) => r.status === v,
      render: (v: number) => (v === 1 ? <Tag color="green">启用</Tag> : <Tag color="red">停用</Tag>),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 170,
      render: (v: string) => (v ? v.replace('T', ' ').slice(0, 19) : '-'),
    },
    {
      title: '操作',
      key: 'actions',
      fixed: 'right',
      width: 180,
      render: (_, r) => (
        <Space size={0}>
          <Tooltip title="详情">
            <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => setDetailId(r.id)}>
              详情
            </Button>
          </Tooltip>
          <Tooltip title="编辑">
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => setEditId(r.id)}>
              编辑
            </Button>
          </Tooltip>
          <Popconfirm
            title={r.status === 1 ? '确认停用该商户？' : '确认启用该商户？'}
            onConfirm={() => toggleStatus.mutate({ id: r.id, status: r.status === 1 ? 0 : 1 })}
          >
            <Button type="link" size="small" danger={r.status === 1}>
              {r.status === 1 ? '停用' : '启用'}
            </Button>
          </Popconfirm>
          <Tooltip title="微信支付配置">
            <Button type="link" size="small" icon={<SettingOutlined />} onClick={() => setWcId(r.id)}>
              微信配置
            </Button>
          </Tooltip>
          <Tooltip title="设置登录手机号与密码（商户工作台登录用）">
            <Button type="link" size="small" icon={<LockOutlined />} onClick={() => { setPwId(r.id); pwForm.resetFields(); }}>
              登录密码
            </Button>
          </Tooltip>
        </Space>
      ),
    },
  ];

  const detail = detailQuery.data as MerchantDetail | undefined;

  return (
    <Card
      title="商户管理"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>
          新建商户
        </Button>
      }
    >
      <Space style={{ marginBottom: 16 }} wrap>
        <Input
          allowClear
          prefix={<SearchOutlined />}
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
          scroll={{ x: 1200 }}
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
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <MerchantFormFields />
          <WechatConfigFormFields />
        </Form>
      </Modal>

      {/* 微信支付配置（独立端点，敏感字段留空 = 不修改，非敏感字段留空 = 清空） */}
      <Modal
        title={`微信支付配置${wcId ? ` · 商户 ${wcId}` : ''}`}
        open={!!wcId}
        onCancel={() => { setWcId(null); wcForm.resetFields(); }}
        onOk={() => {
          wcForm
            .validateFields()
            .then((values: { wechat_config: MerchantWechatConfigInput }) =>
              wcId && saveWechat.mutate({ id: wcId, cfg: values.wechat_config }),
            )
            .catch(() => undefined);
        }}
        confirmLoading={saveWechat.isPending}
        destroyOnClose
        width={560}
      >
        <Form form={wcForm} layout="vertical" style={{ marginTop: 16 }}>
          <Alert
            type="info"
            showIcon
            message="敏感字段（AppSecret / APIv3 密钥 / 商户私钥 / 平台公钥）已加密存储，编辑时留空则不修改。"
            style={{ marginBottom: 8 }}
          />
          <WechatConfigFormFields edit />
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
        destroyOnClose
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

      {/* 详情抽屉 */}
      <Drawer
        title={`商户详情${detail?.entity_code ? ` · ${detail.entity_code}` : ''}`}
        open={!!detailId}
        onClose={() => setDetailId(null)}
        width={560}
      >
        <Row gutter={16} style={{ marginBottom: 8 }}>
          <Col span={8}>
            <Card size="small">
              <Statistic title="钱包余额" value={overviewQuery.data?.balance ?? 0} formatter={(v) => formatCents(Number(v))} />
            </Card>
          </Col>
          <Col span={8}>
            <Card size="small">
              <Statistic title="累计实付" value={overviewQuery.data?.total_paid ?? 0} formatter={(v) => formatCents(Number(v))} />
            </Card>
          </Col>
          <Col span={8}>
            <Card size="small">
              <Statistic title="已支付订单" value={overviewQuery.data?.paid_order_count ?? 0} />
            </Card>
          </Col>
          <Col span={8}>
            <Card size="small">
              <Statistic title="订单总数" value={overviewQuery.data?.order_count ?? 0} />
            </Card>
          </Col>
          <Col span={8}>
            <Card size="small">
              <Statistic title="可用码牌" value={overviewQuery.data?.active_code_count ?? 0} />
            </Card>
          </Col>
          <Col span={8}>
            <Card size="small">
              <Statistic title="冻结金额" value={overviewQuery.data?.frozen ?? 0} formatter={(v) => formatCents(Number(v))} />
            </Card>
          </Col>
        </Row>
        <Descriptions column={1} bordered size="small" title="基本信息" style={{ marginTop: 8 }}>
          <Descriptions.Item label="商户号">{detail?.entity_code ?? '-'}</Descriptions.Item>
          <Descriptions.Item label="商户名称">{detail?.name ?? '-'}</Descriptions.Item>
          <Descriptions.Item label="主体类型">{TYPE_TEXT[detail?.entity_type ?? ''] ?? detail?.entity_type ?? '-'}</Descriptions.Item>
          <Descriptions.Item label="钱包号">
            <span style={{ fontFamily: 'monospace' }}>{detail?.wallet_no ?? '-'}</span>
          </Descriptions.Item>
          <Descriptions.Item label="状态">
            {detail?.status === 1 ? <Tag color="green">启用</Tag> : <Tag color="red">停用</Tag>}
          </Descriptions.Item>
          <Descriptions.Item label="创建时间">{detail?.created_at ? formatDateTime(detail.created_at) : '-'}</Descriptions.Item>
        </Descriptions>
        <Descriptions column={1} bordered size="small" title="商户身份认证资料" style={{ marginTop: 16 }}>
          <Descriptions.Item label="法人 / 经营者">{detail?.kyc_data?.legal_name ?? '-'}</Descriptions.Item>
          <Descriptions.Item label="证件号">{detail?.kyc_data?.license_no ?? '-'}</Descriptions.Item>
          <Descriptions.Item label="结算卡号">{detail?.kyc_data?.bank_account ?? '-'}</Descriptions.Item>
          <Descriptions.Item label="开户行">{detail?.kyc_data?.bank_name ?? '-'}</Descriptions.Item>
          <Descriptions.Item label="联系人">{detail?.kyc_data?.contact_name ?? '-'}</Descriptions.Item>
          <Descriptions.Item label="联系电话">{detail?.kyc_data?.contact_phone ?? '-'}</Descriptions.Item>
        </Descriptions>
        <Descriptions column={1} bordered size="small" title="微信支付配置" style={{ marginTop: 16 }}>
          <Descriptions.Item label="启用">
            {detail?.wechat_config ? (
              detail.wechat_config.enabled ? <Tag color="green">已启用</Tag> : <Tag color="default">未启用</Tag>
            ) : (
              '-'
            )}
          </Descriptions.Item>
          <Descriptions.Item label="商户号">{detail?.wechat_config?.mchid ?? '-'}</Descriptions.Item>
          <Descriptions.Item label="AppID">{detail?.wechat_config?.appid ?? '-'}</Descriptions.Item>
          <Descriptions.Item label="AppSecret">
            {detail?.wechat_config ? <ConfiguredTag configured={detail.wechat_config.app_secret_configured} /> : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="APIv3 密钥">
            {detail?.wechat_config ? <ConfiguredTag configured={detail.wechat_config.api_v3_key_configured} /> : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="商户 API 私钥">
            {detail?.wechat_config ? <ConfiguredTag configured={detail.wechat_config.merchant_private_key_configured} /> : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="微信平台公钥">
            {detail?.wechat_config ? <ConfiguredTag configured={detail.wechat_config.platform_public_key_configured} /> : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="回调地址前缀">{detail?.wechat_config?.notify_base_url ?? '-'}</Descriptions.Item>
        </Descriptions>
      </Drawer>
    </Card>
  );
};
