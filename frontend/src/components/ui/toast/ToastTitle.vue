<script setup lang="ts">
import { ToastTitle as RekaToastTitle } from 'reka-ui'
import { cn } from '@/lib/utils'

defineProps<{
  class?: string
}>()
</script>

<template>
  <RekaToastTitle :class="cn('text-sm font-semibold', $props.class)">
    <slot />
  </RekaToastTitle>
</template>
