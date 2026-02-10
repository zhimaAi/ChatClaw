import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { useChatStore } from '@/stores'
import {
  ConversationsService,
  type Conversation,
  CreateConversationInput,
  UpdateConversationInput,
} from '@bindings/willclaw/internal/services/conversations'
import { Events } from '@wailsio/runtime'

export function useConversations(tabId: string) {
  const { t } = useI18n()
  const chatStore = useChatStore()

  // Conversations state (cached by agent)
  const conversationsByAgent = ref<Record<number, Conversation[]>>({})
  const conversationsLoadedByAgent = ref<Record<number, boolean>>({})
  const conversationsLoadingByAgent = ref<Record<number, boolean>>({})
  const conversationsStaleByAgent = ref<Record<number, boolean>>({})
  const activeConversationId = ref<number | null>(null)

  const activeConversation = computed<Conversation | null>(() => {
    if (!activeConversationId.value) return null
    // Need to find conversation across all agents
    for (const agentId in conversationsByAgent.value) {
      const list = conversationsByAgent.value[agentId] ?? []
      const found = list.find((c) => c.id === activeConversationId.value)
      if (found) return found
    }
    return null
  })

  const setAgentLoading = (agentId: number, val: boolean) => {
    conversationsLoadingByAgent.value = {
      ...conversationsLoadingByAgent.value,
      [agentId]: val,
    }
  }

  type LoadConversationsOptions = {
    preserveSelection?: boolean
    affectActiveSelection?: boolean
    force?: boolean
    activeAgentId?: number | null
  }

  const loadConversations = async (agentId: number, opts: LoadConversationsOptions = {}) => {
    const preserveSelection = opts.preserveSelection ?? false
    const affectActiveSelection = opts.affectActiveSelection ?? true
    const force = opts.force ?? false
    const activeAgentId = opts.activeAgentId ?? null

    if (!force && conversationsLoadingByAgent.value[agentId]) return

    setAgentLoading(agentId, true)
    const previousConversationId = activeConversationId.value
    try {
      const list = await ConversationsService.ListConversations(agentId)
      const next = list || []
      conversationsByAgent.value = {
        ...conversationsByAgent.value,
        [agentId]: next,
      }
      conversationsLoadedByAgent.value = {
        ...conversationsLoadedByAgent.value,
        [agentId]: true,
      }
      conversationsStaleByAgent.value = {
        ...conversationsStaleByAgent.value,
        [agentId]: false,
      }

      // Only adjust active selection when loading the active agent's list
      if (affectActiveSelection && activeAgentId === agentId) {
        if (preserveSelection && previousConversationId !== null) {
          // 保持当前选中状态（如果会话仍存在）
          const stillExists = next.some((c) => c.id === previousConversationId)
          if (!stillExists) {
            if (previousConversationId) {
              chatStore.clearMessages(previousConversationId)
            }
            activeConversationId.value = null
          }
        } else {
          // Don't auto-select any conversation when loading
          if (previousConversationId) {
            chatStore.clearMessages(previousConversationId)
          }
          activeConversationId.value = null
        }
      }
    } catch (error: unknown) {
      toast.error(getErrorMessage(error) || t('assistant.errors.loadConversationsFailed'))
      conversationsByAgent.value = {
        ...conversationsByAgent.value,
        [agentId]: [],
      }
      conversationsLoadedByAgent.value = {
        ...conversationsLoadedByAgent.value,
        [agentId]: true,
      }
      conversationsStaleByAgent.value = {
        ...conversationsStaleByAgent.value,
        [agentId]: false,
      }
    } finally {
      setAgentLoading(agentId, false)
    }
  }

  const ensureConversationsLoaded = async (agentId: number) => {
    const loaded = conversationsLoadedByAgent.value[agentId]
    const stale = conversationsStaleByAgent.value[agentId]
    if (loaded && !stale) return
    await loadConversations(agentId, { affectActiveSelection: false, force: !!stale })
  }

  const markConversationsStale = (agentId: number) => {
    conversationsStaleByAgent.value = {
      ...conversationsStaleByAgent.value,
      [agentId]: true,
    }
  }

  const getAgentConversations = (agentId: number): Conversation[] => {
    return (conversationsByAgent.value[agentId] ?? []).slice(0, 3)
  }

  const getAllAgentConversations = (agentId: number): Conversation[] => {
    return conversationsByAgent.value[agentId] ?? []
  }

  const createConversation = async (input: CreateConversationInput): Promise<Conversation | null> => {
    try {
      const newConversation = await ConversationsService.CreateConversation(input)
      if (newConversation) {
        const agentId = newConversation.agent_id
        const current = conversationsByAgent.value[agentId] ?? []
        // 添加新会话并排序（置顶优先）
        const next = [newConversation, ...current].sort((a, b) => {
          if (a.is_pinned !== b.is_pinned) return a.is_pinned ? -1 : 1
          return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
        })
        conversationsByAgent.value = {
          ...conversationsByAgent.value,
          [agentId]: next,
        }
        conversationsLoadedByAgent.value = {
          ...conversationsLoadedByAgent.value,
          [agentId]: true,
        }
        conversationsStaleByAgent.value = {
          ...conversationsStaleByAgent.value,
          [agentId]: false,
        }
        activeConversationId.value = newConversation.id

        // Notify other assistant tabs to refresh.
        Events.Emit('conversations:changed', {
          agent_id: agentId,
          sourceTabId: tabId,
          action: 'created',
        })

        return newConversation
      }
      return null
    } catch (error: unknown) {
      toast.error(getErrorMessage(error) || t('assistant.errors.createConversationFailed'))
      throw error
    }
  }

  const updateConversation = (updated: Conversation) => {
    const agentId = updated.agent_id
    const current = conversationsByAgent.value[agentId] ?? []
    const exists = current.some((c) => c.id === updated.id)
    // Update and re-sort (pinned first, then by updated_at desc)
    const next = (
      exists ? current.map((c) => (c.id === updated.id ? updated : c)) : [updated, ...current]
    ).sort((a, b) => {
      // Pinned items first
      if (a.is_pinned !== b.is_pinned) {
        return a.is_pinned ? -1 : 1
      }
      // Then by updated_at desc
      return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
    })
    conversationsByAgent.value = {
      ...conversationsByAgent.value,
      [agentId]: next,
    }
    conversationsLoadedByAgent.value = {
      ...conversationsLoadedByAgent.value,
      [agentId]: true,
    }
    conversationsStaleByAgent.value = {
      ...conversationsStaleByAgent.value,
      [agentId]: false,
    }

    // Notify other assistant tabs to refresh.
    Events.Emit('conversations:changed', {
      agent_id: agentId,
      sourceTabId: tabId,
      action: 'updated',
    })
  }

  const deleteConversation = async (conversation: Conversation) => {
    try {
      await ConversationsService.DeleteConversation(conversation.id)
      const agentId = conversation.agent_id
      const current = conversationsByAgent.value[agentId] ?? []
      conversationsByAgent.value = {
        ...conversationsByAgent.value,
        [agentId]: current.filter((c) => c.id !== conversation.id),
      }
      conversationsLoadedByAgent.value = {
        ...conversationsLoadedByAgent.value,
        [agentId]: true,
      }
      conversationsStaleByAgent.value = {
        ...conversationsStaleByAgent.value,
        [agentId]: false,
      }
      if (activeConversationId.value === conversation.id) {
        chatStore.clearMessages(activeConversationId.value)
        activeConversationId.value = null
      }
      toast.success(t('assistant.conversation.delete.success'))

      // Notify other assistant tabs to refresh.
      Events.Emit('conversations:changed', {
        agent_id: agentId,
        sourceTabId: tabId,
        action: 'deleted',
      })
    } catch (error) {
      console.error('Failed to delete conversation:', error)
      toast.error(getErrorMessage(error) || t('assistant.errors.deleteConversationFailed'))
      throw error
    }
  }

  const togglePin = async (conv: Conversation, activeAgentId: number | null) => {
    const isPinning = !conv.is_pinned
    try {
      await ConversationsService.UpdateConversation(
        conv.id,
        new UpdateConversationInput({
          is_pinned: isPinning,
        })
      )
      // 重新加载列表以获取正确的排序和置顶状态
      // （置顶时其他会话可能被取消置顶）
      if (activeAgentId) {
        await loadConversations(activeAgentId, { preserveSelection: true, activeAgentId })
      }

      // Notify other assistant tabs to refresh.
      Events.Emit('conversations:changed', {
        agent_id: conv.agent_id,
        sourceTabId: tabId,
        action: 'pin',
      })
    } catch (error) {
      console.error('Failed to toggle pin:', error)
      toast.error(getErrorMessage(error) || t('assistant.errors.updateConversationFailed'))
      throw error
    }
  }

  const clearAgentCache = (agentId: number) => {
    const next1 = { ...conversationsByAgent.value }
    delete next1[agentId]
    conversationsByAgent.value = next1

    const next2 = { ...conversationsLoadedByAgent.value }
    delete next2[agentId]
    conversationsLoadedByAgent.value = next2

    const next3 = { ...conversationsLoadingByAgent.value }
    delete next3[agentId]
    conversationsLoadingByAgent.value = next3
  }

  return {
    conversationsByAgent,
    conversationsLoadedByAgent,
    conversationsLoadingByAgent,
    conversationsStaleByAgent,
    activeConversationId,
    activeConversation,
    loadConversations,
    ensureConversationsLoaded,
    markConversationsStale,
    getAgentConversations,
    getAllAgentConversations,
    createConversation,
    updateConversation,
    deleteConversation,
    togglePin,
    clearAgentCache,
  }
}
