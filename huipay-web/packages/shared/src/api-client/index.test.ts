import { describe, it, expect } from 'vitest';
import { createApi } from './index';

interface Captured {
  headers: Record<string, unknown>;
}

/** 构造 createApi 并在一次请求中捕获请求头。 */
function captureHeaders(provider?: () => number): Promise<Captured> {
  const ins = createApi({ baseURL: 'http://api.test', merchantIdProvider: provider });
  let captured: Captured = { headers: {} };
  ins.defaults.adapter = async (config) => {
    captured = { headers: config.headers as unknown as Record<string, unknown> };
    return {
      data: { code: '0', data: { ok: true } },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    };
  };
  return ins.get('/v1/checkout/list').then(() => captured);
}

describe('api-client X-Merchant-Id 注入', () => {
  it('merchantIdProvider 返回非 0 时注入 X-Merchant-Id', async () => {
    const captured = await captureHeaders(() => 10001);
    expect(captured.headers['X-Merchant-Id']).toBe('10001');
  });

  it('merchantIdProvider 返回 0 时不注入 X-Merchant-Id', async () => {
    const captured = await captureHeaders(() => 0);
    expect(captured.headers['X-Merchant-Id']).toBeUndefined();
  });

  it('未配置 merchantIdProvider 时不注入 X-Merchant-Id', async () => {
    const captured = await captureHeaders(undefined);
    expect(captured.headers['X-Merchant-Id']).toBeUndefined();
  });

  it('始终注入 trace 头', async () => {
    const captured = await captureHeaders(() => 10001);
    expect(captured.headers['X-Trace-Id']).toBeTruthy();
    expect(captured.headers['Idempotency-Key']).toBeTruthy();
  });
});