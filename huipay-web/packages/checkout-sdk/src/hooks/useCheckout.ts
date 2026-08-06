// 收银台业务 hook：处理支付、轮询订单状态。
import { useMutation, useQuery } from '@tanstack/react-query';
import { post, get } from '@huipay/shared/api-client';
import type { PrecreateResponse, Order } from '@huipay/shared';
import type { CheckoutProps } from '../types';

/** 拉取订单状态（轮询）。 */
export function useOrder(orderNo: string, enabled = true) {
  return useQuery<Order>({
    queryKey: ['order', orderNo],
    queryFn: () => get<Order>(`/v1/checkout/${orderNo}`),
    enabled: enabled && !!orderNo,
    refetchInterval: (q) => {
      const status = q.state.data?.status;
      return status === 'PAID' || status === 'CLOSED' ? false : 2_000;
    },
  });
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
    channels,
    amount,
    discount,
    finalAmount: amount - discount,
  };
}