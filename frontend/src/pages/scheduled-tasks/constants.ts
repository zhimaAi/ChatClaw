import type { SchedulePresetValue } from './types'

// Default suffix appended when users duplicate a scheduled task from the list.
export const TASK_COPY_NAME_SUFFIX = '副本'

export const SCHEDULE_PRESET_LABELS: Record<SchedulePresetValue, string> = {
  every_minute: 'scheduledTasks.presets.everyMinute',
  every_5_minutes: 'scheduledTasks.presets.every5Minutes',
  every_15_minutes: 'scheduledTasks.presets.every15Minutes',
  every_hour: 'scheduledTasks.presets.everyHour',
  every_day_0900: 'scheduledTasks.presets.everyDay0900',
  every_day_1800: 'scheduledTasks.presets.everyDay1800',
  weekdays_0900: 'scheduledTasks.presets.weekdays0900',
  every_monday_0900: 'scheduledTasks.presets.everyMonday0900',
  every_month_1_0900: 'scheduledTasks.presets.everyMonth10900',
}

export const SCHEDULE_PRESETS: Array<{ value: SchedulePresetValue; labelKey: string }> = [
  { value: 'every_minute', labelKey: 'scheduledTasks.presets.everyMinute' },
  { value: 'every_5_minutes', labelKey: 'scheduledTasks.presets.every5Minutes' },
  { value: 'every_15_minutes', labelKey: 'scheduledTasks.presets.every15Minutes' },
  { value: 'every_hour', labelKey: 'scheduledTasks.presets.everyHour' },
  { value: 'every_day_0900', labelKey: 'scheduledTasks.presets.everyDay0900' },
  { value: 'every_day_1800', labelKey: 'scheduledTasks.presets.everyDay1800' },
  { value: 'weekdays_0900', labelKey: 'scheduledTasks.presets.weekdays0900' },
  { value: 'every_monday_0900', labelKey: 'scheduledTasks.presets.everyMonday0900' },
  { value: 'every_month_1_0900', labelKey: 'scheduledTasks.presets.everyMonth10900' },
]

export const WEEKDAY_OPTIONS = [
  {
    value: 0,
    labelKey: 'scheduledTasks.weekdays.sunday',
    shortLabelKey: 'scheduledTasks.weekdaysShort.sunday',
  },
  {
    value: 1,
    labelKey: 'scheduledTasks.weekdays.monday',
    shortLabelKey: 'scheduledTasks.weekdaysShort.monday',
  },
  {
    value: 2,
    labelKey: 'scheduledTasks.weekdays.tuesday',
    shortLabelKey: 'scheduledTasks.weekdaysShort.tuesday',
  },
  {
    value: 3,
    labelKey: 'scheduledTasks.weekdays.wednesday',
    shortLabelKey: 'scheduledTasks.weekdaysShort.wednesday',
  },
  {
    value: 4,
    labelKey: 'scheduledTasks.weekdays.thursday',
    shortLabelKey: 'scheduledTasks.weekdaysShort.thursday',
  },
  {
    value: 5,
    labelKey: 'scheduledTasks.weekdays.friday',
    shortLabelKey: 'scheduledTasks.weekdaysShort.friday',
  },
  {
    value: 6,
    labelKey: 'scheduledTasks.weekdays.saturday',
    shortLabelKey: 'scheduledTasks.weekdaysShort.saturday',
  },
] as const
