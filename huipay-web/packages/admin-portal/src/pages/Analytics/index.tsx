// 概览/BI 看板
import React, { useEffect, useState } from 'react';
import { Card, Col, Row, Skeleton, Alert, Space, Statistic, Button } from 'antd';
import {
  AccountBookOutlined,
  ShoppingCartOutlined,
  TeamOutlined,
  WalletOutlined,
  RiseOutlined,
  FallOutlined,
} from '@ant-design/icons';
import { Money } from '@huipay/ui-kit';
import ReactECharts from 'echarts-for-react';

const kpiCards = [
  {
    title: '今日 GMV',
    value: 1560000,
    prefix: '¥',
    precision: 2,
    icon: <AccountBookOutlined />,
    color: '#1e6fff',
    trend: 12.6,
    up: true,
  },
  {
    title: '今日订单',
    value: 12345,
    prefix: '',
    precision: 0,
    icon: <ShoppingCartOutlined />,
    color: '#06b6a4',
    trend: 8.2,
    up: true,
  },
  {
    title: '活跃商户',
    value: 326,
    prefix: '',
    precision: 0,
    icon: <TeamOutlined />,
    color: '#f59e0b',
    trend: -3.1,
    up: false,
  },
  {
    title: '备付金余额',
    value: 98765400,
    prefix: '¥',
    precision: 2,
    icon: <WalletOutlined />,
    color: '#8b5cf6',
    trend: 5.4,
    up: true,
  },
];

export const Analytics: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [tryCount, setTryCount] = useState(0);

  useEffect(() => {
    setLoading(true);
    setError(false);
    const timer = setTimeout(() => {
      // 模拟接口：演示 20% 概率失败，用于展示错误态
      if (tryCount > 0 && Math.random() < 0.2) {
        setError(true);
      }
      setLoading(false);
    }, 800);
    return () => clearTimeout(timer);
  }, [tryCount]);

  const chartOption = {
    tooltip: { trigger: 'axis' },
    grid: { left: 48, right: 24, top: 24, bottom: 32 },
    xAxis: {
      type: 'category',
      data: ['周一', '周二', '周三', '周四', '周五', '周六', '周日'],
      axisLine: { lineStyle: { color: '#e5eaf3' } },
      axisTick: { show: false },
      axisLabel: { color: '#66718b' },
    },
    yAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: '#f0f2f7' } },
      axisLabel: { color: '#66718b' },
    },
    series: [
      {
        name: '交易额（元）',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 7,
        data: [12000, 19000, 23000, 18000, 24000, 27000, 30000],
        itemStyle: { color: '#1e6fff' },
        lineStyle: { width: 3, color: '#1e6fff' },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(30,111,255,0.22)' },
              { offset: 1, color: 'rgba(30,111,255,0.02)' },
            ],
          },
        },
      },
    ],
  };

  if (error) {
    return (
      <Alert
        type="error"
        showIcon
        message="数据加载失败"
        description="概览数据获取失败，请稍后重试。"
        action={
          <Button size="small" onClick={() => setTryCount((c) => c + 1)}>
            重试
          </Button>
        }
      />
    );
  }

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Row gutter={[16, 16]}>
        {kpiCards.map((k) => (
          <Col xs={12} sm={12} md={6} key={k.title}>
            <Card>
              {loading ? (
                <Skeleton active paragraph={{ rows: 1 }} title={{ width: '60%' }} />
              ) : (
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
                  <div>
                    <div style={{ color: '#66718b', fontSize: 13, marginBottom: 8 }}>{k.title}</div>
                    <Statistic
                      value={k.value}
                      prefix={k.prefix}
                      precision={k.precision}
                      valueStyle={{ color: '#1f2a44', fontWeight: 700 }}
                    />
                  </div>
                  <span
                    style={{
                      width: 40,
                      height: 40,
                      borderRadius: 10,
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontSize: 20,
                      color: '#fff',
                      background: `linear-gradient(135deg, ${k.color}, ${k.color}cc)`,
                    }}
                  >
                    {k.icon}
                  </span>
                </div>
              )}
              {!loading && (
                <div style={{ marginTop: 12, fontSize: 12, color: '#66718b' }}>
                  较昨日 {k.up ? <span style={{ color: '#06b6a4' }}><RiseOutlined />{k.trend}%</span> : <span style={{ color: '#f5222d' }}><FallOutlined />{Math.abs(k.trend)}%</span>}
                </div>
              )}
            </Card>
          </Col>
        ))}
      </Row>

      <Card title="近 7 日交易趋势">
        {loading ? <Skeleton active paragraph={{ rows: 6 }} /> : <ReactECharts option={chartOption} style={{ height: 320 }} />}
      </Card>
    </Space>
  );
};