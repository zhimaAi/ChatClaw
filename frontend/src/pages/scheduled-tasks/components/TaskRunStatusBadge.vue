<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
  label: string
  variant?: 'badge' | 'dot'
}>()

const statusClass = computed(() => {
  switch (props.status) {
    case 'success':
      return 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    case 'failed':
      return 'bg-red-500/10 text-red-700 dark:text-red-300'
    case 'running':
      return 'bg-sky-500/10 text-sky-700 dark:text-sky-300'
    case 'paused':
      return 'bg-slate-500/10 text-slate-700 dark:text-slate-300'
    default:
      return 'bg-muted text-muted-foreground'
  }
})

const dotClass = computed(() => {
  switch (props.status) {
    case 'running':
      return {
        dot: 'bg-sky-500',
        label: 'text-foreground',
      }
    case 'paused':
      return {
        dot: 'bg-slate-400',
        label: 'text-muted-foreground',
      }
    default:
      return {
        dot: 'bg-slate-400',
        label: 'text-muted-foreground',
      }
  }
})
</script>

<template>
  <span
    v-if="variant === 'dot'"
    class="inline-flex items-center gap-2 text-sm font-medium"
    :class="dotClass.label"
  >
    <span class="size-2 rounded-full" :class="dotClass.dot" />
    {{ label }}
  </span>
  <span v-else class="inline-flex rounded-full px-2 py-1 text-xs font-medium" :class="statusClass">
    {{ label }}
  </span>
</template>
