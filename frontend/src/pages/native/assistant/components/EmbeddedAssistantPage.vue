<script setup lang="ts">
import AssistantPage from '../AssistantPage.vue'

const props = defineProps<{
  conversationId: number
  agentId?: number | null
}>()
</script>

<template>
  <AssistantPage
    :key="`embedded-${props.conversationId}`"
    :tab-id="`embedded-${props.conversationId}`"
    mode="embedded"
    :initial-conversation-id="props.conversationId"
    :initial-agent-id="props.agentId ?? null"
  />
</template>
