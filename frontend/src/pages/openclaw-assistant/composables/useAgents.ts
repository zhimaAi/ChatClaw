import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import {
  OpenClawAgentsService,
  type OpenClawAgent,
} from '@bindings/chatclaw/internal/services/openclawagents'

export function useAgents() {
  const { t } = useI18n()
  const agents = ref<OpenClawAgent[]>([])
  const activeAgentId = ref<number | null>(null)
  const loading = ref(false)

  const activeAgent = ref<OpenClawAgent | null>(null)

  const loadAgents = async () => {
    loading.value = true
    try {
      const list = await OpenClawAgentsService.ListAgents()
      agents.value = list

      const currentId = activeAgentId.value
      if (currentId != null && list.some((a) => a.id === currentId)) {
        // keep currentId
      } else {
        activeAgentId.value = list.length > 0 ? list[0].id : null
      }
    } catch (error: unknown) {
      toast.error(getErrorMessage(error) || t('assistant.errors.loadFailed'))
    } finally {
      loading.value = false
    }
  }

  const createAgent = async (data: {
    name: string
    icon: string
    defaultLLMProviderId?: string
    defaultLLMModelId?: string
    identityEmoji?: string
    identityTheme?: string
    groupChatMentionPatterns?: string
    toolsProfile?: string
    toolsAllow?: string
    toolsDeny?: string
    heartbeatEvery?: string
    paramsTemperature?: string
    paramsMaxTokens?: string
  }) => {
    loading.value = true
    try {
      const created = await OpenClawAgentsService.CreateAgent({
        name: data.name,
        icon: data.icon,
        default_llm_provider_id: data.defaultLLMProviderId ?? '',
        default_llm_model_id: data.defaultLLMModelId ?? '',
        identity_emoji: data.identityEmoji ?? '',
        identity_theme: data.identityTheme ?? '',
        group_chat_mention_patterns: data.groupChatMentionPatterns ?? '[]',
        tools_profile: data.toolsProfile ?? '',
        tools_allow: data.toolsAllow ?? '[]',
        tools_deny: data.toolsDeny ?? '[]',
        heartbeat_every: data.heartbeatEvery ?? '',
        params_temperature: data.paramsTemperature ?? '',
        params_max_tokens: data.paramsMaxTokens ?? '',
      })
      if (!created) {
        throw new Error(t('assistant.errors.createFailed'))
      }
      agents.value = [created, ...agents.value]
      activeAgentId.value = created.id
      toast.success(t('assistant.toasts.created'))
      return created
    } catch (error: unknown) {
      toast.error(getErrorMessage(error) || t('assistant.errors.createFailed'))
      throw error
    } finally {
      loading.value = false
    }
  }

  const updateAgent = (updated: OpenClawAgent) => {
    const idx = agents.value.findIndex((a) => a.id === updated.id)
    if (idx >= 0) agents.value[idx] = updated
  }

  const deleteAgent = (id: number) => {
    agents.value = agents.value.filter((a) => a.id !== id)
    if (activeAgentId.value === id) {
      activeAgentId.value = agents.value.length > 0 ? agents.value[0].id : null
    }
  }

  return {
    agents,
    activeAgentId,
    activeAgent,
    loading,
    loadAgents,
    createAgent,
    updateAgent,
    deleteAgent,
  }
}
