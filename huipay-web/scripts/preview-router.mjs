// 预览路由服务器：将 /merchant/ /admin/ /checkout/ 分别代理到各自的 Vite dev server
import http from 'node:http';

const PORT = parseInt(process.env.PORT || '5000', 10);

const BACKENDS = {
  '/merchant/':  { host: 'localhost', port: 5170 },
  '/admin/':     { host: 'localhost', port: 5171 },
  '/checkout/v1/': { host: 'localhost', port: 5001 },
  '/checkout/':  { host: 'localhost', port: 5173 },
  '/v1/':        { host: 'localhost', port: 5001 },
};

// 按路径前缀匹配后端
function matchBackend(url) {
  const keys = Object.keys(BACKENDS).sort((a, b) => b.length - a.length); // 长前缀优先
  for (const prefix of keys) {
    if (url.startsWith(prefix)) {
      return { prefix, target: BACKENDS[prefix] };
    }
  }
  return null;
}

const server = http.createServer((req, res) => {
  const match = matchBackend(req.url);

  // 首页 → 重定向到 merchant
  if (req.url === '/' || req.url === '') {
    res.writeHead(302, { Location: '/merchant/' });
    res.end();
    return;
  }

  if (!match) {
    res.writeHead(404, { 'Content-Type': 'text/plain' });
    res.end('Not found');
    return;
  }

  // 透传 URL，去掉匹配的前缀使其到达后端实际路径
  const path = match.prefix.startsWith('/checkout/v1/')
    ? req.url.replace('/checkout', '')
    : req.url;
  proxyRequest(req, res, match.target, path);
});

function proxyRequest(clientReq, clientRes, target, path) {
  const options = {
    hostname: target.host,
    port: target.port,
    path,
    method: clientReq.method,
    headers: { ...clientReq.headers },
  };

  // 修正 Host header
  delete options.headers['host'];

  const proxyReq = http.request(options, (proxyRes) => {
    // 修正响应中的 Location header（去掉内部端口）
    if (proxyRes.headers.location) {
      proxyRes.headers.location = proxyRes.headers.location.replace(
        /http:\/\/localhost:\d+/,
        '',
      );
    }
    clientRes.writeHead(proxyRes.statusCode, proxyRes.headers);
    proxyRes.pipe(clientRes);
  });

  proxyReq.on('error', (err) => {
    clientRes.writeHead(502, { 'Content-Type': 'text/plain' });
    clientRes.end(`Proxy error: ${err.message}`);
  });

  clientReq.pipe(proxyReq);
}

server.listen(PORT, '0.0.0.0', () => {
  console.log(`Preview router listening on 0.0.0.0:${PORT}`);
  console.log('  /merchant/ → merchant-portal :5170');
  console.log('  /admin/    → admin-portal    :5171');
  console.log('  /checkout/ → checkout-sdk    :5173');
  console.log('  /v1/       → backend         :5001');
});