import { formatUtcDateTime } from '@/composables/useDateTime'
import {
  buildTimeZoneDateTime,
  toDateTimeInputValue,
} from '@/pages/scheduled-tasks/expirationDate'
import {
  CreateOpenClawCronJobInput,
  UpdateOpenClawCronJobInput,
  type OpenClawCronJob,
  type OpenClawCronRunEntry,
} from '@bindings/chatclaw/internal/openclaw/cron'

// These constants keep schedule descriptions consistent and avoid scattering literals.
const SCHEDULE_PREFIX_EVERY = '每隔'
const SCHEDULE_PREFIX_AT = '指定时间'
const DURATION_SUFFIX_HOUR = 'h'
const DURATION_SUFFIX_MINUTE = 'm'
const DURATION_SUFFIX_SECOND = 's'
const DURATION_SUFFIX_MILLISECOND = 'ms'
const EVERY_UNIT_SECONDS = 'seconds'
const EVERY_UNIT_MINUTES = 'minutes'
const EVERY_UNIT_HOURS = 'hours'
const EVERY_UNIT_DAYS = 'days'
const FALLBACK_TIMEZONE = 'Asia/Shanghai'
const DEFAULT_ONE_TIME_HOUR = 9
const DEFAULT_ONE_TIME_MINUTE = 0
const DEFAULT_ONE_TIME_SECOND = 0
const DEFAULT_OPENCLAW_SESSION_TARGET = 'isolated'
const DEFAULT_OPENCLAW_DELIVERY_MODE = 'announce'
const OPENCLAW_DELIVERY_MODE_NONE = 'none'
// Keep the cron form default aligned with the requested half-hour timeout in milliseconds.
const DEFAULT_OPENCLAW_TIMEOUT_MS = 1800000

export type OpenClawCronScheduleKind = 'cron' | 'every' | 'at' | 'custom'
export type OpenClawCronCustomMode = 'daily' | 'weekly' | 'monthly'
export type OpenClawCronEveryUnit = 'seconds' | 'minutes' | 'hours' | 'days'

export interface OpenClawCronFormState {
  id: string | null
  name: string
  description: string
  agentId: string
  scheduleKind: OpenClawCronScheduleKind
  cronExpr: string
  every: string
  everyValue: number
  everyUnit: OpenClawCronEveryUnit
  at: string
  oneTimeDate: string
  oneTimeHour: number
  oneTimeMinute: number
  oneTimeSecond: number
  timezone: string
  exact: boolean
  message: string
  systemEvent: string
  model: string
  thinking: string
  expectFinal: boolean
  lightContext: boolean
  timeoutMs: number
  sessionTarget: string
  sessionKey: string
  wakeMode: string
  announce: boolean
  channelPlatform: string
  deliveryTargetId: string
  bestEffortDeliver: boolean
  deleteAfterRun: boolean
  keepAfterRun: boolean
  enabled: boolean
  customMode: OpenClawCronCustomMode
  customHour: number
  customMinute: number
  customIntervalMinutes: number
  customWeekdays: number[]
  customDayOfMonth: number
}

