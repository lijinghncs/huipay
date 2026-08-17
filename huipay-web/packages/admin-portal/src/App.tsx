// 顶层路由
import React from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { BasicLayout } from './layouts/BasicLayout';
import { Merchants } from './pages/Merchants';
import { MerchantDetailPage } from './pages/Merchants/detail';
import { MerchantWechatConfigPage } from './pages/Merchants/wechat-config';
import { Channels } from './pages/Channels';
import { RiskRules } from './pages/RiskRules';
import { Analytics } from './pages/Analytics';
import { StoreStats } from './pages/StoreStats';
import { SplitManage } from './pages/SplitManage';
import { Login } from './pages/Login';
import { RequireAuth } from './components/RequireAuth';

export const App: React.FC = () => {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        element={
          <RequireAuth>
            <BasicLayout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<Analytics />} />
        <Route path="/merchants" element={<Merchants />} />
        <Route path="/merchants/:id" element={<MerchantDetailPage />} />
        <Route path="/merchants/:id/wechat-config" element={<MerchantWechatConfigPage />} />
        <Route path="/channels" element={<Channels />} />
        <Route path="/risk-rules" element={<RiskRules />} />
        {/* V2 合并版：门店按日统计 + 分账管理 */}
        <Route path="/store-stats" element={<StoreStats />} />
        <Route path="/split-manage" element={<SplitManage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
};
