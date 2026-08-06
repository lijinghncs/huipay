// 金额展示组件（基于 AntD Statistic）
import React from 'react';
import { Statistic } from 'antd';

export interface MoneyProps {
  cents: number;
  prefix?: string;
  color?: string;
}

export const Money: React.FC<MoneyProps> = ({ cents, prefix = '¥', color }) => {
  return (
    <Statistic
      value={cents / 100}
      precision={2}
      prefix={prefix}
      valueStyle={{ color: color ?? '#0e1a2b', fontSize: 20, fontWeight: 700 }}
    />
  );
};