// createEmptyOpenClawCronForm keeps the form aligned with OpenClaw-native defaults.
// createEmptyOpenClawCronForm 保持表单默认值与 OpenClaw 原生命令默认值一致。
export function createEmptyOpenClawCronForm(): OpenClawCronFormState {
  const defaultTimezone = resolveSystemTimezone()
  return {
    id: null,
    name: '',
    description: '',
    agentId: '',
    scheduleKind: 'cron',
    cronExpr: '0 9 * * *',
    every: '1h',
    everyValue: 1,
    everyUnit: EVERY_UNIT_HOURS,
    at: '',
    oneTimeDate: '',
    oneTimeHour: DEFAULT_ONE_TIME_HOUR,
    oneTimeMinute: DEFAULT_ONE_TIME_MINUTE,
    oneTimeSecond: DEFAULT_ONE_TIME_SECOND,
    timezone: defaultTimezone,
    exact: false,
    message: '',
    systemEvent: '',
    model: '',
    thinking: 'off',
    expectFinal: false,
    lightContext: false,
    timeoutMs: DEFAULT_OPENCLAW_TIMEOUT_MS,
    sessionTarget: DEFAULT_OPENCLAW_SESSION_TARGET,
    sessionKey: '',
    wakeMode: 'now',
    announce: true,
    channelPlatform: '',
    deliveryTargetId: '',
    bestEffortDeliver: false,
    deleteAfterRun: false,
    keepAfterRun: false,
    enabled: true,
    customMode: 'daily',
    customHour: 9,
    customMinute: 0,
    customIntervalMinutes: 15,
    customWeekdays: [1],
    customDayOfMonth: 1,
  }
}

export function jobToForm(job: OpenClawCronJob): OpenClawCronFormState {
  const parsedEvery = parseEveryValueAndUnit(job.every_ms)
  const parsedCustom = parseCronExprToCustom(job.cron_expr)
  const parsedOneTime = parseOneTimeSchedule(job.at_iso, job.timezone || resolveSystemTimezone())
  return {
    id: job.id,
    name: job.name || '',
    description: job.description || '',
    agentId: job.agent_id || '',
    scheduleKind:
      job.schedule_kind === 'every'
        ? 'every'
        : job.schedule_kind === 'at'
          ? 'at'
          : parsedCustom
            ? 'custom'
            : 'cron',
    cronExpr: job.cron_expr || '0 9 * * *',
    every: job.every_ms ? formatEvery(job.every_ms) : buildEveryDurationValue(1, EVERY_UNIT_HOURS),
    everyValue: parsedEvery.value,
    everyUnit: parsedEvery.unit,
    at: job.at_iso || '',
    oneTimeDate: parsedOneTime.date,
    oneTimeHour: parsedOneTime.hour,
    oneTimeMinute: parsedOneTime.minute,
    oneTimeSecond: parsedOneTime.second,
    timezone: job.timezone || resolveSystemTimezone(),
    exact: !!job.exact,
    message: job.message || '',
    systemEvent: job.system_event || '',
    model: job.model || '',
    thinking: job.thinking || 'off',
    expectFinal: !!job.expect_final,
    lightContext: !!job.light_context,
    timeoutMs: Number(job.timeout_ms || DEFAULT_OPENCLAW_TIMEOUT_MS),
    sessionTarget: job.session_target || DEFAULT_OPENCLAW_SESSION_TARGET,
    sessionKey: job.session_key || '',
    wakeMode: job.wake_mode || 'now',
    announce: !!job.announce,
    channelPlatform: job.delivery_channel || '',
    deliveryTargetId: job.delivery_target_id || job.delivery_to || '',
    bestEffortDeliver: !!job.best_effort_deliver,
    deleteAfterRun: !!job.delete_after_run,
    keepAfterRun: !!job.keep_after_run,
    enabled: !!job.enabled,
    customMode: parsedCustom?.customMode || 'daily',
    customHour: parsedCustom?.customHour ?? 9,
    customMinute: parsedCustom?.customMinute ?? 0,
    customIntervalMinutes: 15,
    customWeekdays: parsedCustom?.customWeekdays ?? [1],
    customDayOfMonth: parsedCustom?.customDayOfMonth ?? 1,
  }
}

