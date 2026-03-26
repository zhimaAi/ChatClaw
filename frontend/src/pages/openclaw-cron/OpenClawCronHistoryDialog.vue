<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import { AlertCircle, LoaderCircle, RefreshCcw } from 'lucide-vue-next'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { getErrorMessage } from '@/composables/useErrorMessage'
import EmbeddedAssistantPage from '@/pages/openclaw/components/EmbeddedAssistantPage.vue'
import TaskRunStatusBadge from '@/pages/scheduled-tasks/components/TaskRunStatusBadge.vue'
import { ChatEventType, useChatStore } from '@/stores'
import {
  OpenClawCronService,
  type OpenClawCronHistoryListItem,
  type OpenClawCronJob,
  type OpenClawCronRunDetail,
} from '@bindings/chatclaw/internal/openclaw/cron'
import { formatDurationMs, formatOpenClawCronTime } from './utils'

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
const chatStore = useChatStore()
const loading = ref(false)
const runs = ref<OpenClawCronHistoryListItem[]>([])
const selectedRun = ref<OpenClawCronHistoryListItem | null>(null)
const selectedDetail = ref<OpenClawCronRunDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
const liveFinished = ref(false)
let detailLoadSequence = 0

type LiveToolState = {
  id: string
  name: string
  status: string
  detail: string
  agentLabel?: string
}

type LiveRetrievalItem = {
  source: string
  content: string
  score?: number
}

type LivePreviewSegment =
  | { type: 'thinking'; content: string; label?: string }
  | { type: 'content'; content: string; label?: string }
  | { type: 'tools'; toolCalls: LiveToolState[] }
  | { type: 'retrieval'; items: LiveRetrievalItem[]; label?: string }

const liveSegments = ref<LivePreviewSegment[]>([])
const liveToolMap = new Map<string, LiveToolState>()
let detailRefreshTimer: number | null = null
let eventUnsubscribe: (() => void) | null = null
let currentWatchId: string | null = null
let forwardedStreamKey = ''
let forwardedRequestId = ''
let forwardedMessageId = 0

// DETAIL_LOADING_SKELETON_ROWS keeps the loading placeholder visually stable while
// the right-side transcript detail is still resolving.
const DETAIL_LOADING_SKELETON_ROWS = 6

const hasRuns = computed(() => runs.value.length > 0)
const waitingForTriggeredRun = computed(() => false)
const shouldShowPreparingState = computed(
  () =>
    !detailLoading.value &&
    !detailError.value &&
    !selectedDetail.value?.conversation_id &&
    !selectedRun.value?.conversation_id
)

function displayRunStatusLabel(status: string) {
  if (status === 'running') return t('scheduledTasks.statusRunning')
  if (status === 'failed') return t('scheduledTasks.statusFailed')
  if (status === 'success') return t('scheduledTasks.statusSuccess')
  return t('scheduledTasks.statusPending')
}

function displayRunTriggerLabel(triggerType: string) {
  if (triggerType === 'schedule') return t('scheduledTasks.runTriggerSchedule')
  if (triggerType === 'manual') return t('scheduledTasks.runTriggerManual')
  return triggerType || t('scheduledTasks.statusPending')
}

