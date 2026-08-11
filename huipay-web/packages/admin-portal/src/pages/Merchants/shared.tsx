// 商户模块共享 UI 组件与常量（列表 / 详情 / 微信配置子页共用）
import React from 'react';
import { Tag, Switch, Input, Form, Collapse } from 'antd';

export const TYPE_TEXT: Record<string, string> = {
  MERCHANT: '企业',
  STORE: '门店',
  PROMOTER: '推广员',
  PLATFORM: '平台',
  ISV: '服务商',
};

export function typeText(t: string): string {
  return TYPE_TEXT[t] ?? t;
}

/** 商户状态徽标：启用 / 停用（带状态圆点） */
export const MerchantStatusTag: React.FC<{ status: number }> = ({ status }) =>
  status === 1 ? (
    <Tag color="success" style={{ borderRadius: 10 }}>启用</Tag>
  ) : (
    <Tag color="error" style={{ borderRadius: 10 }}>停用</Tag>
  );

/** 微信支付启用状态徽标 */
export const WechatEnabledTag: React.FC<{ enabled?: boolean }> = ({ enabled }) =>
  enabled ? (
    <Tag color="success" style={{ borderRadius: 10 }}>已启用</Tag>
  ) : (
    <Tag color="default" style={{ borderRadius: 10 }}>未启用</Tag>
  );

/** 敏感字段 configured 标记（已配置 / 未配置） */
export const ConfiguredTag: React.FC<{ configured?: boolean }> = ({ configured }) =>
  configured ? (
    <Tag color="processing" style={{ borderRadius: 10 }}>已配置</Tag>
  ) : (
    <Tag color="default" style={{ borderRadius: 10 }}>未配置</Tag>
  );

/** 等宽文本（商户号 / 钱包号） */
export const Mono: React.FC<{ children?: React.ReactNode }> = ({ children }) => (
  <span style={{ fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace', fontSize: 13 }}>
    {children}
  </span>
);

// 微信支付配置表单字段（进件 / 微信配置子页共用）。
// edit=true 时敏感字段留空 = 不修改（不覆盖后端既有密文）。
export const WechatConfigFormFields: React.FC<{ edit?: boolean }> = ({ edit }) => {
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
