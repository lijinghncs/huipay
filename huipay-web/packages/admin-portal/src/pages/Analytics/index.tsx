// 概览/BI 看板
import React from 'react';
import { Card, Col, Row, Statistic, Space } from 'antd';
import { Money } from '@huipay/ui-kit';
import ReactECharts from 'echarts-for-react';

export const Analytics: React.FC = () => {
  const chartOption = {
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'] },
    yAxis: { type: 'value' },
    series: [
      {
        name: '交易额（元）',
        type: 'line',
        smooth: true,
        data: [12000, 19000, 23000, 18000, 24000, 27000, 30000],
        itemStyle: { color: '#1e6fff' },
      },
    ],
  };

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <Statistic title="今日 GMV" value={1560000} prefix="¥" precision={2} valueStyle={{ color: '#1e6fff' }} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="今日订单" value={12345} valueStyle={{ color: '#06b6a4' }} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="活跃商户" value={326} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Money cents={98765400} prefix="备付金余额 ¥" />
          </Card>
        </Col>
      </Row>
      <Card title="近 7 日交易趋势">
        <ReactECharts option={chartOption} style={{ height: 320 }} />
      </Card>
    </Space>
  );
};