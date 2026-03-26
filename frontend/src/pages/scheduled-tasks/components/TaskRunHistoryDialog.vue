<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ScheduledTasksService } from '@bindings/chatclaw/internal/services/scheduledtasks'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import type { ScheduledTask, ScheduledTaskRun, ScheduledTaskRunDetail } from '../types'
import { formatDuration, formatTaskTime } from '../utils'
import TaskRunStatusBadge from './TaskRunStatusBadge.vue'
import EmbeddedAssistantPage from '@/pages/native/assistant/components/EmbeddedAssistantPage.vue'

const props = defineProps<{
  open: boolean
  task: ScheduledTask | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const loading = ref(false)
const runs = ref<ScheduledTaskRun[]>([])
const selectedRunId = ref<number | null>(null)
const selectedDetail = ref<ScheduledTaskRunDetail | null>(null)
const { t } = useI18n()

function displayRunStatusLabel(status: string) {
  if (status === 'running') return t('scheduledTasks.statusRunning')
  if (status === 'failed') return t('scheduledTasks.statusFailed')
  if (status === 'success') return t('scheduledTasks.statusSuccess')
  return t('scheduledTasks.statusPending')
}

function displayRunTriggerLabel(triggerType: string) {
  if (triggerType === 'schedule') return t('scheduledTasks.runTriggerSchedule')
  if (triggerType === 'manual') return t('scheduledTasks.runTriggerManual')
  return triggerType
}

watch(
  () => props.open,
  async (value) => {
    if (!value || !props.task) return
    loading.value = true
    try {
      runs.value = await ScheduledTasksService.ListScheduledTaskRuns(props.task.id, 1, 50)
      selectedRunId.value = runs.value[0]?.id ?? null
      if (selectedRunId.value) {
        selectedDetail.value = await ScheduledTasksService.GetScheduledTaskRunDetail(
          selectedRunId.value
        )
      } else {
        selectedDetail.value = null
      }
    } finally {
      loading.value = false
    }
  },
  { immediate: true }
)

async function selectRun(run: ScheduledTaskRun) {
  selectedRunId.value = run.id
  selectedDetail.value = await ScheduledTasksService.GetScheduledTaskRunDetail(run.id)
}
</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent
      class="max-h-[90vh] overflow-hidden sm:!w-auto sm:min-w-[1000px] sm:!max-w-[1760px]"
    >
      <DialogHeader>
        <DialogTitle>{{ task?.name }} / {{ t('scheduledTasks.runHistoryTitle') }}</DialogTitle>
      </DialogHeader>

      <div class="flex h-[70vh] min-h-0 gap-4">
        <div
          class="shrink-0 overflow-y-auto overflow-x-hidden rounded-lg border border-border sm:w-[248px]"
        >
          <div v-if="loading" class="p-4 text-sm text-muted-foreground">
            {{ t('common.loading') }}
          </div>
          <div v-else-if="runs.length === 0" class="p-4 text-sm text-muted-foreground">
            {{ t('scheduledTasks.noRuns') }}
          </div>
          <button
            v-for="run in runs"
            :key="run.id"
            class="w-full border-b border-border px-3 py-3 text-left transition-colors hover:bg-accent/40"
            :class="selectedRunId === run.id ? 'bg-accent/50' : ''"
            @click="selectRun(run)"
          >
            <div class="flex items-start gap-2 overflow-hidden">
              <TaskRunStatusBadge
                class="mt-0.5 shrink-0"
                :status="run.status"
                :label="displayRunStatusLabel(run.status)"
              />
              <div class="flex min-w-0 flex-1 flex-col items-start gap-1 text-left">
                <div class="w-full text-[11px] leading-4 text-foreground">
                  {{ formatTaskTime(run.started_at) }}
                </div>
                <div class="flex w-full items-center gap-1 text-[11px] text-muted-foreground">
                  <span class="shrink-0">{{ displayRunTriggerLabel(run.trigger_type) }}</span>
                  <span class="shrink-0 text-muted-foreground/50">&middot;</span>
                  <span class="truncate">{{ formatDuration(run.duration_ms) }}</span>
                </div>
              </div>
            </div>

            <TooltipProvider>
              <Tooltip v-if="run.error_message">
                <TooltipTrigger as-child>
                  <div class="mt-2 line-clamp-1 text-xs text-red-600">{{ run.error_message }}</div>
                </TooltipTrigger>
                <TooltipContent>
                  <p class="max-w-sm whitespace-pre-wrap text-xs">{{ run.error_message }}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </button>
        </div>

        <div class="min-h-0 flex-1 overflow-hidden rounded-lg border border-border">
          <div
            v-if="!selectedDetail?.conversation?.id"
            class="flex h-full items-center justify-center text-sm text-muted-foreground"
          >
            {{ t('scheduledTasks.conversationEmpty') }}
          </div>
          <EmbeddedAssistantPage
            v-else
            :conversation-id="selectedDetail.conversation.id"
            :agent-id="selectedDetail.conversation.agent_id"
          />
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
