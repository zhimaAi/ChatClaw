import type { ScheduledTaskFormState, ScheduledTask } from './types'

type CreateTaskDialogState = {
  editingTask: ScheduledTask | null
  form: ScheduledTaskFormState
  createDialogOpen: boolean
}

type LoadBaseOptions = () => Promise<void>
type CreateEmptyForm = () => ScheduledTaskFormState

export async function prepareCreateTaskDialogState(
  loadBaseOptions: LoadBaseOptions,
  buildEmptyForm: CreateEmptyForm
): Promise<CreateTaskDialogState> {
  await loadBaseOptions()

  return {
    editingTask: null,
    form: buildEmptyForm(),
    createDialogOpen: true,
  }
}
