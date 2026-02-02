<script setup lang="ts">
import AlertIcon from '@/assets/icons/alert.svg'
import { cn } from '@/lib/utils'

const props = defineProps<{
  text: string
  className?: string
}>()
</script>

<template>
  <div :class="cn('rounded-md border border-orange-500/30 bg-orange-500/5 p-3', props.className)">
    <div
      class="flex items-start gap-2 self-stretch justify-start text-orange-500 text-sm leading-5"
    >
      <AlertIcon class="mt-0.5 size-4 shrink-0" />
      <div>{{ text }}</div>
    </div>
  </div>
</template>
