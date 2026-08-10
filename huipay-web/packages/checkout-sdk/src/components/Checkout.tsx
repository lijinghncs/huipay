// 收银台组件式集成入口：组件式（Component）形态
import React from 'react';
import { Button, Radio, Space, Typography, Divider } from 'antd';
import { useCheckoutUI } from '../hooks/useCheckoutUI';
import { useCheckout } from '../hooks/useCheckout';
import { usePay } from '../hooks/usePay';
import { formatCents } from '@huipay/shared/utils';
import type { CheckoutProps, PayType } from '../types';

const { Text, Title } = Typography;

const PAY_TYPES: { value: PayType; label: string }[] = [
  { value: 'NATIVE', label: '扫码支付' },
  { value: 'H5', label: 'H5 网页支付' },
  { value: 'JSAPI', label: '微信内拉起' },
];

/** 收银台组件（Component 形态）。 */
export const HuiPayCheckout: React.FC<CheckoutProps> = (props) => {
  const {
    orderNo,
    channels,
    onChannelChange,
    onPayTypeChange,
    onSuccess,
    onError,
    onJSAPIReady,
    amount,
    discount = 0,
    openId,
    defaultPayType = 'NATIVE',
    showPayTypeSelector = true,
  } = props;
  const { order, isPaid, finalAmount } = useCheckout(props);
  const { selectedChannel, setSelectedChannel, selectedPayType, setSelectedPayType, isProcessing, setProcessing } =
    useCheckoutUI();
  const payMutation = usePay();

  React.useEffect(() => {
    if (isPaid) {
      onSuccess?.({ orderNo, channel: order?.channel ?? '' });
    }
  }, [isPaid, orderNo, order?.channel, onSuccess]);

  React.useEffect(() => {
    if (selectedChannel) onChannelChange?.(selectedChannel);
  }, [selectedChannel, onChannelChange]);

  React.useEffect(() => {
    onPayTypeChange?.(selectedPayType);
  }, [selectedPayType, onPayTypeChange]);

  const handlePay = async () => {
    if (!selectedChannel) {
      onError?.(new Error('请选择支付通道'));
      return;
    }
    setProcessing(true);
    payMutation.mutate(
      { orderNo, payType: selectedPayType, openId },
      {
        onSuccess: (resp) => {
          if (resp.pay_url) {
            window.location.href = resp.pay_url; // H5 跳转
          } else if (resp.prepay_id) {
            onJSAPIReady?.(resp.prepay_id); // JSAPI 拉起（前端负责 WeixinJSBridge）
          }
          // qr_code 由 useOrder 轮询触发 onSuccess
        },
        onError: (err) => onError?.(err),
        onSettled: () => setProcessing(false),
      },
    );
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
        {showPayTypeSelector && (
          <div>
            <Text strong>选择支付场景</Text>
            <Radio.Group
              value={selectedPayType}
              onChange={(e) => setSelectedPayType(e.target.value as PayType)}
              defaultValue={defaultPayType}
              style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 8 }}
            >
              {PAY_TYPES.map((p) => {
                // JSAPI 单选项仅在提供了 openId 时显示
                const disabled = p.value === 'JSAPI' && !openId;
                return (
                  <Radio key={p.value} value={p.value} disabled={disabled}>
                    {p.label}
                    {p.value === 'JSAPI' && !openId && '（需 openId）'}
                  </Radio>
                );
              })}
            </Radio.Group>
          </div>
        )}
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