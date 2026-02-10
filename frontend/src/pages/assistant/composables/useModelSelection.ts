import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import {
  ProvidersService,
  type ProviderWithModels,
} from '@bindings/willclaw/internal/services/providers'
import {
  ConversationsService,
  type Conversation,
  UpdateConversationInput,
} from '@bindings/willclaw/internal/services/conversations'
import type { Agent } from '@bindings/willclaw/internal/services/agents'

export function useModelSelection() {
  const { t } = useI18n()

  const providersWithModels = ref<ProviderWithModels[]>([])
  const selectedModelKey = ref('')

  const hasModels = computed(() => {
    return providersWithModels.value.some((pw) =>
      pw.model_groups.some((g) => g.type === 'llm' && g.models.length > 0)
    )
  })

  const selectedModelInfo = computed(() => {
    if (!selectedModelKey.value) return null
    const [providerId, modelId] = selectedModelKey.value.split('::')
    if (!providerId || !modelId) return null
    for (const pw of providersWithModels.value) {
      if (pw.provider.provider_id !== providerId) continue
      for (const group of pw.model_groups) {
        if (group.type !== 'llm') continue
        const model = group.models.find((m) => m.model_id === modelId)
        if (model) {
          return {
            providerId,
            modelId,
            modelName: model.name,
          }
        }
      }
    }
    return null
  })

  const loadModels = async () => {
    try {
      const providers = await ProvidersService.ListProviders()
      const enabled = providers.filter((p) => p.enabled)
      // Load provider models in parallel; allow partial failures.
      const settled = await Promise.allSettled(
        enabled.map((p) => ProvidersService.GetProviderWithModels(p.provider_id))
      )
      const ok: ProviderWithModels[] = []
      let failedCount = 0
      for (const s of settled) {
        if (s.status === 'fulfilled') {
          if (s.value) ok.push(s.value)
        } else {
          failedCount += 1
          console.warn('Failed to load provider models:', s.reason)
        }
      }
      providersWithModels.value = ok

      // If some providers failed but we still have models, keep UI usable and show a gentle hint.
      if (failedCount > 0 && ok.length > 0) {
        toast.default(t('assistant.errors.loadModelsPartialFailed'))
      } else if (failedCount > 0 && ok.length === 0) {
        toast.error(t('assistant.errors.loadModelsFailed'))
      }
    } catch (error: unknown) {
      toast.error(getErrorMessage(error) || t('assistant.errors.loadModelsFailed'))
    }
  }

  const selectDefaultModel = (
    activeAgent: Agent | null,
    activeConversation: Conversation | null
  ) => {
    if (!activeAgent) {
      selectedModelKey.value = ''
      return
    }

    // Prefer the active conversation's model if available.
    {
      const conv = activeConversation
      if (conv?.llm_provider_id && conv?.llm_model_id) {
        const key = `${conv.llm_provider_id}::${conv.llm_model_id}`
        // Verify the model still exists
        for (const pw of providersWithModels.value) {
          if (pw.provider.provider_id !== conv.llm_provider_id) continue
          for (const group of pw.model_groups) {
            if (group.type !== 'llm') continue
            const found = group.models.find((m) => m.model_id === conv.llm_model_id)
            if (found) {
              selectedModelKey.value = key
              return
            }
          }
        }
      }
    }

    // Check if agent has a default model configured
    const agentProviderId = activeAgent.default_llm_provider_id
    const agentModelId = activeAgent.default_llm_model_id

    if (agentProviderId && agentModelId) {
      // Verify the model still exists
      for (const pw of providersWithModels.value) {
        if (pw.provider.provider_id !== agentProviderId) continue
        for (const group of pw.model_groups) {
          if (group.type !== 'llm') continue
          const found = group.models.find((m) => m.model_id === agentModelId)
          if (found) {
            selectedModelKey.value = `${agentProviderId}::${agentModelId}`
            return
          }
        }
      }
    }

    // Fall back to first available LLM model
    for (const pw of providersWithModels.value) {
      for (const group of pw.model_groups) {
        if (group.type !== 'llm' || group.models.length === 0) continue
        const firstModel = group.models[0]
        selectedModelKey.value = `${pw.provider.provider_id}::${firstModel.model_id}`
        return
      }
    }

    selectedModelKey.value = ''
  }

  const parseSelectedModelKey = (
    key: string
  ): { providerId: string; modelId: string } | null => {
    if (!key) return null
    const [providerId, modelId] = key.split('::')
    if (!providerId || !modelId) return null
    return { providerId, modelId }
  }

  const saveModelToConversationIfNeeded = async (
    activeConversationId: number | null,
    activeConversation: Conversation | null,
    opts?: { silent?: boolean }
  ) => {
    const silent = opts?.silent ?? true
    if (!activeConversationId) return

    const parsed = parseSelectedModelKey(selectedModelKey.value)
    if (!parsed) return

    // Avoid redundant updates when switching conversations (we already read from DB into selectedModelKey).
    const current = activeConversation
    if (
      current &&
      current.llm_provider_id === parsed.providerId &&
      current.llm_model_id === parsed.modelId
    ) {
      return
    }

    try {
      const updated = await ConversationsService.UpdateConversation(
        activeConversationId,
        new UpdateConversationInput({
          llm_provider_id: parsed.providerId,
          llm_model_id: parsed.modelId,
        })
      )
      return updated
    } catch (error: unknown) {
      // Non-critical: if this fails, backend will continue using the previously saved model.
      if (!silent) {
        toast.error(getErrorMessage(error) || t('assistant.errors.updateConversationFailed'))
      } else {
        console.warn('Failed to save model to conversation:', error)
      }
      return null
    }
  }

  return {
    providersWithModels,
    selectedModelKey,
    hasModels,
    selectedModelInfo,
    loadModels,
    selectDefaultModel,
    saveModelToConversationIfNeeded,
  }
}
