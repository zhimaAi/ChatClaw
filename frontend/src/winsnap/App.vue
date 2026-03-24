<script setup lang="ts">
/**
 * Snap window entry point
 * Reuses AssistantPage with snap mode
 */
import { onUnmounted } from 'vue'
import AssistantPage from '@/pages/native/assistant/AssistantPage.vue'
import { Toaster } from '@/components/ui/toast'
import { useLocaleSync } from '@/composables/useLocale'
import { useThemeSync } from '@/composables/useThemeSync'

// Sync locale and theme when main window switches settings
const unsubLocale = useLocaleSync()
const unsubTheme = useThemeSync()

onUnmounted(() => {
  unsubLocale()
  unsubTheme()
})
</script>

<template>
  <div
    class="flex h-screen w-screen flex-col overflow-hidden border border-border bg-background text-foreground"
  >
    <AssistantPage tab-id="winsnap" mode="snap" />
    <Toaster />
  </div>
</template>
