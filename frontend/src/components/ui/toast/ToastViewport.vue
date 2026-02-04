<script setup lang="ts">
import { ToastViewport as RekaToastViewport } from 'reka-ui'
import { cn } from '@/lib/utils'

defineProps<{
  class?: string
}>()
</script>

<template>
  <RekaToastViewport
    :class="
      cn(
        'fixed top-10 right-0 z-[100] flex max-h-screen w-full flex-col-reverse gap-2 p-4 sm:flex-col sm:max-w-[420px]',
        $props.class
      )
    "
  />
</template>
