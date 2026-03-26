// Keep fallback values stable across the operation-log table rendering.
export const OPERATION_LOG_EMPTY_FIELD_VALUE = '-'
export const OPERATION_LOG_EMPTY_CHANGE_VALUE = '--'

const SCHEDULE_FIELD_KEY = 'schedule_time'
const NOTIFICATION_CHANNELS_FIELD_KEY = 'notification_channels'
const STATUS_FIELD_KEY = 'status'
const NOTIFICATION_PLATFORM_SEPARATOR = ':'

export interface ScheduleTextFormatter {
  interval: (params: { value: number }) => string
  monthly: (params: { day: number; time: string }) => string
  weekly: (params: { labels: string; time: string }) => string
  daily: (params: { time: string }) => string
  weekdayLabel: (value: number) => string
  notificationPlatformLabel: (value: string) => string
  operationStatusEnabledLabel: () => string
  operationStatusDisabledLabel: () => string
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

export type OperationLogFieldLabelResolver = (
  fieldKey: string,
  fieldLabel: string
) => string

interface ParsedCustomScheduleValue {
  hour?: number
  minute?: number
  interval_minutes?: number
  weekdays?: number[]
  day_of_month?: number
}

const FALLBACK_WEEKDAY_LABELS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

function createDefaultScheduleTextFormatter(): ScheduleTextFormatter {
  return {
    interval: ({ value }) => `每隔 ${value} 分钟`,
    monthly: ({ day, time }) => `每月 ${day} 号 ${time}`,
    weekly: ({ labels, time }) => `每${labels} ${time}`,
    daily: ({ time }) => `每天 ${time}`,
    weekdayLabel: (value) => FALLBACK_WEEKDAY_LABELS[value] || String(value),
    notificationPlatformLabel: (value) => {
      const labels: Record<string, string> = {
        dingtalk: '钉钉',
        feishu: '飞书',
        lark: '飞书',
        qq: 'QQ',
        wechat: '微信',
        wecom: '企微',
      }
      return labels[value.trim().toLowerCase()] || value
    },
    operationStatusEnabledLabel: () => '启用',
    operationStatusDisabledLabel: () => '停用',
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

function formatNotificationChannelsValue(
  value: string,
  formatter: ScheduleTextFormatter = createDefaultScheduleTextFormatter()
) {
  const trimmedValue = value.trim()
  const separatorIndex = trimmedValue.indexOf(NOTIFICATION_PLATFORM_SEPARATOR)
  if (separatorIndex < 0) {
    return trimmedValue
  }

  const rawPlatform = trimmedValue.slice(0, separatorIndex).trim()
  const rawChannelValue = trimmedValue.slice(separatorIndex + 1).trim()
  const localizedPlatform = formatter.notificationPlatformLabel(rawPlatform)

  return `${localizedPlatform}${NOTIFICATION_PLATFORM_SEPARATOR} ${rawChannelValue}`
}

function formatOperationStatusValue(
  value: string,
  formatter: Pick<
    ScheduleTextFormatter,
    'operationStatusEnabledLabel' | 'operationStatusDisabledLabel'
  >
) {
  const normalizedValue = value.trim().toLowerCase()
  if (normalizedValue === '启用' || normalizedValue === 'enabled') {
    return formatter.operationStatusEnabledLabel()
  }
  if (normalizedValue === '停用' || normalizedValue === 'disabled') {
    return formatter.operationStatusDisabledLabel()
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

  if (fieldKey === NOTIFICATION_CHANNELS_FIELD_KEY) {
    return formatNotificationChannelsValue(value, formatter)
  }

  if (fieldKey === STATUS_FIELD_KEY) {
    return formatter ? formatOperationStatusValue(value, formatter) : value
  }

  return value
}

/**
 * Expand each operation log into display rows so every changed field stays aligned.
 */
export function buildOperationLogDisplayRows(
  logs: OperationLogLike[],
  formatter: ScheduleTextFormatter = createDefaultScheduleTextFormatter(),
  fieldLabelResolver?: OperationLogFieldLabelResolver
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
      fieldLabel: fieldLabelResolver
        ? fieldLabelResolver(field.field_key, field.field_label)
        : field.field_label || OPERATION_LOG_EMPTY_FIELD_VALUE,
      beforeValue: formatOperationLogFieldValue(field.field_key, field.before, formatter),
      afterValue: formatOperationLogFieldValue(field.field_key, field.after, formatter),
      logId: log.id,
      showSharedColumns: index === 0,
    }))
  })
}
