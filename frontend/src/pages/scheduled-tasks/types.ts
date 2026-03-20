import type {
  ScheduledTask as BindingScheduledTask,
  ScheduledTaskOperationLog as BindingScheduledTaskOperationLog,
  ScheduledTaskOperationLogDetail as BindingScheduledTaskOperationLogDetail,
  ScheduledTaskOperationSnapshot as BindingScheduledTaskOperationSnapshot,
  ScheduledTaskRun as BindingScheduledTaskRun,
  ScheduledTaskRunDetail as BindingScheduledTaskRunDetail,
  ScheduledTaskSummary as BindingScheduledTaskSummary,
} from '@bindings/chatclaw/internal/services/scheduledtasks'
import type { Agent } from '@bindings/chatclaw/internal/services/agents'
import type { Channel } from '@bindings/chatclaw/internal/services/channels'

export type ScheduledTask = BindingScheduledTask & {
  expires_at?: string | Date | null
  is_expired?: boolean
}

export type ScheduledTaskOperationSnapshot = BindingScheduledTaskOperationSnapshot & {
  expires_at?: string | Date | null
  is_expired?: boolean
}

export type ScheduledTaskOperationLog = BindingScheduledTaskOperationLog
export type ScheduledTaskOperationLogDetail = BindingScheduledTaskOperationLogDetail
export type ScheduledTaskRun = BindingScheduledTaskRun
export type ScheduledTaskRunDetail = BindingScheduledTaskRunDetail
export type ScheduledTaskSummary = BindingScheduledTaskSummary
export type { Agent, Channel }

export type SchedulePresetValue =
  | 'every_minute'
  | 'every_5_minutes'
  | 'every_15_minutes'
  | 'every_hour'
  | 'every_day_0900'
  | 'every_day_1800'
  | 'weekdays_0900'
  | 'every_monday_0900'
  | 'every_month_1_0900'

export type ScheduleCustomMode = 'interval' | 'daily' | 'weekly' | 'monthly'

export interface ScheduledTaskFormState {
  id: number | null
  name: string
  prompt: string
  agentId: number | null
  expiresAtDate: string
  isExpired: boolean
  notificationPlatform: string
  notificationChannelIds: number[]
  enabled: boolean
  scheduleType: 'preset' | 'custom' | 'cron'
  schedulePreset: SchedulePresetValue
  customMode: ScheduleCustomMode
  customHour: number
  customMinute: number
  customIntervalMinutes: number
  customWeekdays: number[]
  customDayOfMonth: number
  cronExpr: string
}