export function buildCreateInput(form: OpenClawCronFormState) {
  const schedulePayload = resolveSchedulePayload(form)
  // OpenClaw default / OpenClaw 默认：agentTurn cron tasks should use isolated sessions unless caller overrides it.
  const sessionTarget = form.sessionTarget || DEFAULT_OPENCLAW_SESSION_TARGET
  const deliveryConfig = resolveDeliveryConfig(form)
  return new CreateOpenClawCronJobInput({
    name: form.name,
    description: form.description,
    agent_id: form.agentId,
    schedule_kind: schedulePayload.scheduleKind,
    cron_expr: schedulePayload.cronExpr,
    every: schedulePayload.every,
    at: schedulePayload.at,
    timezone: form.timezone,
    exact: form.exact,
    message: form.message,
    system_event: form.systemEvent,
    model: '',
    thinking: form.thinking,
    expect_final: form.expectFinal,
    light_context: form.lightContext,
    timeout_ms: form.timeoutMs,
    session_target: sessionTarget,
    session_key: form.sessionKey,
    wake_mode: form.wakeMode,
    announce: form.announce,
    delivery_mode: deliveryConfig.mode,
    delivery_target_id: deliveryConfig.targetId,
    delivery_channel: deliveryConfig.channelPlatform,
    best_effort_deliver: form.bestEffortDeliver,
    delete_after_run: form.deleteAfterRun,
    keep_after_run: form.keepAfterRun,
    enabled: form.enabled,
  })
}

export function buildUpdateInput(form: OpenClawCronFormState) {
  const clearedModel = ''
  const schedulePayload = resolveSchedulePayload(form)
  // OpenClaw default / OpenClaw 默认：keep update payload aligned with create payload for isolated-session cron jobs.
  const sessionTarget = form.sessionTarget || DEFAULT_OPENCLAW_SESSION_TARGET
  const deliveryConfig = resolveDeliveryConfig(form)
  return new UpdateOpenClawCronJobInput({
    name: form.name,
    description: form.description,
    agent_id: form.agentId,
    schedule_kind: schedulePayload.scheduleKind,
    cron_expr: schedulePayload.scheduleKind === 'cron' ? schedulePayload.cronExpr : undefined,
    every: schedulePayload.scheduleKind === 'every' ? schedulePayload.every : undefined,
    at: schedulePayload.scheduleKind === 'at' ? schedulePayload.at : undefined,
    timezone: form.timezone || undefined,
    exact: form.exact,
    message: form.message,
    system_event: form.systemEvent,
    model: clearedModel,
    thinking: form.thinking || undefined,
    expect_final: form.expectFinal,
    light_context: form.lightContext,
    timeout_ms: form.timeoutMs,
    session_target: sessionTarget,
    session_key: form.sessionKey || undefined,
    wake_mode: form.wakeMode || undefined,
    announce: form.announce,
    delivery_mode: deliveryConfig.mode,
    delivery_target_id: deliveryConfig.targetId || undefined,
    delivery_channel: deliveryConfig.channelPlatform || undefined,
    best_effort_deliver: form.bestEffortDeliver,
    delete_after_run: form.deleteAfterRun,
    keep_after_run: form.keepAfterRun,
    enabled: form.enabled,
  })
}

function resolveDeliveryConfig(form: OpenClawCronFormState) {
  const channelPlatform = form.channelPlatform.trim()
  const targetId = form.deliveryTargetId.trim()
  // Incomplete delivery settings must fall back to internal-only mode so create/edit stays optional.
  if (!channelPlatform || !targetId) {
    return {
      mode: OPENCLAW_DELIVERY_MODE_NONE,
      channelPlatform: '',
      targetId: '',
    }
  }
  return {
    mode: DEFAULT_OPENCLAW_DELIVERY_MODE,
    channelPlatform,
    targetId,
  }
}

export function formatOpenClawCronTime(value?: number | null) {
  if (!value) return '-'
  return formatUtcDateTime(new Date(value))
}

export function formatDurationMs(ms?: number | null) {
  if (!ms) return '-'
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  if (ms < 3600000) return `${(ms / 60000).toFixed(1)}m`
  return `${(ms / 3600000).toFixed(1)}h`
}

