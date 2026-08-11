// 支付结果页：成功 / 已关闭（超时）/ 失败
import React from 'react';
import { Button, Result, Typography } from 'antd';
import { formatCents, formatDateTime } from '@huipay/shared/utils';

const { Text } = Typography;

export interface PaymentResultProps {
  type: 'success' | 'closed' | 'failed';
  orderNo?: string;
  amount?: number;
  paidAt?: string;
  message?: string;
  onRetry?: () => void;
  retryText?: string;
}

/** 收银台结果视图（H5/Embed 共用）。 */
export const PaymentResult: React.FC<PaymentResultProps> = ({
  type,
  orderNo,
  amount,
  paidAt,
  message,
  onRetry,
  retryText = '重新支付',
}) => {
  if (type === 'success') {
    return (
      <Result
        status="success"
        title="支付成功"
        subTitle={
          <div style={{ marginTop: 8 }}>
            {amount != null && (
              <div style={{ fontSize: 22, fontWeight: 700, color: '#06b6a4' }}>{formatCents(amount)}</div>
            )}
            <div style={{ color: '#8a94a6', marginTop: 8 }}>
              {orderNo && <span>订单号：{orderNo}</span>}
              {paidAt && <div>支付时间：{formatDateTime(paidAt)}</div>}
            </div>
          </div>
        }
      />
    );
  }

  if (type === 'closed') {
    return (
      <Result
        status="warning"
        title="订单已关闭"
        subTitle={
          <div>
            <Text type="secondary">该订单已超时或已关闭，请返回重新创建订单。</Text>
            {orderNo && (
              <div style={{ color: '#8a94a6', marginTop: 8 }}>订单号：{orderNo}</div>
            )}
          </div>
        }
        extra={
          onRetry && (
            <Button type="primary" onClick={onRetry}>
              {retryText}
            </Button>
          )
        }
      />
    );
  }

  return (
    <Result
      status="error"
      title="支付失败"
      subTitle={message || '支付未完成，请重试。'}
      extra={
        onRetry && (
          <Button type="primary" onClick={onRetry}>
            {retryText}
          </Button>
        )
      }
    />
  );
};
