// 收银台支付 hook：调用后端 Pay 接口发起真实支付。
import { useMutation } from '@tanstack/react-query';
import { post } from '@huipay/shared/api-client';
import type { PayResponse, PayType } from '../types';

export interface PayParams {
  orderNo: string;
  payType: PayType;
  openId?: string;
}

/** 发起支付（真实调 /v1/checkout/:orderNo/pay）。 */
export function usePay() {
  return useMutation<PayResponse, Error, PayParams>({
    mutationFn: ({ orderNo, payType, openId }) =>
      post<PayResponse>(`/v1/checkout/${orderNo}/pay`, { pay_type: payType, openid: openId }),
  });
}