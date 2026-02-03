<script setup lang="ts">
import HelpTip from './HelpTip.vue'

defineProps<{
  label: string
  help: string
  required?: boolean
}>()
</script>

<template>
  <label class="flex items-center gap-2 text-sm font-medium text-foreground">
    <span v-if="required" class="text-destructive">*</span>
    <span>{{ label }}</span>
    <HelpTip :content="help" class="translate-y-[0.5px]" />
  </label>
</template>
