// 收银台嵌入式（Embed）形态入口。
// 在商家页面通过 <iframe src="https://checkout.huipay.cn/embed?token=xxx"> 加载。
// 本组件为商家端的"加载器"封装，便于在 React 项目里直接使用。
import React, { useEffect, useRef } from 'react';

export interface HuiPayEmbeddedProps {
  /** SDK token（后端签发的短时 token） */
  token: string;
  /** 嵌入地址 */
  src?: string;
  /** iframe 高度 */
  height?: number | string;
  /** 接收来自 iframe 的 postMessage（如支付成功） */
  onMessage?: (msg: { type: string; payload?: unknown }) => void;
}

/** React 端嵌入式加载组件。 */
export const HuiPayEmbedded: React.FC<HuiPayEmbeddedProps> = ({
  token,
  src = 'https://checkout.huipay.cn/embed',
  height = 640,
  onMessage,
}) => {
  const ref = useRef<HTMLIFrameElement>(null);

  useEffect(() => {
    const handler = (e: MessageEvent) => {
      // 仅接受 checkout.huipay.cn 的消息
      if (!String(e.origin).endsWith('huipay.cn')) return;
      onMessage?.(e.data as { type: string; payload?: unknown });
    };
    window.addEventListener('message', handler);
    return () => window.removeEventListener('message', handler);
  }, [onMessage]);

  return (
    <iframe
      ref={ref}
      title="HuiPay Checkout"
      src={`${src}?token=${encodeURIComponent(token)}`}
      style={{ width: '100%', height, border: 0 }}
      allow="payment"
    />
  );
};