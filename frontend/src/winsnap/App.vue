<script setup lang="ts">
/**
 * Snap window entry point
 * Dynamically renders AssistantPage (chatclaw) or OpenClawPage (openclaw)
 * based on the current system mode, kept in sync with the main window.
 *
 * This WebView has its own Pinia instance — OpenClaw gateway status must be
 * polled + subscribed here (same as SideNav in the main window), not only in the main UI.
 */
import { onUnmounted, watch } from 'vue'
import AssistantPage from '@/pages/assistant/AssistantPage.vue'
import OpenClawPage from '@/pages/openclaw/assistant/OpenClawPage.vue'
import { Toaster } from '@/components/ui/toast'
import { useLocaleSync } from '@/composables/useLocale'
import { useThemeSync } from '@/composables/useThemeSync'
import { useSystemSync } from '@/composables/useSystemSync'
import { useAppStore, useOpenClawGatewayStore } from '@/stores'

// Sync locale, theme, and system mode when main window switches settings
const unsubLocale = useLocaleSync()
const unsubTheme = useThemeSync()
const unsubSystem = useSystemSync()

const appStore = useAppStore()
const gatewayStore = useOpenClawGatewayStore()

watch(
  () => appStore.currentSystem,
  (sys) => {
    if (sys === 'openclaw') {
      void gatewayStore.poll()
      gatewayStore.startHeartbeat()
    } else {
      gatewayStore.stopHeartbeat()
    }
  },
  { immediate: true }
)

onUnmounted(() => {
  unsubLocale()
  unsubTheme()
  unsubSystem()
  gatewayStore.stopHeartbeat()
})
</script>

<template>
  <div
    class="flex h-screen w-screen flex-col overflow-hidden border border-border bg-background text-foreground"
  >
    <AssistantPage v-if="appStore.currentSystem !== 'openclaw'" tab-id="winsnap" mode="snap" />
    <OpenClawPage v-else tab-id="winsnap" mode="snap" />
    <Toaster />
  </div>
</template>
