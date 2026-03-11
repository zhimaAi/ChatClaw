import type { SchedulePresetValue } from './types'

export const SCHEDULE_PRESET_LABELS: Record<SchedulePresetValue, string> = {
  every_minute: '每分钟',
  every_5_minutes: '每 5 分钟',
  every_15_minutes: '每 15 分钟',
  every_hour: '每小时',
  every_day_0900: '每天上午 9 点',
  every_day_1800: '每天下午 6 点',
  weekdays_0900: '工作日上午 9 点',
  every_monday_0900: '每周一上午 9 点',
  every_month_1_0900: '每月（1号上午 9 点）',
}

export const SCHEDULE_PRESETS: Array<{ value: SchedulePresetValue; label: string }> = [
  { value: 'every_minute', label: '每分钟' },
  { value: 'every_5_minutes', label: '每 5 分钟' },
  { value: 'every_15_minutes', label: '每 15 分钟' },
  { value: 'every_hour', label: '每小时' },
  { value: 'every_day_0900', label: '每天上午 9 点' },
  { value: 'every_day_1800', label: '每天下午 6 点' },
  { value: 'every_monday_0900', label: '每周一上午 9 点' },
  { value: 'every_month_1_0900', label: '每月（1号上午 9 点）' },
]

export const WEEKDAY_OPTIONS = [
  { value: 0, label: '星期日', shortLabel: '周日' },
  { value: 1, label: '星期一', shortLabel: '周一' },
  { value: 2, label: '星期二', shortLabel: '周二' },
  { value: 3, label: '星期三', shortLabel: '周三' },
  { value: 4, label: '星期四', shortLabel: '周四' },
  { value: 5, label: '星期五', shortLabel: '周五' },
  { value: 6, label: '星期六', shortLabel: '周六' },
]
