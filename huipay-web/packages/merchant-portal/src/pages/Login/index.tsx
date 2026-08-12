// 商户登录页（品牌渐变背景 + 强化卡片）
import React from 'react';
import { App as AntApp, Button, Card, Form, Input, Typography } from 'antd';
import { useMutation } from '@tanstack/react-query';
import { useLocation, useNavigate } from 'react-router-dom';
import { ShopOutlined, MobileOutlined, LockOutlined } from '@ant-design/icons';
import { merchantLogin, saveToken } from '../../services/auth';

const { Title, Text } = Typography;

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
    <div
      style={{
        minHeight: '100vh',
        background: 'linear-gradient(135deg, #0e1a2b 0%, #16294d 55%, #1e6fff 130%)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <Card
        style={{ width: 400, boxShadow: '0 20px 50px rgba(16,24,40,0.3)', border: 'none' }}
        styles={{ body: { padding: 32 } }}
      >
        <div style={{ textAlign: 'center', marginBottom: 28 }}>
          <span
            style={{
              width: 52,
              height: 52,
              borderRadius: 14,
              background: 'linear-gradient(135deg,#1e6fff,#06b6a4)',
              boxShadow: '0 8px 20px rgba(30,111,255,0.4)',
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#fff',
              fontSize: 26,
              marginBottom: 16,
            }}
          >
            <ShopOutlined />
          </span>
          <Title level={3} style={{ marginBottom: 4 }}>
            汇聚付 · 商家工作台
          </Title>
          <Text type="secondary">请使用管理员下发的商户账号登录</Text>
        </div>
        <Form layout="vertical" onFinish={(v) => mutation.mutate(v)} disabled={mutation.isPending}>
          <Form.Item name="phone" label="手机号" rules={[{ required: true, message: '请输入手机号' }]}>
            <Input prefix={<MobileOutlined style={{ color: 'rgba(0,0,0,0.3)' }} />} placeholder="登录手机号" size="large" autoFocus />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined style={{ color: 'rgba(0,0,0,0.3)' }} />} placeholder="登录密码" size="large" />
          </Form.Item>
          <Button type="primary" htmlType="submit" size="large" block loading={mutation.isPending} style={{ marginTop: 8 }}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  );
};