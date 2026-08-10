import { beforeEach, describe, expect, it } from 'vitest';
import { useCheckoutUI } from './useCheckoutUI';

describe('useCheckoutUI 状态', () => {
  beforeEach(() => {
    useCheckoutUI.getState().reset();
  });

  it('默认选中 NATIVE 且未选通道', () => {
    const s = useCheckoutUI.getState();
    expect(s.selectedPayType).toBe('NATIVE');
    expect(s.selectedChannel).toBeNull();
    expect(s.isProcessing).toBe(false);
  });

  it('setSelectedPayType 切换支付场景', () => {
    useCheckoutUI.getState().setSelectedPayType('H5');
    expect(useCheckoutUI.getState().selectedPayType).toBe('H5');
  });

  it('setSelectedChannel 选择通道', () => {
    useCheckoutUI.getState().setSelectedChannel('WECHAT');
    expect(useCheckoutUI.getState().selectedChannel).toBe('WECHAT');
  });

  it('setProcessing 控制处理中状态', () => {
    useCheckoutUI.getState().setProcessing(true);
    expect(useCheckoutUI.getState().isProcessing).toBe(true);
  });

  it('reset 恢复默认', () => {
    useCheckoutUI.getState().setSelectedChannel('WECHAT');
    useCheckoutUI.getState().setSelectedPayType('JSAPI');
    useCheckoutUI.getState().setProcessing(true);
    useCheckoutUI.getState().reset();
    const s = useCheckoutUI.getState();
    expect(s.selectedChannel).toBeNull();
    expect(s.selectedPayType).toBe('NATIVE');
    expect(s.isProcessing).toBe(false);
  });
});