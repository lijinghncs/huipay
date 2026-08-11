// H5 收银台（独立 URL 形态）。
// 真实部署：路由 /h5 → https://checkout.huipay.cn/h5
// URL 参数：order 必填（订单号）；code 可选（收款码牌短码，无 order 时用于码牌建单）；
// amount/discount/merchant_id 可选。
import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { HuiPayCheckout } from '../components/Checkout';
import { AmountInput } from '../components/AmountInput';
import { PaymentResult } from '../components/PaymentResult';
import { useOrder } from '../hooks/useCheckout';
import { createApi } from '@huipay/shared/api-client';
import { ensureWechatOpenId, isWeixinBrowser, readOpenId } from '../utils/wechatOAuth';
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

  // 微信内且未授权：先跳转 OAuth 获取 openid，再回到本页
  if (ensureWechatOpenId(apiBase)) {
    return <div style={{ padding: 24 }}>微信授权中…</div>;
  }
  const openid = readOpenId();

  // 码牌模式：无 order 但有 code → 先输入金额建单
  if (!orderNo && code) {
    return (
      <QueryClientProvider client={queryClient}>
        <div style={{ minHeight: '100vh', background: '#f6f7fb', display: 'flex', justifyContent: 'center' }}>
          <AmountInput code={code} onCreated={setCreatedOrder} />
        </div>
      </QueryClientProvider>
    );
  }

  const activeOrderNo = orderNo || createdOrder;
  const { data: order, isLoading } = useOrder(activeOrderNo, !!activeOrderNo);

  // 支付结果页：成功 / 已关闭
  if (order?.status === 'PAID') {
    return (
      <QueryClientProvider client={queryClient}>
        <PaymentResult
          type="success"
          orderNo={order.order_no}
          amount={order.paid_amount || order.amount}
          paidAt={order.paid_at}
        />
      </QueryClientProvider>
    );
  }
  if (order?.status === 'CLOSED') {
    return (
      <QueryClientProvider client={queryClient}>
        <PaymentResult
          type="closed"
          orderNo={order.order_no}
          onRetry={code ? () => (window.location.href = `/h5?code=${code}`) : undefined}
          retryText="重新扫码收款"
        />
      </QueryClientProvider>
    );
  }

  // 支付失败：展示失败页，重试回到收银台
  if (payError) {
    return (
      <QueryClientProvider client={queryClient}>
        <PaymentResult type="failed" message={payError} onRetry={() => setPayError('')} />
      </QueryClientProvider>
    );
  }

  // 若订单已带 pay_url 且为 H5 场景，自动跳转微信支付
  React.useEffect(() => {
    const payUrl = (order as unknown as { pay_url?: string })?.pay_url;
    if (payUrl) {
      window.location.href = payUrl;
    }
  }, [order]);

  if (isLoading) {
    return <div style={{ padding: 24 }}>加载中…</div>;
  }

  const amount = order?.amount ?? Number(params.get('amount') ?? 0);
  const discount = Number(params.get('discount') ?? 0);
  const channels = [
    { code: 'WECHAT', fee_rate: '0.60%', available: true },
    { code: 'ALIPAY', fee_rate: '0.55%', available: true },
  ];

  return (
    <QueryClientProvider client={queryClient}>
      <div style={{ minHeight: '100vh', background: '#f6f7fb' }}>
        <HuiPayCheckout
          orderNo={activeOrderNo}
          channels={channels}
          amount={amount}
          discount={discount}
          openId={openid}
          defaultPayType={code ? (isWeixinBrowser() ? 'JSAPI' : 'H5') : 'NATIVE'}
          onError={(e) => setPayError(e.message)}
        />
      </div>
    </QueryClientProvider>
  );
}

ReactDOM.createRoot(document.getElementById('root')!).render(<App />);
