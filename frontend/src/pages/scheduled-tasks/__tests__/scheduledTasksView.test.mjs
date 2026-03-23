import test from 'node:test'
import assert from 'node:assert/strict'

import {
  SCHEDULED_TASKS_VIEW_ACTION_BACK_TO_TASK_LIST,
  SCHEDULED_TASKS_VIEW_ACTION_ENTER_OPERATION_LOG,
  SCHEDULED_TASKS_VIEW_OPERATION_LOG_LIST,
  SCHEDULED_TASKS_VIEW_TASK_LIST,
  transitionScheduledTasksView,
} from '../scheduledTasksView.ts'

test('enter-operation-log switches task list to operation log list', () => {
  const nextView = transitionScheduledTasksView(
    SCHEDULED_TASKS_VIEW_TASK_LIST,
    SCHEDULED_TASKS_VIEW_ACTION_ENTER_OPERATION_LOG
  )

  assert.equal(nextView, SCHEDULED_TASKS_VIEW_OPERATION_LOG_LIST)
})

test('back-to-task-list switches operation log list to task list', () => {
  const nextView = transitionScheduledTasksView(
    SCHEDULED_TASKS_VIEW_OPERATION_LOG_LIST,
    SCHEDULED_TASKS_VIEW_ACTION_BACK_TO_TASK_LIST
  )

  assert.equal(nextView, SCHEDULED_TASKS_VIEW_TASK_LIST)
})

test('unknown action keeps current view unchanged', () => {
  const nextView = transitionScheduledTasksView(
    SCHEDULED_TASKS_VIEW_OPERATION_LOG_LIST,
    /** @type {any} */ ('unknown-action')
  )

  assert.equal(nextView, SCHEDULED_TASKS_VIEW_OPERATION_LOG_LIST)
})
