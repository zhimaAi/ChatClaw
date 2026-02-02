<script setup lang="ts">
import { ToastProvider as RekaToastProvider } from 'reka-ui'

defineProps<{
  duration?: number
  swipeDirection?: 'up' | 'down' | 'left' | 'right'
}>()
</script>

<template>
  <RekaToastProvider :duration="duration" :swipe-direction="swipeDirection">
    <slot />
  </RekaToastProvider>
</template>
