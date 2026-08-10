// 收银台 UI 本地状态（Zustand）
import { create } from 'zustand';
import type { PayType } from '../types';

interface CheckoutUIState {
  selectedChannel: string | null;
  selectedPayType: PayType;
  couponApplied: string | null;
  isProcessing: boolean;
  setSelectedChannel: (code: string) => void;
  setSelectedPayType: (t: PayType) => void;
  applyCoupon: (code: string) => void;
  setProcessing: (b: boolean) => void;
  reset: () => void;
}

export const useCheckoutUI = create<CheckoutUIState>((set) => ({
  selectedChannel: null,
  selectedPayType: 'NATIVE',
  couponApplied: null,
  isProcessing: false,
  setSelectedChannel: (code) => set({ selectedChannel: code }),
  setSelectedPayType: (t) => set({ selectedPayType: t }),
  applyCoupon: (code) => set({ couponApplied: code }),
  setProcessing: (b) => set({ isProcessing: b }),
  reset: () => set({ selectedChannel: null, selectedPayType: 'NATIVE', couponApplied: null, isProcessing: false }),
}));