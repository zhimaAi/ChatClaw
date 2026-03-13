<script setup lang="ts">
import { computed, ref } from 'vue'
import { Clock3, LoaderCircle, Plus, RefreshCcw } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { CreateScheduledTaskInput, UpdateScheduledTaskInput } from '@bindings/chatclaw/internal/services/scheduledtasks'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { useScheduledTasks } from './composables/useScheduledTasks'
import CreateTaskDialog from './components/CreateTaskDialog.vue'
import TaskRunHistoryDialog from './components/TaskRunHistoryDialog.vue'
import TaskSummaryCards from './components/TaskSummaryCards.vue'
import TaskTable from './components/TaskTable.vue'
import { createDeleteTaskConfirmation } from './deleteTaskConfirmation'
import type { ScheduledTaskFormState, ScheduledTask } from './types'
import { taskToForm } from './utils'

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
  reloadAll,
  openCreateDialog,
  openEditDialog,
  submitForm,
  deleteTask,
  toggleTask,
  runTaskNow,
} = useScheduledTasks()

const summaryLabels = computed(() => ({
  total: t('scheduledTasks.total'),
  running: t('scheduledTasks.running'),
  paused: t('scheduledTasks.paused'),
  failed: t('scheduledTasks.failed'),
}))

const hasTasks = computed(() => tasks.value.length > 0)
const deleteDialogOpen = ref(false)
const deletingTask = ref(false)
const pendingDeleteTask = ref<Pick<ScheduledTask, 'id' | 'name'> | null>(null)
const deleteTaskConfirmation = createDeleteTaskConfirmation(async (task) => {
  deletingTask.value = true
  try {
    await deleteTask(task as ScheduledTask)
  } finally {
    deletingTask.value = false
  }
})

function buildPayload(state: ScheduledTaskFormState) {
  const normalizedIntervalMinutes = Math.min(59, Math.max(1, Number(state.customIntervalMinutes) || 1))
  const customPayload =
    state.customMode === 'interval'
      ? JSON.stringify({
          interval_minutes: normalizedIntervalMinutes,
        })
      : state.customMode === 'monthly'
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
  if (saving.value) return
  await submitForm(buildPayload)
}

async function handleEdit(task: ScheduledTask) {
  await openEditDialog(task, taskToForm)
}

function syncPendingDeleteTask() {
  pendingDeleteTask.value = deleteTaskConfirmation.pendingTask()
}

function handleDeleteRequest(task: ScheduledTask) {
  deleteTaskConfirmation.request({
    id: task.id,
    name: task.name,
  })
  syncPendingDeleteTask()
  deleteDialogOpen.value = true
}

function handleDeleteCancel() {
  if (deletingTask.value) return
  deleteTaskConfirmation.cancel()
  syncPendingDeleteTask()
  deleteDialogOpen.value = false
}

async function handleDeleteConfirm() {
  if (deletingTask.value) return
  await deleteTaskConfirmation.confirm()
  syncPendingDeleteTask()
  deleteDialogOpen.value = false
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col overflow-auto bg-[#fafafa] px-6 py-6">
    <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
      <div class="space-y-1">
        <div class="text-[28px] font-semibold tracking-[-0.02em] text-[#171717]">
          {{ t('scheduledTasks.title') }}
        </div>
        <div class="text-sm text-[#737373]">
          {{ t('scheduledTasks.subtitle') }}
        </div>
      </div>
      <div class="flex items-center gap-2 self-start">
        <button
          type="button"
          class="inline-flex h-9 items-center gap-2 rounded-lg bg-[#f5f5f5] px-4 text-sm font-medium text-[#171717] transition-colors hover:bg-[#ebebeb]"
          @click="reloadAll"
        >
          <RefreshCcw class="size-4" />
          {{ t('scheduledTasks.refresh') }}
        </button>
        <button
          type="button"
          class="inline-flex h-9 items-center gap-2 rounded-lg bg-[#171717] px-4 text-sm font-medium text-white transition-colors hover:bg-[#0f0f0f]"
          @click="openCreateDialog"
        >
          <Plus class="size-4" />
          {{ t('scheduledTasks.addTask') }}
        </button>
      </div>
    </div>

    <div class="mt-6">
      <TaskSummaryCards :summary="summary" :labels="summaryLabels" />
    </div>

    <div class="mt-4">
      <div
        v-if="loading"
        class="rounded-2xl border border-[#e5e5e5] bg-white px-4 py-16 text-center text-sm text-[#737373]"
      >
        {{ t('common.loading', 'Loading...') }}
      </div>
      <div v-else-if="!hasTasks" class="flex min-h-[420px] items-center justify-center px-4 py-16">
        <div class="flex w-full max-w-[356px] flex-col items-center gap-4 text-center">
          <div class="flex size-10 items-center justify-center rounded-lg bg-[#f5f5f5] text-[#171717]">
            <Clock3 class="size-5" />
          </div>
          <div class="space-y-1">
            <div class="text-base font-medium leading-6 text-[#171717]">
              {{ t('scheduledTasks.empty') }}
            </div>
            <div class="text-sm leading-5 text-[#737373]">
              {{ t('scheduledTasks.emptyDescription') }}
            </div>
          </div>
          <button
            type="button"
            class="inline-flex h-9 items-center gap-2 rounded-lg bg-[#171717] px-4 text-sm font-medium text-white transition-colors hover:bg-[#0f0f0f]"
            @click="openCreateDialog"
          >
            <Plus class="size-4" />
            {{ t('scheduledTasks.create') }}
          </button>
        </div>
      </div>
      <TaskTable
        v-else
        :tasks="tasks"
        :agents="agents"
        @edit="handleEdit"
        @run="runTaskNow"
        @history="(task) => (historyTask = task)"
        @toggle="toggleTask"
        @delete="handleDeleteRequest"
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

    <AlertDialog :open="deleteDialogOpen" @update:open="(value) => !value && handleDeleteCancel()">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('scheduledTasks.deleteConfirmTitle') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('scheduledTasks.deleteConfirmDescription', { name: pendingDeleteTask?.name || '' }) }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel :disabled="deletingTask" @click="handleDeleteCancel">
            {{ t('common.cancel', '取消') }}
          </AlertDialogCancel>
          <Button
            :disabled="deletingTask"
            variant="default"
            @click.prevent="handleDeleteConfirm"
          >
            <LoaderCircle v-if="deletingTask" class="size-4 shrink-0 animate-spin" />
            {{ t('scheduledTasks.confirmDelete') }}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
