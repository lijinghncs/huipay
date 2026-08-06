// 顶层路由 + Layout
import React from 'react';
import { Route, Routes } from 'react-router-dom';
import { BasicLayout } from './layouts/BasicLayout';
import { Dashboard } from './pages/Dashboard';
import { Transactions } from './pages/Transactions';
import { Wallets } from './pages/Wallets';
import { SplitRules } from './pages/SplitRules';

export const App: React.FC = () => {
  return (
    <Routes>
      <Route element={<BasicLayout />}>
        <Route path="/" element={<Dashboard />} />
        <Route path="/transactions" element={<Transactions />} />
        <Route path="/wallets" element={<Wallets />} />
        <Route path="/split-rules" element={<SplitRules />} />
      </Route>
    </Routes>
  );
};