// 商家工作台入口
import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { App as AntApp, ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn'; // 日期组件月份/星期等文案使用中文
import { BrowserRouter } from 'react-router-dom';
import { createApi } from '@huipay/shared/api-client';
import { App } from './App';
import { getToken, merchantIdFromToken } from './services/auth';
import './styles/global.css';

// 让 antd DatePicker / dayjs 的月份、星期等显示为中文
dayjs.locale('zh-cn');

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
            headerBg: '#0e1a2b',
            headerHeight: 56,
          },
          Menu: {
            darkItemBg: '#0e1a2b',
            darkSubMenuItemBg: '#0e1a2b',
            darkItemSelectedBg: '#1e6fff',
            darkItemColor: 'rgba(255,255,255,0.72)',
            darkItemHoverColor: '#ffffff',
            darkItemHoverBg: 'rgba(255,255,255,0.08)',
            itemHeight: 40,
            itemMarginInline: 10,
            itemBorderRadius: 10,
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
      <AntApp>
        <QueryClientProvider client={queryClient}>
          <BrowserRouter basename="/merchant">
            <App />
          </BrowserRouter>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>
  </React.StrictMode>,
);
