<script setup lang="ts">
import OpenClawRuntimeSettings from '@/pages/settings/components/OpenClawRuntimeSettings.vue'
</script>

<template>
  <div class="flex h-full w-full bg-background text-foreground">
    <main class="flex flex-1 flex-col overflow-auto py-6 sm:py-8">
      <!-- Content ~80-86% of viewport width, cards centered within -->
      <div class="mx-auto w-full max-w-[88%] px-6 sm:px-8">
        <OpenClawRuntimeSettings :figma-layout="true" />
      </div>
    </main>
  </div>
</template>
