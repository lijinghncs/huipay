// 共享工具函数

/** 分 → 元 格式化（人民币）。 */
export function formatCents(cents: number, withSymbol = true): string {
  const yuan = (cents / 100).toFixed(2);
  return withSymbol ? `¥${yuan}` : yuan;
}

/** ISO 时间 → "YYYY-MM-DD HH:mm:ss"。 */
export function formatDateTime(iso?: string): string {
  if (!iso) return '-';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

/** 生成雪花 ID 骨架（前端仅用于占位，生产由后端生成）。 */
export function genIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/** 金额按比例拆分，尾差归到最后一项。 */
export function splitByRatio(amount: number, ratios: number[]): number[] {
  const total = ratios.reduce((a, b) => a + b, 0);
  const result = ratios.map((r) => Math.floor((amount * r) / total));
  const sum = result.reduce((a, b) => a + b, 0);
  const idx = result.length - 1;
  result[idx] += amount - sum;
  return result;
}