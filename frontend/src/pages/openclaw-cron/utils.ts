import { formatUtcDateTime } from '@/composables/useDateTime'
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

export interface OpenClawCronFormState {
  id: string | null
  name: string
  description: string
  agentId: string
  scheduleKind: 'cron' | 'every' | 'at'
  cronExpr: string
  every: string
  at: string
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
  deliveryChannel: string
  deliveryTo: string
  deliveryAccountId: string
  bestEffortDeliver: boolean
  deleteAfterRun: boolean
  keepAfterRun: boolean
  enabled: boolean
}

// createEmptyOpenClawCronForm keeps the form aligned with OpenClaw-native defaults.
// createEmptyOpenClawCronForm 保持表单默认值与 OpenClaw 原生命令默认值一致。
export function createEmptyOpenClawCronForm(): OpenClawCronFormState {
  return {
    id: null,
    name: '',
    description: '',
    agentId: '',
    scheduleKind: 'cron',
    cronExpr: '0 9 * * *',
    every: '1h',
    at: '',
    timezone: '',
    exact: false,
    message: '',
    systemEvent: '',
    model: '',
    thinking: 'off',
    expectFinal: false,
    lightContext: false,
    timeoutMs: 30000,
    sessionTarget: 'isolated',
    sessionKey: '',
    wakeMode: 'now',
    announce: true,
    deliveryChannel: 'last',
    deliveryTo: '',
    deliveryAccountId: '',
    bestEffortDeliver: false,
    deleteAfterRun: false,
    keepAfterRun: false,
    enabled: true,
  }
}

export function jobToForm(job: OpenClawCronJob): OpenClawCronFormState {
  return {
    id: job.id,
    name: job.name || '',
    description: job.description || '',
    agentId: job.agent_id || '',
    scheduleKind: (job.schedule_kind as OpenClawCronFormState['scheduleKind']) || 'cron',
    cronExpr: job.cron_expr || '0 9 * * *',
    every: job.every_ms ? formatEvery(job.every_ms) : '1h',
    at: job.at_iso || '',
    timezone: job.timezone || '',
    exact: !!job.exact,
    message: job.message || '',
    systemEvent: job.system_event || '',
    model: job.model || '',
    thinking: job.thinking || 'off',
    expectFinal: !!job.expect_final,
    lightContext: !!job.light_context,
    timeoutMs: Number(job.timeout_ms || 30000),
    sessionTarget: job.session_target || 'isolated',
    sessionKey: job.session_key || '',
    wakeMode: job.wake_mode || 'now',
    announce: !!job.announce,
    deliveryChannel: job.delivery_channel || 'last',
    deliveryTo: job.delivery_to || '',
    deliveryAccountId: job.delivery_account_id || '',
    bestEffortDeliver: !!job.best_effort_deliver,
    deleteAfterRun: !!job.delete_after_run,
    keepAfterRun: !!job.keep_after_run,
    enabled: !!job.enabled,
  }
}

export function buildCreateInput(form: OpenClawCronFormState) {
  return new CreateOpenClawCronJobInput({
    name: form.name,
    description: form.description,
    agent_id: form.agentId,
    schedule_kind: form.scheduleKind,
    cron_expr: form.cronExpr,
    every: form.every,
    at: form.at,
    timezone: form.timezone,
    exact: form.exact,
    message: form.message,
    system_event: form.systemEvent,
    model: '',
    thinking: form.thinking,
    expect_final: form.expectFinal,
    light_context: form.lightContext,
    timeout_ms: form.timeoutMs,
    session_target: form.sessionTarget,
    session_key: form.sessionKey,
    wake_mode: form.wakeMode,
    announce: form.announce,
    delivery_channel: form.deliveryChannel,
    delivery_to: form.deliveryTo,
    delivery_account_id: form.deliveryAccountId,
    best_effort_deliver: form.bestEffortDeliver,
    delete_after_run: form.deleteAfterRun,
    keep_after_run: form.keepAfterRun,
    enabled: form.enabled,
  })
}

export function buildUpdateInput(form: OpenClawCronFormState) {
  const clearedModel = ''
  return new UpdateOpenClawCronJobInput({
    name: form.name,
    description: form.description,
    agent_id: form.agentId,
    schedule_kind: form.scheduleKind,
    cron_expr: form.scheduleKind === 'cron' ? form.cronExpr : undefined,
    every: form.scheduleKind === 'every' ? form.every : undefined,
    at: form.scheduleKind === 'at' ? form.at : undefined,
    timezone: form.timezone || undefined,
    exact: form.exact,
    message: form.message,
    system_event: form.systemEvent,
    model: clearedModel,
    thinking: form.thinking || undefined,
    expect_final: form.expectFinal,
    light_context: form.lightContext,
    timeout_ms: form.timeoutMs,
    session_target: form.sessionTarget || undefined,
    session_key: form.sessionKey || undefined,
    wake_mode: form.wakeMode || undefined,
    announce: form.announce,
    delivery_channel: form.deliveryChannel || undefined,
    delivery_to: form.deliveryTo || undefined,
    delivery_account_id: form.deliveryAccountId || undefined,
    best_effort_deliver: form.bestEffortDeliver,
    delete_after_run: form.deleteAfterRun,
    keep_after_run: form.keepAfterRun,
    enabled: form.enabled,
  })
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
