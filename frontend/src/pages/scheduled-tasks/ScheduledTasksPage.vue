<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronLeft, Clock3, FileText, LoaderCircle, Plus, RefreshCcw } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
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
import OperationLogListPage from './components/OperationLogListPage.vue'
import TaskRunHistoryDialog from './components/TaskRunHistoryDialog.vue'
import TaskSummaryCards from './components/TaskSummaryCards.vue'
import TaskTable from './components/TaskTable.vue'
import { createDeleteTaskConfirmation } from './deleteTaskConfirmation'
import {
  SCHEDULED_TASKS_VIEW_ACTION_BACK_TO_TASK_LIST,
  SCHEDULED_TASKS_VIEW_ACTION_ENTER_OPERATION_LOG,
  SCHEDULED_TASKS_VIEW_TASK_LIST,
  SCHEDULED_TASKS_VIEW_OPERATION_LOG_LIST,
  transitionScheduledTasksView,
  type ScheduledTasksPageView,
} from './scheduledTasksView'
import type { ScheduledTask, ScheduledTaskFormState } from './types'
import { buildExpirationDateTime, taskToCopyForm, taskToForm } from './utils'

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
  channels,
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

// Keep the operation-log entry copy stable while the page is not localized yet.
const OPERATION_LOG_TITLE = '操作记录'

const hasTasks = computed(() => tasks.value.length > 0)
const currentView = ref<ScheduledTasksPageView>(SCHEDULED_TASKS_VIEW_TASK_LIST)
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
  const normalizedIntervalMinutes = Math.min(
    59,
    Math.max(1, Number(state.customIntervalMinutes) || 1)
  )
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

  const create = {
    name: state.name,
    prompt: state.prompt,
    agent_id: state.agentId || 0,
    expires_at: buildExpirationDateTime(state.expiresAtDate),
    notification_platform: state.notificationPlatform,
    notification_channel_ids: state.notificationChannelIds,
    schedule_type: state.scheduleType,
    schedule_value: scheduleValue,
    cron_expr: state.scheduleType === 'cron' ? state.cronExpr : '',
    enabled: state.enabled,
  }

  const update = {
    name: state.name,
    prompt: state.prompt,
    agent_id: state.agentId || 0,
    expires_at: buildExpirationDateTime(state.expiresAtDate),
    notification_platform: state.notificationPlatform,
    notification_channel_ids: state.notificationChannelIds,
    schedule_type: state.scheduleType,
    schedule_value: scheduleValue,
    cron_expr: state.scheduleType === 'cron' ? state.cronExpr : '',
    enabled: state.enabled,
  }

  return { create, update }
}

async function handleSubmit() {
  if (saving.value) return
  await submitForm(buildPayload)
}

async function handleEdit(task: ScheduledTask) {
  await openEditDialog(task, taskToForm)
}

