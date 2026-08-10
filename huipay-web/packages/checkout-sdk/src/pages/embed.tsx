// 嵌入式页面（iframe 内容）。
// 真实部署：路由 /embed → https://checkout.huipay.cn/embed
// URL 参数：order 必填（订单号）；merchant_id 可选。
import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query';
import { HuiPayCheckout } from '../components/Checkout';
import { post } from '@huipay/shared/api-client';
import { createApi } from '@huipay/shared/api-client';
import '../styles/global.css';

const queryClient = new QueryClient();
const params = new URLSearchParams(window.location.search);
createApi({
  baseURL: import.meta.env.VITE_API_BASE ?? 'https://api.huipay.cn',
  merchantIdProvider: () => Number(params.get('merchant_id') ?? 0),
});

interface EmbedInfo {
  order_no: string;
  channels: { code: string; fee_rate: string; available: boolean }[];
  amount: number;
  discount: number;
}

function useEmbedInfo(order: string) {
  return useQuery<EmbedInfo>({
    queryKey: ['embed-info', order],
    queryFn: () => post<EmbedInfo>('/v1/checkout/embed-info', { order_no: order }),
    enabled: !!order,
  });
}

function App() {
  const order = params.get('order') ?? '';
  const { data: info, isLoading } = useEmbedInfo(order);

  if (isLoading || !info) {
    return <div style={{ padding: 24 }}>加载中…</div>;
  }
  return (
    <div
      style={{
        minHeight: '100vh',
        background: '#f6f7fb',
        display: 'flex',
        justifyContent: 'center',
        paddingTop: 32,
      }}
    >
      <HuiPayCheckout
        orderNo={info.order_no}
        channels={info.channels}
        amount={info.amount}
        discount={info.discount}
        onSuccess={(r) => window.parent.postMessage({ type: 'huipay:success', payload: r }, '*')}
        onError={(e) => window.parent.postMessage({ type: 'huipay:error', payload: { message: e.message } }, '*')}
      />
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <QueryClientProvider client={queryClient}>
    <App />
  </QueryClientProvider>,
);