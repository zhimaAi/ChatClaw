<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
  label: string
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
</script>

<template>
  <span class="inline-flex rounded-full px-2 py-1 text-xs font-medium" :class="statusClass">
    {{ label }}
  </span>
</template>
