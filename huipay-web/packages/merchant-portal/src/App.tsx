// 顶层路由 + Layout
import React from 'react';
import { Route, Routes } from 'react-router-dom';
import { BasicLayout } from './layouts/BasicLayout';
import { Dashboard } from './pages/Dashboard';
import { Transactions } from './pages/Transactions';
import { TransactionStats } from './pages/TransactionStats';
import { Codes } from './pages/Codes';
import { Wallets } from './pages/Wallets';
import { SplitRules } from './pages/SplitRules';
import { Splits } from './pages/Splits';
import { Stores } from './pages/Stores';
import { StoreDetail } from './pages/Stores/detail';
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
        <Route path="/" element={<Dashboard />} />
        <Route path="/transactions" element={<Transactions />} />
        <Route path="/transaction-stats" element={<TransactionStats />} />
        <Route path="/codes" element={<Codes />} />
        <Route path="/wallets" element={<Wallets />} />
        <Route path="/split-rules" element={<SplitRules />} />
        <Route path="/splits" element={<Splits />} />
        <Route path="/stores" element={<Stores />} />
        <Route path="/stores/:id" element={<StoreDetail />} />
      </Route>
    </Routes>
  );
};
