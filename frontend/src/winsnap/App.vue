<script setup lang="ts">
/**
 * Snap window entry point
 * Reuses AssistantPage with snap mode
 */
import AssistantPage from '@/pages/assistant/AssistantPage.vue'
import { Toaster } from '@/components/ui/toast'
</script>

<template>
  <div class="flex h-screen w-screen flex-col overflow-hidden bg-background text-foreground">
    <AssistantPage tab-id="winsnap" mode="snap" />
    <Toaster />
  </div>
</template>
