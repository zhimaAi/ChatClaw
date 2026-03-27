<script setup lang="ts">
import OpenClawPage from '../OpenClawPage.vue'

const props = defineProps<{
  conversationId: number
  agentId?: number | null
}>()
</script>

<template>
  <OpenClawPage
    :key="`embedded-${props.conversationId}`"
    :tab-id="`embedded-${props.conversationId}`"
    mode="embedded"
    :initial-conversation-id="props.conversationId"
    :initial-agent-id="props.agentId ?? null"
  />
</template>
