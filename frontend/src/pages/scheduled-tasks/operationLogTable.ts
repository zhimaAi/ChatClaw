// Keep fallback values stable across the operation-log table rendering.
export const OPERATION_LOG_EMPTY_FIELD_VALUE = '-'
export const OPERATION_LOG_EMPTY_CHANGE_VALUE = '--'

const SCHEDULE_FIELD_KEY = 'schedule_time'

export interface ScheduleTextFormatter {
  interval: (params: { value: number }) => string
  monthly: (params: { day: number; time: string }) => string
  weekly: (params: { labels: string; time: string }) => string
  daily: (params: { time: string }) => string
  weekdayLabel: (value: number) => string
}

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

const FALLBACK_WEEKDAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

function createDefaultScheduleTextFormatter(): ScheduleTextFormatter {
  return {
    interval: ({ value }) => `Every ${value} minutes`,
    monthly: ({ day, time }) => `Day ${day} ${time}`,
    weekly: ({ labels, time }) => `Weekly ${labels} ${time}`,
    daily: ({ time }) => `Daily ${time}`,
    weekdayLabel: (value) => FALLBACK_WEEKDAY_LABELS[value] || String(value),
  }
}

function formatHourMinute(hour: number, minute: number) {
  const normalizedHour = String(hour).padStart(2, '0')
  const normalizedMinute = String(minute).padStart(2, '0')
  return `${normalizedHour}:${normalizedMinute}`
}

function formatCustomScheduleValue(
  value: string,
  formatter: ScheduleTextFormatter = createDefaultScheduleTextFormatter()
) {
  try {
    const parsed = JSON.parse(value) as ParsedCustomScheduleValue

    if (parsed.interval_minutes) {
      return formatter.interval({ value: parsed.interval_minutes })
    }

    const hour = parsed.hour ?? 0
    const minute = parsed.minute ?? 0
    const timeLabel = formatHourMinute(hour, minute)

    if (parsed.day_of_month) {
      return formatter.monthly({ day: parsed.day_of_month, time: timeLabel })
    }

    if (Array.isArray(parsed.weekdays) && parsed.weekdays.length > 0) {
      const weekdayLabels = parsed.weekdays
        .map((weekday) => formatter.weekdayLabel(weekday))
        .join(' ')
      return formatter.weekly({ labels: weekdayLabels, time: timeLabel })
    }

    if (typeof parsed.hour === 'number' || typeof parsed.minute === 'number') {
      return formatter.daily({ time: timeLabel })
    }
  } catch {
    // Keep the original value when it is not a custom schedule JSON string.
  }

  return value
}

/**
 * Format changed-field values for the operation-log list without mutating detail data.
 */
export function formatOperationLogFieldValue(
  fieldKey: string,
  value: string,
  formatter?: ScheduleTextFormatter
) {
  if (!value) {
    return OPERATION_LOG_EMPTY_CHANGE_VALUE
  }

  if (fieldKey === SCHEDULE_FIELD_KEY) {
    return formatCustomScheduleValue(value, formatter)
  }

  return value
}

/**
 * Expand each operation log into display rows so every changed field stays aligned.
 */
export function buildOperationLogDisplayRows(
  logs: OperationLogLike[],
  formatter: ScheduleTextFormatter = createDefaultScheduleTextFormatter()
): OperationLogDisplayRow[] {
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
      beforeValue: formatOperationLogFieldValue(field.field_key, field.before, formatter),
      afterValue: formatOperationLogFieldValue(field.field_key, field.after, formatter),
      logId: log.id,
      showSharedColumns: index === 0,
    }))
  })
}
