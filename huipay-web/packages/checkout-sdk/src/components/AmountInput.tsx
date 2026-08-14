// 码牌建单金额输入组件：消费者扫码牌后输入支付金额，确认后直接建单并拉起微信支付。
import React from 'react';
import { App as AntApp, InputNumber } from 'antd';
import { useMutation, useQuery } from '@tanstack/react-query';
import { get, post } from '@huipay/shared/api-client';
import { invokeWechatPay } from '../utils/wechatPay';
import { isWeixinBrowser } from '../utils/wechatOAuth';
import type { PrecreateResponse, PayResponse } from '../types';

/** 码牌公开信息（H5 收银台顶部展示门店名称）。 */
interface CodeInfoResponse {
  code_id: string;
  store_id?: number;
  store_name?: string;
}

/** 码牌建单：POST /v1/checkout/precreate-by-code。 */
function usePrecreateByCode() {
  return useMutation<PrecreateResponse, Error, { code: string; amount: number }>({
    mutationFn: ({ code, amount }) =>
      post<PrecreateResponse>('/v1/checkout/precreate-by-code', { code_id: code, amount }),
  });
}

/** 码牌门店信息：GET /v1/checkout/code/:code_id。 */
function useCodeInfo(code: string) {
  return useQuery<CodeInfoResponse | null>({
    queryKey: ['code-info', code],
    queryFn: () => get<CodeInfoResponse>(`/v1/checkout/code/${code}`),
    enabled: !!code,
    retry: false,
  });
}

/** 发起支付：POST /v1/checkout/:orderNo/pay（JSAPI 拉起微信）。 */
function usePayOnce() {
  return useMutation<PayResponse, Error, { orderNo: string; openId: string }>({
    mutationFn: ({ orderNo, openId }) =>
      post<PayResponse>(`/v1/checkout/${orderNo}/pay`, { pay_type: 'JSAPI', openid: openId }),
  });
}

/** 快捷金额选项（元）。 */
const QUICK_AMOUNTS = [10, 20, 50, 100, 200];

/** 金额输入（单位：元，仅码牌模式展示）。 */
export const AmountInput: React.FC<{
  code: string;
  openId: string;
  onCreated: (orderNo: string) => void;
  onError: (msg: string) => void;
}> = ({ code, openId, onCreated, onError }) => {
  const { message } = AntApp.useApp();
  const [yuan, setYuan] = React.useState<number | null>(null);
  const precreate = usePrecreateByCode();
  const pay = usePayOnce();
  const { data: codeInfo } = useCodeInfo(code);
  const storeName = codeInfo?.store_name;
  const cents = Math.round((yuan ?? 0) * 100);
  // 小数位校验：最多 2 位（避免浮点误差，按 100 倍取整判断）
  const hasValidDecimals = yuan == null || Number.isInteger(Math.round(yuan * 1e6) / 1e4);
  const inRange = !!yuan && cents >= 1 && cents <= 5_000_000;
  const valid = hasValidDecimals && inRange;
  const pending = precreate.isPending || pay.isPending;

  const handleSubmit = () => {
    if (!hasValidDecimals) {
      message.warning('金额最多保留 2 位小数');
      return;
    }
    if (!inRange) {
      message.warning('请输入 0.01 ~ 50000 元之间的金额');
      return;
    }
    if (!openId) {
      message.error('未获取到微信 openid，请在微信内打开');
      onError('未获取到微信 openid，请在微信内打开');
      return;
    }
    precreate.mutate(
      { code, amount: cents },
      {
        onSuccess: (resp) => {
          // 建单成功后立即发起微信内拉起支付
          pay.mutate(
            { orderNo: resp.order_no, openId },
            {
              onSuccess: (payResp) => {
                if (payResp.jsapi) {
                  if (isWeixinBrowser()) {
                    // 微信内：真实拉起微信支付窗口
                    invokeWechatPay(
                      payResp.jsapi,
                      () => onCreated(resp.order_no),
                      (e) => onError(e.message),
                    );
                  } else {
                    // 非微信环境（本地/浏览器联调）：后端 mock 挡板已同步完成支付，直接进入结果页
                    onCreated(resp.order_no);
                  }
                } else if (payResp.pay_url) {
                  // H5 兜底跳转
                  window.location.href = payResp.pay_url;
                } else {
                  onError('暂无可用的支付方式');
                }
              },
              onError: (e) => onError(e.message),
            },
          );
        },
        onError: (e) => {
          onError(
            /not found|disabled/i.test(e.message ?? '')
              ? '该收款码不存在或已停用'
              : (e.message ?? '创建订单失败，请重试'),
          );
        },
      },
    );
  };

  return (
    <div className="huipay-amount" style={{ minHeight: '100vh', width: '100%', maxWidth: 520 }}>
      {/* 顶部商户标识 + 门店名称 */}
      <header className="huipay-amount__brand">
        <span className="huipay-amount__seal">惠</span>
        <div className="huipay-amount__brand-info">
          {storeName ? (
            <>
              <span className="huipay-amount__store-name">{storeName}</span>
              <span className="huipay-amount__brand-text">惠付 · 扫码收款</span>
            </>
          ) : (
            <span className="huipay-amount__brand-text">惠付 · 扫码收款</span>
          )}
        </div>
      </header>

      {/* 金额主视觉 */}
      <section className="huipay-amount__hero">
        <div className="huipay-amount__label">
          <span className="huipay-amount__label-dot" />
          请输入应付金额（元）
          <span className="huipay-amount__label-dot" />
        </div>
        <div className="huipay-amount__display">
          <span className="huipay-amount__currency">¥</span>
          <InputNumber
            className="huipay-amount__input"
            min={0.01}
            max={50000}
            step={0.01}
            value={yuan}
            onChange={(v) => setYuan(v)}
            placeholder="0.00"
            autoFocus
            controls={false}
            onPressEnter={handleSubmit}
          />
          <span className="huipay-amount__underline" />
        </div>
        <p className="huipay-amount__hint">付款将实时进入商户账户，请核对金额后确认</p>
      </section>

      {/* 快捷金额 */}
      <section className="huipay-amount__quick">
        <p className="huipay-amount__quick-title">快捷金额</p>
        <div className="huipay-amount__chips">
          {QUICK_AMOUNTS.map((v) => (
            <button
              key={v}
              type="button"
              className={`huipay-amount__chip${yuan === v ? ' huipay-amount__chip--active' : ''}`}
              onClick={() => setYuan(v)}
            >
              {v}
            </button>
          ))}
        </div>
      </section>

      {/* 确认按钮 */}
      <section className="huipay-amount__action">
        <button type="button" className="huipay-amount__submit" disabled={!yuan || pending} onClick={handleSubmit}>
          {pending ? (
            <>
              <span className="huipay-amount__spinner" />
              正在拉起微信支付…
            </>
          ) : (
            <>
              确认支付
              <span className="huipay-amount__submit-arrow">→</span>
            </>
          )}
        </button>
        {precreate.isError && (
          <p className="huipay-amount__error">
            {/not found|disabled/i.test((precreate.error as Error)?.message ?? '')
              ? '该收款码不存在或已停用'
              : ((precreate.error as Error)?.message ?? '创建订单失败，请重试')}
          </p>
        )}
      </section>

      {/* 信任背书 */}
      <footer className="huipay-amount__trust">
        <span>安全支付</span>
        <span className="huipay-amount__trust-sep">·</span>
        <span>微信 / 支付宝</span>
        <span className="huipay-amount__trust-sep">·</span>
        <span>资金由银行存管</span>
      </footer>
    </div>
  );
};