export function describeOpenClawSchedule(job: Pick<OpenClawCronJob, 'schedule_kind' | 'cron_expr' | 'every_ms' | 'at_iso'>) {
  if (job.schedule_kind === 'every' && job.every_ms) {
    return `${SCHEDULE_PREFIX_EVERY} ${formatEvery(job.every_ms)}`
  }
  if (job.schedule_kind === 'at' && job.at_iso) {
    return `${SCHEDULE_PREFIX_AT} ${job.at_iso}`
  }
  const parsedCustom = parseCronExprToCustom(job.cron_expr)
  if (parsedCustom) {
    return describeCustomSchedule(parsedCustom)
  }
  return job.cron_expr || '-'
}

export function displayRunStatus(run: Pick<OpenClawCronRunEntry, 'status' | 'action'>) {
  return run.status || run.action || 'unknown'
}

function formatEvery(ms: number) {
  if (ms % 3600000 === 0) return `${ms / 3600000}${DURATION_SUFFIX_HOUR}`
  if (ms % 60000 === 0) return `${ms / 60000}${DURATION_SUFFIX_MINUTE}`
  if (ms % 1000 === 0) return `${ms / 1000}${DURATION_SUFFIX_SECOND}`
  return `${ms}${DURATION_SUFFIX_MILLISECOND}`
}

function resolveSchedulePayload(form: OpenClawCronFormState) {
  if (form.scheduleKind === 'every') {
    return {
      scheduleKind: 'every' as const,
      cronExpr: '',
      every: buildEveryDurationValue(form.everyValue, form.everyUnit),
      at: '',
    }
  }
  if (form.scheduleKind === 'at') {
    return {
      scheduleKind: 'at' as const,
      cronExpr: '',
      every: '',
      at: buildOneTimeScheduleValue(
        form.oneTimeDate,
        form.oneTimeHour,
        form.oneTimeMinute,
        form.oneTimeSecond,
        form.timezone
      ),
    }
  }
  if (form.scheduleKind === 'custom') {
    return {
      scheduleKind: 'cron' as const,
      cronExpr: buildCronExprFromCustom(form),
      every: '',
      at: '',
    }
  }
  return {
    scheduleKind: 'cron' as const,
    cronExpr: form.cronExpr,
    every: '',
    at: '',
  }
}

function buildOneTimeScheduleValue(
  date: string,
  hour: number,
  minute: number,
  second: number,
  timezone: string
) {
  const value = buildTimeZoneDateTime(date, hour, minute, second, timezone)
  if (!value) return ''
  return value.toISOString().replace('.000Z', 'Z')
}

function parseOneTimeSchedule(value?: string | null, timezone?: string) {
  const parsed = toDateTimeInputValue(value || '', timezone || resolveSystemTimezone())
  if (!parsed) {
    return {
      date: '',
      hour: DEFAULT_ONE_TIME_HOUR,
      minute: DEFAULT_ONE_TIME_MINUTE,
      second: DEFAULT_ONE_TIME_SECOND,
    }
  }
  return parsed
}

function resolveSystemTimezone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || FALLBACK_TIMEZONE
  } catch {
    return FALLBACK_TIMEZONE
  }
}

function buildEveryDurationValue(value: number, unit: OpenClawCronEveryUnit) {
  const normalizedValue = Math.max(1, Math.floor(Number(value) || 1))
  switch (unit) {
    case EVERY_UNIT_SECONDS:
      return `${normalizedValue}s`
    case EVERY_UNIT_MINUTES:
      return `${normalizedValue}m`
    case EVERY_UNIT_DAYS:
      return `${normalizedValue * 24}h`
    case EVERY_UNIT_HOURS:
    default:
      return `${normalizedValue}h`
  }
}

