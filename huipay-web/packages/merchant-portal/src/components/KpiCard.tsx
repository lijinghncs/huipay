// 通用 KPI 统计卡片：图标渐变块 + 数值
import React from 'react';
import { Card, Statistic, Skeleton, theme } from 'antd';

export interface KpiCardProps {
  title: string;
  value: number;
  icon: React.ReactNode;
  color: string;
  loading?: boolean;
  formatter?: (value: number) => string;
}

export const KpiCard: React.FC<KpiCardProps> = ({ title, value, icon, color, loading, formatter }) => {
  const { token } = theme.useToken();

  return (
    <Card className="hp-kpi" styles={{ body: { padding: 20 } }}>
      {loading ? (
        <Skeleton active paragraph={{ rows: 1 }} title={{ width: '60%' }} />
      ) : (
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
          <div>
            <div style={{ color: token.colorTextSecondary, fontSize: 13, marginBottom: 8 }}>{title}</div>
            <Statistic
              value={formatter ? formatter(value) : value}
              valueStyle={{ color: token.colorText, fontWeight: 700 }}
            />
          </div>
          <span
            className="hp-kpi-icon"
            style={{
              width: 40,
              height: 40,
              borderRadius: 10,
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 20,
              color: '#fff',
              background: `linear-gradient(135deg, ${color}, ${color}cc)`,
            }}
          >
            {icon}
          </span>
        </div>
      )}
    </Card>
  );
};