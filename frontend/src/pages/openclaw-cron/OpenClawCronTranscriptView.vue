<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Brain, Bot, FileJson, Wrench } from 'lucide-vue-next'
import MarkdownRenderer from '@/components/MarkdownRenderer.vue'
import TaskRunStatusBadge from '@/pages/scheduled-tasks/components/TaskRunStatusBadge.vue'
import type {
  OpenClawCronMessageBlock,
  OpenClawCronRunDetail,
  OpenClawCronTranscriptMessage,
} from '@bindings/chatclaw/internal/openclaw/cron'
import { normalizeOpenClawRunStatus } from './status'
import { formatDurationMs, formatOpenClawCronTime } from './utils'

const props = defineProps<{
  detail: OpenClawCronRunDetail
}>()

const { t } = useI18n()

const TOOL_CALL_BLOCK_TYPES = new Set(['tool_call', 'tool-call', 'toolCall', 'tool_use'])
const TOOL_RESULT_BLOCK_TYPES = new Set(['tool_result', 'tool-result', 'toolResult', 'tool_output'])
const THINKING_BLOCK_TYPE = 'thinking'
const TEXT_BLOCK_TYPE = 'text'

const transcriptMessages = computed(() => props.detail.messages ?? [])

function normalizeRunStatus(status: string) {
  return normalizeOpenClawRunStatus(status)
}

function displayRunStatusLabel(status: string) {
  return normalizeRunStatus(status) === 'failed'
    ? t('scheduledTasks.statusFailed')
    : t('scheduledTasks.statusSuccess')
}

function displayRoleLabel(role: string) {
  if (role === 'assistant') return t('chat.role.assistant', 'Assistant')
  if (role === 'user') return t('chat.role.user', 'User')
  if (role === 'tool') return t('chat.role.tool', 'Tool')
  if (role === 'system') return t('chat.role.system', 'System')
  return role || t('common.unknown', 'Unknown')
}

function roleToneClass(role: string) {
  if (role === 'assistant') return 'border-[#bfdbfe] bg-[#eff6ff] text-[#1d4ed8]'
  if (role === 'tool') return 'border-[#d9f99d] bg-[#f7fee7] text-[#4d7c0f]'
  if (role === 'system') return 'border-[#e5e7eb] bg-[#f8fafc] text-[#475569]'
  return 'border-border bg-muted/40 text-foreground'
}

function isThinkingBlock(block: OpenClawCronMessageBlock) {
  return block.type === THINKING_BLOCK_TYPE
}

function isTextBlock(block: OpenClawCronMessageBlock) {
  return block.type === TEXT_BLOCK_TYPE
}

function isToolCallBlock(block: OpenClawCronMessageBlock) {
  return (
    TOOL_CALL_BLOCK_TYPES.has(block.type) ||
    (!!block.name && !!block.args_json && !block.result_json)
  )
}

function isToolResultBlock(block: OpenClawCronMessageBlock) {
  return TOOL_RESULT_BLOCK_TYPES.has(block.type) || (!!block.name && !!block.result_json)
}

function prettyStructuredText(value: string | undefined) {
  const trimmed = String(value || '').trim()
  if (!trimmed) return ''
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2)
  } catch {
    return trimmed
  }
}

function fallbackRawText(block: OpenClawCronMessageBlock) {
  const rawValue = block.raw as unknown
  if (typeof rawValue === 'string') {
    return prettyStructuredText(rawValue)
  }
  try {
    return JSON.stringify(rawValue ?? {}, null, 2)
  } catch {
    return ''
  }
}

function blockTitle(block: OpenClawCronMessageBlock) {
  if (isThinkingBlock(block)) return t('openclawCron.history.thinking', '思考过程')
  if (isToolCallBlock(block)) return t('openclawCron.history.toolCall', '工具调用')
  if (isToolResultBlock(block)) return t('openclawCron.history.toolResult', '工具结果')
  if (isTextBlock(block)) return t('openclawCron.history.output', '输出')
  return t('openclawCron.history.rawBlock', '原始块')
}

function metadataItems(detail: OpenClawCronRunDetail) {
  const items = [
    {
      key: 'run_at',
      label: t('openclawCron.history.runAt', '运行时间'),
      value: formatOpenClawCronTime(detail.run.run_at_ms || detail.run.ts || 0),
    },
    {
      key: 'duration',
      label: t('openclawCron.history.duration', '耗时'),
      value: formatDurationMs(detail.run.duration_ms),
    },
  ]

  if (detail.run.provider) {
    items.push({
      key: 'provider',
      label: t('openclawCron.history.provider', '提供商'),
      value: detail.run.provider,
    })
  }
  if (detail.run.model) {
    items.push({
      key: 'model',
      label: t('openclawCron.history.model', '模型'),
      value: detail.run.model,
    })
  }
  if (detail.run.delivery_status) {
    items.push({
      key: 'delivery',
      label: t('openclawCron.history.delivery', '投递状态'),
      value: detail.run.delivery_status,
    })
  }

  return items
}

