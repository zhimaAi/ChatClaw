import type { ScheduledTask } from './types'

type DeleteTaskConfirmationItem = Pick<ScheduledTask, 'id' | 'name'>

export function createDeleteTaskConfirmation(
  onConfirmDelete: (task: DeleteTaskConfirmationItem) => Promise<void>
) {
  let pending: DeleteTaskConfirmationItem | null = null

  return {
    request(task: DeleteTaskConfirmationItem) {
      pending = task
    },
    pendingTask() {
      return pending
    },
    cancel() {
      pending = null
    },
    async confirm() {
      if (!pending) return
      const task = pending
      await onConfirmDelete(task)
      pending = null
    },
  }
}
