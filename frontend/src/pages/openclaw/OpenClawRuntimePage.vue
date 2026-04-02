<script setup lang="ts">
import OpenClawRuntimeSettings from '@/pages/settings/components/OpenClawRuntimeSettings.vue'
</script>

<template>
  <div class="flex h-full w-full bg-background text-foreground">
    <main class="flex flex-1 flex-col overflow-auto py-8">
      <div class="mx-auto w-full max-w-3xl px-8">
        <OpenClawRuntimeSettings />
      </div>
    </main>
  </div>
</template>
