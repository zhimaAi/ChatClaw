<script setup lang="ts">
import MarkdownRenderer from '@/components/MarkdownRenderer.vue'
import type { ScheduledTaskRunDetail } from '../types'
import { formatTaskTime } from '../utils'

defineProps<{
  detail: ScheduledTaskRunDetail | null
  emptyText: string
}>()
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div v-if="!detail?.conversation" class="flex h-full items-center justify-center text-sm text-muted-foreground">
      {{ emptyText }}
    </div>
    <template v-else>
      <div class="border-b border-border px-4 py-3">
        <div class="text-sm font-medium text-foreground">{{ detail.conversation.name }}</div>
        <div class="mt-1 text-xs text-muted-foreground">
          {{ formatTaskTime(detail.conversation.created_at) }}
        </div>
      </div>
      <div class="min-h-0 flex-1 overflow-auto px-4 py-3">
        <div v-for="message in detail.messages" :key="message.id" class="mb-3 rounded-lg border border-border bg-card p-3">
          <div class="mb-2 text-xs uppercase tracking-wide text-muted-foreground">{{ message.role }}</div>
          <MarkdownRenderer :content="message.content || ''" />
        </div>
      </div>
    </template>
  </div>
</template>
