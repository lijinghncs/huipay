import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@huipay/shared': path.resolve(__dirname, '../shared/src'),
    },
  },
  build: {
    lib: {
      entry: path.resolve(__dirname, 'src/index.ts'),
      name: 'HuiPayCheckout',
      formats: ['es', 'cjs'],
      fileName: (format) => `huipay-checkout.${format}.js`,
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'antd', 'axios', 'zustand', '@tanstack/react-query'],
    },
    sourcemap: true,
  },
  server: {
    port: 5173,
  },
});