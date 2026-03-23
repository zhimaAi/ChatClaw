<script setup lang="ts">
import AssistantPage from '@/pages/assistant/AssistantPage.vue'
import { OpenClawAgentsService } from '@bindings/chatclaw/internal/services/openclawagents'
import type { AgentServiceApi } from '@/pages/assistant/composables/agentServiceProvider'

defineProps<{
  tabId: string
}>()

const openClawAgentService: AgentServiceApi = {
  ListAgents: OpenClawAgentsService.ListAgents as any,
  GetAgent: OpenClawAgentsService.GetAgent as any,
  CreateAgent: OpenClawAgentsService.CreateAgent as any,
  UpdateAgent: OpenClawAgentsService.UpdateAgent as any,
  DeleteAgent: OpenClawAgentsService.DeleteAgent,
  GetDefaultPrompt: OpenClawAgentsService.GetDefaultPrompt,
  ReadIconFile: OpenClawAgentsService.ReadIconFile,
  GetDefaultWorkDir: OpenClawAgentsService.GetDefaultWorkDir,
  GetWorkspaceDir: OpenClawAgentsService.GetWorkspaceDir,
  ListWorkspaceFiles: OpenClawAgentsService.ListWorkspaceFiles as any,
}
</script>

<template>
  <AssistantPage :tab-id="tabId" :agent-service="openClawAgentService" />
</template>
