export type ScheduledTaskTranslate = (
  key: string,
  params?: Record<string, unknown>
) => string

export interface ScheduleTextFormatter {
  interval: (params: { value: number }) => string
  monthly: (params: { day: number; time: string }) => string
  weekly: (params: { labels: string; time: string }) => string
  daily: (params: { time: string }) => string
  weekdayLabel: (value: number) => string
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
}

function interpolateText(template: string, params?: Record<string, unknown>) {
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (_, key) => String(params[key] ?? `{${key}}`))
}

function resolveTranslate(translate?: ScheduledTaskTranslate): ScheduledTaskTranslate {
  if (translate) {
    return translate
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
