// H5 收银台（独立 URL 形态）。
// 真实部署：路由 /h5 → https://checkout.huipay.cn/h5
// URL 参数：order 必填（订单号）；amount/discount/merchant_id 可选。
import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { HuiPayCheckout } from '../components/Checkout';
import { useOrder } from '../hooks/useCheckout';
import { createApi } from '@huipay/shared/api-client';
import { ensureWechatOpenId, readOpenId } from '../utils/wechatOAuth';
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
  // 微信内且未授权：先跳转 OAuth 获取 openid，再回到本页
  if (ensureWechatOpenId(apiBase)) {
    return <div style={{ padding: 24 }}>微信授权中…</div>;
  }
  const openid = readOpenId();
  const { data: order, isLoading } = useOrder(orderNo, !!orderNo);

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
        <HuiPayCheckout orderNo={orderNo} channels={channels} amount={amount} discount={discount} openId={openid} />
      </div>
    </QueryClientProvider>
  );
}

ReactDOM.createRoot(document.getElementById('root')!).render(<App />);