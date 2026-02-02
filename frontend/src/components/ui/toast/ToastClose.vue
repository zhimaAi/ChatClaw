<script setup lang="ts">
import { ToastClose as RekaToastClose } from 'reka-ui'
import { X } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

defineProps<{
  class?: string
}>()
</script>

<template>
  <RekaToastClose
    :class="
      cn(
        'absolute right-2 top-2 rounded-md p-1 opacity-0 transition-opacity',
        'hover:bg-secondary focus:opacity-100 focus:outline-none focus:ring-2',
        'group-hover:opacity-100',
        $props.class
      )
    "
  >
    <X class="size-4" />
  </RekaToastClose>
</template>
