<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, Lightbulb } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

const props = defineProps<{
  content: string
  isStreaming?: boolean
}>()

const { t } = useI18n()

const isExpanded = ref(true)
</script>

<template>
  <div class="flex w-full flex-col gap-2">
    <!-- Header -->
    <button
      class="flex w-fit items-center gap-1.5 text-left text-muted-foreground/60 hover:text-muted-foreground transition-colors"
      @click="isExpanded = !isExpanded"
    >
      <Lightbulb class="size-3.5" />
      <span class="text-xs font-medium">
        {{ isStreaming ? t('assistant.chat.thinkingInProgress') : t('assistant.chat.thinking') }}
      </span>
      <ChevronDown :class="cn('size-3.5 transition-transform', isExpanded && 'rotate-180')" />
    </button>

    <!-- Content (only show when expanded) -->
    <div v-if="isExpanded" class="w-full border-l-2 border-border/30 pl-3 text-xs text-muted-foreground/70">
      <span class="whitespace-pre-wrap wrap-break-word">{{ content }}</span>
      <span v-if="isStreaming" class="ml-0.5 inline-block h-3.5 w-[2px] animate-pulse bg-current align-middle"></span>
    </div>
  </div>
</template>
