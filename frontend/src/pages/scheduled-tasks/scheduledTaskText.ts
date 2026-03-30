export type ScheduledTaskTranslate = (key: string, params?: Record<string, unknown>) => string

export interface ScheduleTextFormatter {
  interval: (params: { value: number }) => string
  monthly: (params: { day: number; time: string }) => string
  weekly: (params: { labels: string; time: string }) => string
  daily: (params: { time: string }) => string
  weekdayLabel: (value: number) => string
  notificationPlatformLabel: (value: string) => string
  notificationOnDemandLabel: () => string
  operationStatusEnabledLabel: () => string
  operationStatusDisabledLabel: () => string
}

const FALLBACK_TEXT: Record<string, string> = {
  'scheduledTasks.copySuffix': ' Copy',
  'scheduledTasks.describe.interval': 'Every {value} minutes',
  'scheduledTasks.describe.monthly': 'Day {day} {time}',
  'scheduledTasks.describe.weekly': 'Weekly {labels} {time}',
  'scheduledTasks.describe.daily': 'Daily {time}',
  'scheduledTasks.weekdaysShort.sunday': 'Sun',
  'scheduledTasks.weekdaysShort.monday': 'Mon',
  'scheduledTasks.weekdaysShort.tuesday': 'Tue',
  'scheduledTasks.weekdaysShort.wednesday': 'Wed',
  'scheduledTasks.weekdaysShort.thursday': 'Thu',
  'scheduledTasks.weekdaysShort.friday': 'Fri',
  'scheduledTasks.weekdaysShort.saturday': 'Sat',
  'scheduledTasks.operationLog.types.create': 'Create Task',
  'scheduledTasks.operationLog.types.delete': 'Delete Task',
  'scheduledTasks.operationLog.types.update': 'Update Task',
  'scheduledTasks.operationLog.sources.ai': 'AI Assistant',
  'scheduledTasks.operationLog.sources.manual': 'Manual',
  'scheduledTasks.operationLog.notification.onDemand': 'On demand',
  'scheduledTasks.operationLog.fields.status': 'Status',
  'scheduledTasks.operationLog.fields.name': 'Name',
  'scheduledTasks.operationLog.fields.prompt': 'Prompt',
  'scheduledTasks.operationLog.fields.agent': 'Assistant',
  'scheduledTasks.operationLog.fields.notificationChannels': 'Notification Channels',
  'scheduledTasks.operationLog.fields.scheduleTime': 'Schedule Time',
  'scheduledTasks.operationLog.status.enabled': 'Enabled',
  'scheduledTasks.operationLog.status.disabled': 'Disabled',
}

const OPERATION_LOG_FIELD_TRANSLATION_KEYS: Record<string, string> = {
  status: 'scheduledTasks.operationLog.fields.status',
  name: 'scheduledTasks.operationLog.fields.name',
  prompt: 'scheduledTasks.operationLog.fields.prompt',
  agent: 'scheduledTasks.operationLog.fields.agent',
  notification_channels: 'scheduledTasks.operationLog.fields.notificationChannels',
  schedule_time: 'scheduledTasks.operationLog.fields.scheduleTime',
}

// Keep platform aliases stable so operation-log snapshots can reuse the same translation keys.
const NOTIFICATION_PLATFORM_TRANSLATION_KEYS: Record<string, string> = {
  dingtalk: 'channels.platforms.dingtalk',
  feishu: 'channels.platforms.feishu',
  lark: 'channels.platforms.feishu',
  qq: 'channels.platforms.qq',
  wechat: 'channels.platforms.wechat',
  wecom: 'channels.platforms.wecom',
}

function interpolateText(template: string, params?: Record<string, unknown>) {
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (_, key) => String(params[key] ?? `{${key}}`))
}

function resolveTranslate(translate?: ScheduledTaskTranslate): ScheduledTaskTranslate {
  if (translate) {
    return (key: string, params?: Record<string, unknown>) => {
      const translated = translate(key, params)
      if (translated === key && FALLBACK_TEXT[key]) {
        return interpolateText(FALLBACK_TEXT[key], params)
      }
      return translated
    }
  }

  return (key: string, params?: Record<string, unknown>) =>
    interpolateText(FALLBACK_TEXT[key] || key, params)
}

