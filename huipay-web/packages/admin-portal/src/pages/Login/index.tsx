// 管理后台登录页
import React from 'react';
import { App as AntApp, Button, Form, Input, Typography } from 'antd';
import { useMutation } from '@tanstack/react-query';
import { useLocation, useNavigate } from 'react-router-dom';
import { LockOutlined, SafetyCertificateOutlined, UserOutlined } from '@ant-design/icons';
import { adminLogin, saveToken } from '../../services/auth';

const { Text } = Typography;

export const Login: React.FC = () => {
  const { message } = AntApp.useApp();
  const nav = useNavigate();
  const location = useLocation();
  const from = (location.state as { from?: { pathname?: string } } | null)?.from?.pathname ?? '/';

  const mutation = useMutation({
    mutationFn: ({ username, password }: { username: string; password: string }) => adminLogin(username, password),
    onSuccess: (res) => {
      saveToken(res.token);
      message.success('登录成功');
      nav(from, { replace: true });
    },
    onError: () => message.error('登录失败，请检查账号或密码'),
  });

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'linear-gradient(135deg, #0e1a33 0%, #152642 100%)',
        padding: 24,
      }}
    >
      <div
        style={{
          width: 400,
          maxWidth: '100%',
          background: '#fff',
          borderRadius: 12,
          padding: '36px 32px',
          boxShadow: '0 12px 40px rgba(14,26,51,0.35)',
        }}
      >
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <span
            style={{
              display: 'inline-flex',
              width: 44,
              height: 44,
              borderRadius: 12,
              background: 'linear-gradient(135deg,#1e6fff,#06b6a4)',
              color: '#fff',
              alignItems: 'center',
              justifyContent: 'center',
              fontWeight: 800,
              fontSize: 20,
              marginBottom: 12,
            }}
          >
            汇
          </span>
          <h2 style={{ margin: 0, fontSize: 20, fontWeight: 700, color: '#0e1a33' }}>汇聚付 · 管理后台</h2>
          <p style={{ margin: '6px 0 0', color: '#66718b', fontSize: 13 }}>请使用平台管理员账号登录</p>
        </div>

        <Form layout="vertical" onFinish={(v) => mutation.mutate(v)} disabled={mutation.isPending} requiredMark={false}>
          <Form.Item name="username" label="账号" rules={[{ required: true, message: '请输入账号' }]}>
            <Input prefix={<UserOutlined />} placeholder="管理员账号" size="large" autoFocus allowClear />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="登录密码" size="large" />
          </Form.Item>

          <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 16, color: '#66718b', fontSize: 12 }}>
            <SafetyCertificateOutlined />
            <Text type="secondary">账号由运维配置，请妥善保管</Text>
          </div>

          <Button type="primary" htmlType="submit" size="large" block loading={mutation.isPending}>
            登录
          </Button>
        </Form>

        <div style={{ textAlign: 'center', marginTop: 20, color: '#a0a6b3', fontSize: 12 }}>
          © 2026 汇聚付 HuiPay · 管理后台
        </div>
      </div>
    </div>
  );
};