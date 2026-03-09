<script setup lang="ts">
import type { ScheduledTaskSummary } from '../types'

defineProps<{
  summary: ScheduledTaskSummary | null
  labels: {
    total: string
    running: string
    paused: string
    failed: string
  }
}>()
</script>

<template>
  <div class="grid gap-3 md:grid-cols-4">
    <div class="rounded-xl border border-border bg-card p-4">
      <div class="text-xs text-muted-foreground">{{ labels.total }}</div>
      <div class="mt-2 text-2xl font-semibold text-foreground">{{ summary?.total ?? 0 }}</div>
    </div>
    <div class="rounded-xl border border-border bg-card p-4">
      <div class="text-xs text-muted-foreground">{{ labels.running }}</div>
      <div class="mt-2 text-2xl font-semibold text-sky-600">{{ summary?.running ?? 0 }}</div>
    </div>
    <div class="rounded-xl border border-border bg-card p-4">
      <div class="text-xs text-muted-foreground">{{ labels.paused }}</div>
      <div class="mt-2 text-2xl font-semibold text-amber-600">{{ summary?.paused ?? 0 }}</div>
    </div>
    <div class="rounded-xl border border-border bg-card p-4">
      <div class="text-xs text-muted-foreground">{{ labels.failed }}</div>
      <div class="mt-2 text-2xl font-semibold text-red-600">{{ summary?.failed ?? 0 }}</div>
    </div>
  </div>
</template>
