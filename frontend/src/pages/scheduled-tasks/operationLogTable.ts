// Keep fallback values stable across the operation-log table rendering.
export const OPERATION_LOG_EMPTY_FIELD_VALUE = '-'
export const OPERATION_LOG_EMPTY_CHANGE_VALUE = '--'

// Weekday labels are fixed here because the operation-log page currently uses Chinese copy.
const WEEKDAY_LABELS: Record<number, string> = {
  0: '周日',
  1: '周一',
  2: '周二',
  3: '周三',
  4: '周四',
  5: '周五',
  6: '周六',
}

const SCHEDULE_FIELD_KEY = 'schedule_time'

export interface OperationLogChangedFieldLike {
  field_key: string
  field_label: string
  before: string
  after: string
}

export interface OperationLogLike {
  id: number
  task_name_snapshot: string
  operation_type: string
  operation_source: string
  created_at: string | Date
  changed_fields: OperationLogChangedFieldLike[]
}

export interface OperationLogDisplayRow {
  rowKey: string
  taskName: string
  operationType: string
  operationSource: string
  createdAt: string | Date
  fieldLabel: string
  beforeValue: string
  afterValue: string
  logId: number
  showSharedColumns: boolean
}

interface ParsedCustomScheduleValue {
  hour?: number
  minute?: number
  interval_minutes?: number
  weekdays?: number[]
  day_of_month?: number
}

function formatHourMinute(hour: number, minute: number) {
  const normalizedHour = String(hour).padStart(2, '0')
  const normalizedMinute = String(minute).padStart(2, '0')
  return `${normalizedHour}:${normalizedMinute}`
}

function formatCustomScheduleValue(value: string) {
  try {
    const parsed = JSON.parse(value) as ParsedCustomScheduleValue

    if (parsed.interval_minutes) {
      return `每 ${parsed.interval_minutes} 分钟`
    }

    const hour = parsed.hour ?? 0
    const minute = parsed.minute ?? 0
    const timeLabel = formatHourMinute(hour, minute)

    if (parsed.day_of_month) {
      return `每月 ${parsed.day_of_month} 号 ${timeLabel}`
    }

    if (Array.isArray(parsed.weekdays) && parsed.weekdays.length > 0) {
      const weekdayLabels = parsed.weekdays
        .map((weekday) => WEEKDAY_LABELS[weekday] || String(weekday))
        .join(' ')
      return `每${weekdayLabels} ${timeLabel}`
    }

    if (typeof parsed.hour === 'number' || typeof parsed.minute === 'number') {
      return `每天 ${timeLabel}`
    }
  } catch {
    // Keep the original value when it is not a custom schedule JSON string.
  }

  return value
}

/**
 * Format changed-field values for the operation-log list without mutating detail data.
 */
export function formatOperationLogFieldValue(fieldKey: string, value: string) {
  if (!value) {
    return OPERATION_LOG_EMPTY_CHANGE_VALUE
  }

  if (fieldKey === SCHEDULE_FIELD_KEY) {
    return formatCustomScheduleValue(value)
  }

  return value
}

/**
 * Expand each operation log into display rows so every changed field stays aligned.
 */
export function buildOperationLogDisplayRows(logs: OperationLogLike[]): OperationLogDisplayRow[] {
  return logs.flatMap((log) => {
    const changedFields = Array.isArray(log.changed_fields) ? log.changed_fields : []
    const normalizedFields =
      changedFields.length > 0
        ? changedFields
        : [
            {
              field_key: '',
              field_label: OPERATION_LOG_EMPTY_FIELD_VALUE,
              before: OPERATION_LOG_EMPTY_CHANGE_VALUE,
              after: OPERATION_LOG_EMPTY_CHANGE_VALUE,
            },
          ]

    return normalizedFields.map((field, index) => ({
      rowKey: `${log.id}-${field.field_key || 'empty'}-${index}`,
      taskName: index === 0 ? log.task_name_snapshot || OPERATION_LOG_EMPTY_FIELD_VALUE : '',
      operationType: index === 0 ? log.operation_type : '',
      operationSource: index === 0 ? log.operation_source : '',
      createdAt: index === 0 ? log.created_at : '',
      fieldLabel: field.field_label || OPERATION_LOG_EMPTY_FIELD_VALUE,
      beforeValue: formatOperationLogFieldValue(field.field_key, field.before),
      afterValue: formatOperationLogFieldValue(field.field_key, field.after),
      logId: log.id,
      showSharedColumns: index === 0,
    }))
  })
}