export function createScheduleTextFormatter(
  translate?: ScheduledTaskTranslate
): ScheduleTextFormatter {
  const t = resolveTranslate(translate)

  return {
    interval: ({ value }) => t('scheduledTasks.describe.interval', { value }),
    monthly: ({ day, time }) => t('scheduledTasks.describe.monthly', { day, time }),
    weekly: ({ labels, time }) => t('scheduledTasks.describe.weekly', { labels, time }),
    daily: ({ time }) => t('scheduledTasks.describe.daily', { time }),
    weekdayLabel: (value) =>
      t(
        [
          'scheduledTasks.weekdaysShort.sunday',
          'scheduledTasks.weekdaysShort.monday',
          'scheduledTasks.weekdaysShort.tuesday',
          'scheduledTasks.weekdaysShort.wednesday',
          'scheduledTasks.weekdaysShort.thursday',
          'scheduledTasks.weekdaysShort.friday',
          'scheduledTasks.weekdaysShort.saturday',
        ][value] || String(value)
      ),
    notificationPlatformLabel: (value) => {
      const normalizedValue = value.trim().toLowerCase()
      const translationKey = NOTIFICATION_PLATFORM_TRANSLATION_KEYS[normalizedValue]
      return translationKey ? t(translationKey) : value
    },
    notificationOnDemandLabel: () => t('scheduledTasks.operationLog.notification.onDemand'),
    operationStatusEnabledLabel: () => t('scheduledTasks.operationLog.status.enabled'),
    operationStatusDisabledLabel: () => t('scheduledTasks.operationLog.status.disabled'),
  }
}

export function getScheduledTaskCopySuffix(translate?: ScheduledTaskTranslate) {
  return resolveTranslate(translate)('scheduledTasks.copySuffix')
}

export function buildScheduledTaskCopyName(
  name: string,
  copySuffix?: string,
  translate?: ScheduledTaskTranslate
) {
  const normalizedName = name.trim()
  const suffix = copySuffix ?? getScheduledTaskCopySuffix(translate)
  return normalizedName ? `${normalizedName}${suffix}` : suffix
}

export function getOperationLogOperationTypeLabel(
  value: string | undefined,
  translate?: ScheduledTaskTranslate
) {
  const t = resolveTranslate(translate)
  if (value === 'create') return t('scheduledTasks.operationLog.types.create')
  if (value === 'delete') return t('scheduledTasks.operationLog.types.delete')
  return t('scheduledTasks.operationLog.types.update')
}

export function getOperationLogOperationSourceLabel(
  value: string | undefined,
  translate?: ScheduledTaskTranslate
) {
  const t = resolveTranslate(translate)
  if (value === 'ai') return t('scheduledTasks.operationLog.sources.ai')
  return t('scheduledTasks.operationLog.sources.manual')
}

export function getOperationLogFieldLabel(
  fieldKey: string | undefined,
  fallbackLabel?: string,
  translate?: ScheduledTaskTranslate
) {
  const normalizedFieldKey = String(fieldKey || '').trim()
  if (!normalizedFieldKey) return fallbackLabel || '-'

  const t = resolveTranslate(translate)
  const translationKey = OPERATION_LOG_FIELD_TRANSLATION_KEYS[normalizedFieldKey]
  if (translationKey) {
    return t(translationKey)
  }

  return fallbackLabel || normalizedFieldKey
}

export function getOperationLogStatusValueLabel(
  value: string | undefined,
  formatter: Pick<
    ScheduleTextFormatter,
    'operationStatusEnabledLabel' | 'operationStatusDisabledLabel'
  >
) {
  const normalizedValue = String(value || '')
    .trim()
    .toLowerCase()
  if (!normalizedValue) return ''
  if (normalizedValue === '启用' || normalizedValue === 'enabled') {
    return formatter.operationStatusEnabledLabel()
  }
  if (normalizedValue === '停用' || normalizedValue === 'disabled') {
    return formatter.operationStatusDisabledLabel()
  }
  return String(value)
}
