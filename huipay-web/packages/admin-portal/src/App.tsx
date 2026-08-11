// 顶层路由
import React from 'react';
import { Route, Routes } from 'react-router-dom';
import { BasicLayout } from './layouts/BasicLayout';
import { Merchants } from './pages/Merchants';
import { MerchantDetailPage } from './pages/Merchants/detail';
import { MerchantWechatConfigPage } from './pages/Merchants/wechat-config';
import { Channels } from './pages/Channels';
import { RiskRules } from './pages/RiskRules';
import { Analytics } from './pages/Analytics';

export const App: React.FC = () => {
  return (
    <Routes>
      <Route element={<BasicLayout />}>
        <Route path="/" element={<Analytics />} />
        <Route path="/merchants" element={<Merchants />} />
        <Route path="/merchants/:id" element={<MerchantDetailPage />} />
        <Route path="/merchants/:id/wechat-config" element={<MerchantWechatConfigPage />} />
        <Route path="/channels" element={<Channels />} />
        <Route path="/risk-rules" element={<RiskRules />} />
      </Route>
    </Routes>
  );
};
