// 商户登录页
import React from 'react';
import { App as AntApp, Button, Card, Form, Input, Typography } from 'antd';
import { useMutation } from '@tanstack/react-query';
import { useLocation, useNavigate } from 'react-router-dom';
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
        background: '#f6f7fb',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <Card style={{ width: 380 }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Title level={3} style={{ marginBottom: 4 }}>
            汇聚付 · 商家工作台
          </Title>
          <Text type="secondary">请使用管理员下发的商户账号登录</Text>
        </div>
        <Form
          layout="vertical"
          onFinish={(v) => mutation.mutate(v)}
          disabled={mutation.isPending}
        >
          <Form.Item name="phone" label="手机号" rules={[{ required: true, message: '请输入手机号' }]}>
            <Input placeholder="登录手机号" size="large" autoFocus />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password placeholder="登录密码" size="large" />
          </Form.Item>
          <Button type="primary" htmlType="submit" size="large" block loading={mutation.isPending}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  );
};
