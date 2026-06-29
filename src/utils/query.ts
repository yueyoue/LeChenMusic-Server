/** 从 Express query 中安全提取字符串 */
export function qs(val: unknown): string | undefined {
  if (typeof val === 'string') return val;
  if (Array.isArray(val)) return val[0];
  return undefined;
}

/** 从 Express query 中安全提取数字 */
export function qn(val: unknown, fallback?: number): number | undefined {
  const s = qs(val);
  if (s === undefined) return fallback;
  const n = parseInt(s, 10);
  return isNaN(n) ? fallback : n;
}

/** 从 Express params 中安全提取字符串 */
export function ps(val: unknown): string {
  if (typeof val === 'string') return val;
  if (Array.isArray(val)) return val[0];
  return '';
}
