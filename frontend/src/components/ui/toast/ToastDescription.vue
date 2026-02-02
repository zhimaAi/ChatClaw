<script setup lang="ts">
import { ToastDescription as RekaToastDescription } from 'reka-ui'
import { cn } from '@/lib/utils'

defineProps<{
  class?: string
}>()
</script>

<template>
  <RekaToastDescription :class="cn('text-sm opacity-90', $props.class)">
    <slot />
  </RekaToastDescription>
</template>
