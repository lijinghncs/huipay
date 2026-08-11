// 商户微信支付配置子页：非敏感字段回填，敏感字段留空 = 不修改（加密存储，不回显明文）
import React from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Form, Button, Space, Alert, Skeleton, Result, Tag, App } from 'antd';
import { ArrowLeftOutlined, SaveOutlined } from '@ant-design/icons';
import { useNavigate, useParams } from 'react-router-dom';
import {
  getMerchant,
  getMerchantWechatConfig,
  updateMerchantWechatConfig,
  type MerchantWechatConfigInput,
} from '../../services/admin';
import { WechatConfigFormFields, ConfiguredTag, Mono } from './shared';

export const MerchantWechatConfigPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const merchantId = Number(id);
  const nav = useNavigate();
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm();

  const merchantQuery = useQuery({
    queryKey: ['merchant-detail', merchantId],
    queryFn: () => getMerchant(merchantId),
    enabled: !!merchantId,
  });
  const wcQuery = useQuery({
    queryKey: ['merchant-wechat', merchantId],
    queryFn: () => getMerchantWechatConfig(merchantId),
    enabled: !!merchantId,
  });

  // 非敏感字段回填；敏感字段不预填明文（留空 = 不修改）
  React.useEffect(() => {
    if (wcQuery.data) {
      form.setFieldsValue({
        wechat_config: {
          enabled: wcQuery.data.enabled ?? false,
          mchid: wcQuery.data.mchid ?? '',
          appid: wcQuery.data.appid ?? '',
          merchant_serial_no: wcQuery.data.merchant_serial_no ?? '',
          platform_serial_no: wcQuery.data.platform_serial_no ?? '',
          notify_base_url: wcQuery.data.notify_base_url ?? '',
        },
      });
    }
  }, [wcQuery.data, form]);

  const save = useMutation({
    mutationFn: (cfg: MerchantWechatConfigInput) => updateMerchantWechatConfig(merchantId, cfg),
    onSuccess: () => {
      message.success('微信支付配置已保存');
      queryClient.invalidateQueries({ queryKey: ['merchant-detail', merchantId] });
      queryClient.invalidateQueries({ queryKey: ['merchant-wechat', merchantId] });
      nav(`/merchants/${merchantId}`);
    },
    onError: (err) => message.error(`保存失败：${err.message}`),
  });

  if (merchantQuery.isLoading || wcQuery.isLoading) {
    return <Card><Skeleton active paragraph={{ rows: 8 }} /></Card>;
  }
  if (merchantQuery.isError || !merchantQuery.data) {
    return (
      <Result
        status="404"
        title="商户不存在"
        extra={
          <Button type="primary" icon={<ArrowLeftOutlined />} onClick={() => nav('/merchants')}>
            返回商户列表
          </Button>
        }
      />
    );
  }

  const m = merchantQuery.data;
  const view = wcQuery.data;

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {/* 页头 */}
      <Card>
        <Space style={{ display: 'flex', justifyContent: 'space-between', width: '100%' }} align="center">
          <Space size={12} align="center">
            <Button icon={<ArrowLeftOutlined />} onClick={() => nav(`/merchants/${merchantId}`)}>
              返回详情
            </Button>
            <span style={{ fontSize: 18, fontWeight: 700 }}>微信支付配置</span>
            <Tag style={{ borderRadius: 10 }} color="blue">{m.name}</Tag>
            <Mono>{m.entity_code}</Mono>
          </Space>
          <Button type="primary" icon={<SaveOutlined />} loading={save.isPending} onClick={() => form.submit()}>
            保存配置
          </Button>
        </Space>
      </Card>

      <Card title="配置说明" size="small">
        <Alert
          type="info"
          showIcon
          description={
            <Space direction="vertical" size={4}>
              <span>敏感字段（AppSecret / APIv3 密钥 / 商户 API 私钥 / 微信平台公钥）已 AES 加密存储，仅显示「已配置 / 未配置」状态，不回显明文。</span>
              <span>编辑时敏感字段留空 = 不修改；非敏感字段留空 = 清空。</span>
              <Space size={16} style={{ marginTop: 4 }}>
                <span>AppSecret <ConfiguredTag configured={view?.app_secret_configured} /></span>
                <span>APIv3 密钥 <ConfiguredTag configured={view?.api_v3_key_configured} /></span>
                <span>商户 API 私钥 <ConfiguredTag configured={view?.merchant_private_key_configured} /></span>
                <span>微信平台公钥 <ConfiguredTag configured={view?.platform_public_key_configured} /></span>
              </Space>
            </Space>
          }
        />
      </Card>

      <Card title="配置项" size="small">
        <Form
          form={form}
          layout="vertical"
          style={{ maxWidth: 720 }}
          onFinish={(values: { wechat_config: MerchantWechatConfigInput }) => save.mutate(values.wechat_config)}
        >
          <WechatConfigFormFields edit />
        </Form>
      </Card>
    </Space>
  );
};
