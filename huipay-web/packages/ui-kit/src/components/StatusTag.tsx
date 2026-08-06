// 通用状态标签（订单/分账状态）
import React from 'react';
import { Tag } from 'antd';

export interface StatusTagProps {
  status: string;
}

const COLOR_MAP: Record<string, string> = {
  CREATED: 'blue',
  PAID: 'green',
  CLOSED: 'default',
  REFUNDED: 'red',
  PENDING: 'orange',
  PROCESSING: 'blue',
  SUCCESS: 'green',
  FAILED: 'red',
  RETURNED: 'purple',
};

const LABEL_MAP: Record<string, string> = {
  CREATED: '已创建',
  PAID: '已支付',
  CLOSED: '已关闭',
  REFUNDED: '已退款',
  PENDING: '待处理',
  PROCESSING: '处理中',
  SUCCESS: '成功',
  FAILED: '失败',
  RETURNED: '已回退',
};

export const StatusTag: React.FC<StatusTagProps> = ({ status }) => {
  return <Tag color={COLOR_MAP[status] ?? 'default'}>{LABEL_MAP[status] ?? status}</Tag>;
};