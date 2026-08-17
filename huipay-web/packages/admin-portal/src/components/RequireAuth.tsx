// 管理后台路由守卫：未登录跳转登录页
import React from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { isLoggedIn } from '../services/auth';

export const RequireAuth: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const location = useLocation();
  if (!isLoggedIn()) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }
  return <>{children}</>;
};