// 商户登录页：左品牌展示 + 右登录表单（B2B 后台形态）
import React from 'react';
import { App as AntApp, Button, Form, Input, Typography } from 'antd';
import { useMutation } from '@tanstack/react-query';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  BankOutlined,
  LockOutlined,
  MobileOutlined,
  SafetyCertificateOutlined,
  ShopOutlined,
} from '@ant-design/icons';
import { merchantLogin, saveToken } from '../../services/auth';

const { Text } = Typography;

export const Login: React.FC = () => {
  const { message } = AntApp.useApp();
  const nav = useNavigate();
  const location = useLocation();
  const from = (location.state as { from?: { pathname?: string } } | null)?.from?.pathname ?? '/';

  const mutation = useMutation({
    mutationFn: ({ phone, password }: { phone: string; password: string }) => merchantLogin(phone, password),
    onSuccess: (res) => {
      saveToken(res.token);
      message.success('登录成功');
      nav(from, { replace: true });
    },
    onError: () => message.error('登录失败，请检查手机号或密码'),
  });

  return (
    <div className="hp-login">
      {/* 品牌展示区（桌面端） */}
      <div className="hp-login-brand">
        <div className="hp-login-glow hp-login-glow-a" />
        <div className="hp-login-glow hp-login-glow-b" />
        <div className="hp-login-brand-inner">
          <span className="hp-logo hp-logo-lg" aria-hidden>
            <ShopOutlined />
          </span>
          <div className="hp-login-brand-title">汇聚付</div>
          <div className="hp-login-brand-sub">商户收款 · 分账 · 资金管理一体化平台</div>
          <ul className="hp-login-trust">
            <li>
              <SafetyCertificateOutlined />
              <span>微信支付官方通道</span>
            </li>
            <li>
              <LockOutlined />
              <span>AES-256 敏感数据加密存储</span>
            </li>
            <li>
              <BankOutlined />
              <span>账户式资金与流水管理</span>
            </li>
          </ul>
          <span className="hp-login-version">商户端 v1.0.0</span>
        </div>
      </div>

      {/* 登录表单区 */}
      <div className="hp-login-form">
        <div className="hp-login-form-card">
          <div className="hp-login-form-head">
            <span className="hp-logo hp-logo-sm" aria-hidden>
              <ShopOutlined />
            </span>
            <h2>欢迎回来</h2>
            <p>请使用管理员下发的商户账号登录</p>
          </div>

          <Form layout="vertical" onFinish={(v) => mutation.mutate(v)} disabled={mutation.isPending} requiredMark={false}>
            <Form.Item
              name="phone"
              label="手机号"
              rules={[
                { required: true, message: '请输入手机号' },
                { pattern: /^1[3-9]\d{9}$/, message: '请输入正确的 11 位手机号' },
              ]}
            >
              <Input
                prefix={<MobileOutlined />}
                placeholder="登录手机号"
                size="large"
                autoFocus
                maxLength={11}
                allowClear
              />
            </Form.Item>
            <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}>
              <Input.Password prefix={<LockOutlined />} placeholder="登录密码" size="large" />
            </Form.Item>

            <div className="hp-login-extra">
              <Text type="secondary">账号由平台管理员开通，如遗忘密码请联系管理员</Text>
            </div>

            <Button
              className="hp-login-submit"
              type="primary"
              htmlType="submit"
              size="large"
              block
              loading={mutation.isPending}
            >
              登录
            </Button>
          </Form>
        </div>
        <div className="hp-login-foot">© 2026 汇聚付 HuiPay · 商户端</div>
      </div>
    </div>
  );
};
