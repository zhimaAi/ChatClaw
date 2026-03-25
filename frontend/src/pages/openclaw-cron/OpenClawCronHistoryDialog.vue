<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { RefreshCcw } from 'lucide-vue-next'
import EmbeddedAssistantPage from '@/pages/openclaw/components/EmbeddedAssistantPage.vue'
import {
  OpenClawCronService,
  type OpenClawCronJob,
  type OpenClawCronRunDetail,
  type OpenClawCronRunEntry,
} from '@bindings/chatclaw/internal/openclaw/cron'
import { formatDurationMs, formatOpenClawCronTime } from './utils'

const props = defineProps<{
  open: boolean
  job: OpenClawCronJob | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const { t } = useI18n()
const loading = ref(false)
const runs = ref<OpenClawCronRunEntry[]>([])
const selectedRun = ref<OpenClawCronRunEntry | null>(null)
const selectedDetail = ref<OpenClawCronRunDetail | null>(null)
const liveFinished = ref(false)
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
let detailRefreshTimer: ReturnType<typeof setTimeout> | null = null
let eventUnsubscribe: (() => void) | null = null
let currentWatchId: string | null = null

const hasRuns = computed(() => runs.value.length > 0)

async function loadRuns() {
  if (!props.job?.id) return
  loading.value = true
  try {
    runs.value = await OpenClawCronService.ListRuns(props.job.id, 50)
    if (!selectedRun.value && runs.value[0]) {
      selectedRun.value = runs.value[0]
    }
    if (selectedRun.value) {
      const latest = runs.value.find((item) => item.session_id === selectedRun.value?.session_id)
      if (latest) selectedRun.value = latest
      await loadDetail(!currentWatchId)
    }
  } finally {
    loading.value = false
  }
}

async function loadDetail(reconnect = false) {
  if (!props.job?.id || !selectedRun.value?.session_id) {
    selectedDetail.value = null
    return
  }
  selectedDetail.value = await OpenClawCronService.GetRunDetail(
    props.job.id,
    selectedRun.value.session_id
  )
  if (reconnect) {
    await reconnectGatewayStream()
  }
}

// scheduleDetailRefresh batches transcript reloads after gateway events.
// scheduleDetailRefresh 对 gateway 高频事件做节流刷新，避免每个 chunk 都触发一次详情读取。
function scheduleDetailRefresh() {
  if (detailRefreshTimer) return
  detailRefreshTimer = setTimeout(async () => {
    detailRefreshTimer = null
    await loadDetail(false)
  }, 500)
}

async function reconnectGatewayStream() {
  await cleanupGatewayStream()
  resetLivePreview()

  if (
    !props.open ||
    !props.job?.id ||
    !selectedRun.value?.session_id ||
    !selectedRun.value?.session_key
  ) {
    return
  }

  currentWatchId = await OpenClawCronService.StartRunStream(
    props.job.id,
    selectedRun.value.session_id,
    selectedRun.value.session_key
  )

  eventUnsubscribe = Events.On('openclaw:cron-run-event', async (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    if (!payload || payload.watch_id !== currentWatchId) return
    applyLiveEvent(payload.event, payload.payload)
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
}

// appendTextSegment keeps live preview in transcript order by merging adjacent same-type text blocks.
// appendTextSegment 按消息顺序维护实时片段，并合并相邻同类型文本块。
function appendTextSegment(
  type: 'thinking' | 'content',
  chunk: string,
  forceNewBlock = false,
  label?: string
) {
  if (!chunk) return
  const lastSegment = liveSegments.value[liveSegments.value.length - 1]
  if (
    !forceNewBlock &&
    lastSegment?.type === type &&
    (lastSegment.label || '') === (label || '')
  ) {
    lastSegment.content += chunk
    return
  }
  liveSegments.value.push({ type, content: chunk, label })
}

// upsertLiveTool updates tool state in place so the UI stays stable across start/update/end events.
// upsertLiveTool 原地更新工具调用状态，保证 start/update/end 过程不会在 UI 中抖动。
function upsertLiveTool(tool: LiveToolState) {
  const existing = liveToolMap.get(tool.id)
  if (existing) {
    existing.name = tool.name || existing.name
    existing.status = tool.status || existing.status
    existing.detail = tool.detail || existing.detail
    return
  }
  const created = { ...tool }
  liveToolMap.set(created.id, created)
  const lastSegment = liveSegments.value[liveSegments.value.length - 1]
  if (lastSegment?.type === 'tools') {
    lastSegment.toolCalls.push(created)
    return
  }
  liveSegments.value.push({ type: 'tools', toolCalls: [created] })
}

function stringifyPreviewPayload(value: unknown) {
  if (value == null) return ''
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function normalizeRunPath(value: unknown) {
  if (!Array.isArray(value)) return ''
  const items = value
    .map((item) => String(item || '').trim())
    .filter(Boolean)
  return items.join(' > ')
}

function extractAgentLabel(payload: any, data: any) {
  const runPath =
    normalizeRunPath(data?.runPath) ||
    normalizeRunPath(payload?.runPath) ||
    normalizeRunPath(data?.run_path) ||
    normalizeRunPath(payload?.run_path)
  if (runPath) return runPath
  const agentName = String(data?.agentName || payload?.agentName || data?.agent || payload?.agent || '')
  return agentName.trim()
}

function appendRetrievalSegment(items: LiveRetrievalItem[], label?: string) {
  if (items.length === 0) return
  liveSegments.value.push({ type: 'retrieval', items, label })
}

function applyLiveEvent(eventName: string, rawPayload: any) {
  let payload: any = rawPayload
  if (typeof rawPayload === 'string') {
    try {
      payload = JSON.parse(rawPayload)
    } catch {
      return
    }
  }
  if (!payload || typeof payload !== 'object') return

  if (eventName === 'chat') {
    const state = String(payload.state || '')
    if (state === 'final' || state === 'aborted' || state === 'error') {
      liveFinished.value = true
    }
    if (state === 'error' && payload.errorMessage) {
      appendTextSegment('content', `\n[error] ${String(payload.errorMessage)}`, true)
    }
    return
  }

  if (eventName !== 'agent') return

  const stream = String(payload.stream || '')
  const data = payload.data || {}
  const agentLabel = extractAgentLabel(payload, data)

  if (stream === 'assistant') {
    const delta = String(data.delta || data.text || '')
    appendTextSegment('content', delta, false, agentLabel)
    return
  }
  if (stream === 'thinking') {
    const delta = String(data.delta || data.text || '')
    appendTextSegment('thinking', delta, Boolean(data.newBlock), agentLabel)
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
        source: String(item?.source || item?.type || 'retrieval'),
        content: String(item?.content || item?.text || item?.snippet || ''),
        score: item?.score == null ? undefined : Number(item.score),
      }))
      .filter((item: LiveRetrievalItem) => item.content)
    appendRetrievalSegment(items, agentLabel)
    return
  }
  if (stream === 'tool') {
    const toolCallId = String(data.toolCallId || '')
    const toolName = String(data.name || '')
    const phase = String(data.phase || '')
    const detailParts = [
      phase ? `phase: ${phase}` : '',
      data.isError ? 'isError: true' : '',
      stringifyPreviewPayload(data.args),
      stringifyPreviewPayload(data.result),
      stringifyPreviewPayload(data.meta),
      stringifyPreviewPayload(data.error || data.message),
    ].filter(Boolean)
    upsertLiveTool({
      id: toolCallId || `${toolName}-${liveToolMap.size}`,
      name: toolName || 'tool',
      status: phase || 'unknown',
      detail: detailParts.join('\n\n'),
      agentLabel,
    })
    return
  }
  if (stream === 'lifecycle') {
    const phase = String(data.phase || '')
    if (phase === 'end' || phase === 'error') {
      liveFinished.value = true
    }
    if (phase === 'error') {
      appendTextSegment(
        'content',
        `\n[error] ${String(data.error || data.message || '')}`,
        true,
        agentLabel
      )
    }
  }
}

watch(
  () => props.open,
  async (open) => {
    if (!open) {
      await cleanupGatewayStream()
      return
    }
    await loadRuns()
  },
  { immediate: true }
)

watch(
  () => selectedRun.value?.session_id,
  async () => {
    if (props.open) {
      await loadDetail(true)
    }
  }
)

onBeforeUnmount(() => {
  if (detailRefreshTimer) {
    clearTimeout(detailRefreshTimer)
    detailRefreshTimer = null
  }
  void cleanupGatewayStream()
})
</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent
      class="max-h-[90vh] overflow-hidden sm:!w-auto sm:min-w-[1100px] sm:!max-w-[1760px]"
    >
      <DialogHeader class="flex flex-row items-center justify-between gap-4">
        <DialogTitle>{{ job?.name }} / {{ t('openclawCron.history.title', '运行历史') }}</DialogTitle>
        <Button variant="outline" size="sm" class="gap-1" @click="loadRuns">
          <RefreshCcw class="size-4" />
          {{ t('openclawCron.refresh', '刷新') }}
        </Button>
      </DialogHeader>

      <div class="flex h-[70vh] min-h-0 gap-4">
        <div
          class="shrink-0 overflow-y-auto overflow-x-hidden rounded-lg border border-border sm:w-[280px]"
        >
          <div v-if="loading" class="p-4 text-sm text-muted-foreground">
            {{ t('common.loading', '加载中...') }}
          </div>
          <div v-else-if="!hasRuns" class="p-4 text-sm text-muted-foreground">
            {{ t('openclawCron.history.empty', '暂无运行历史') }}
          </div>
          <button
            v-for="run in runs"
            :key="`${run.session_id}-${run.run_at_ms}`"
            class="w-full border-b border-border px-3 py-3 text-left transition-colors hover:bg-accent/40"
            :class="selectedRun?.session_id === run.session_id ? 'bg-accent/50' : ''"
            @click="selectedRun = run"
          >
            <div class="space-y-1">
              <div class="flex items-center justify-between gap-2">
                <div class="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {{ run.status || run.action }}
                </div>
                <div class="text-xs text-muted-foreground">
                  {{ formatDurationMs(run.duration_ms) }}
                </div>
              </div>
              <div class="text-sm text-foreground">
                {{ formatOpenClawCronTime(run.run_at_ms || run.ts) }}
              </div>
              <div v-if="run.error" class="line-clamp-2 text-xs text-red-600">
                {{ run.error }}
              </div>
            </div>
          </button>
        </div>

        <div class="min-h-0 flex-1 overflow-hidden rounded-lg border border-border">
          <div
            v-if="!selectedDetail"
            class="flex h-full items-center justify-center text-sm text-muted-foreground"
          >
            {{ t('openclawCron.history.selectRun', '请选择一次运行记录') }}
          </div>
          <div v-else class="flex h-full min-h-0 flex-col">
            <div class="border-b border-border px-4 py-3">
              <div class="flex flex-wrap items-center gap-3">
                <span class="text-sm font-medium text-foreground">{{ selectedDetail.run.status }}</span>
                <span class="text-xs text-muted-foreground">
                  {{ formatOpenClawCronTime(selectedDetail.run.run_at_ms || selectedDetail.run.ts) }}
                </span>
                <span v-if="selectedDetail.is_live" class="text-xs font-medium text-emerald-600">
                  {{ t('openclawCron.history.live', '运行中，实时刷新') }}
                </span>
              </div>
              <div class="mt-1 text-xs text-muted-foreground">
                {{ selectedDetail.run.session_key || selectedDetail.run.session_id }}
              </div>
              <div
                v-if="selectedDetail.run.error"
                class="mt-2 whitespace-pre-wrap text-xs text-red-600"
              >
                {{ selectedDetail.run.error }}
              </div>
            </div>

            <div class="min-h-0 flex-1 overflow-auto px-4 py-3">
              <div
                v-if="selectedDetail.is_live && liveSegments.length > 0"
                class="mb-4 rounded-lg border border-emerald-200 bg-emerald-50/60 p-4"
              >
                <div class="mb-2 flex items-center justify-between gap-3">
                  <div class="text-xs font-semibold uppercase tracking-wide text-emerald-700">
                    {{ t('openclawCron.history.livePreview', '实时预览') }}
                  </div>
                  <div class="text-xs text-emerald-700">
                    {{ liveFinished ? t('openclawCron.history.liveFinished', '等待落盘完成') : t('openclawCron.history.liveStreaming', 'Gateway 推流中') }}
                  </div>
                </div>
                <div class="space-y-3">
                  <div
                    v-for="(segment, segmentIndex) in liveSegments"
                    :key="`live-segment-${segmentIndex}`"
                    class="rounded-md border border-emerald-200 bg-white px-3 py-2"
                  >
                    <template v-if="segment.type === 'thinking'">
                      <div class="mb-1 flex items-center justify-between gap-2 text-[11px] uppercase tracking-wide text-muted-foreground">
                        <span>thinking</span>
                        <span v-if="segment.label" class="normal-case tracking-normal">{{ segment.label }}</span>
                      </div>
                      <pre class="whitespace-pre-wrap break-words text-sm text-muted-foreground">{{ segment.content }}</pre>
                    </template>
                    <template v-else-if="segment.type === 'content'">
                      <div class="mb-1 flex items-center justify-between gap-2 text-[11px] uppercase tracking-wide text-muted-foreground">
                        <span>assistant</span>
                        <span v-if="segment.label" class="normal-case tracking-normal">{{ segment.label }}</span>
                      </div>
                      <pre class="whitespace-pre-wrap break-words text-sm text-foreground">{{ segment.content }}</pre>
                    </template>
                    <template v-else-if="segment.type === 'retrieval'">
                      <div class="mb-1 flex items-center justify-between gap-2 text-[11px] uppercase tracking-wide text-muted-foreground">
                        <span>retrieval</span>
                        <span v-if="segment.label" class="normal-case tracking-normal">{{ segment.label }}</span>
                      </div>
                      <div
                        v-for="(item, itemIndex) in segment.items"
                        :key="`retrieval-${segmentIndex}-${itemIndex}`"
                        class="rounded-md border border-emerald-100 bg-emerald-50/30 px-3 py-2 not-first:mt-2"
                      >
                        <div class="mb-1 flex items-center justify-between gap-2 text-[11px] uppercase tracking-wide text-muted-foreground">
                          <span>{{ item.source }}</span>
                          <span v-if="item.score != null">{{ item.score }}</span>
                        </div>
                        <pre class="whitespace-pre-wrap break-words text-xs text-foreground">{{ item.content }}</pre>
                      </div>
                    </template>
                    <template v-else>
                      <div
                        v-for="tool in segment.toolCalls"
                        :key="tool.id"
                        class="rounded-md border border-emerald-100 bg-emerald-50/30 px-3 py-2 not-first:mt-2"
                      >
                        <div class="mb-1 flex items-center justify-between gap-2 text-[11px] uppercase tracking-wide text-muted-foreground">
                          <span>{{ tool.agentLabel ? `${tool.name} · ${tool.agentLabel}` : tool.name }}</span>
                          <span>{{ tool.status }}</span>
                        </div>
                        <pre class="whitespace-pre-wrap break-words text-xs text-foreground">{{ tool.detail }}</pre>
                      </div>
                    </template>
                  </div>
                </div>
              </div>

              <div v-if="selectedDetail.conversation_id" class="h-full min-h-0 overflow-hidden">
                <EmbeddedAssistantPage
                  :conversation-id="selectedDetail.conversation_id"
                  :agent-id="selectedDetail.conversation_agent_id"
                  :read-only="true"
                />
              </div>

              <div
                v-else
                class="flex h-full items-center justify-center text-sm text-muted-foreground"
              >
                {{ t('openclawCron.history.conversationPreparing', '正在准备历史会话，请稍候...') }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
