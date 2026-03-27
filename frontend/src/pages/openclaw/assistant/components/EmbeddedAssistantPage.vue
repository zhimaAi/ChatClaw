<script setup lang="ts">
import OpenClawPage from '../OpenClawPage.vue'

const props = defineProps<{
  conversationId: number
  agentId?: number | null
  readOnly?: boolean
}>()
</script>

<template>
  <OpenClawPage
    :key="`openclaw-embedded-${props.conversationId}`"
    :tab-id="`openclaw-embedded-${props.conversationId}`"
    mode="embedded"
    :readonly="props.readOnly ?? false"
    :initial-conversation-id="props.conversationId"
    :initial-agent-id="props.agentId ?? null"
  />
</template>
