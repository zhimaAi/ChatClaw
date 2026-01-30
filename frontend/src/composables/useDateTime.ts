export type FormatUtcDateTimeOptions = {
  /**
   * IANA 时区名，例如 "Asia/Shanghai"
   * - 不传则使用系统默认时区
   */
  timeZone?: string
}

function parseUtcStringLikeSQLite(input: string): Date | null {
  const m = input.match(
    /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,3}))?$/,
  )
  if (!m) return null

  const year = Number(m[1])
  const month = Number(m[2])
  const day = Number(m[3])
  const hour = Number(m[4])
  const minute = Number(m[5])
  const second = Number(m[6])
  const msRaw = m[7] ?? '0'
  const ms = Number(msRaw.padEnd(3, '0'))

  const t = Date.UTC(year, month - 1, day, hour, minute, second, ms)
  const d = new Date(t)
  return Number.isNaN(d.getTime()) ? null : d
}

function toDate(input: string | number | Date): Date {
  if (input instanceof Date) return input
  if (typeof input === 'string') {
    const trimmed = input.trim()
    const parsed = parseUtcStringLikeSQLite(trimmed)
    if (parsed) return parsed
    // 兜底：支持 RFC3339（带 Z/+08:00 等）或浏览器可解析的格式
    return new Date(trimmed)
  }
  return new Date(input)
}

/**
 * 把「UTC 时间」格式化为本地（或指定时区）可直接展示的字符串：YYYY-MM-DD HH:mm:ss
 *
 * 约定：
 * - input 可以是后端返回的 "YYYY-MM-DD HH:mm:ss"（约定为 UTC）
 * - 也兼容 RFC3339（例如 2026-01-30T03:56:29Z）
 * - 数据库存储 UTC、不带时区信息没关系；展示时统一在前端转本地/指定时区
 */
export function formatUtcDateTime(
  input: string | number | Date,
  opts: FormatUtcDateTimeOptions = {},
): string {
  const date = toDate(input)
  if (Number.isNaN(date.getTime())) return ''

  const dtf = new Intl.DateTimeFormat('en-CA', {
    timeZone: opts.timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  })

  const parts = dtf.formatToParts(date)
  const map: Record<string, string> = {}
  for (const p of parts) {
    if (p.type !== 'literal') map[p.type] = p.value
  }
  const y = map.year ?? ''
  const m = map.month ?? ''
  const d = map.day ?? ''
  const hh = map.hour ?? ''
  const mm = map.minute ?? ''
  const ss = map.second ?? ''
  if (!y || !m || !d || !hh || !mm || !ss) return ''
  return `${y}-${m}-${d} ${hh}:${mm}:${ss}`
}

export function useDateTime() {
  return {
    formatUtcDateTime,
  }
}

