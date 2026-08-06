// 收银台 UI 本地状态（Zustand）
import { create } from 'zustand';

interface CheckoutUIState {
  selectedChannel: string | null;
  couponApplied: string | null;
  isProcessing: boolean;
  setSelectedChannel: (code: string) => void;
  applyCoupon: (code: string) => void;
  setProcessing: (b: boolean) => void;
  reset: () => void;
}

export const useCheckoutUI = create<CheckoutUIState>((set) => ({
  selectedChannel: null,
  couponApplied: null,
  isProcessing: false,
  setSelectedChannel: (code) => set({ selectedChannel: code }),
  applyCoupon: (code) => set({ couponApplied: code }),
  setProcessing: (b) => set({ isProcessing: b }),
  reset: () => set({ selectedChannel: null, couponApplied: null, isProcessing: false }),
}));