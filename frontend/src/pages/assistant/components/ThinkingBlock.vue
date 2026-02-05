<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, Brain } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

const props = defineProps<{
  content: string
  isStreaming?: boolean
}>()

const { t } = useI18n()

const isExpanded = ref(true)

const previewContent = computed(() => {
  const lines = props.content.split('\n').filter((line) => line.trim())
  if (lines.length <= 3) return props.content
  return lines.slice(0, 3).join('\n') + '...'
})
</script>

<template>
  <div
    class="flex min-w-0 max-w-full flex-col gap-1 rounded-lg border border-border/50 bg-muted/30 px-3 py-2 text-sm dark:bg-zinc-900/50"
  >
    <!-- Header -->
    <button
      class="flex items-center gap-2 text-left text-muted-foreground hover:text-foreground"
      @click="isExpanded = !isExpanded"
    >
      <Brain class="size-4" />
      <span class="flex-1 text-xs font-medium">{{ t('assistant.chat.thinking') }}</span>
      <span v-if="isStreaming" class="text-xs opacity-70">
        {{ t('assistant.chat.thinkingInProgress') }}
      </span>
      <ChevronDown
        :class="cn('size-4 transition-transform', isExpanded && 'rotate-180')"
      />
    </button>

    <!-- Content -->
    <div v-if="isExpanded" class="text-xs text-muted-foreground">
      <p class="whitespace-pre-wrap wrap-break-word opacity-80">{{ content }}</p>
      <span v-if="isStreaming" class="animate-pulse">▌</span>
    </div>
    <div v-else class="text-xs text-muted-foreground">
      <p class="line-clamp-2 opacity-60">{{ previewContent }}</p>
    </div>
  </div>
</template>
