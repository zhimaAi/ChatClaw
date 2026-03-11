import { onMounted, ref } from 'vue'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { AgentsService } from '@bindings/chatclaw/internal/services/agents'
import {
  CreateScheduledTaskInput,
  ScheduledTasksService,
  UpdateScheduledTaskInput,
} from '@bindings/chatclaw/internal/services/scheduledtasks'
import type { ScheduledTask, ScheduledTaskFormState, ScheduledTaskSummary, Agent } from '../types'
import { createEmptyForm } from '../utils'

export function useScheduledTasks() {
  const loading = ref(false)
  const saving = ref(false)
  const tasks = ref<ScheduledTask[]>([])
  const summary = ref<ScheduledTaskSummary | null>(null)
  const agents = ref<Agent[]>([])

  const createDialogOpen = ref(false)
  const historyTask = ref<ScheduledTask | null>(null)
  const editingTask = ref<ScheduledTask | null>(null)
  const form = ref<ScheduledTaskFormState>(createEmptyForm())

  async function loadBaseOptions() {
    const agentList = await AgentsService.ListAgents()
    agents.value = agentList || []
  }

  async function loadTasks() {
    loading.value = true
    try {
      const [taskList, summaryValue] = await Promise.all([
        ScheduledTasksService.ListScheduledTasks(),
        ScheduledTasksService.GetScheduledTaskSummary(),
      ])
      tasks.value = taskList || []
      summary.value = summaryValue
    } catch (error) {
      toast.error(getErrorMessage(error))
    } finally {
      loading.value = false
    }
  }

  async function reloadAll() {
    await Promise.all([loadTasks(), loadBaseOptions()])
  }

  function openCreateDialog() {
    editingTask.value = null
    form.value = createEmptyForm()
    createDialogOpen.value = true
  }

  async function openEditDialog(task: ScheduledTask, toForm: (task: ScheduledTask) => ScheduledTaskFormState) {
    editingTask.value = task
    form.value = toForm(task)
    createDialogOpen.value = true
  }

  async function submitForm(buildPayload: (form: ScheduledTaskFormState) => {
    create: CreateScheduledTaskInput
    update: UpdateScheduledTaskInput
  }) {
    saving.value = true
    try {
      const payload = buildPayload(form.value)
      if (editingTask.value?.id) {
        await ScheduledTasksService.UpdateScheduledTask(editingTask.value.id, payload.update)
      } else {
        await ScheduledTasksService.CreateScheduledTask(payload.create)
      }
      createDialogOpen.value = false
      await loadTasks()
    } catch (error) {
      toast.error(getErrorMessage(error))
      throw error
    } finally {
      saving.value = false
    }
  }

  async function deleteTask(task: ScheduledTask) {
    try {
      await ScheduledTasksService.DeleteScheduledTask(task.id)
      await loadTasks()
    } catch (error) {
      toast.error(getErrorMessage(error))
    }
  }

  async function toggleTask(task: ScheduledTask, enabled: boolean) {
    try {
      await ScheduledTasksService.SetScheduledTaskEnabled(task.id, enabled)
      await loadTasks()
    } catch (error) {
      toast.error(getErrorMessage(error))
    }
  }

  async function runTaskNow(task: ScheduledTask) {
    try {
      await ScheduledTasksService.RunScheduledTaskNow(task.id)
      await loadTasks()
      historyTask.value = task
    } catch (error) {
      toast.error(getErrorMessage(error))
    }
  }

  onMounted(() => {
    void reloadAll()
  })

  return {
    loading,
    saving,
    tasks,
    summary,
    agents,
    createDialogOpen,
    historyTask,
    editingTask,
    form,
    loadTasks,
    reloadAll,
    openCreateDialog,
    openEditDialog,
    submitForm,
    deleteTask,
    toggleTask,
    runTaskNow,
  }
}
