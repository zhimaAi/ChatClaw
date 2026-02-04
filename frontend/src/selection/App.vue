<script setup lang="ts">
import { onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Logo from '@/assets/images/logo.svg'
import { Events } from '@wailsio/runtime'

const { t } = useI18n()

// Emit button click event to backend
const handleButtonClick = () => {
  try {
    Events.Emit('text-selection:button-click')
  } catch (err) {
    console.error('[SelectionPopup] Button click error:', err)
  }
}

// Hide popup when mouse leaves (with delay)
let hideTimer: number | null = null

const handleMouseEnter = () => {
  if (hideTimer) {
    clearTimeout(hideTimer)
    hideTimer = null
  }
}

const handleMouseLeave = () => {
  // Delay hide to allow user to click
  hideTimer = window.setTimeout(() => {
    Events.Emit('text-selection:hide')
  }, 500)
}

onUnmounted(() => {
  if (hideTimer) {
    clearTimeout(hideTimer)
    hideTimer = null
  }
})
</script>

<template>
  <div
    class="flex h-screen w-screen cursor-pointer select-none items-center justify-center"
    @mouseenter="handleMouseEnter"
    @mouseleave="handleMouseLeave"
    @click="handleButtonClick"
  >
    <div
      class="flex items-center gap-2 rounded-full border border-border bg-background px-4 py-2 shadow-lg transition-all hover:shadow-xl"
    >
      <Logo class="size-6" />
      <span class="text-sm font-medium text-foreground">{{ t('selection.aiChat') }}</span>
    </div>
  </div>
</template>

<style>
/* Make background transparent */
html,
body,
#app {
  background: transparent !important;
  margin: 0;
  padding: 0;
  overflow: hidden;
}
</style>
