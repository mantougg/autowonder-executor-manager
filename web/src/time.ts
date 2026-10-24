// time.ts：本地时区时间格式化（时间带 tooltip/数据起点用；等宽 12px 展示）。
function pad(n: number): string {
  return String(n).padStart(2, '0')
}

/** 本地时间 YYYY-MM-DD HH:mm（数据起点提示用）。 */
export function fmtLocalMinute(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 本地时间 YYYY-MM-DD HH:mm:ss（区间起止 tooltip 用）。 */
export function fmtLocal(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return `${fmtLocalMinute(iso)}:${pad(d.getSeconds())}`
}
