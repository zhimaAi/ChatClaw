import type {
  ScheduledTask,
  ScheduledTaskRun,
  ScheduledTaskRunDetail,
  ScheduledTaskSummary,
} from '@bindings/chatclaw/internal/services/scheduledtasks'
import type { Agent } from '@bindings/chatclaw/internal/services/agents'

export type { ScheduledTask, ScheduledTaskRun, ScheduledTaskRunDetail, ScheduledTaskSummary, Agent }

export type SchedulePresetValue =
  | 'every_hour'
  | 'every_day_0900'
  | 'weekdays_0900'
  | 'every_monday_0900'

export type ScheduleCustomMode = 'daily' | 'weekly' | 'monthly'

export interface ScheduledTaskFormState {
  id: number | null
  name: string
  prompt: string
  agentId: number | null
  enabled: boolean
  scheduleType: 'preset' | 'custom' | 'cron'
  schedulePreset: SchedulePresetValue
  customMode: ScheduleCustomMode
  customHour: number
  customMinute: number
  customWeekdays: number[]
  customDayOfMonth: number
  cronExpr: string
}
