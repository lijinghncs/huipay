// H5 收银台（独立 URL 形态）。
// 真实部署：路由 /h5 → https://checkout.huipay.cn/h5
// URL 参数：order 必填（订单号）；code 可选（收款码牌短码，无 order 时用于码牌建单）；
// amount/discount/merchant_id 可选。
//
// 码牌流程：输入金额 → 确认支付 → 建单 + 发支付 + 拉起微信支付窗口 → 轮询订单状态 → 结果页。
import React from 'react';
import ReactDOM from 'react-dom/client';
import { App as AntApp } from 'antd';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AmountInput } from '../components/AmountInput';
import { PaymentResult } from '../components/PaymentResult';
import { useOrder } from '../hooks/useCheckout';
import { createApi } from '@huipay/shared/api-client';
import { ensureWechatOpenId, resolveOpenId } from '../utils/wechatOAuth';
import '../styles/global.css';

const queryClient = new QueryClient();
const params = new URLSearchParams(window.location.search);
const apiBase = import.meta.env.VITE_API_BASE ?? 'https://api.huipay.cn';
createApi({
  baseURL: apiBase,
  merchantIdProvider: () => Number(params.get('merchant_id') ?? 0),
});

function App() {
  const orderNo = params.get('order') ?? '';
  const code = params.get('code') ?? '';
  const [createdOrder, setCreatedOrder] = React.useState('');
  const [payError, setPayError] = React.useState('');

  // 注意：所有 Hooks 必须在任何条件 return 之前调用，保证 Hook 顺序稳定。
  const activeOrderNo = orderNo || createdOrder;
  const { data: order, isLoading } = useOrder(activeOrderNo, !!activeOrderNo);

  // 微信内且未授权：先跳转 OAuth 获取 openid，再回到本页
  if (ensureWechatOpenId(apiBase)) {
    return <div style={{ padding: 24 }}>微信授权中…</div>;
  }
  const openid = resolveOpenId();

  // 支付失败：展示失败页，重试回到收银台（优先于码牌/支付分支判断）
  if (payError) {
    return (
      <PaymentResult
        type="failed"
        message={payError}
        onRetry={() => setPayError('')}
        retryText={code ? '重新收款' : '重新支付'}
      />
    );
  }

  // 码牌模式：无 order 且有 code 时先输入金额建单；建单即拉起支付，成功后进入结果页
  if (!orderNo && code && !createdOrder) {
    return (
      <div style={{ minHeight: '100vh', display: 'flex', justifyContent: 'center' }}>
        <AmountInput
          code={code}
          openId={openid}
          onCreated={setCreatedOrder}
          onError={setPayError}
        />
      </div>
    );
  }

  // 支付结果页：成功 / 已关闭
  if (order?.status === 'PAID') {
    return (
      <PaymentResult
        type="success"
        orderNo={order.order_no}
        amount={order.paid_amount || order.amount}
        paidAt={order.paid_at}
      />
    );
  }
  if (order?.status === 'CLOSED') {
    return (
      <PaymentResult
        type="closed"
        orderNo={order.order_no}
        onRetry={code ? () => (window.location.href = `/h5?code=${code}`) : undefined}
        retryText="重新扫码收款"
      />
    );
  }

  if (isLoading) {
    return <div style={{ padding: 24 }}>加载中…</div>;
  }

  // 码牌模式：已建单、正在等待微信支付结果
  if (code && createdOrder) {
    return (
      <div style={{ minHeight: '100vh', background: '#f6f7fb', display: 'flex', justifyContent: 'center' }}>
        <div style={{ padding: '30vh 24px', textAlign: 'center' }}>
          <div
            style={{
              width: 40,
              height: 40,
              margin: '0 auto 16px',
              border: '3px solid rgba(17,107,85,0.2)',
              borderTopColor: '#116b55',
              borderRadius: '50%',
              animation: 'hp-spin 0.8s linear infinite',
            }}
          />
          <div style={{ fontSize: 16, color: '#1f2a24' }}>请在微信支付窗口中完成支付…</div>
          <div style={{ fontSize: 13, color: '#5b6b62', marginTop: 8 }}>订单号 {activeOrderNo}</div>
        </div>
      </div>
    );
  }

  // 常规订单支付（有 order 参数）
  return <div style={{ minHeight: '100vh', background: '#f6f7fb' }} />;
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <QueryClientProvider client={queryClient}>
    <AntApp>
      <App />
    </AntApp>
  </QueryClientProvider>,
);