function handleCopy(task: ScheduledTask) {
  editingTask.value = null
  form.value = taskToCopyForm(task)
  createDialogOpen.value = true
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

function openOperationLogPage() {
  currentView.value = transitionScheduledTasksView(
    currentView.value,
    SCHEDULED_TASKS_VIEW_ACTION_ENTER_OPERATION_LOG
  )
}

function backToTaskListPage() {
  currentView.value = transitionScheduledTasksView(
    currentView.value,
    SCHEDULED_TASKS_VIEW_ACTION_BACK_TO_TASK_LIST
  )
}

function handleHistoryDialogOpenChange(value: boolean) {
  // Close the history dialog by clearing the source task explicitly.
  // 通过显式清空来源任务来关闭历史弹窗，避免模板内联赋值带来的歧义。
  if (!value) {
    historyTask.value = null
  }
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col overflow-y-auto bg-white dark:bg-background">
    <div
      v-if="currentView === SCHEDULED_TASKS_VIEW_TASK_LIST"
      class="flex h-20 shrink-0 items-center justify-between px-6"
    >
      <div class="flex flex-col gap-1">
        <h1 class="text-base font-semibold text-[#262626] dark:text-foreground">
          {{ t('scheduledTasks.title') }}
        </h1>
        <p class="text-sm text-[#737373] dark:text-muted-foreground">
          {{ t('scheduledTasks.subtitle') }}
        </p>
      </div>
      <div class="mt-5 flex items-center gap-2">
        <Button
          class="h-9 gap-1 border-none bg-[#f5f5f5] text-[#171717] shadow-none hover:bg-[#e5e5e5] dark:bg-muted dark:text-foreground dark:hover:bg-muted/80"
          @click="openOperationLogPage"
        >
          <FileText class="h-4 w-4 shrink-0" />
          {{ OPERATION_LOG_TITLE }}
        </Button>
        <Button
          class="h-9 gap-1 border-none bg-[#f5f5f5] text-[#171717] shadow-none hover:bg-[#e5e5e5] dark:bg-muted dark:text-foreground dark:hover:bg-muted/80"
          @click="reloadAll"
        >
          <RefreshCcw class="h-4 w-4 shrink-0" />
          {{ t('scheduledTasks.refresh') }}
        </Button>
        <Button class="h-9 gap-1" variant="default" @click="openCreateDialog">
          <Plus class="h-4 w-4 shrink-0" />
          {{ t('scheduledTasks.addTask') }}
        </Button>
      </div>
    </div>

    <div
      v-else-if="currentView === SCHEDULED_TASKS_VIEW_OPERATION_LOG_LIST"
      class="flex h-20 shrink-0 items-center px-6"
    >
      <button
        type="button"
        class="inline-flex h-9 w-9 items-center justify-center rounded-full text-[#737373] transition-colors hover:bg-[#f5f5f5] hover:text-[#171717]"
        @click="backToTaskListPage"
      >
        <ChevronLeft class="h-5 w-5" />
      </button>
      <h1 class="ml-3 text-base font-semibold text-[#262626] dark:text-foreground">
        {{ OPERATION_LOG_TITLE }}
      </h1>
    </div>

    <div class="flex flex-1 flex-col min-h-0 overflow-auto px-6 pb-6">
      <template v-if="currentView === SCHEDULED_TASKS_VIEW_TASK_LIST">
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
              <div
                class="flex size-10 items-center justify-center rounded-lg bg-[#f5f5f5] text-[#171717]"
              >
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
            @copy="handleCopy"
            @run="runTaskNow"
            @history="(task) => (historyTask = task)"
            @toggle="toggleTask"
            @delete="handleDeleteRequest"
          />
        </div>
      </template>

      <div v-else-if="currentView === SCHEDULED_TASKS_VIEW_OPERATION_LOG_LIST" class="mt-2 flex min-h-0 flex-1 flex-col">
        <OperationLogListPage />
      </div>
    </div>

    <CreateTaskDialog
      :open="createDialogOpen"
      :saving="saving"
      :title="editingTask ? t('scheduledTasks.edit') : t('scheduledTasks.create')"
      :form="form"
      :agents="agents"
      :channels="channels"
      @update:open="(value) => (createDialogOpen = value)"
      @submit="handleSubmit"
    />

    <TaskRunHistoryDialog
      v-if="historyTask"
      :open="!!historyTask"
      :task="historyTask"
      @update:open="handleHistoryDialogOpenChange"
    />

    <AlertDialog :open="deleteDialogOpen" @update:open="(value) => !value && handleDeleteCancel()">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('scheduledTasks.deleteConfirmTitle') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{
              t('scheduledTasks.deleteConfirmDescription', { name: pendingDeleteTask?.name || '' })
            }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel :disabled="deletingTask" @click="handleDeleteCancel">
            {{ t('common.cancel') }}
          </AlertDialogCancel>
          <Button :disabled="deletingTask" variant="default" @click.prevent="handleDeleteConfirm">
            <LoaderCircle v-if="deletingTask" class="size-4 shrink-0 animate-spin" />
            {{ t('scheduledTasks.confirmDelete') }}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
