<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import type { RetrievalItemInfo } from '@/stores'
import IconKnowledge from '@/assets/icons/knowledge.svg'
import IconMemory from '@/assets/icons/memory.svg'

const props = defineProps<{
  items: RetrievalItemInfo[]
}>()

const { t } = useI18n()

const knowledgeExpanded = ref(false)
const memoryExpanded = ref(false)

const knowledgeItems = computed(() => props.items.filter((i) => i.source === 'knowledge'))
const memoryItems = computed(() => props.items.filter((i) => i.source === 'memory'))
</script>

<template>
  <div class="min-w-0 w-full max-w-[560px] flex flex-col gap-2">
    <!-- Knowledge base retrieval -->
    <div
      v-if="knowledgeItems.length > 0"
      class="w-full min-w-0 flex flex-col gap-1 rounded-lg border border-border/50 bg-muted/30 px-3 py-2 text-sm dark:bg-zinc-900/50"
    >
      <button
        class="flex w-full min-w-0 items-center gap-2 text-left text-muted-foreground hover:text-foreground"
        @click="knowledgeExpanded = !knowledgeExpanded"
      >
        <IconKnowledge class="size-4 shrink-0" />
        <span class="min-w-0 flex-1 truncate text-xs font-medium">
          {{ t('assistant.chat.retrievalKnowledge') }}
        </span>
        <span class="text-xs opacity-70">
          {{ t('assistant.chat.retrievalItems', { count: knowledgeItems.length }) }}
        </span>
        <ChevronDown
          :class="cn('size-4 transition-transform', knowledgeExpanded && 'rotate-180')"
        />
      </button>

      <div v-if="knowledgeExpanded" class="mt-1 space-y-2 text-xs">
        <div
          v-for="(item, idx) in knowledgeItems"
          :key="'kb-' + idx"
          class="rounded-md border border-border/50 bg-background/30 p-2"
        >
          <p class="whitespace-pre-wrap text-muted-foreground leading-relaxed">
            {{ item.content }}
          </p>
        </div>
      </div>
    </div>

    <!-- Memory retrieval -->
    <div
      v-if="memoryItems.length > 0"
      class="w-full min-w-0 flex flex-col gap-1 rounded-lg border border-border/50 bg-muted/30 px-3 py-2 text-sm dark:bg-zinc-900/50"
    >
      <button
        class="flex w-full min-w-0 items-center gap-2 text-left text-muted-foreground hover:text-foreground"
        @click="memoryExpanded = !memoryExpanded"
      >
        <IconMemory class="size-4 shrink-0" />
        <span class="min-w-0 flex-1 truncate text-xs font-medium">
          {{ t('assistant.chat.retrievalMemory') }}
        </span>
        <span class="text-xs opacity-70">
          {{ t('assistant.chat.retrievalItems', { count: memoryItems.length }) }}
        </span>
        <ChevronDown :class="cn('size-4 transition-transform', memoryExpanded && 'rotate-180')" />
      </button>

      <div v-if="memoryExpanded" class="mt-1 space-y-2 text-xs">
        <div
          v-for="(item, idx) in memoryItems"
          :key="'mem-' + idx"
          class="rounded-md border border-border/50 bg-background/30 p-2"
        >
          <p class="whitespace-pre-wrap text-muted-foreground leading-relaxed">
            {{ item.content }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
