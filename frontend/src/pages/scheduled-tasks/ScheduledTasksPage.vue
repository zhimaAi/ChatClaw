<script setup lang="ts">
import { computed } from 'vue'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { useI18n } from 'vue-i18n'
import { CreateScheduledTaskInput, UpdateScheduledTaskInput } from '@bindings/chatclaw/internal/services/scheduledtasks'
import { useScheduledTasks } from './composables/useScheduledTasks'
import CreateTaskDialog from './components/CreateTaskDialog.vue'
import TaskRunHistoryDialog from './components/TaskRunHistoryDialog.vue'
import TaskSummaryCards from './components/TaskSummaryCards.vue'
import TaskTable from './components/TaskTable.vue'
import type { ScheduledTaskFormState, ScheduledTask } from './types'
import { createEmptyForm, taskToForm } from './utils'

defineProps<{
  tabId: string
}>()

const { t } = useI18n()
const {
  loading,
  saving,
  tasks,
  summary,
  agents,
  createDialogOpen,
  historyTask,
  editingTask,
  form,
  openCreateDialog,
  openEditDialog,
  submitForm,
  deleteTask,
  toggleTask,
  runTaskNow,
} = useScheduledTasks()

const deleteTarget = computed<ScheduledTask | null>({
  get: () => null,
  set: () => undefined,
})

const summaryLabels = computed(() => ({
  total: t('scheduledTasks.total'),
  running: t('scheduledTasks.running'),
  paused: t('scheduledTasks.paused'),
  failed: t('scheduledTasks.failed'),
}))

function buildPayload(state: ScheduledTaskFormState) {
  const customPayload =
    state.customMode === 'monthly'
      ? JSON.stringify({
          hour: state.customHour,
          minute: state.customMinute,
          day_of_month: state.customDayOfMonth,
        })
      : state.customMode === 'weekly'
        ? JSON.stringify({
            hour: state.customHour,
            minute: state.customMinute,
            weekdays: state.customWeekdays,
          })
        : JSON.stringify({
            hour: state.customHour,
            minute: state.customMinute,
          })

  const scheduleValue =
    state.scheduleType === 'preset'
      ? state.schedulePreset
      : state.scheduleType === 'custom'
        ? customPayload
        : state.cronExpr

  const create = new CreateScheduledTaskInput({
    name: state.name,
    prompt: state.prompt,
    agent_id: state.agentId || 0,
    schedule_type: state.scheduleType,
    schedule_value: scheduleValue,
    cron_expr: state.scheduleType === 'cron' ? state.cronExpr : '',
    enabled: state.enabled,
  })

  const update = new UpdateScheduledTaskInput({
    name: state.name,
    prompt: state.prompt,
    agent_id: state.agentId || 0,
    schedule_type: state.scheduleType,
    schedule_value: scheduleValue,
    cron_expr: state.scheduleType === 'cron' ? state.cronExpr : '',
    enabled: state.enabled,
  })

  return { create, update }
}

async function handleSubmit() {
  await submitForm(buildPayload)
}

async function handleEdit(task: ScheduledTask) {
  await openEditDialog(task, taskToForm)
}

const deleteDialogOpen = computed({
  get: () => false,
  set: () => undefined,
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col overflow-auto bg-background px-6 py-5">
    <div class="mb-5 flex items-center justify-between">
      <div>
        <div class="text-xl font-semibold text-foreground">{{ t('scheduledTasks.title') }}</div>
        <div class="mt-1 text-sm text-muted-foreground">创建、编辑并追踪自动会话任务</div>
      </div>
      <button class="rounded-md bg-foreground px-4 py-2 text-sm text-background" @click="openCreateDialog">
        {{ t('scheduledTasks.create') }}
      </button>
    </div>

    <TaskSummaryCards :summary="summary" :labels="summaryLabels" />

    <div class="mt-5">
      <div v-if="loading" class="rounded-xl border border-border bg-card px-4 py-12 text-center text-sm text-muted-foreground">
        加载中...
      </div>
      <TaskTable
        v-else
        :tasks="tasks"
        @create="openCreateDialog"
        @edit="handleEdit"
        @run="runTaskNow"
        @history="(task) => (historyTask = task)"
        @toggle="toggleTask"
        @delete="deleteTask"
      />
    </div>

    <CreateTaskDialog
      :open="createDialogOpen"
      :saving="saving"
      :title="editingTask ? t('scheduledTasks.edit') : t('scheduledTasks.create')"
      :form="form"
      :agents="agents"
      @update:open="(value) => (createDialogOpen = value)"
      @submit="handleSubmit"
    />

    <TaskRunHistoryDialog
      :open="!!historyTask"
      :task="historyTask"
      @update:open="(value) => !value && (historyTask = null)"
    />
  </div>
</template>