function parseEveryValueAndUnit(everyMs?: number | null) {
  const ms = Number(everyMs || 0)
  if (ms > 0 && ms % 86400000 === 0) {
    return { value: ms / 86400000, unit: EVERY_UNIT_DAYS as OpenClawCronEveryUnit }
  }
  if (ms > 0 && ms % 3600000 === 0) {
    return { value: ms / 3600000, unit: EVERY_UNIT_HOURS as OpenClawCronEveryUnit }
  }
  if (ms > 0 && ms % 60000 === 0) {
    return { value: ms / 60000, unit: EVERY_UNIT_MINUTES as OpenClawCronEveryUnit }
  }
  if (ms > 0 && ms % 1000 === 0) {
    return { value: ms / 1000, unit: EVERY_UNIT_SECONDS as OpenClawCronEveryUnit }
  }
  return { value: 1, unit: EVERY_UNIT_HOURS as OpenClawCronEveryUnit }
}

function buildCronExprFromCustom(form: OpenClawCronFormState) {
  const minute = clampCronField(form.customMinute, 0, 59)
  const hour = clampCronField(form.customHour, 0, 23)

  switch (form.customMode) {
    case 'weekly': {
      const weekday = form.customWeekdays[0] ?? 1
      return `${minute} ${hour} * * ${weekday}`
    }
    case 'monthly':
      return `${minute} ${hour} ${clampCronField(form.customDayOfMonth, 1, 31)} * *`
    case 'daily':
    default:
      return `${minute} ${hour} * * *`
  }
}

function clampCronField(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, Math.floor(Number(value) || min)))
}

function parseCronExprToCustom(cronExpr?: string | null) {
  const expr = String(cronExpr || '').trim()
  const match = /^(\d{1,2})\s+(\d{1,2})\s+(\*|\d{1,2})\s+\*\s+(\*|[\d,]+)$/.exec(expr)
  if (!match) return null

  const minute = Number(match[1])
  const hour = Number(match[2])
  const dayOfMonth = match[3]
  const dayOfWeek = match[4]

  if (dayOfMonth !== '*' && dayOfWeek === '*') {
    return {
      customMode: 'monthly' as OpenClawCronCustomMode,
      customHour: hour,
      customMinute: minute,
      customWeekdays: [1],
      customDayOfMonth: Number(dayOfMonth),
    }
  }

  if (dayOfMonth === '*' && dayOfWeek !== '*') {
    const weekdays = dayOfWeek.split(',').map((item) => Number(item)).filter((item) => !Number.isNaN(item))
    if (weekdays.length === 1) {
      return {
        customMode: 'weekly' as OpenClawCronCustomMode,
        customHour: hour,
        customMinute: minute,
        customWeekdays: weekdays,
        customDayOfMonth: 1,
      }
    }
    return null
  }

  if (dayOfMonth === '*' && dayOfWeek === '*') {
    return {
      customMode: 'daily' as OpenClawCronCustomMode,
      customHour: hour,
      customMinute: minute,
      customWeekdays: [1],
      customDayOfMonth: 1,
    }
  }

  return null
}

function describeCustomSchedule(schedule: {
  customMode: OpenClawCronCustomMode
  customHour: number
  customMinute: number
  customWeekdays: number[]
  customDayOfMonth: number
}) {
  const timeLabel = `${String(schedule.customHour).padStart(2, '0')}:${String(schedule.customMinute).padStart(2, '0')}`
  if (schedule.customMode === 'monthly') {
    return `每月 ${schedule.customDayOfMonth} 号 ${timeLabel}`
  }
  if (schedule.customMode === 'weekly') {
    return `每周 ${weekdayLabel(schedule.customWeekdays[0] ?? 1)} ${timeLabel}`
  }
  return `每天 ${timeLabel}`
}

function weekdayLabel(value: number) {
  switch (value) {
    case 0:
      return '周日'
    case 1:
      return '周一'
    case 2:
      return '周二'
    case 3:
      return '周三'
    case 4:
      return '周四'
    case 5:
      return '周五'
    case 6:
      return '周六'
    default:
      return '周一'
  }
}
