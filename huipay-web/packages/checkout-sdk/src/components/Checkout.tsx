// 收银台组件式集成入口：组件式（Component）形态
import React from 'react';
import { Button, Radio, Space, Typography, Divider } from 'antd';
import { useCheckoutUI } from '../hooks/useCheckoutUI';
import { useCheckout } from '../hooks/useCheckout';
import { formatCents } from '@huipay/shared/utils';
import type { CheckoutProps } from '../types';

const { Text, Title } = Typography;

/** 收银台组件（Component 形态）。 */
export const HuiPayCheckout: React.FC<CheckoutProps> = (props) => {
  const { orderNo, channels, onChannelChange, onSuccess, onError, amount, discount = 0 } = props;
  const { order, isPaid, finalAmount } = useCheckout(props);
  const { selectedChannel, setSelectedChannel, isProcessing, setProcessing } = useCheckoutUI();

  React.useEffect(() => {
    if (isPaid) {
      onSuccess?.({ orderNo, channel: order?.channel ?? '' });
    }
  }, [isPaid, orderNo, order?.channel, onSuccess]);

  React.useEffect(() => {
    if (selectedChannel) onChannelChange?.(selectedChannel);
  }, [selectedChannel, onChannelChange]);

  const handlePay = async () => {
    if (!selectedChannel) {
      onError?.(new Error('请选择支付通道'));
      return;
    }
    setProcessing(true);
    try {
      // 真实项目：调用 channel.CreatePayment，跳转到支付 URL / 拉起 SDK
      // 骨架阶段：仅模拟
      await new Promise((r) => setTimeout(r, 800));
    } catch (e) {
      onError?.(e as Error);
    } finally {
      setProcessing(false);
    }
  };

  return (
    <div className="huipay-checkout" style={{ padding: 16, maxWidth: 480 }}>
      <Title level={4} style={{ marginTop: 0 }}>
        订单 {orderNo}
      </Title>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <div>
          <Text type="secondary">订单金额</Text>
          <div style={{ fontSize: 28, fontWeight: 700 }}>{formatCents(amount)}</div>
        </div>
        {discount > 0 && (
          <div>
            <Text type="secondary">优惠</Text>
            <div style={{ fontSize: 16, color: '#06b6a4' }}>- {formatCents(discount)}</div>
          </div>
        )}
        <Divider style={{ margin: '8px 0' }} />
        <div>
          <Text strong>选择支付方式</Text>
          <Radio.Group
            value={selectedChannel}
            onChange={(e) => setSelectedChannel(e.target.value)}
            style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 8 }}
          >
            {channels.map((c) => (
              <Radio key={c.code} value={c.code} disabled={!c.available}>
                {c.code}（费率 {c.fee_rate}）
              </Radio>
            ))}
          </Radio.Group>
        </div>
        <Button
          type="primary"
          size="large"
          block
          loading={isProcessing}
          disabled={!selectedChannel || isPaid}
          onClick={handlePay}
        >
          {isPaid ? '已支付' : `支付 ${formatCents(finalAmount)}`}
        </Button>
      </Space>
    </div>
  );
};