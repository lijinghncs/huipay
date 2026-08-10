// 管理后台入口
import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ConfigProvider, App as AntApp } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { BrowserRouter } from 'react-router-dom';
import { createApi } from '@huipay/shared/api-client';
import { App } from './App';
import './styles/global.css';

createApi({
  baseURL: import.meta.env.VITE_API_BASE ?? 'https://api.huipay.cn',
});

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 30_000, refetchOnWindowFocus: false },
  },
});

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider
      locale={zhCN}
      theme={{
        token: {
          colorPrimary: '#1e6fff',
          colorInfo: '#1e6fff',
          colorBgLayout: '#f4f6fb',
          colorText: '#1f2a44',
          colorTextSecondary: '#66718b',
          borderRadius: 8,
          fontFamily: `'PingFang SC','Microsoft YaHei',-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif`,
        },
        components: {
          Layout: {
            headerBg: '#0e1a33',
            siderBg: '#0e1a33',
            headerHeight: 56,
          },
          Menu: {
            darkItemBg: '#0e1a33',
            darkSubMenuItemBg: '#0e1a33',
            darkItemSelectedBg: '#1e6fff',
            darkItemColor: 'rgba(255,255,255,0.72)',
            darkItemHoverColor: '#ffffff',
          },
          Card: {
            headerBg: 'transparent',
            headerFontSize: 15,
          },
          Table: {
            headerBg: '#f7f9fc',
            headerColor: '#66718b',
            rowHoverBg: '#f5f8ff',
          },
        },
      }}
    >
      <QueryClientProvider client={queryClient}>
        <AntApp>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </AntApp>
      </QueryClientProvider>
    </ConfigProvider>
  </React.StrictMode>,
);