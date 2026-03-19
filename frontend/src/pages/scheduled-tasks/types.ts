import type {
  ScheduledTask,
  ScheduledTaskRun,
  ScheduledTaskRunDetail,
  ScheduledTaskSummary,
} from '@bindings/chatclaw/internal/services/scheduledtasks'
import type { Agent } from '@bindings/chatclaw/internal/services/agents'
import type { Channel } from '@bindings/chatclaw/internal/services/channels'

export type {
  ScheduledTask,
  ScheduledTaskRun,
  ScheduledTaskRunDetail,
  ScheduledTaskSummary,
  Agent,
  Channel,
}

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
