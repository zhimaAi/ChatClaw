// Scheduled-task page views. Keep the values stable for simple conditional rendering.
export const SCHEDULED_TASKS_VIEW_TASK_LIST = 'task-list'
export const SCHEDULED_TASKS_VIEW_OPERATION_LOG_LIST = 'operation-log-list'

export type ScheduledTasksPageView =
  | typeof SCHEDULED_TASKS_VIEW_TASK_LIST
  | typeof SCHEDULED_TASKS_VIEW_OPERATION_LOG_LIST

// View transition actions are extracted as constants to avoid magic strings in the page.
export const SCHEDULED_TASKS_VIEW_ACTION_ENTER_OPERATION_LOG = 'enter-operation-log'
export const SCHEDULED_TASKS_VIEW_ACTION_BACK_TO_TASK_LIST = 'back-to-task-list'

export type ScheduledTasksPageViewAction =
  | typeof SCHEDULED_TASKS_VIEW_ACTION_ENTER_OPERATION_LOG
  | typeof SCHEDULED_TASKS_VIEW_ACTION_BACK_TO_TASK_LIST

/**
 * Resolve the next scheduled-task page view from a user action.
 * Unknown actions intentionally keep the current view unchanged.
 */
export function transitionScheduledTasksView(
  currentView: ScheduledTasksPageView,
  action: ScheduledTasksPageViewAction
): ScheduledTasksPageView {
  if (action === SCHEDULED_TASKS_VIEW_ACTION_ENTER_OPERATION_LOG) {
    return SCHEDULED_TASKS_VIEW_OPERATION_LOG_LIST
  }

  if (action === SCHEDULED_TASKS_VIEW_ACTION_BACK_TO_TASK_LIST) {
    return SCHEDULED_TASKS_VIEW_TASK_LIST
  }

  return currentView
}
