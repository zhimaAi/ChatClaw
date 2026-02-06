import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { AgentsService, type Agent } from '@bindings/willchat/internal/services/agents'

export function useAgents() {
  const { t } = useI18n()
  const agents = ref<Agent[]>([])
  const activeAgentId = ref<number | null>(null)
  const loading = ref(false)

  const activeAgent = ref<Agent | null>(null)

  const loadAgents = async () => {
    loading.value = true
    try {
      const list = await AgentsService.ListAgents()
      agents.value = list

      // Preserve current selection if possible; otherwise fall back to first agent (or null)
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

  const createAgent = async (data: { name: string; prompt: string; icon: string }) => {
    loading.value = true
    try {
      const created = await AgentsService.CreateAgent({
        name: data.name,
        prompt: data.prompt,
        icon: data.icon,
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

  const updateAgent = (updated: Agent) => {
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