function messageTimestamp(message: OpenClawCronTranscriptMessage) {
  const timestamp = String(message.timestamp || '').trim()
  if (!timestamp) return ''
  return timestamp.replace('T', ' ').replace('Z', '')
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col overflow-y-auto bg-background">
    <div class="shrink-0 border-b border-border px-5 py-4">
      <div class="flex flex-wrap items-center gap-3">
        <TaskRunStatusBadge
          :status="normalizeRunStatus(detail.run.status)"
          :label="displayRunStatusLabel(detail.run.status)"
        />
        <span
          v-if="detail.is_live"
          class="rounded-full border border-[#bfdbfe] bg-[#eff6ff] px-2 py-0.5 text-xs font-medium text-[#2563eb]"
        >
          {{ t('openclawCron.history.live', '实时中') }}
        </span>
      </div>

      <div class="mt-4 grid gap-3 text-sm text-muted-foreground md:grid-cols-2 xl:grid-cols-4">
        <div
          v-for="item in metadataItems(detail)"
          :key="item.key"
          class="rounded-lg border border-border bg-muted/20 px-3 py-2"
        >
          <div class="text-xs uppercase tracking-wide text-muted-foreground/80">
            {{ item.label }}
          </div>
          <div class="mt-1 break-all text-foreground">
            {{ item.value || '-' }}
          </div>
        </div>
      </div>

      <div
        v-if="detail.run.summary"
        class="mt-4 rounded-lg border border-border bg-muted/20 px-3 py-3"
      >
        <div class="text-xs uppercase tracking-wide text-muted-foreground/80">
          {{ t('openclawCron.history.summary', '运行摘要') }}
        </div>
        <p class="mt-1 whitespace-pre-wrap break-words text-sm text-foreground">
          {{ detail.run.summary }}
        </p>
      </div>

      <div
        v-if="detail.run.error"
        class="mt-4 rounded-lg border border-[#fecaca] bg-[#fff5f5] px-3 py-3"
      >
        <div class="text-xs uppercase tracking-wide text-[#b91c1c]">
          {{ t('openclawCron.history.error', '错误信息') }}
        </div>
        <p class="mt-1 whitespace-pre-wrap break-words text-sm text-[#7f1d1d]">
          {{ detail.run.error }}
        </p>
      </div>
    </div>

    <!--
    <div v-if="transcriptMessages.length === 0" class="flex flex-1 items-center justify-center p-6">
      <div class="max-w-md text-center text-sm text-muted-foreground">
        {{ t('openclawCron.history.transcriptEmpty', 'OpenClaw 尚未写入这次运行的 transcript。') }}
      </div>
    </div>

    <div v-else class="flex-1 overflow-y-auto px-5 py-4">
      <div class="space-y-4">
        <article
          v-for="message in transcriptMessages"
          :key="message.id || `${message.role}-${messageTimestamp(message)}`"
          class="rounded-xl border border-border bg-card p-4 shadow-sm"
        >
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div
              class="inline-flex items-center gap-2 rounded-full border px-2.5 py-1 text-xs font-medium"
              :class="roleToneClass(message.role)"
            >
              <Bot v-if="message.role === 'assistant'" class="size-3.5" />
              <Wrench v-else-if="message.role === 'tool'" class="size-3.5" />
              <FileJson v-else-if="message.role === 'system'" class="size-3.5" />
              <Brain v-else class="size-3.5" />
              <span>{{ displayRoleLabel(message.role) }}</span>
            </div>
            <div class="text-xs text-muted-foreground">
              {{ messageTimestamp(message) || '-' }}
            </div>
          </div>

          <div class="mt-4 space-y-3">
            <div
              v-for="(block, index) in message.blocks"
              :key="`${message.id}-${index}-${block.type}`"
              class="rounded-lg border border-border/70 bg-muted/10 p-3"
            >
              <div
                class="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground/80"
              >
                {{ blockTitle(block) }}
                <span v-if="block.name" class="normal-case tracking-normal text-foreground">
                  · {{ block.name }}
                </span>
              </div>

              <MarkdownRenderer v-if="isTextBlock(block)" :content="block.text || ''" />

              <pre
                v-else-if="isThinkingBlock(block)"
                class="whitespace-pre-wrap break-words rounded-md bg-[#f8fafc] px-3 py-2 font-mono text-xs leading-6 text-[#475569]"
                >{{ block.thinking || '' }}</pre
              >

              <div v-else-if="isToolCallBlock(block)" class="space-y-2">
                <div v-if="block.tool_call_id" class="text-xs text-muted-foreground">
                  ID: {{ block.tool_call_id }}
                </div>
                <pre
                  class="whitespace-pre-wrap break-words rounded-md bg-[#0f172a] px-3 py-2 font-mono text-xs leading-6 text-[#e2e8f0]"
                  >{{ prettyStructuredText(block.args_json) }}</pre
                >
              </div>

              <div v-else-if="isToolResultBlock(block)" class="space-y-2">
                <div v-if="block.tool_call_id" class="text-xs text-muted-foreground">
                  ID: {{ block.tool_call_id }}
                </div>
                <pre
                  class="whitespace-pre-wrap break-words rounded-md bg-[#111827] px-3 py-2 font-mono text-xs leading-6 text-[#e5e7eb]"
                  >{{ prettyStructuredText(block.result_json) }}</pre
                >
              </div>

              <pre
                v-else
                class="whitespace-pre-wrap break-words rounded-md bg-[#111827] px-3 py-2 font-mono text-xs leading-6 text-[#e5e7eb]"
                >{{ fallbackRawText(block) }}</pre
              >
            </div>
          </div>
        </article>
      </div>
    </div>
    -->
  </div>
</template>
