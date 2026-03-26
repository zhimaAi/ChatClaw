<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { AgentsService, type Agent } from '@bindings/chatclaw/internal/services/agents'
import ChatMessageList from './ChatMessageList.vue'

const props = defineProps<{
  conversationId: number
  agentId?: number | null
  runId?: number | null
}>()

const agentName = ref<string>('')
const agentIcon = ref<string>('')

// Keep a stable virtual tab id for embedded history rendering.
// 为内嵌历史视图保留稳定的虚拟 tab id，避免消息列表依赖缺参。
const embeddedTabId = computed(() => `embedded-${props.runId ?? 'no-run'}-${props.conversationId}`)

async function loadEmbeddedAgent(agentId: number | null | undefined) {
  if (!agentId || agentId <= 0) {
    agentName.value = ''
    agentIcon.value = ''
    return
  }

  try {
    const agents = await AgentsService.ListAgents()
    const currentAgent = (agents || []).find((item: Agent) => item.id === agentId)
    agentName.value = currentAgent?.name || ''
    agentIcon.value = currentAgent?.icon || ''
  } catch (error) {
    console.warn('Failed to load embedded assistant agent:', error)
    agentName.value = ''
    agentIcon.value = ''
  }
}

watch(
  () => props.agentId,
  async (newAgentId) => {
    await loadEmbeddedAgent(newAgentId)
  },
  { immediate: true }
)
</script>

<template>
  <div class="flex h-full min-h-0 min-w-0 flex-col bg-background">
    <ChatMessageList
      :key="embeddedTabId"
      :conversation-id="props.conversationId"
      :tab-id="embeddedTabId"
      mode="embedded"
      :agent-name="agentName || undefined"
      :agent-icon="agentIcon || undefined"
      class="min-h-0 flex-1"
    />
  </div>
</template>
