// 收银台组件式集成入口：组件式（Component）形态
import React from 'react';
import { Alert, Button, Radio, Space, Typography, Divider, QRCode } from 'antd';
import { useCheckoutUI } from '../hooks/useCheckoutUI';
import { useCheckout } from '../hooks/useCheckout';
import { usePay } from '../hooks/usePay';
import { invokeWechatPay } from '../utils/wechatPay';
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
    showChannelSelector = true,
  } = props;
  const { order, isPaid, channelPaid, finalAmount } = useCheckout(props);
  const { selectedChannel, setSelectedChannel, selectedPayType, setSelectedPayType, isProcessing, setProcessing } =
    useCheckoutUI();
  const payMutation = usePay();
  const [qrCode, setQrCode] = React.useState('');
  const [payError, setPayError] = React.useState('');
  const [now, setNow] = React.useState(Date.now());

  // 支付中倒计时（每秒刷新，用于展示剩余支付时间）
  React.useEffect(() => {
    if (isPaid || !order?.expire_at) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [isPaid, order?.expire_at]);

  const remainSecs = order?.expire_at ? Math.max(0, Math.ceil((new Date(order.expire_at).getTime() - now) / 1000)) : 0;

  // 未选通道时自动选中第一个可用通道（码牌扫码收款默认即可支付，无需手动选择）
  React.useEffect(() => {
    if (!selectedChannel && channels.length > 0) {
      const first = channels.find((c) => c.available) ?? channels[0];
      if (first) setSelectedChannel(first.code);
    }
  }, [selectedChannel, channels, setSelectedChannel]);

  React.useEffect(() => {
    if (isPaid) {
      setQrCode('');
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
    setPayError('');
    payMutation.mutate(
      { orderNo, payType: selectedPayType, openId },
      {
        onSuccess: (resp) => {
          if (resp.pay_url) {
            window.location.href = resp.pay_url; // H5 跳转
          } else if (resp.jsapi) {
            // JSAPI：用后端签名的调起参数拉起微信支付
            invokeWechatPay(
              resp.jsapi,
              () => onSuccess?.({ orderNo, channel: resp.channel }),
              (e) => onError?.(e),
            );
          } else if (resp.prepay_id) {
            onJSAPIReady?.(resp.prepay_id); // 兼容：外部自行拉起
          } else if (resp.qr_code) {
            setQrCode(resp.qr_code); // NATIVE：展示二维码，由轮询命中 PAID 触发成功
          }
        },
        onError: (err) => {
          setPayError(err.message);
          onError?.(err);
        },
        onSettled: () => setProcessing(false),
      },
    );
  };

  return (
    <div className="huipay-checkout" style={{ padding: 16, maxWidth: 480 }}>
      <Title level={4} style={{ marginTop: 0 }}>
        订单 {orderNo}
      </Title>
      {!isPaid && !payError && remainSecs > 0 && (
        <div style={{ textAlign: 'right', color: remainSecs <= 60 ? '#fa541c' : '#8a94a6', marginBottom: 8 }}>
          剩余支付时间 {Math.floor(remainSecs / 60)} 分 {remainSecs % 60} 秒
        </div>
      )}
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
        {payError && (
          <Alert type="error" showIcon message={payError} closable onClose={() => setPayError('')} />
        )}
        {order?.status === 'CLOSED' && !isPaid && (
          <Alert type="warning" showIcon message="订单已关闭或超时，请重新创建订单" />
        )}
        {channelPaid && !isPaid && (
          <Alert type="info" showIcon message="支付处理中，请稍候…（已收到支付结果，正在确认到账）" />
        )}
        {qrCode && !isPaid && (
          <div style={{ textAlign: 'center', padding: 8 }}>
            <QRCode value={qrCode} size={220} />
            <div style={{ marginTop: 8 }}>
              <Text type="secondary">请使用微信扫码支付</Text>
            </div>
          </div>
        )}
        {showChannelSelector && (
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
        )}
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
