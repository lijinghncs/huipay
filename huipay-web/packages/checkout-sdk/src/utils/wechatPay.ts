// 微信内拉起 JSAPI 支付（WeixinJSBridge.getBrandWCPayRequest）。
import type { JSAPIParams } from '../types';

/** 拉起微信支付窗口。 */
export function invokeWechatPay(params: JSAPIParams, onOk: () => void, onFail: (e: Error) => void) {
  const w = window as { WeixinJSBridge?: { invoke: (name: string, args: unknown, cb: (res: { err_msg: string }) => void) => void } };
  const doInvoke = () => {
    w.WeixinJSBridge?.invoke('getBrandWCPayRequest', params, (res) => {
      const msg = res?.err_msg ?? '';
      if (msg === 'get_brand_wcpay_request:ok') {
        onOk();
      } else if (msg === 'get_brand_wcpay_request:cancel') {
        onFail(new Error('用户已取消支付'));
      } else {
        onFail(new Error(msg || '拉起微信支付失败'));
      }
    });
  };
  // JSBridge 可能尚未就绪，监听 ready 事件后再拉起；超过 3 秒未就绪则判定为拉起失败
  if (w.WeixinJSBridge && typeof w.WeixinJSBridge.invoke === 'function') {
    doInvoke();
  } else {
    const timer = setTimeout(() => onFail(new Error('请在微信中打开完成支付')), 3000);
    document.addEventListener(
      'WeixinJSBridgeReady',
      () => {
        clearTimeout(timer);
        doInvoke();
      },
      { once: true },
    );
  }
}