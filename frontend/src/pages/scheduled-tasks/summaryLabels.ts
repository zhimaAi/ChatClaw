// Separator used by the combined paused/ended summary label.
const SUMMARY_PAUSED_ENDED_SEPARATOR = '/'

type SummaryTranslate = (key: string) => string

export function buildScheduledTaskSummaryLabels(t: SummaryTranslate) {
  return {
    total: t('scheduledTasks.total'),
    running: t('scheduledTasks.running'),
    paused: `${t('scheduledTasks.disabled')}${SUMMARY_PAUSED_ENDED_SEPARATOR}${t('scheduledTasks.statusExpired')}`,
    failed: t('scheduledTasks.failed'),
  }
}
