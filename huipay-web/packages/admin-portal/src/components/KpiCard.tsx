// 通用 KPI 统计卡片：图标渐变块 + 数值 + 较昨日趋势
import React from 'react';
import { Card, Statistic, Skeleton, theme } from 'antd';
import { RiseOutlined, FallOutlined } from '@ant-design/icons';

export interface KpiCardProps {
  title: string;
  value: number;
  prefix?: string;
  precision?: number;
  icon: React.ReactNode;
  color: string;
  trend?: number;
  up?: boolean;
  loading?: boolean;
  formatter?: (value: number) => string;
}

export const KpiCard: React.FC<KpiCardProps> = ({
  title,
  value,
  prefix = '',
  precision = 0,
  icon,
  color,
  trend,
  up,
  loading,
  formatter,
}) => {
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
              prefix={formatter ? undefined : prefix}
              precision={formatter ? undefined : precision}
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
      {!loading && trend !== undefined && (
        <div style={{ marginTop: 12, fontSize: 12, color: token.colorTextSecondary }}>
          较昨日{' '}
          {up ? (
            <span style={{ color: '#06b6a4' }}>
              <RiseOutlined /> {trend}%
            </span>
          ) : (
            <span style={{ color: '#f5222d' }}>
              <FallOutlined /> {Math.abs(trend)}%
            </span>
          )}
        </div>
      )}
    </Card>
  );
};