import { computed, onMounted, ref } from 'vue'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { AgentsService } from '@bindings/chatclaw/internal/services/agents'
import { LibraryService } from '@bindings/chatclaw/internal/services/library'
import { ProvidersService } from '@bindings/chatclaw/internal/services/providers'
import {
  CreateScheduledTaskInput,
  ScheduledTasksService,
  UpdateScheduledTaskInput,
} from '@bindings/chatclaw/internal/services/scheduledtasks'
import type { ModelGroup } from '@bindings/chatclaw/internal/services/providers'
import type { ScheduledTask, ScheduledTaskFormState, ScheduledTaskSummary, Agent, Library, Provider, Model } from '../types'
import { createEmptyForm } from '../utils'

export function useScheduledTasks() {
  const loading = ref(false)
  const saving = ref(false)
  const tasks = ref<ScheduledTask[]>([])
  const summary = ref<ScheduledTaskSummary | null>(null)
  const agents = ref<Agent[]>([])
  const libraries = ref<Library[]>([])
  const providers = ref<Provider[]>([])
  const providerModels = ref<Record<string, Model[]>>({})

  const createDialogOpen = ref(false)
  const historyTask = ref<ScheduledTask | null>(null)
  const editingTask = ref<ScheduledTask | null>(null)
  const form = ref<ScheduledTaskFormState>(createEmptyForm())

  const activeProviderModels = computed(() => providerModels.value[form.value.llmProviderId] || [])

  async function loadBaseOptions() {
    const [agentList, libraryList, providerList] = await Promise.all([
      AgentsService.ListAgents(),
      LibraryService.ListLibraries(),
      ProvidersService.ListProviders(),
    ])
    agents.value = agentList || []
    libraries.value = libraryList || []
    providers.value = (providerList || []).filter((item) => item.enabled)
  }

  async function ensureProviderModels(providerID: string) {
    if (!providerID || providerModels.value[providerID]) return
    const providerWithModels = await ProvidersService.GetProviderWithModels(providerID)
    const groups = providerWithModels?.model_groups || []
    providerModels.value[providerID] = flattenModels(groups)
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
    if (form.value.llmProviderId) {
      await ensureProviderModels(form.value.llmProviderId)
    }
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
    libraries,
    providers,
    activeProviderModels,
    createDialogOpen,
    historyTask,
    editingTask,
    form,
    loadTasks,
    reloadAll,
    ensureProviderModels,
    openCreateDialog,
    openEditDialog,
    submitForm,
    deleteTask,
    toggleTask,
    runTaskNow,
  }
}

function flattenModels(groups: ModelGroup[]): Model[] {
  return groups
    .filter((group) => group.type === 'llm')
    .flatMap((group) => group.models || [])
    .filter((model) => model.enabled)
}
