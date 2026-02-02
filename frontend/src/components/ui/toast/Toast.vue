<script setup lang="ts">
import { ToastRoot } from 'reka-ui'
import { computed } from 'vue'
import { cn } from '@/lib/utils'

const props = withDefaults(defineProps<{
  class?: string
  variant?: 'default' | 'success' | 'error'
  duration?: number
  open?: boolean
}>(), {
  variant: 'default',
  open: true,
})

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const variantClasses = computed(() => {
  switch (props.variant) {
    case 'success':
      return 'border-green-500 bg-green-50 text-green-800 dark:bg-green-950 dark:text-green-200 dark:border-green-800'
    case 'error':
      return 'border-destructive bg-red-50 text-red-800 dark:bg-red-950 dark:text-red-200 dark:border-red-800'
    default:
      return 'border-border bg-background text-foreground'
  }
})
</script>

<template>
  <ToastRoot
    :class="cn(
      'group pointer-events-auto relative flex w-full items-center justify-between gap-4 overflow-hidden rounded-lg border p-4 shadow-lg transition-all',
      'data-[state=open]:animate-in data-[state=closed]:animate-out',
      'data-[swipe=end]:animate-out data-[state=closed]:fade-out-80',
      'data-[state=closed]:slide-out-to-right-full data-[state=open]:slide-in-from-top-full',
      variantClasses,
      $props.class
    )"
    :duration="duration"
    :open="open"
    @update:open="emit('update:open', $event)"
  >
    <slot />
  </ToastRoot>
</template>
