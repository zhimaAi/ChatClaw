import type { SchedulePresetValue } from './types'

export const SCHEDULE_PRESETS: Array<{ value: SchedulePresetValue; label: string }> = [
  { value: 'every_hour', label: '每小时整点' },
  { value: 'every_day_0900', label: '每天 09:00' },
  { value: 'weekdays_0900', label: '工作日 09:00' },
  { value: 'every_monday_0900', label: '每周一 09:00' },
]

export const WEEKDAY_OPTIONS = [
  { value: 1, label: '一' },
  { value: 2, label: '二' },
  { value: 3, label: '三' },
  { value: 4, label: '四' },
  { value: 5, label: '五' },
  { value: 6, label: '六' },
  { value: 0, label: '日' },
]
