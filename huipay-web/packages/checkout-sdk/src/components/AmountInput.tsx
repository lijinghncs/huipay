// 码牌建单金额输入组件：消费者扫码牌后输入支付金额。
import React from 'react';
import { Button, InputNumber, Typography } from 'antd';
import { useMutation } from '@tanstack/react-query';
import { post } from '@huipay/shared/api-client';
import type { PrecreateResponse } from '../types';

const { Title, Text } = Typography;

/** 码牌建单：POST /v1/checkout/precreate-by-code。 */
function usePrecreateByCode() {
  return useMutation<PrecreateResponse, Error, { code: string; amount: number }>({
    mutationFn: ({ code, amount }) =>
      post<PrecreateResponse>('/v1/checkout/precreate-by-code', { code_id: code, amount }),
  });
}

/** 金额输入（单位：元，仅码牌模式展示）。 */
export const AmountInput: React.FC<{ code: string; onCreated: (orderNo: string) => void }> = ({ code, onCreated }) => {
  const [yuan, setYuan] = React.useState<number | null>(null);
  const mutation = usePrecreateByCode();
  const cents = Math.round((yuan ?? 0) * 100);

  const handleSubmit = () => {
    if (!yuan || cents < 1 || cents > 5_000_000) {
      mutation.reset();
      return;
    }
    mutation.mutate(
      { code, amount: cents },
      {
        onSuccess: (resp) => onCreated(resp.order_no),
        onError: () => undefined,
      },
    );
  };

  return (
    <div style={{ padding: 16, maxWidth: 480 }}>
      <Title level={4} style={{ marginTop: 0 }}>
        扫码收款
      </Title>
      <Text type="secondary">请输入支付金额（元）</Text>
      <div style={{ margin: '16px 0' }}>
        <InputNumber
          style={{ width: '100%', fontSize: 20 }}
          min={0.01}
          max={50000}
          step={0.01}
          precision={2}
          value={yuan}
          onChange={(v) => setYuan(v)}
          placeholder="0.00"
          prefix="¥"
          autoFocus
        />
      </div>
      <Button
        type="primary"
        size="large"
        block
        loading={mutation.isPending}
        disabled={!yuan || cents < 1 || cents > 5_000_000}
        onClick={handleSubmit}
      >
        确认支付
      </Button>
      {mutation.isError && (
        <Text type="danger" style={{ display: 'block', marginTop: 8 }}>
          {(mutation.error as Error)?.message ?? '创建订单失败，请重试'}
        </Text>
      )}
    </div>
  );
};