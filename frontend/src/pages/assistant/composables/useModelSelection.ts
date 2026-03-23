import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import {
  ProvidersService,
  type ProviderWithModels,
  type Model,
} from '@bindings/chatclaw/internal/services/providers'
import {
  ConversationsService,
  type Conversation,
  UpdateConversationInput,
} from '@bindings/chatclaw/internal/services/conversations'
import type { Agent } from '@bindings/chatclaw/internal/services/agents'
import { getBinding as getChatwikiBinding } from '@/lib/chatwikiCache'
import {
  formatModelDisplayLabel,
  getChatwikiAvailabilityStatus,
  getFirstSelectableModelKey,
  isSelectionAvailable,
} from '@/lib/chatwikiModelAvailability'

export function useModelSelection() {
  const { t } = useI18n()

  const getDisplayModelName = (providerId: string, model: Model) =>
    formatModelDisplayLabel(
      providerId,
      model.name?.trim() || model.model_id?.trim() || '-',
      chatwikiAvailability.value
    )

  const providersWithModels = ref<ProviderWithModels[]>([])
  const selectedModelKey = ref('')
  const chatwikiAvailability = ref<'available' | 'unbound' | 'non_cloud'>('available')

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
            modelName: getDisplayModelName(providerId, model),
            capabilities: model.capabilities,
          }
        }
      }
    }
    return null
  })

  const loadModels = async () => {
    try {
      console.info('[assistant][models] loadModels:start')
      const [providers, binding] = await Promise.all([
        ProvidersService.ListProviders(),
        getChatwikiBinding().catch(() => null),
      ])
      chatwikiAvailability.value = getChatwikiAvailabilityStatus(binding)
      console.info('[assistant][models] providers:list', {
        count: providers.length,
        providerIds: providers.map((p) => p.provider_id),
      })
      const enabled = providers.filter((p) => p.enabled)
      console.info('[assistant][models] providers:enabled', {
        count: enabled.length,
        providerIds: enabled.map((p) => p.provider_id),
      })
      // Load provider models in parallel; allow partial failures.
      const settled = await Promise.allSettled(
        enabled.map((p) => ProvidersService.GetProviderWithModels(p.provider_id))
      )
      const ok: ProviderWithModels[] = []
      let failedCount = 0
      settled.forEach((s, index) => {
        const providerId = enabled[index]?.provider_id || '(unknown)'
        if (s.status === 'fulfilled') {
          if (s.value) {
            const llmCount = s.value.model_groups
              .filter((g) => g.type === 'llm')
              .reduce((sum, g) => sum + g.models.length, 0)
            console.info('[assistant][models] provider:loaded', {
              providerId,
              groupCount: s.value.model_groups.length,
              llmCount,
            })
            ok.push(s.value)
          } else {
            console.warn('[assistant][models] provider:fulfilled-empty', { providerId })
          }
        } else {
          failedCount += 1
          console.warn('[assistant][models] provider:failed', {
            providerId,
            reason: s.reason,
          })
        }
      })
      // Sort free providers to the end so user-configured (stronger) models come first.
      ok.sort((a, b) => {
        const aFree = Boolean((a.provider as { is_free?: boolean }).is_free)
        const bFree = Boolean((b.provider as { is_free?: boolean }).is_free)
        if (aFree === bFree) return 0
        return aFree ? 1 : -1
      })
      providersWithModels.value = ok
      console.info('[assistant][models] loadModels:done', {
        successCount: ok.length,
        failedCount,
        providerIds: ok.map((item) => item.provider.provider_id),
      })

      // If some providers failed but we still have models, keep UI usable and show a gentle hint.
      if (failedCount > 0 && ok.length > 0) {
        toast.default(t('assistant.errors.loadModelsPartialFailed'))
      } else if (failedCount > 0 && ok.length === 0) {
        toast.error(t('assistant.errors.loadModelsFailed'))
      }
    } catch (error: unknown) {
      console.error('[assistant][models] loadModels:error', error)
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
        if (
          isSelectionAvailable(providersWithModels.value, key, 'llm', chatwikiAvailability.value)
        ) {
          selectedModelKey.value = key
          return
        }
      }
    }

    // Check if agent has a default model configured
    const agentProviderId = activeAgent.default_llm_provider_id
    const agentModelId = activeAgent.default_llm_model_id

    if (agentProviderId && agentModelId) {
      const key = `${agentProviderId}::${agentModelId}`
      if (isSelectionAvailable(providersWithModels.value, key, 'llm', chatwikiAvailability.value)) {
        selectedModelKey.value = key
        return
      }
    }

    // Fall back to first available LLM model
    selectedModelKey.value = getFirstSelectableModelKey(
      providersWithModels.value,
      'llm',
      chatwikiAvailability.value
    )
  }

  const parseSelectedModelKey = (key: string): { providerId: string; modelId: string } | null => {
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