async function loadRuns() {
  if (!props.job?.id) return
  loading.value = true
  try {
    runs.value = await OpenClawCronService.ListHistory(props.job.id, 50)
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
  const conversationId = Number(props.conversationId || 0)
  if (conversationId > 0) {
    return runs.value.find((item) => Number(item.conversation_id || 0) === conversationId) ?? null
  }
  const runId = String(props.runId || '').trim()
  if (runId) {
    return runs.value.find((item) => String(item.run_id || '').trim() === runId) ?? null
  }
  const triggerAtMs = Number(props.triggerAtMs || 0)
  if (!triggerAtMs) return null
  const thresholdMs = triggerAtMs - 5000
  return runs.value.find((item) => runTimestampMs(item) >= thresholdMs) ?? null
}

function isRunWaitingForAssociation(_run: OpenClawCronHistoryListItem) {
  return false
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
    // Prefer the local conversation mapping whenever it already exists so the
    // right panel can embed the standard OpenClaw assistant immediately.
    // 优先复用本地 conversation 映射，确保右侧直接嵌入任务助手对话框。
    if (Number(currentRun.conversation_id || 0) > 0 || !props.job?.id || !currentRun.session_id) {
      if (requestSequence !== detailLoadSequence) return
      selectedDetail.value = buildSyntheticDetail(currentRun)
      if (reconnect) {
        try {
          await reconnectGatewayStream()
        } catch (error) {
          console.warn('[openclaw-cron] reconnect gateway stream failed:', error)
        }
      }
    } else {
      const detail = await OpenClawCronService.GetRunDetail(props.job.id, currentRun.session_id)
      if (requestSequence !== detailLoadSequence) return
      selectedDetail.value = detail
      if (reconnect) {
        try {
          await reconnectGatewayStream()
        } catch (error) {
          console.warn('[openclaw-cron] reconnect gateway stream failed:', error)
        }
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
      action: run.source || '',
      status: run.status || '',
      error: '',
      summary: '',
      run_at_ms: run.run_at_ms || 0,
      duration_ms: 0,
      next_run_at_ms: 0,
      model: '',
      provider: '',
      delivery_status: '',
      session_id: run.session_id || '',
      session_key: run.session_key || '',
    },
    session_file_path: '',
    run_file_path: '',
    conversation_id: Number(run.conversation_id || 0),
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
  if (left.run_id && right.run_id) return left.run_id === right.run_id
  if (left.conversation_id && right.conversation_id) {
    return left.conversation_id === right.conversation_id
  }
  return false
}

// scheduleDetailRefresh batches transcript reloads after gateway events.
function scheduleDetailRefresh() {
  if (detailRefreshTimer) return
  detailRefreshTimer = window.setTimeout(async () => {
    detailRefreshTimer = null
    await loadDetail(false)
  }, 500)
}

async function reconnectGatewayStream() {
  await cleanupGatewayStream()
  resetLivePreview()

  if (!props.open || !props.job?.id || !selectedRun.value?.session_key) {
    return
  }

  currentWatchId = await OpenClawCronService.StartRunStream(
    props.job.id,
    selectedRun.value.session_id || '',
    selectedRun.value.session_key
  )

  eventUnsubscribe = Events.On('openclaw:cron-run-event', async (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    if (!payload || payload.watch_id !== currentWatchId) return
    forwardLiveEventToEmbeddedConversation(payload.event, payload.payload)
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
  resetLivePreview()
}

function resetLivePreview() {
  liveSegments.value = []
  liveToolMap.clear()
  liveFinished.value = false
  forwardedStreamKey = ''
  forwardedRequestId = ''
  forwardedMessageId = 0
}

// ensureForwardedChatStart boots the shared assistant store with a synthetic stream identity.
// ensureForwardedChatStart 用合成的 request/message 标识启动共享聊天状态，让右侧复用助手对话框实时刷新。
function ensureForwardedChatStart() {
  const conversationId = Number(selectedDetail.value?.conversation_id || selectedRun.value?.conversation_id || 0)
  if (!conversationId) return null

  const streamKey = String(
    selectedRun.value?.run_id ||
      selectedRun.value?.session_key ||
      selectedRun.value?.session_id ||
      conversationId
  ).trim()
  if (!streamKey) return null

  if (forwardedStreamKey === streamKey && forwardedRequestId && forwardedMessageId) {
    return {
      conversationId,
      requestId: forwardedRequestId,
      messageId: forwardedMessageId,
    }
  }

  forwardedStreamKey = streamKey
  forwardedRequestId = `openclaw-cron-${streamKey}`
  // Use a stable negative id so history view can render one in-flight assistant message per run.
  // 使用稳定的负数 messageId，保证每次运行只生成一条进行中的助手消息占位。
  forwardedMessageId = -Math.max(1, conversationId) * 1000000 - (Number(selectedRun.value?.run_at_ms || 0) % 1000000)

  chatStore.markOpenClawConversation(conversationId)
  chatStore.handleForwardedEvent(ChatEventType.START, {
    conversation_id: conversationId,
    request_id: forwardedRequestId,
    message_id: forwardedMessageId,
    status: 'streaming',
  })

  return {
    conversationId,
    requestId: forwardedRequestId,
    messageId: forwardedMessageId,
  }
}

// forwardLiveEventToEmbeddedConversation converts gateway raw frames into the same chat events
// consumed by the standard assistant page, instead of rendering a custom preview panel.
// forwardLiveEventToEmbeddedConversation 将网关原始事件转换为标准聊天事件，直接驱动复用的助手对话框。
function forwardLiveEventToEmbeddedConversation(eventName: string, rawPayload: any) {
  let payload: any = rawPayload
  if (typeof rawPayload === 'string') {
    try {
      payload = JSON.parse(rawPayload)
    } catch {
      return
    }
  }
  if (!payload || typeof payload !== 'object') return

  const streamState = ensureForwardedChatStart()
  if (!streamState) return

  if (eventName === 'chat') {
    const state = String(payload.state || '')
    if (state === 'final') {
      liveFinished.value = true
      chatStore.handleForwardedEvent(ChatEventType.COMPLETE, {
        conversation_id: streamState.conversationId,
        request_id: streamState.requestId,
        message_id: streamState.messageId,
        status: 'success',
        finish_reason: 'stop',
      })
    } else if (state === 'aborted') {
      liveFinished.value = true
      chatStore.handleForwardedEvent(ChatEventType.STOPPED, {
        conversation_id: streamState.conversationId,
        request_id: streamState.requestId,
        message_id: streamState.messageId,
        status: 'cancelled',
      })
    } else if (state === 'error') {
      liveFinished.value = true
      chatStore.handleForwardedEvent(ChatEventType.ERROR, {
        conversation_id: streamState.conversationId,
        request_id: streamState.requestId,
        message_id: streamState.messageId,
        status: 'error',
        error_key: 'error.chat_generation_failed',
        error_data: { Error: String(payload.errorMessage || '') },
      })
    }
    return
  }

  if (eventName !== 'agent') return

  const stream = String(payload.stream || '')
  const data = payload.data || {}

  if (stream === 'assistant') {
    const delta = String(data.delta || data.text || '')
    if (!delta) return
    chatStore.handleForwardedEvent(ChatEventType.CHUNK, {
      conversation_id: streamState.conversationId,
      request_id: streamState.requestId,
      message_id: streamState.messageId,
      delta,
    })
    return
  }
  if (stream === 'thinking') {
    const delta = String(data.delta || data.text || '')
    if (!delta) return
    chatStore.handleForwardedEvent(ChatEventType.THINKING, {
      conversation_id: streamState.conversationId,
      request_id: streamState.requestId,
      message_id: streamState.messageId,
      delta,
      new_block: Boolean(data.newBlock),
    })
    return
  }
  if (stream === 'retrieval') {
    const rawItems = Array.isArray(data.items)
      ? data.items
      : Array.isArray(data.results)
        ? data.results
        : Array.isArray(data.documents)
          ? data.documents
          : []
    const items = rawItems
      .map((item: any) => ({
        source: item?.source === 'memory' ? 'memory' : 'knowledge',
        content: String(item?.content || item?.text || item?.snippet || ''),
        score: Number(item?.score ?? 0),
      }))
      .filter((item: { content: string }) => item.content)
    if (items.length === 0) return
    chatStore.handleForwardedEvent(ChatEventType.RETRIEVAL, {
      conversation_id: streamState.conversationId,
      request_id: streamState.requestId,
      message_id: streamState.messageId,
      items,
    })
    return
  }
  if (stream === 'tool') {
    const phase = String(data.phase || '')
    if (phase !== 'start' && phase !== 'result') return
    chatStore.handleForwardedEvent(ChatEventType.TOOL, {
      conversation_id: streamState.conversationId,
      request_id: streamState.requestId,
      message_id: streamState.messageId,
      type: phase === 'start' ? 'call' : 'result',
      tool_call_id: String(data.toolCallId || ''),
      tool_name: String(data.name || ''),
      args_json: data.args == null ? '' : JSON.stringify(data.args),
      result_json:
        data.result == null
          ? String(data.meta || data.error || data.message || '')
          : JSON.stringify(data.result),
    })
    return
  }
  if (stream === 'lifecycle') {
    const phase = String(data.phase || '')
    if (phase === 'end' || phase === 'error') {
      liveFinished.value = true
    }
    if (phase === 'error') {
      chatStore.handleForwardedEvent(ChatEventType.ERROR, {
        conversation_id: streamState.conversationId,
        request_id: streamState.requestId,
        message_id: streamState.messageId,
        status: 'error',
        error_key: 'error.chat_generation_failed',
        error_data: { Error: String(data.error || data.message || '') },
      })
      return
    }
    if (phase === 'end') {
      chatStore.handleForwardedEvent(ChatEventType.COMPLETE, {
        conversation_id: streamState.conversationId,
        request_id: streamState.requestId,
        message_id: streamState.messageId,
        status: 'success',
        finish_reason: 'stop',
      })
    }
  }
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
  () => [
    selectedRun.value?.session_id,
    selectedRun.value?.session_key,
    selectedRun.value?.conversation_id,
    selectedRun.value?.run_id,
  ],
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
        <DialogTitle>{{ job?.name }} / {{ t('openclawCron.history.title', '运行历史') }}</DialogTitle>
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
            {{ t('openclawCron.history.waitingTriggeredRun', '任务已触发，正在等待 OpenClaw 写入新的运行记录...') }}
          </div>
          <div v-else-if="!hasRuns" class="p-4 text-sm text-muted-foreground">
            {{ t('openclawCron.history.empty', '暂无运行历史') }}
          </div>
          <button
            v-for="run in runs"
            :key="`${run.run_id || run.session_id || run.conversation_id || run.run_at_ms}`"
            class="w-full border-b border-border px-3 py-3 text-left transition-colors hover:bg-accent/40"
            :class="isSameHistoryItem(run, selectedRun) ? 'bg-accent/50' : ''"
            @click="selectedRun = run"
          >
            <div class="flex items-start gap-2 overflow-hidden">
              <TaskRunStatusBadge
                class="mt-0.5 shrink-0"
                :status="run.status"
                :label="displayRunStatusLabel(run.status)"
              />
              <div class="flex min-w-0 flex-1 flex-col items-start gap-1 text-left">
                <div class="w-full text-[11px] leading-4 text-foreground">
                  {{ formatOpenClawCronTime(run.run_at_ms) }}
                </div>
                <div class="flex w-full items-center gap-1 text-[11px] text-muted-foreground">
                  <span class="shrink-0">{{ displayRunTriggerLabel(run.trigger_type || run.source) }}</span>
                  <span class="shrink-0 text-muted-foreground/50">&middot;</span>
                  <span class="truncate">{{ formatDurationMs(run.duration_ms) }}</span>
                  <span
                    v-if="isRunWaitingForAssociation(run)"
                    class="shrink-0 text-muted-foreground/70"
                  >
                    · {{ t('openclawCron.history.pending', '等待关联') }}
                  </span>
                </div>
              </div>
            </div>
          </button>
        </div>

        <div class="min-h-0 flex-1 overflow-hidden rounded-lg border border-border">
          <div
            v-if="detailLoading"
            class="flex h-full flex-col gap-4 p-6"
          >
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
          <div
            v-else-if="detailError"
            class="flex h-full items-center justify-center p-6"
          >
            <div class="w-full max-w-lg rounded-xl border border-[#fecaca] bg-[#fff5f5] p-5 text-left">
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
          <EmbeddedAssistantPage
            v-else
            :conversation-id="selectedDetail?.conversation_id || selectedRun?.conversation_id || 0"
            :agent-id="selectedDetail?.conversation_agent_id || null"
            :read-only="true"
          />
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
