// H5 收银台骨架（独立 URL 形态）。
// 真实部署：路由 /h5 → https://checkout.huipay.cn/h5
import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { HuiPayCheckout } from '../components/Checkout';
import { createApi } from '@huipay/shared/api-client';
import '../styles/global.css';

const queryClient = new QueryClient();
createApi({ baseURL: import.meta.env.VITE_API_BASE ?? 'https://api.huipay.cn' });

function App() {
  const params = new URLSearchParams(window.location.search);
  const orderNo = params.get('order') ?? '';
  return (
    <QueryClientProvider client={queryClient}>
      <div style={{ minHeight: '100vh', background: '#f6f7fb' }}>
        <HuiPayCheckout
          orderNo={orderNo}
          channels={[
            { code: 'WECHAT', fee_rate: '0.60%', available: true },
            { code: 'ALIPAY', fee_rate: '0.55%', available: true },
          ]}
          amount={10000}
        />
      </div>
    </QueryClientProvider>
  );
}

ReactDOM.createRoot(document.getElementById('root')!).render(<App />);