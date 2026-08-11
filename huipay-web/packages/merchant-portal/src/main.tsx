// 商家工作台入口
import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { App as AntApp, ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { BrowserRouter } from 'react-router-dom';
import { createApi } from '@huipay/shared/api-client';
import { App } from './App';
import { getToken, merchantIdFromToken } from './services/auth';

// 初始化 API 客户端
createApi({
  // 置空则走 vite 代理（/v1 → 本机 http://localhost:8080）
  baseURL: import.meta.env.VITE_API_BASE ?? '',
  // 登录态：Bearer token 由后端优先解析；明文头仅开发模式兜底
  tokenProvider: () => getToken(),
  merchantIdProvider: () => merchantIdFromToken(),
});

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 30_000, refetchOnWindowFocus: false },
  },
});

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN} theme={{ token: { colorPrimary: '#1e6fff' } }}>
      <AntApp>
        <QueryClientProvider client={queryClient}>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>
  </React.StrictMode>,
);
