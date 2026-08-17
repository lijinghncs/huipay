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
  // 多页面模式：/ 与 /embed → index.html(embed)，/h5 → h5.html
  appType: 'mpa',
  server: {
    port: 5173,
    fs: {
      strict: false,
    },
    // 将无扩展名的 /h5 重写到 h5.html，避免回退到 index.html 导致空白
    // 兼容 Vite --base /checkout/ 场景，自动剥离 base 前缀再匹配
    configureServer(server) {
      server.middlewares.use((req, _res, next) => {
        if (!req.url) return next();
        // 自动识别 base 前缀（如有），剥离后匹配
        const matched = req.url.match(/^(.*?)\/h5(\?|$)/);
        if (matched) {
          const base = matched[1] || '';
          req.url = base + '/h5.html' + req.url.slice(matched[1].length + 2);
        }
        next();
      });
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
});