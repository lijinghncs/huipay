// 收银台业务 hook：处理支付、轮询订单状态。
import { useMutation, useQuery } from '@tanstack/react-query';
import { post, get } from '@huipay/shared/api-client';
import type { PrecreateResponse, Order, QueryResult } from '@huipay/shared';
import type { CheckoutProps } from '../types';

/** 拉取订单状态（轮询）。 */
export function useOrder(orderNo: string, enabled = true) {
  const orderQuery = useQuery<Order>({
    queryKey: ['order', orderNo],
    queryFn: () => get<Order>(`/v1/checkout/${orderNo}`),
    enabled: enabled && !!orderNo,
    refetchInterval: (q) => {
      const status = q.state.data?.status;
      return status === 'PAID' || status === 'CLOSED' ? false : 2_000;
    },
  });

  const terminal = orderQuery.data?.status === 'PAID' || orderQuery.data?.status === 'CLOSED';
  // 通道侧查询兜底：本地未到终态时每 10s 调一次 /query（只读），
  // 用于微信回调延迟场景：通道侧已支付时提示"支付处理中"，避免用户重复支付。
  const queryResult = useQuery<QueryResult>({
    queryKey: ['order-query', orderNo],
    queryFn: () => get<QueryResult>(`/v1/checkout/${orderNo}/query`),
    enabled: enabled && !!orderNo && !terminal,
    refetchInterval: 10_000,
  });

  return {
    ...orderQuery,
    /** 通道侧已支付但本地订单尚未入账（回调延迟） */
    channelPaid: !terminal && !!queryResult.data?.paid,
  };
}

/** 触发支付（调通道）。骨架：返回预下单响应。 */
export function usePrecreate() {
  return useMutation<PrecreateResponse, Error, Parameters<typeof doPrecreate>[0]>({
    mutationFn: doPrecreate,
  });
}

async function doPrecreate(req: {
  merchant_id: number;
  merchant_order_no: string;
  amount: number;
  subject?: string;
  notify_url?: string;
  expire_seconds?: number;
}) {
  return post<PrecreateResponse>('/v1/checkout/precreate', req);
}

/** 聚合 hook：暴露 orderNo、channels、amount、回调。 */
export function useCheckout(props: CheckoutProps) {
  const { orderNo, channels, amount, discount = 0 } = props;
  const orderQuery = useOrder(orderNo);

  return {
    order: orderQuery.data,
    isLoading: orderQuery.isLoading,
    isPaid: orderQuery.data?.status === 'PAID',
    channelPaid: orderQuery.channelPaid,
    channels,
    amount,
    discount,
    finalAmount: amount - discount,
  };
}
