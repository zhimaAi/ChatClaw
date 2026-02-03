<script setup lang="ts">
import { ToastRoot } from 'reka-ui'
import { computed } from 'vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    class?: string
    variant?: 'default' | 'success' | 'error'
    duration?: number
  }>(),
  {
    variant: 'default',
  }
)

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const variantClasses = computed(() => {
  switch (props.variant) {
    case 'success':
      return 'border-border bg-popover text-popover-foreground'
    case 'error':
      return 'border-border bg-popover text-popover-foreground'
    default:
      return 'border-border bg-popover text-popover-foreground'
  }
})
</script>

<template>
  <ToastRoot
    :class="
      cn(
        'group pointer-events-auto relative flex w-full items-center justify-between gap-4 overflow-hidden rounded-lg border p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10 transition-all',
        'data-[state=open]:animate-in data-[state=closed]:animate-out',
        'data-[swipe=end]:animate-out data-[state=closed]:fade-out-80',
        'data-[state=closed]:slide-out-to-right-full data-[state=open]:slide-in-from-top-full',
        variantClasses,
        $props.class
      )
    "
    :duration="props.duration"
    @update:open="emit('update:open', $event)"
  >
    <slot />
  </ToastRoot>
</template>
