// 部署静态文件服务器：serve 3 个 portal 的构建产物 + 代理 /v1/ 到后端 API
import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, '..');
const HUIPAY_WEB = path.join(ROOT, 'huipay-web');

const PORT = parseInt(process.env.PORT || '5000', 10);
const BACKEND_PORT = process.env.BACKEND_PORT || '8080';

const SPA_ROUTES = {
  '/merchant/': path.join(HUIPAY_WEB, 'packages/merchant-portal/dist'),
  '/admin/':    path.join(HUIPAY_WEB, 'packages/admin-portal/dist'),
  '/checkout/': path.join(HUIPAY_WEB, 'packages/checkout-sdk/dist'),
};

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js':   'application/javascript; charset=utf-8',
  '.css':  'text/css; charset=utf-8',
  '.json': 'application/json',
  '.png':  'image/png',
  '.jpg':  'image/jpeg',
  '.svg':  'image/svg+xml',
  '.ico':  'image/x-icon',
  '.woff2':'font/woff2',
  '.map':  'application/json',
};

function mimeType(filePath) {
  const ext = path.extname(filePath).toLowerCase();
  return MIME[ext] || 'application/octet-stream';
}

// 匹配 SPA 静态文件路径
function matchStatic(url) {
  const keys = Object.keys(SPA_ROUTES).sort((a, b) => b.length - a.length);
  for (const prefix of keys) {
    if (url.startsWith(prefix)) {
      const relativePath = url.slice(prefix.length - 1) || '/index.html';
      const distDir = SPA_ROUTES[prefix];
      return { distDir, relativePath };
    }
  }
  return null;
}

function serveStatic(res, distDir, relativePath) {
  // SPA fallback: 如果请求的不是带扩展名的文件，回退到 index.html
  let filePath = path.join(distDir, relativePath);
  if (!path.extname(filePath)) {
    filePath = path.join(distDir, 'index.html');
  }

  fs.readFile(filePath, (err, data) => {
    if (err) {
      // 文件不存在时也 fallback 到 index.html（SPA 路由）
      const fallback = path.join(distDir, 'index.html');
      fs.readFile(fallback, (err2, data2) => {
        if (err2) {
          res.writeHead(404, { 'Content-Type': 'text/plain' });
          res.end(`Not found: ${relativePath}`);
          return;
        }
        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        res.end(data2);
      });
      return;
    }
    res.writeHead(200, { 'Content-Type': mimeType(filePath) });
    res.end(data);
  });
}

function proxyToBackend(clientReq, clientRes) {
  const options = {
    hostname: 'localhost',
    port: BACKEND_PORT,
    path: clientReq.url,
    method: clientReq.method,
    headers: { ...clientReq.headers },
  };
  delete options.headers['host'];

  const proxyReq = http.request(options, (proxyRes) => {
    clientRes.writeHead(proxyRes.statusCode, proxyRes.headers);
    proxyRes.pipe(clientRes);
  });
  proxyReq.on('error', () => {
    clientRes.writeHead(502, { 'Content-Type': 'text/plain' });
    clientRes.end('Backend unavailable');
  });
  clientReq.pipe(proxyReq);
}

const server = http.createServer((req, res) => {
  // 首页 → 重定向到 merchant
  if (req.url === '/' || req.url === '') {
    res.writeHead(302, { Location: '/merchant/' });
    res.end();
    return;
  }

  // /v1/ → 代理到后端 API
  if (req.url.startsWith('/v1/') || req.url === '/healthz' || req.url === '/metrics') {
    proxyToBackend(req, res);
    return;
  }

  // 静态文件
  const match = matchStatic(req.url);
  if (match) {
    serveStatic(res, match.distDir, match.relativePath);
    return;
  }

  res.writeHead(404, { 'Content-Type': 'text/plain' });
  res.end('Not found');
});

server.listen(PORT, '0.0.0.0', () => {
  console.log(`Static file server listening on 0.0.0.0:${PORT}`);
  console.log('  /merchant/ → merchant-portal (build)');
  console.log('  /admin/    → admin-portal (build)');
  console.log('  /checkout/ → checkout-sdk (build)');
  console.log(`  /v1/       → backend :${BACKEND_PORT}`);
});