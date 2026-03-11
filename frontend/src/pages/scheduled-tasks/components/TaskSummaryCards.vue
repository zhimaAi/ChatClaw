<script setup lang="ts">
import { computed } from 'vue'
import { CircleAlert, CircleCheck, Pause, Play } from 'lucide-vue-next'
import type { ScheduledTaskSummary } from '../types'

const props = defineProps<{
  summary: ScheduledTaskSummary | null
  labels: {
    total: string
    running: string
    paused: string
    failed: string
  }
}>()

const cards = computed(() => [
  {
    key: 'total',
    label: props.labels.total,
    value: props.summary?.total ?? 0,
    icon: CircleCheck,
  },
  {
    key: 'running',
    label: props.labels.running,
    value: props.summary?.running ?? 0,
    icon: Play,
  },
  {
    key: 'paused',
    label: props.labels.paused,
    value: props.summary?.paused ?? 0,
    icon: Pause,
  },
  {
    key: 'failed',
    label: props.labels.failed,
    value: props.summary?.failed ?? 0,
    icon: CircleAlert,
  },
])
</script>

<template>
  <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
    <div
      v-for="card in cards"
      :key="card.key"
      class="flex items-center gap-4 rounded-2xl border border-[#d9d9d9] bg-white px-6 py-5 shadow-[0px_1px_3px_rgba(0,0,0,0.10),0px_1px_2px_rgba(0,0,0,0.06)]"
    >
      <div class="flex size-12 shrink-0 items-center justify-center rounded-full bg-[#f5f5f5] text-[#171717]">
        <component :is="card.icon" class="size-5" />
      </div>
      <div class="min-w-0">
        <div class="text-[32px] font-semibold leading-none tracking-[-0.04em] text-[#171717]">
          {{ card.value }}
        </div>
        <div class="mt-1 text-sm text-[#737373]">
          {{ card.label }}
        </div>
      </div>
    </div>
  </div>
</template>
