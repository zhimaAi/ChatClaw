<script setup lang="ts">
/**
 * Snap window entry point
 * Reuses AssistantPage with snap mode
 */
import { onUnmounted } from 'vue'
import AssistantPage from '@/pages/assistant/AssistantPage.vue'
import { Toaster } from '@/components/ui/toast'
import { useLocaleSync } from '@/composables/useLocale'

// Sync locale when main window switches language
const unsubLocale = useLocaleSync()
onUnmounted(() => unsubLocale())
</script>

<template>
  <div class="flex h-screen w-screen flex-col overflow-hidden bg-background text-foreground">
    <AssistantPage tab-id="winsnap" mode="snap" />
    <Toaster />
  </div>
</template>
