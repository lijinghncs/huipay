import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@huipay/shared': path.resolve(__dirname, '../shared/src'),
      '@huipay/ui-kit': path.resolve(__dirname, '../ui-kit/src'),
    },
  },
  server: {
    port: 5171,
    open: true,
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
  },
});