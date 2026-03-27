<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import { AlertCircle, LoaderCircle, RefreshCcw } from 'lucide-vue-next'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { getErrorMessage } from '@/composables/useErrorMessage'
import TaskRunStatusBadge from '@/pages/scheduled-tasks/components/TaskRunStatusBadge.vue'
import {
  OpenClawCronService,
  type OpenClawCronHistoryListItem,
  type OpenClawCronJob,
  type OpenClawCronRunDetail,
} from '@bindings/chatclaw/internal/openclaw/cron'
import OpenClawCronTranscriptView from './OpenClawCronTranscriptView.vue'
import { normalizeOpenClawRunStatus } from './status'
import { formatDurationMs, formatOpenClawCronTime } from './utils'

const DETAIL_LOADING_SKELETON_ROWS = 6
const OPENCLAW_CRON_HISTORY_ALL_LIMIT = 0
const RUN_MATCH_THRESHOLD_MS = 5000

const props = defineProps<{
  open: boolean
  job: OpenClawCronJob | null
  conversationId?: number | null
  triggerAtMs?: number | null
  runId?: string | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const { t } = useI18n()
const loading = ref(false)
const runs = ref<OpenClawCronHistoryListItem[]>([])
const selectedRun = ref<OpenClawCronHistoryListItem | null>(null)
const selectedDetail = ref<OpenClawCronRunDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
let detailLoadSequence = 0
let detailRefreshTimer: number | null = null
let eventUnsubscribe: (() => void) | null = null
let currentWatchId: string | null = null

const hasRuns = computed(() => runs.value.length > 0)
const waitingForTriggeredRun = computed(() => false)
const shouldShowPreparingState = computed(
  () => !detailLoading.value && !detailError.value && !!selectedRun.value && !selectedDetail.value
)

function normalizeRunStatus(status: string) {
  return normalizeOpenClawRunStatus(status)
}

function displayRunStatusLabel(status: string) {
  return normalizeRunStatus(status) === 'failed'
    ? t('scheduledTasks.statusFailed')
    : t('scheduledTasks.statusSuccess')
}

async function loadRuns() {
  if (!props.job?.id) return
  loading.value = true
  try {
    runs.value = await OpenClawCronService.ListHistory(
      props.job.id,
      OPENCLAW_CRON_HISTORY_ALL_LIMIT
    )
    if (!selectedRun.value && runs.value[0]) {
      selectedRun.value = runs.value[0]
    }
    if (selectedRun.value) {
      const latest = runs.value.find((item) => isSameHistoryItem(item, selectedRun.value))
      if (latest) selectedRun.value = latest
      await loadDetail(!currentWatchId)
    }
  } finally {
    loading.value = false
  }
}

function runTimestampMs(run: OpenClawCronHistoryListItem) {
  return Number(run.run_at_ms || 0)
}

function findTriggeredRun() {
  const runId = String(props.runId || '').trim()
  if (runId) {
    return runs.value.find((item) => String(item.run_id || '').trim() === runId) ?? null
  }

  const triggerAtMs = Number(props.triggerAtMs || 0)
  if (!triggerAtMs) return null

  const thresholdMs = triggerAtMs - RUN_MATCH_THRESHOLD_MS
  return runs.value.find((item) => runTimestampMs(item) >= thresholdMs) ?? null
}

async function loadDetail(reconnect = false) {
  if (!selectedRun.value) {
    selectedDetail.value = null
    detailError.value = ''
    detailLoading.value = false
    return
  }

  const currentRun = selectedRun.value
  const requestSequence = ++detailLoadSequence
  detailLoading.value = true
  detailError.value = ''
  selectedDetail.value = null

  try {
    if (!props.job?.id || !currentRun.session_id) {
      if (requestSequence !== detailLoadSequence) return
      selectedDetail.value = buildSyntheticDetail(currentRun)
    } else {
      const detail = await OpenClawCronService.GetRunDetail(props.job.id, currentRun.session_id)
      if (requestSequence !== detailLoadSequence) return
      selectedDetail.value = detail
    }

    if (reconnect) {
      try {
        await reconnectGatewayStream()
      } catch (error) {
        console.warn('[openclaw-cron] reconnect gateway stream failed:', error)
      }
    }
  } catch (error) {
    if (requestSequence !== detailLoadSequence) return
    selectedDetail.value = null
    detailError.value =
      getErrorMessage(error) ||
      t(
        'openclawCron.history.detailLoadFailedReason',
        '请确认 OpenClaw Gateway 已启动，并稍后重试。'
      )
    await cleanupGatewayStream()
  } finally {
    if (requestSequence === detailLoadSequence) {
      detailLoading.value = false
    }
  }
}

function buildSyntheticDetail(run: OpenClawCronHistoryListItem): OpenClawCronRunDetail {
  return {
    run: {
      ts: run.run_at_ms || 0,
      job_id: run.job_id || props.job?.id || '',
      action: run.trigger_type || run.source || '',
      status: run.status || '',
      error: '',
      summary: '',
      run_at_ms: run.run_at_ms || 0,
      duration_ms: run.duration_ms || 0,
      next_run_at_ms: 0,
      model: '',
      provider: '',
      delivery_status: '',
      session_id: run.session_id || '',
      session_key: run.session_key || '',
    },
    session_file_path: '',
    run_file_path: '',
    conversation_id: 0,
    conversation_agent_id: 0,
    messages: [],
    is_live: !!run.is_pending_local || run.status === 'running',
  } as OpenClawCronRunDetail
}

function isSameHistoryItem(
  left: OpenClawCronHistoryListItem,
  right: OpenClawCronHistoryListItem | null
) {
  if (!right) return false
  if (left.session_key && right.session_key) return left.session_key === right.session_key
  if (left.session_id && right.session_id) return left.session_id === right.session_id
  if (left.run_id && right.run_id) return left.run_id === right.run_id
  return false
}

function scheduleDetailRefresh() {
  if (detailRefreshTimer) return
  detailRefreshTimer = window.setTimeout(async () => {
    detailRefreshTimer = null
    await loadDetail(false)
  }, 500)
}

async function reconnectGatewayStream() {
  await cleanupGatewayStream()

  if (!props.open || !props.job?.id || !selectedRun.value?.session_key) {
    return
  }

  currentWatchId = await OpenClawCronService.StartRunStream(
    props.job.id,
    selectedRun.value.session_id || '',
    selectedRun.value.session_key
  )

  eventUnsubscribe = Events.On('openclaw:cron-run-event', (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    if (!payload || payload.watch_id !== currentWatchId) return
    scheduleDetailRefresh()
  })
}

async function cleanupGatewayStream() {
  if (currentWatchId) {
    await OpenClawCronService.StopRunStream(currentWatchId)
    currentWatchId = null
  }
  eventUnsubscribe?.()
  eventUnsubscribe = null
}

watch(
  () => props.open,
  async (open) => {
    if (!open) {
      selectedRun.value = null
      selectedDetail.value = null
      detailError.value = ''
      detailLoading.value = false
      await cleanupGatewayStream()
      return
    }

    selectedRun.value = null
    selectedDetail.value = null
    await loadRuns()

    const matched = findTriggeredRun()
    if (matched) {
      selectedRun.value = matched
      await loadDetail(true)
    }
  },
  { immediate: true }
)

watch(
  () => [selectedRun.value?.session_id, selectedRun.value?.session_key, selectedRun.value?.run_id],
  async () => {
    if (props.open) {
      await loadDetail(true)
    }
  }
)

onBeforeUnmount(() => {
  if (detailRefreshTimer) {
    window.clearTimeout(detailRefreshTimer)
    detailRefreshTimer = null
  }
  void cleanupGatewayStream()
})
</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent
      class="max-h-[90vh] overflow-hidden sm:!w-auto sm:min-w-[1000px] sm:!max-w-[1760px]"
    >
      <DialogHeader>
        <DialogTitle
          >{{ job?.name }} / {{ t('openclawCron.history.title', '运行历史') }}</DialogTitle
        >
      </DialogHeader>

      <div class="flex h-[70vh] min-h-0 gap-4">
        <div
          class="shrink-0 overflow-y-auto overflow-x-hidden rounded-lg border border-border sm:w-[248px]"
        >
          <div v-if="loading" class="p-4 text-sm text-muted-foreground">
            {{ t('common.loading', '加载中...') }}
          </div>
          <div
            v-else-if="waitingForTriggeredRun && !hasRuns"
            class="p-4 text-sm text-muted-foreground"
          >
            {{
              t(
                'openclawCron.history.waitingTriggeredRun',
                '任务已触发，正在等待 OpenClaw 写入新的运行记录...'
              )
            }}
          </div>
          <div v-else-if="!hasRuns" class="p-4 text-sm text-muted-foreground">
            {{ t('openclawCron.history.empty', '暂无运行历史') }}
          </div>
          <button
            v-for="run in runs"
            :key="`${run.run_id || run.session_id || run.run_at_ms}`"
            class="w-full border-b border-border px-3 py-3 text-left transition-colors hover:bg-accent/40"
            :class="isSameHistoryItem(run, selectedRun) ? 'bg-accent/50' : ''"
            @click="selectedRun = run"
          >
            <div class="flex items-start gap-2 overflow-hidden">
              <TaskRunStatusBadge
                class="mt-0.5 shrink-0"
                :status="normalizeRunStatus(run.status)"
                :label="displayRunStatusLabel(run.status)"
              />
              <div class="flex min-w-0 flex-1 flex-col items-start gap-1 text-left">
                <div class="w-full text-[11px] leading-4 text-foreground">
                  {{ formatOpenClawCronTime(run.run_at_ms) }}
                </div>
                <div class="flex w-full items-center gap-1 text-[11px] text-muted-foreground">
                  <span class="truncate">{{ formatDurationMs(run.duration_ms) }}</span>
                </div>
              </div>
            </div>
          </button>
        </div>

        <div class="min-h-0 flex-1 overflow-hidden rounded-lg border border-border">
          <div v-if="detailLoading" class="flex h-full flex-col gap-4 p-6">
            <div class="flex items-center gap-2 text-sm text-muted-foreground">
              <LoaderCircle class="size-4 animate-spin" />
              <span>{{ t('openclawCron.history.detailLoading', '正在加载对话明细...') }}</span>
            </div>
            <div
              v-for="index in DETAIL_LOADING_SKELETON_ROWS"
              :key="`detail-loading-${index}`"
              class="space-y-2"
            >
              <div class="h-4 w-24 animate-pulse rounded bg-muted/70" />
              <div class="h-4 animate-pulse rounded bg-muted/60" />
              <div class="h-4 w-4/5 animate-pulse rounded bg-muted/50" />
            </div>
          </div>
          <div v-else-if="detailError" class="h-full overflow-y-auto p-6">
            <div
              class="mx-auto w-full max-w-lg rounded-xl border border-[#fecaca] bg-[#fff5f5] p-5 text-left"
            >
              <div class="flex items-start gap-3">
                <div class="rounded-full bg-[#fee2e2] p-2 text-[#dc2626]">
                  <AlertCircle class="size-5" />
                </div>
                <div class="min-w-0 flex-1 space-y-3">
                  <div class="space-y-1">
                    <p class="text-sm font-medium text-[#991b1b]">
                      {{ t('openclawCron.history.detailLoadFailed', '加载运行明细失败') }}
                    </p>
                    <p class="text-sm text-[#7f1d1d]">
                      {{
                        t(
                          'openclawCron.history.detailLoadFailedDescription',
                          '未能获取这次运行的对话明细，请检查 OpenClaw 运行状态后重试。'
                        )
                      }}
                    </p>
                  </div>
                  <div class="rounded-lg border border-[#fecaca] bg-white/80 p-3">
                    <p class="text-xs font-medium uppercase tracking-wide text-[#b91c1c]">
                      {{ t('openclawCron.history.detailErrorReason', '失败原因') }}
                    </p>
                    <p class="mt-1 whitespace-pre-wrap break-all text-sm text-[#7f1d1d]">
                      {{ detailError }}
                    </p>
                  </div>
                  <Button variant="outline" class="gap-2" @click="void loadDetail(true)">
                    <RefreshCcw class="size-4" />
                    {{ t('openclawCron.history.retryLoadDetail', '重试加载') }}
                  </Button>
                </div>
              </div>
            </div>
          </div>
          <div
            v-else-if="shouldShowPreparingState"
            class="flex h-full items-center justify-center text-sm text-muted-foreground"
          >
            {{
              selectedRun
                ? t('openclawCron.history.conversationPreparing', '正在准备历史会话，请稍候...')
                : t('openclawCron.history.selectRun', '请选择一次运行记录')
            }}
          </div>
          <div v-else-if="selectedDetail" class="h-full">
            <OpenClawCronTranscriptView :detail="selectedDetail" />
          </div>
          <div v-else class="flex h-full items-center justify-center text-sm text-muted-foreground">
            {{ t('openclawCron.history.selectRun', '请选择一次运行记录') }}
          </div>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
