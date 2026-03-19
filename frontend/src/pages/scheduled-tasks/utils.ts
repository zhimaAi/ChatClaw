import { formatUtcDateTime } from '@/composables/useDateTime'
import { SCHEDULE_PRESET_LABELS, WEEKDAY_OPTIONS } from './constants'
import type { ScheduledTask, ScheduledTaskFormState } from './types'

export function formatTaskTime(value?: string | Date | null) {
  if (!value) return '-'
  return formatUtcDateTime(value)
}

export function formatDuration(ms: number) {
  if (!ms) return '-'
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${(ms / 60000).toFixed(1)}m`
}

export function describeSchedule(
  task: Pick<ScheduledTask, 'schedule_type' | 'schedule_value' | 'cron_expr'>
) {
  if (task.schedule_type === 'preset') {
    return (
      SCHEDULE_PRESET_LABELS[task.schedule_value as keyof typeof SCHEDULE_PRESET_LABELS] ||
      task.schedule_value
    )
  }
  if (task.schedule_type === 'custom') {
    try {
      const parsed = JSON.parse(task.schedule_value) as {
        hour: number
        minute: number
        interval_minutes?: number
        weekdays?: number[]
        day_of_month?: number
      }
      if (parsed.interval_minutes) {
        return `每隔 ${parsed.interval_minutes} 分钟`
      }
      const hh = String(parsed.hour).padStart(2, '0')
      const mm = String(parsed.minute).padStart(2, '0')
      if (parsed.day_of_month) {
        return `每月 ${parsed.day_of_month} 号 ${hh}:${mm}`
      }
      if (parsed.weekdays?.length) {
        const labels = parsed.weekdays
          .map(
            (value) =>
              WEEKDAY_OPTIONS.find((item) => item.value === value)?.shortLabel || String(value)
          )
          .join(' ')
        return `每周 ${labels} ${hh}:${mm}`
      }
      return `每天 ${hh}:${mm}`
    } catch {
      return task.schedule_value
    }
  }
  return task.cron_expr
}

export function createEmptyForm(): ScheduledTaskFormState {
  return {
    id: null,
    name: '',
    prompt: '',
    agentId: null,
    notificationPlatform: '',
    notificationChannelIds: [],
    enabled: true,
    scheduleType: 'preset',
    schedulePreset: 'every_day_0900',
    customMode: 'daily',
    customHour: 9,
    customMinute: 0,
    customIntervalMinutes: 15,
    customWeekdays: [1, 2, 3, 4, 5],
    customDayOfMonth: 1,
    cronExpr: '0 9 * * *',
  }
}

export function taskToForm(task: ScheduledTask): ScheduledTaskFormState {
  const form = createEmptyForm()
  form.id = task.id
  form.name = task.name
  form.prompt = task.prompt
  form.agentId = task.agent_id
  form.notificationPlatform = (task as any).notification_platform || ''
  form.notificationChannelIds = Array.isArray((task as any).notification_channel_ids)
    ? [...(task as any).notification_channel_ids]
    : []
  form.enabled = task.enabled
  form.scheduleType = task.schedule_type as ScheduledTaskFormState['scheduleType']
  form.cronExpr = task.cron_expr

  if (task.schedule_type === 'preset') {
    form.schedulePreset = (task.schedule_value ||
      'every_day_0900') as ScheduledTaskFormState['schedulePreset']
  }

  if (task.schedule_type === 'custom') {
    try {
      const parsed = JSON.parse(task.schedule_value) as {
        hour: number
        minute: number
        interval_minutes?: number
        weekdays?: number[]
        day_of_month?: number
      }
      if (parsed.interval_minutes) {
        form.customMode = 'interval'
        form.customIntervalMinutes = parsed.interval_minutes
        return form
      }
      form.customHour = parsed.hour ?? 9
      form.customMinute = parsed.minute ?? 0
      if (parsed.day_of_month) {
        form.customMode = 'monthly'
        form.customDayOfMonth = parsed.day_of_month
      } else if (parsed.weekdays?.length) {
        form.customMode = 'weekly'
        form.customWeekdays = parsed.weekdays
      }
    } catch {
      //
    }
  }

  if (task.schedule_type === 'cron') {
    form.cronExpr = task.cron_expr
  }

  return form
}
