// 封装 Axios，提供统一签名、错误处理、Idempotency-Key 注入。
import axios, { AxiosError, type AxiosInstance, type AxiosRequestConfig } from 'axios';
import type { ApiResponse } from '../types';

let instance: AxiosInstance | null = null;

export interface ApiClientOptions {
  baseURL: string;
  /** 商户签名密钥（如有）；管理后台 JWT 由各自的拦截器注入。 */
  signSecret?: string;
  /** 自定义 trace ID 生成函数（默认用 crypto.randomUUID） */
  traceIdGenerator?: () => string;
  /** 自定义错误处理回调（如 Toast） */
  onError?: (err: AxiosError<ApiResponse>) => void;
}

function createIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/** 构造单例 api client。 */
export function createApi(opts: ApiClientOptions): AxiosInstance {
  const ins = axios.create({
    baseURL: opts.baseURL,
    timeout: 30_000,
    headers: { 'Content-Type': 'application/json' },
  });

  // request 拦截器：注入 trace id 与 idempotency-key
  ins.interceptors.request.use((cfg) => {
    const traceId = opts.traceIdGenerator?.() ?? createIdempotencyKey();
    cfg.headers['X-Trace-Id'] = traceId;
    cfg.headers['Idempotency-Key'] = createIdempotencyKey();
    // 简化签名：预留扩展点（实际项目接 MD5/HMAC）
    if (opts.signSecret) {
      cfg.headers['X-Merchant-Sign'] = 'signed-stub';
    }
    return cfg;
  });

  // response 拦截器：解包 data，统一错误处理
  ins.interceptors.response.use(
    (resp) => {
      const body = resp.data as ApiResponse;
      if (body && body.code !== '0') {
        const err = new Error(body.message || 'biz error') as AxiosError<ApiResponse>;
        err.code = body.code as unknown as string;
        err.message = body.message;
        opts.onError?.?.(err);
        return Promise.reject(err);
      }
      return body?.data as never;
    },
    (err: AxiosError<ApiResponse>) => {
      opts.onError?.?.(err);
      return Promise.reject(err);
    },
  );

  instance = ins;
  return ins;
}

/** 获取单例 api client（需要先调用 createApi）。 */
export function useApi(): AxiosInstance {
  if (!instance) {
    throw new Error('api client not initialized, call createApi() first');
  }
  return instance;
}

/** 类型友好的 GET 请求。 */
export async function get<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
  return (await useApi().get(url, config)) as T;
}

/** 类型友好的 POST 请求。 */
export async function post<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
  return (await useApi().post(url, data, config)) as T;
}

/** 类型友好的 PUT 请求。 */
export async function put<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
  return (await useApi().put(url, data, config)) as T;
}

/** 类型友好的 DELETE 请求。 */
export async function del<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
  return (await useApi().delete(url, config)) as T;
}