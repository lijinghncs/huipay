// 嵌入式页面骨架（iframe 内容）。
// 真实部署：路由 /embed → https://checkout.huipay.cn/embed
import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { HuiPayCheckout } from '../components/Checkout';
import { createApi } from '@huipay/shared/api-client';
import '../styles/global.css';

const queryClient = new QueryClient();

// 初始化 api client
createApi({ baseURL: import.meta.env.VITE_API_BASE ?? 'https://api.huipay.cn' });

function parseToken(): { orderNo: string; channels: { code: string; fee_rate: string; available: boolean }[]; amount: number; discount: number } {
  // 实际项目：用 token 调后端换预下单信息
  const params = new URLSearchParams(window.location.search);
  return {
    orderNo: params.get('orderNo') ?? 'demo',
    channels: [
      { code: 'WECHAT', fee_rate: '0.60%', available: true },
      { code: 'ALIPAY', fee_rate: '0.55%', available: true },
    ],
    amount: Number(params.get('amount') ?? 10000),
    discount: Number(params.get('discount') ?? 0),
  };
}

function App() {
  const info = parseToken();
  return (
    <QueryClientProvider client={queryClient}>
      <div style={{ minHeight: '100vh', background: '#f6f7fb', display: 'flex', justifyContent: 'center', paddingTop: 32 }}>
        <HuiPayCheckout
          orderNo={info.orderNo}
          channels={info.channels}
          amount={info.amount}
          discount={info.discount}
          onSuccess={(r) => window.parent.postMessage({ type: 'huipay:success', payload: r }, '*')}
          onError={(e) => window.parent.postMessage({ type: 'huipay:error', payload: { message: e.message } }, '*')}
        />
      </div>
    </QueryClientProvider>
  );
}

ReactDOM.createRoot(document.getElementById('root')!).render(<App />);