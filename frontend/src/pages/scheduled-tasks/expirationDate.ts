// Scheduled task expiration should use one stable fallback timezone when the task record has no explicit zone.
export const DEFAULT_SCHEDULED_TASK_TIMEZONE = 'UTC'

const EXPIRATION_END_OF_DAY_HOUR = 23
const EXPIRATION_END_OF_DAY_MINUTE = 59
const EXPIRATION_END_OF_DAY_SECOND = 59
const TIME_INPUT_DEFAULT_SECOND = 0

type TimeZoneDateParts = {
  year: string
  month: string
  day: string
  hour?: string
  minute?: string
  second?: string
}

function normalizeScheduledTaskTimezone(timezone?: string | null) {
  const value = String(timezone || '').trim()
  return value || DEFAULT_SCHEDULED_TASK_TIMEZONE
}

function getTimeZoneDateParts(date: Date, timezone?: string | null): TimeZoneDateParts | null {
  if (Number.isNaN(date.getTime())) return null

  const formatter = new Intl.DateTimeFormat('en-CA', {
    timeZone: normalizeScheduledTaskTimezone(timezone),
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  })

  const parts = formatter.formatToParts(date)
  const map: Partial<TimeZoneDateParts> = {}
  for (const part of parts) {
    if (part.type === 'literal') continue
    if (
      part.type === 'year' ||
      part.type === 'month' ||
      part.type === 'day' ||
      part.type === 'hour' ||
      part.type === 'minute' ||
      part.type === 'second'
    ) {
      map[part.type] = part.value
    }
  }
  if (!map.year || !map.month || !map.day) return null
  return map as TimeZoneDateParts
}

function getTimeZoneOffsetMilliseconds(date: Date, timezone?: string | null) {
  const parts = getTimeZoneDateParts(date, timezone)
  if (!parts?.hour || !parts.minute || !parts.second) return 0

  const utcTimestamp = Date.UTC(
    Number(parts.year),
    Number(parts.month) - 1,
    Number(parts.day),
    Number(parts.hour),
    Number(parts.minute),
    Number(parts.second)
  )
  return utcTimestamp - date.getTime()
}

function buildTimeZoneDate(
  year: number,
  month: number,
  day: number,
  hour: number,
  minute: number,
  second: number,
  timezone?: string | null
) {
  const normalizedTimezone = normalizeScheduledTaskTimezone(timezone)
  const utcGuess = Date.UTC(year, month - 1, day, hour, minute, second)
  const offset = getTimeZoneOffsetMilliseconds(new Date(utcGuess), normalizedTimezone)
  let timestamp = utcGuess - offset
  const adjustedOffset = getTimeZoneOffsetMilliseconds(new Date(timestamp), normalizedTimezone)
  if (adjustedOffset !== offset) {
    timestamp = utcGuess - adjustedOffset
  }
  return new Date(timestamp)
}

export function formatDateOnly(value?: string | Date | null, timezone?: string | null) {
  if (!value) return '-'
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const parts = getTimeZoneDateParts(date, timezone)
  if (!parts) return '-'
  return `${parts.year}-${parts.month}-${parts.day}`
}

export function toDateInputValue(value?: string | Date | null, timezone?: string | null) {
  if (!value) return ''
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const parts = getTimeZoneDateParts(date, timezone)
  if (!parts) return ''
  return `${parts.year}-${parts.month}-${parts.day}`
}

export function buildExpirationDateTime(dateOnly: string, timezone?: string | null) {
  const value = dateOnly.trim()
  if (!value) return null
  const [year, month, day] = value.split('-').map((item) => Number(item))
  if (!year || !month || !day) return null
  return buildTimeZoneDate(
    year,
    month,
    day,
    EXPIRATION_END_OF_DAY_HOUR,
    EXPIRATION_END_OF_DAY_MINUTE,
    EXPIRATION_END_OF_DAY_SECOND,
    timezone
  )
}

export function buildTimeZoneDateTime(
  dateOnly: string,
  hour: number,
  minute: number,
  second: number,
  timezone?: string | null
) {
  const value = dateOnly.trim()
  if (!value) return null
  const [year, month, day] = value.split('-').map((item) => Number(item))
  if (!year || !month || !day) return null
  return buildTimeZoneDate(
    year,
    month,
    day,
    Math.min(23, Math.max(0, Math.floor(Number(hour) || 0))),
    Math.min(59, Math.max(0, Math.floor(Number(minute) || 0))),
    Math.min(59, Math.max(0, Math.floor(Number(second) || 0))),
    timezone
  )
}

export function toDateTimeInputValue(value?: string | Date | null, timezone?: string | null) {
  if (!value) return null
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return null
  const parts = getTimeZoneDateParts(date, timezone)
  if (!parts?.hour || !parts.minute) return null
  return {
    date: `${parts.year}-${parts.month}-${parts.day}`,
    hour: Number(parts.hour),
    minute: Number(parts.minute),
    second: Number(parts.second || TIME_INPUT_DEFAULT_SECOND),
  }
}

export function isExpirationDateExpired(
  dateOnly: string,
  timezone?: string | null,
  now: Date = new Date()
) {
  if (!dateOnly) return false
  const expiresAt = buildExpirationDateTime(dateOnly, timezone)
  if (!expiresAt || Number.isNaN(expiresAt.getTime())) return false
  return expiresAt.getTime() <= now.getTime()
}
