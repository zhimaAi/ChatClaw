<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowUp, Check, MoreHorizontal, Pin, PinOff, Square } from 'lucide-vue-next'
import IconAgentAdd from '@/assets/icons/agent-add.svg'
import IconNewConversation from '@/assets/icons/new-conversation.svg'
import IconSidebarCollapse from '@/assets/icons/sidebar-collapse.svg'
import IconSidebarExpand from '@/assets/icons/sidebar-expand.svg'
import IconSettings from '@/assets/icons/settings.svg'
import IconSelectKnowledge from '@/assets/icons/select-knowledge.svg'
import IconSelectImage from '@/assets/icons/select-image.svg'
import IconRename from '@/assets/icons/library-rename.svg'
import IconDelete from '@/assets/icons/library-delete.svg'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import LogoIcon from '@/assets/images/logo.svg'
import { getLogoDataUrl } from '@/composables/useLogo'
import CreateAgentDialog from './components/CreateAgentDialog.vue'
import AgentSettingsDialog from './components/AgentSettingsDialog.vue'
import RenameConversationDialog from './components/RenameConversationDialog.vue'
import ChatMessageList from './components/ChatMessageList.vue'
import { useNavigationStore, useChatStore } from '@/stores'
import { AgentsService, type Agent } from '@bindings/willchat/internal/services/agents'
import { Events } from '@wailsio/runtime'
import {
  ConversationsService,
  type Conversation,
  CreateConversationInput,
  UpdateConversationInput,
} from '@bindings/willchat/internal/services/conversations'
import {
  ProvidersService,
  type ProviderWithModels,
} from '@bindings/willchat/internal/services/providers'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import { LibraryService, type Library } from '@bindings/willchat/internal/services/library'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
} from '@/components/ui/select'
import {
  SelectRoot,
  SelectTrigger as SelectTriggerRaw,
  SelectPortal,
  SelectContent as SelectContentRaw,
  SelectViewport,
  SelectItem as SelectItemRaw,
  SelectItemIndicator,
  SelectItemText,
  SelectSeparator,
} from 'reka-ui'
import { ProviderIcon } from '@/components/ui/provider-icon'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

/**
 * Props - 每个标签页实例都有自己独立的 tabId
 * 通过 v-show 控制显示/隐藏，组件实例不会被销毁，状态自然保留
 */
const props = defineProps<{
  tabId: string
}>()

type ListMode = 'personal' | 'team'

const { t } = useI18n()
const navigationStore = useNavigationStore()
const chatStore = useChatStore()

const mode = ref<ListMode>('personal')

const agents = ref<Agent[]>([])
const activeAgentId = ref<number | null>(null)

const createOpen = ref(false)
const settingsOpen = ref(false)
const settingsAgent = ref<Agent | null>(null)
const loading = ref(false)

// Sidebar collapse state
const sidebarCollapsed = ref(false)

// Chat input
const chatInput = ref('')

// Model selection
const providersWithModels = ref<ProviderWithModels[]>([])
const selectedModelKey = ref('')

// Knowledge base selection
const libraries = ref<Library[]>([])
const selectedLibraryIds = ref<number[]>([])

// Helper function to clear knowledge base selection
const clearKnowledgeSelection = () => {
  selectedLibraryIds.value = []
}

// Conversations state (cached by agent)
const conversationsByAgent = ref<Record<number, Conversation[]>>({})
const conversationsLoadedByAgent = ref<Record<number, boolean>>({})
const conversationsLoadingByAgent = ref<Record<number, boolean>>({})
const conversationsStaleByAgent = ref<Record<number, boolean>>({})
const activeConversationId = ref<number | null>(null)

// Conversation dialogs
const renameConversationOpen = ref(false)
const deleteConversationOpen = ref(false)
const actionConversation = ref<Conversation | null>(null)

const activeAgent = computed(() => {
  if (activeAgentId.value == null) return null
  return agents.value.find((a) => a.id === activeAgentId.value) ?? null
})

// Check if currently generating
const isGenerating = computed(() => {
  if (!activeConversationId.value) return false
  return chatStore.isGenerating(activeConversationId.value).value
})

// Get messages for current conversation
const chatMessages = computed(() => {
  if (!activeConversationId.value) return []
  return chatStore.getMessages(activeConversationId.value).value
})

const canSend = computed(() => {
  return chatInput.value.trim() !== '' && selectedModelKey.value !== '' && !isGenerating.value
})

const hasModels = computed(() => {
  return providersWithModels.value.some((pw) =>
    pw.model_groups.some((g) => g.type === 'llm' && g.models.length > 0)
  )
})

/**
 * Get recent conversations for display under agent (max 3)
 */
const getAgentConversations = (agentId: number): Conversation[] => {
  return (conversationsByAgent.value[agentId] ?? []).slice(0, 3)
}

/**
 * Get all conversations for a specific agent (for dropdown menu)
 */
const getAllAgentConversations = (agentId: number): Conversation[] => {
  return conversationsByAgent.value[agentId] ?? []
}

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

    // Update tab icon and title
    updateCurrentTab()
  } catch (error: unknown) {
    toast.error(getErrorMessage(error) || t('assistant.errors.loadFailed'))
  } finally {
    loading.value = false
  }
}

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
}

const loadConversations = async (agentId: number, opts: LoadConversationsOptions = {}) => {
  const preserveSelection = opts.preserveSelection ?? false
  const affectActiveSelection = opts.affectActiveSelection ?? true
  const force = opts.force ?? false

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
    if (affectActiveSelection && activeAgentId.value === agentId) {
      if (preserveSelection && previousConversationId !== null) {
        // 保持当前选中状态（如果会话仍存在）
        const stillExists = next.some((c) => c.id === previousConversationId)
        if (!stillExists) {
          if (previousConversationId) {
            chatStore.clearMessages(previousConversationId)
          }
          activeConversationId.value = null
          // Clear knowledge base selection when conversation no longer exists
          clearKnowledgeSelection()
        }
      } else {
        // Don't auto-select any conversation when loading
        if (previousConversationId) {
          chatStore.clearMessages(previousConversationId)
        }
        activeConversationId.value = null
        // Clear knowledge base selection for new conversation state
        clearKnowledgeSelection()
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

const loadModels = async () => {
  try {
    const providers = await ProvidersService.ListProviders()
    const enabled = providers.filter((p) => p.enabled)
    // Load all provider models in parallel
    const results = await Promise.all(
      enabled.map((p) => ProvidersService.GetProviderWithModels(p.provider_id))
    )
    providersWithModels.value = results.filter(Boolean) as ProviderWithModels[]
  } catch (error: unknown) {
    toast.error(getErrorMessage(error) || t('assistant.errors.loadModelsFailed'))
  }
}

// Load knowledge base list
const loadLibraries = async () => {
  try {
    const list = await LibraryService.ListLibraries()
    libraries.value = list || []
  } catch (error: unknown) {
    console.error('Failed to load libraries:', error)
  }
}

// Select default model based on agent settings or first available
const selectDefaultModel = () => {
  if (!activeAgent.value) {
    selectedModelKey.value = ''
    return
  }

  // Check if agent has a default model configured
  const agentProviderId = activeAgent.value.default_llm_provider_id
  const agentModelId = activeAgent.value.default_llm_model_id

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

// Get selected model info for display
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

const handleCreate = async (data: { name: string; prompt: string; icon: string }) => {
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
    createOpen.value = false
    agents.value = [created, ...agents.value]
    activeAgentId.value = created.id
    toast.success(t('assistant.toasts.created'))
  } catch (error: unknown) {
    toast.error(getErrorMessage(error) || t('assistant.errors.createFailed'))
  } finally {
    loading.value = false
  }
}

const openSettings = (agent: Agent) => {
  settingsAgent.value = agent
  settingsOpen.value = true
}

const handleUpdated = (updated: Agent) => {
  const idx = agents.value.findIndex((a) => a.id === updated.id)
  // Get old agent info to check if default model changed
  const oldAgent = idx >= 0 ? agents.value[idx] : null
  const hadDefaultModel = oldAgent?.default_llm_provider_id && oldAgent?.default_llm_model_id
  const hasDefaultModel = updated.default_llm_provider_id && updated.default_llm_model_id

  if (idx >= 0) agents.value[idx] = updated

  // If updating the currently selected agent
  if (activeAgentId.value === updated.id) {
    updateCurrentTab()
    // Sync model selection if default model changed
    if (hasDefaultModel || hadDefaultModel) {
      selectDefaultModel()
    }
  }
}

const handleDeleted = (id: number) => {
  agents.value = agents.value.filter((a) => a.id !== id)
  if (activeAgentId.value === id) {
    activeAgentId.value = agents.value.length > 0 ? agents.value[0].id : null
  }

  // Clear cached conversations for this agent
  {
    const next = { ...conversationsByAgent.value }
    delete next[id]
    conversationsByAgent.value = next
  }
  {
    const next = { ...conversationsLoadedByAgent.value }
    delete next[id]
    conversationsLoadedByAgent.value = next
  }
  {
    const next = { ...conversationsLoadingByAgent.value }
    delete next[id]
    conversationsLoadingByAgent.value = next
  }
}

const handleNewConversation = () => {
  // Just clear the current conversation selection and chat messages
  // Don't create a conversation record until user sends first message
  if (activeConversationId.value) {
    chatStore.clearMessages(activeConversationId.value)
  }
  activeConversationId.value = null
  chatInput.value = ''
  // Clear knowledge base selection for new conversation
  clearKnowledgeSelection()
}

const handleNewConversationForAgent = (agentId: number) => {
  if (activeAgentId.value !== agentId) {
    activeAgentId.value = agentId
  }
  handleNewConversation()
}

const handleSelectConversationForAgent = (agentId: number, conversation: Conversation) => {
  if (activeAgentId.value !== agentId) {
    activeAgentId.value = agentId
  }
  handleSelectConversation(conversation)
}

const handleSend = async () => {
  if (!canSend.value || !activeAgentId.value) return

  const messageContent = chatInput.value.trim()
  chatInput.value = ''

  // If no active conversation, create one first
  if (!activeConversationId.value) {
    try {
      // Get current model selection
      const [providerId, modelId] = selectedModelKey.value.split('::')

      const newConversation = await ConversationsService.CreateConversation(
        new CreateConversationInput({
          agent_id: activeAgentId.value,
          name: messageContent.slice(0, 50), // First message becomes conversation name (truncated)
          last_message: messageContent,
          llm_provider_id: providerId || '',
          llm_model_id: modelId || '',
          library_ids: selectedLibraryIds.value,
        })
      )
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
          sourceTabId: props.tabId,
          action: 'created',
        })
      }
    } catch (error: unknown) {
      toast.error(getErrorMessage(error) || t('assistant.errors.createConversationFailed'))
      return
    }
  }

  // Now send the message via ChatService
  if (activeConversationId.value) {
    try {
      await chatStore.sendMessage(activeConversationId.value, messageContent, props.tabId)

      // Update conversation's last_message
      try {
        const updated = await ConversationsService.UpdateConversation(
          activeConversationId.value,
          new UpdateConversationInput({
            last_message: messageContent,
          })
        )
        if (updated) {
          handleConversationUpdated(updated)
        }
      } catch {
        // Non-critical error
      }
    } catch (error: unknown) {
      toast.error(getErrorMessage(error) || t('assistant.errors.sendFailed'))
    }
  }
}

const handleStop = () => {
  if (!activeConversationId.value) return
  void chatStore.stopGeneration(activeConversationId.value)
}

const handleChatEnter = (event: KeyboardEvent) => {
  // Prevent sending when IME is composing (Chinese/Japanese/Korean input).
  // Some browsers report keyCode=229 during composition.

  const anyEvent = event as any
  if (anyEvent?.isComposing || anyEvent?.keyCode === 229) {
    return
  }

  event.preventDefault()
  void handleSend()
}

const handleSelectConversation = (conversation: Conversation) => {
  activeConversationId.value = conversation.id
  // Load messages from backend via chatStore
  chatStore.loadMessages(conversation.id)

  // Set model selection from conversation's saved model
  if (conversation.llm_provider_id && conversation.llm_model_id) {
    selectedModelKey.value = `${conversation.llm_provider_id}::${conversation.llm_model_id}`
  }

  // Set knowledge base selection from conversation
  selectedLibraryIds.value = conversation.library_ids || []
}

// Handle library selection change from Select component
const handleLibrarySelectionChange = async () => {
  // Save to conversation if one is active
  await saveLibraryIdsToConversation()
}

// Clear all selected libraries (for UI button)
const clearLibrarySelection = async () => {
  clearKnowledgeSelection()
  await saveLibraryIdsToConversation()
}

// Save library_ids to current conversation
const saveLibraryIdsToConversation = async () => {
  if (!activeConversationId.value) return

  try {
    await ConversationsService.UpdateConversation(
      activeConversationId.value,
      new UpdateConversationInput({
        library_ids: selectedLibraryIds.value,
      })
    )
  } catch (error: unknown) {
    console.error('Failed to save library selection:', error)
  }
}

// Sync library_ids from current conversation (for multi-tab sync)
const syncLibraryIdsFromConversation = async () => {
  if (!activeConversationId.value || !activeAgentId.value) {
    clearKnowledgeSelection()
    return
  }

  // Find current conversation from cached list
  const conversations = conversationsByAgent.value[activeAgentId.value] || []
  const currentConversation = conversations.find((c) => c.id === activeConversationId.value)
  if (currentConversation) {
    selectedLibraryIds.value = currentConversation.library_ids || []
  }
}

const handleSelectImage = () => {
  // TODO: Implement image selection
}

// Handle message editing (resend from that point)
const handleEditMessage = async (messageId: number, newContent: string) => {
  if (!activeConversationId.value) return

  try {
    await chatStore.editAndResend(activeConversationId.value, messageId, newContent, props.tabId)
  } catch (error: unknown) {
    toast.error(getErrorMessage(error) || t('assistant.errors.resendFailed'))
  }
}

// Conversation menu actions
const handleOpenRenameConversation = (conv: Conversation) => {
  actionConversation.value = conv
  renameConversationOpen.value = true
}

const handleOpenDeleteConversation = (conv: Conversation) => {
  actionConversation.value = conv
  deleteConversationOpen.value = true
}

const handleTogglePin = async (conv: Conversation) => {
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
    if (activeAgentId.value) {
      await loadConversations(activeAgentId.value, { preserveSelection: true })
    }

    // Notify other assistant tabs to refresh.
    Events.Emit('conversations:changed', {
      agent_id: conv.agent_id,
      sourceTabId: props.tabId,
      action: 'pin',
    })
  } catch (error) {
    console.error('Failed to toggle pin:', error)
    toast.error(getErrorMessage(error) || t('assistant.errors.updateConversationFailed'))
  }
}

const handleConversationUpdated = (updated: Conversation) => {
  const agentId = updated.agent_id
  const current = conversationsByAgent.value[agentId] ?? []
  // Update and re-sort (pinned first, then by updated_at desc)
  const next = current
    .map((c) => (c.id === updated.id ? updated : c))
    .sort((a, b) => {
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
    sourceTabId: props.tabId,
    action: 'updated',
  })
}

const confirmDeleteConversation = async () => {
  if (!actionConversation.value) return
  try {
    await ConversationsService.DeleteConversation(actionConversation.value.id)
    const agentId = actionConversation.value.agent_id
    const current = conversationsByAgent.value[agentId] ?? []
    conversationsByAgent.value = {
      ...conversationsByAgent.value,
      [agentId]: current.filter((c) => c.id !== actionConversation.value?.id),
    }
    conversationsLoadedByAgent.value = {
      ...conversationsLoadedByAgent.value,
      [agentId]: true,
    }
    conversationsStaleByAgent.value = {
      ...conversationsStaleByAgent.value,
      [agentId]: false,
    }
    if (activeConversationId.value === actionConversation.value.id) {
      chatStore.clearMessages(activeConversationId.value)
      activeConversationId.value = null
      // Clear knowledge base selection when active conversation is deleted
      clearKnowledgeSelection()
    }
    toast.success(t('assistant.conversation.delete.success'))
    deleteConversationOpen.value = false

    // Notify other assistant tabs to refresh.
    Events.Emit('conversations:changed', {
      agent_id: agentId,
      sourceTabId: props.tabId,
      action: 'deleted',
    })
  } catch (error) {
    console.error('Failed to delete conversation:', error)
    toast.error(getErrorMessage(error) || t('assistant.errors.deleteConversationFailed'))
  }
}

/**
 * Update current tab's icon and title to match selected agent
 */
const updateCurrentTab = () => {
  const agent = activeAgent.value
  // Use agent's custom icon or default logo
  const icon = agent?.icon || getLogoDataUrl()
  navigationStore.updateTabIcon(props.tabId, icon, { isDefault: !agent?.icon })
  // Use agent name as tab title
  navigationStore.updateTabTitle(props.tabId, agent?.name)
}

// Watch for active agent changes to update selected model, tab info, and load conversations
watch(activeAgentId, (newAgentId, oldAgentId) => {
  selectDefaultModel()
  updateCurrentTab()
  // 切换助手时加载新助手的会话列表
  if (newAgentId && oldAgentId !== undefined) {
    loadConversations(newAgentId)
  } else if (!newAgentId) {
    if (activeConversationId.value) {
      chatStore.clearMessages(activeConversationId.value)
    }
    activeConversationId.value = null
    // Clear knowledge base selection when no agent is active
    clearKnowledgeSelection()
  }
})

// Watch for models loaded
watch(providersWithModels, () => {
  selectDefaultModel()
})

// 当前标签页是否激活
const isTabActive = computed(() => navigationStore.activeTabId === props.tabId)

// 监听标签页激活状态，激活时刷新模型/助手列表
// - 模型：用户可能在设置页启用/禁用 provider/model
// - 助手：其它标签页可能创建/更新/删除了助手
watch(isTabActive, (active) => {
  if (active) {
    void (async () => {
      await loadModels()
      await loadAgents()
      // Refresh knowledge base list (user may have created/deleted libraries in other pages)
      await loadLibraries()

      // Multi-tab reliability: always refresh conversations from DB when this tab becomes active.
      if (activeAgentId.value != null) {
        await loadConversations(activeAgentId.value, { preserveSelection: true, force: true })
        // Sync library_ids for current conversation (may have changed from other tabs)
        await syncLibraryIdsFromConversation()
      }
    })()
  }
})

// Listen for text selection events
let unsubscribeTextSelection: (() => void) | null = null
let unsubscribeConversationsChanged: (() => void) | null = null

onMounted(() => {
  void (async () => {
    await loadAgents()
    await loadModels()
    await loadLibraries()

    if (activeAgentId.value != null) {
      await loadConversations(activeAgentId.value, { force: true })
    }
  })()

  // Subscribe to chat events (important: do this at page level, not in ChatMessageList)
  chatStore.subscribe()

  // Listen for text selection to send to assistant
  unsubscribeTextSelection = Events.On('text-selection:send-to-assistant', (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    const text = payload?.text ?? ''
    if (text) {
      chatInput.value = text
      // Auto-send after a short delay (if model is selected)
      if (canSend.value) {
        window.setTimeout(() => {
          handleSend()
        }, 100)
      }
    }
  })

  // Listen for conversation changes from other assistant tabs.
  unsubscribeConversationsChanged = Events.On('conversations:changed', (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    const agentId = Number(payload?.agent_id)
    const sourceTabId = String(payload?.sourceTabId ?? '')
    if (!Number.isFinite(agentId) || agentId <= 0) return
    if (sourceTabId && sourceTabId === props.tabId) return

    markConversationsStale(agentId)

    // If this tab is active and currently viewing the same agent, refresh immediately.
    if (isTabActive.value && activeAgentId.value === agentId) {
      void loadConversations(agentId, { preserveSelection: true, force: true })
    }
  })
})

onUnmounted(() => {
  // Unsubscribe from chat events
  chatStore.unsubscribe()

  unsubscribeTextSelection?.()
  unsubscribeTextSelection = null
  unsubscribeConversationsChanged?.()
  unsubscribeConversationsChanged = null
})
</script>

<template>
  <div class="flex h-full w-full overflow-hidden bg-background">
    <!-- Left side: Agent list -->
    <aside
      :class="
        cn(
          'flex shrink-0 flex-col border-r border-border transition-all duration-200',
          sidebarCollapsed ? 'w-0 overflow-hidden' : 'w-sidebar'
        )
      "
    >
      <div class="flex items-center justify-between gap-2 p-3">
        <div class="inline-flex rounded-md bg-muted p-1">
          <button
            :class="
              cn(
                'rounded px-3 py-1 text-sm transition-colors',
                mode === 'personal'
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              )
            "
            @click="mode = 'personal'"
          >
            {{ t('assistant.modes.personal') }}
          </button>
          <button
            :class="
              cn('rounded px-3 py-1 text-sm transition-colors', 'cursor-not-allowed opacity-50')
            "
            disabled
          >
            {{ t('assistant.modes.team') }}
          </button>
        </div>

        <Button size="icon" variant="ghost" :disabled="loading" @click="createOpen = true">
          <IconAgentAdd class="size-4 text-muted-foreground" />
        </Button>
      </div>

      <div class="flex-1 overflow-auto px-2 pb-3">
        <div
          v-if="agents.length === 0"
          class="mx-2 mt-2 flex items-center justify-center rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground"
        >
          <div class="text-center text-sm text-muted-foreground">
            {{ t('assistant.empty') }}
          </div>
        </div>

        <div class="flex flex-col gap-1.5">
          <div v-for="a in agents" :key="a.id" class="flex flex-col">
            <!-- Agent item -->
            <div
              :class="
                cn(
                  'group flex h-11 w-full items-center gap-2 rounded px-2 text-left outline-none transition-colors',
                  a.id === activeAgentId
                    ? 'bg-zinc-100 text-foreground dark:bg-accent'
                    : 'bg-white text-muted-foreground shadow-[0px_1px_4px_0px_rgba(0,0,0,0.1)] hover:bg-accent/50 hover:text-foreground dark:bg-zinc-800/50 dark:shadow-[0px_1px_4px_0px_rgba(255,255,255,0.05)]'
                )
              "
              role="button"
              tabindex="0"
              @click="activeAgentId = a.id"
              @keydown.enter.prevent="activeAgentId = a.id"
              @keydown.space.prevent="activeAgentId = a.id"
            >
              <div
                class="flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-[10px] border border-border bg-white text-foreground dark:border-white/15 dark:bg-white/5"
              >
                <img v-if="a.icon" :src="a.icon" class="size-6 object-contain" />
                <LogoIcon v-else class="size-6 opacity-90" />
              </div>

              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-normal">
                  {{ a.name }}
                </div>
              </div>

              <!-- Action buttons -->
              <div class="flex items-center gap-0">
                <!-- New conversation button -->
                <Button
                  size="icon"
                  variant="ghost"
                  class="size-7 opacity-0 group-hover:opacity-100 hover:bg-muted/60 dark:hover:bg-white/10"
                  :title="t('assistant.sidebar.newConversation')"
                  @click.stop="handleNewConversationForAgent(a.id)"
                >
                  <IconNewConversation class="size-4 text-muted-foreground" />
                </Button>

                <!-- Settings dropdown -->
                <DropdownMenu>
                  <DropdownMenuTrigger as-child>
                    <Button
                      size="icon"
                      variant="ghost"
                      class="size-7 opacity-0 group-hover:opacity-100 hover:bg-muted/60 dark:hover:bg-white/10"
                      :title="t('assistant.actions.settings')"
                      @click.stop
                    >
                      <IconSettings class="size-4 opacity-80 group-hover:opacity-100" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start" class="w-48">
                    <DropdownMenuItem @click="openSettings(a)">
                      {{ t('assistant.menu.settings') }}
                    </DropdownMenuItem>
                    <DropdownMenuSub>
                      <DropdownMenuSubTrigger
                        @mouseenter="ensureConversationsLoaded(a.id)"
                        @focus="ensureConversationsLoaded(a.id)"
                      >
                        {{ t('assistant.menu.history') }}
                      </DropdownMenuSubTrigger>
                      <DropdownMenuSubContent class="w-56">
                        <template v-if="getAllAgentConversations(a.id).length > 0">
                          <DropdownMenuItem
                            v-for="conv in getAllAgentConversations(a.id)"
                            :key="conv.id"
                            @click="handleSelectConversationForAgent(a.id, conv)"
                          >
                            <span class="truncate">{{ conv.name }}</span>
                          </DropdownMenuItem>
                        </template>
                        <DropdownMenuItem v-else disabled>
                          {{ t('assistant.conversation.empty') }}
                        </DropdownMenuItem>
                      </DropdownMenuSubContent>
                    </DropdownMenuSub>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>

            <!-- Conversation list (max 3 items) - only show for active agent -->
            <div
              v-if="a.id === activeAgentId && getAgentConversations(a.id).length > 0"
              class="mt-1 flex flex-col gap-0.5"
            >
              <div
                v-for="conv in getAgentConversations(a.id)"
                :key="conv.id"
                :class="
                  cn(
                    'group flex items-center gap-1 rounded px-2 py-1.5 text-left text-sm transition-colors',
                    activeConversationId === conv.id
                      ? 'bg-accent/60 text-foreground'
                      : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
                  )
                "
                role="button"
                tabindex="0"
                @click="handleSelectConversation(conv)"
                @keydown.enter.prevent="handleSelectConversation(conv)"
              >
                <Pin v-if="conv.is_pinned" class="size-3 shrink-0 text-muted-foreground" />
                <span class="min-w-0 flex-1 truncate">{{ conv.name }}</span>
                <!-- Conversation menu -->
                <DropdownMenu>
                  <DropdownMenuTrigger
                    class="flex h-5 w-5 shrink-0 items-center justify-center rounded text-muted-foreground opacity-0 transition-opacity hover:bg-background/60 hover:text-foreground group-hover:opacity-100"
                    @click.stop
                  >
                    <MoreHorizontal class="size-3.5" />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" class="w-36">
                    <DropdownMenuItem class="gap-2" @select="handleTogglePin(conv)">
                      <PinOff v-if="conv.is_pinned" class="size-4 text-muted-foreground" />
                      <Pin v-else class="size-4 text-muted-foreground" />
                      {{ conv.is_pinned ? t('assistant.menu.unpin') : t('assistant.menu.pin') }}
                    </DropdownMenuItem>
                    <DropdownMenuItem class="gap-2" @select="handleOpenRenameConversation(conv)">
                      <IconRename class="size-4 text-muted-foreground" />
                      {{ t('assistant.menu.rename') }}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      class="gap-2 text-destructive focus:text-destructive"
                      @select="handleOpenDeleteConversation(conv)"
                    >
                      <IconDelete class="size-4" />
                      {{ t('assistant.menu.delete') }}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>
          </div>
        </div>
      </div>
    </aside>

    <!-- Collapse/Expand button -->
    <div class="flex w-8 shrink-0 items-center justify-center">
      <Button
        size="icon"
        variant="ghost"
        class="size-6"
        :title="sidebarCollapsed ? t('assistant.sidebar.expand') : t('assistant.sidebar.collapse')"
        @click="sidebarCollapsed = !sidebarCollapsed"
      >
        <IconSidebarExpand v-if="sidebarCollapsed" class="size-5 text-muted-foreground" />
        <IconSidebarCollapse v-else class="size-5 text-muted-foreground" />
      </Button>
    </div>

    <!-- Right side: Chat area -->
    <section class="flex min-w-0 flex-1 flex-col overflow-hidden">
      <!-- Chat messages area - show when we have messages OR when generating -->
      <ChatMessageList
        v-if="activeConversationId && (chatMessages.length > 0 || isGenerating)"
        :conversation-id="activeConversationId"
        :tab-id="tabId"
        class="min-w-0 flex-1 overflow-hidden"
        @edit-message="handleEditMessage"
      />

      <!-- Empty state / Input area -->
      <div
        :class="
          cn(
            'flex px-6',
            chatMessages.length > 0 || isGenerating ? 'pb-4' : 'flex-1 items-center justify-center'
          )
        "
      >
        <div
          :class="
            cn(
              'flex w-full flex-col items-center gap-10',
              chatMessages.length === 0 && !isGenerating && '-translate-y-10'
            )
          "
        >
          <div v-if="chatMessages.length === 0 && !isGenerating" class="flex items-center gap-3">
            <LogoIcon class="size-10 text-foreground" />
            <div class="text-2xl font-semibold text-foreground">
              {{ t('app.title') }}
            </div>
          </div>

          <div
            class="w-full max-w-[800px] rounded-2xl border border-border bg-background px-4 py-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
          >
            <textarea
              v-model="chatInput"
              :placeholder="t('assistant.placeholders.inputPlaceholder')"
              class="min-h-[64px] w-full resize-none bg-transparent text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
              rows="2"
              @keydown.enter.exact="handleChatEnter"
            />

            <div class="mt-3 flex items-center justify-between">
              <div class="flex items-center gap-2">
                <Select v-model="selectedModelKey" :disabled="!hasModels">
                  <SelectTrigger
                    class="h-8 w-auto min-w-[160px] max-w-[240px] rounded-full border border-border bg-background px-3 text-xs shadow-[0_1px_2px_rgba(0,0,0,0.04)] hover:bg-muted/40"
                  >
                    <div v-if="selectedModelInfo" class="flex min-w-0 items-center gap-1.5">
                      <ProviderIcon
                        :icon="selectedModelInfo.providerId"
                        :size="14"
                        class="shrink-0 text-foreground"
                      />
                      <span class="truncate">{{ selectedModelInfo.modelName }}</span>
                    </div>
                    <span v-else class="text-muted-foreground">
                      {{ t('assistant.chat.noModel') }}
                    </span>
                  </SelectTrigger>
                  <SelectContent class="max-h-[260px]">
                    <SelectGroup>
                      <SelectLabel>{{ t('assistant.chat.selectModel') }}</SelectLabel>
                      <template v-for="pw in providersWithModels" :key="pw.provider.provider_id">
                        <SelectLabel class="mt-2 text-xs text-muted-foreground">
                          {{ pw.provider.name }}
                        </SelectLabel>
                        <template v-for="g in pw.model_groups" :key="g.type">
                          <template v-if="g.type === 'llm'">
                            <SelectItem
                              v-for="m in g.models"
                              :key="pw.provider.provider_id + '::' + m.model_id"
                              :value="pw.provider.provider_id + '::' + m.model_id"
                            >
                              {{ m.name }}
                            </SelectItem>
                          </template>
                        </template>
                      </template>
                    </SelectGroup>
                  </SelectContent>
                </Select>

                <!-- Knowledge base multi-select using reka-ui Select with multiple -->
                <SelectRoot
                  v-model="selectedLibraryIds"
                  multiple
                  @update:model-value="handleLibrarySelectionChange"
                  @update:open="(open: boolean) => open && loadLibraries()"
                >
                  <SelectTriggerRaw
                    as-child
                    :title="
                      selectedLibraryIds.length > 0
                        ? t('assistant.chat.selectedCount', { count: selectedLibraryIds.length })
                        : t('assistant.chat.selectKnowledge')
                    "
                  >
                    <Button
                      size="icon"
                      variant="ghost"
                      class="size-8 rounded-full border border-border bg-background hover:bg-muted/40"
                      :class="{
                        'border-primary/50 bg-primary/10': selectedLibraryIds.length > 0,
                      }"
                    >
                      <IconSelectKnowledge
                        class="size-4"
                        :class="
                          selectedLibraryIds.length > 0 ? 'text-primary' : 'text-muted-foreground'
                        "
                      />
                    </Button>
                  </SelectTriggerRaw>
                  <SelectPortal>
                    <SelectContentRaw
                      class="z-50 max-h-[300px] min-w-[200px] overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
                      position="popper"
                      :side-offset="5"
                    >
                      <SelectViewport>
                        <!-- Clear selection option - use a div with click handler since SelectItem would add it to selection -->
                        <div
                          class="relative flex cursor-pointer select-none items-center rounded-sm px-2 py-1.5 text-sm text-muted-foreground outline-none hover:bg-accent hover:text-accent-foreground"
                          @click="clearLibrarySelection"
                        >
                          {{ t('assistant.chat.clearSelected') }}
                        </div>
                        <SelectSeparator
                          v-if="libraries.length > 0"
                          class="mx-1 my-1 h-px bg-muted"
                        />
                        <!-- Library list -->
                        <template v-if="libraries.length > 0">
                          <SelectItemRaw
                            v-for="lib in libraries"
                            :key="lib.id"
                            :value="Number(lib.id)"
                            class="relative flex cursor-pointer select-none items-center rounded-sm py-1.5 pl-8 pr-2 text-sm outline-none data-highlighted:bg-accent data-highlighted:text-accent-foreground data-disabled:pointer-events-none data-disabled:opacity-50"
                          >
                            <SelectItemIndicator
                              class="absolute left-2 flex size-4 items-center justify-center"
                            >
                              <Check class="size-4 text-primary" />
                            </SelectItemIndicator>
                            <SelectItemText>{{ lib.name }}</SelectItemText>
                          </SelectItemRaw>
                        </template>
                        <template v-else>
                          <div class="px-2 py-1.5 text-sm text-muted-foreground">
                            {{ t('assistant.chat.noKnowledge') }}
                          </div>
                        </template>
                      </SelectViewport>
                    </SelectContentRaw>
                  </SelectPortal>
                </SelectRoot>

                <Button
                  size="icon"
                  variant="ghost"
                  class="size-8 rounded-full border border-border bg-background hover:bg-muted/40"
                  :title="t('assistant.chat.selectImage')"
                  @click="handleSelectImage"
                >
                  <IconSelectImage class="size-4 text-muted-foreground" />
                </Button>
              </div>

              <template v-if="isGenerating">
                <Button
                  size="icon"
                  class="size-6 rounded-full bg-muted-foreground/20 text-foreground hover:bg-muted-foreground/30"
                  :title="t('assistant.chat.stop')"
                  @click="handleStop"
                >
                  <Square class="size-4" />
                </Button>
              </template>
              <template v-else>
                <TooltipProvider v-if="!canSend">
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <!-- disabled button has pointer-events-none; use wrapper to keep tooltip hover -->
                      <span class="inline-flex">
                        <Button
                          size="icon"
                          class="size-6 rounded-full bg-muted-foreground/20 text-muted-foreground disabled:opacity-100"
                          disabled
                        >
                          <ArrowUp class="size-4" />
                        </Button>
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>{{ t('assistant.placeholders.enterToSend') }}</p>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
                <Button
                  v-else
                  size="icon"
                  class="size-6 rounded-full bg-primary text-primary-foreground hover:bg-primary/90"
                  :title="t('assistant.chat.send')"
                  @click="handleSend"
                >
                  <ArrowUp class="size-4" />
                </Button>
              </template>
            </div>
          </div>
        </div>
      </div>
    </section>

    <CreateAgentDialog v-model:open="createOpen" :loading="loading" @create="handleCreate" />
    <AgentSettingsDialog
      v-model:open="settingsOpen"
      :agent="settingsAgent"
      @updated="handleUpdated"
      @deleted="handleDeleted"
    />

    <!-- Conversation dialogs -->
    <RenameConversationDialog
      v-model:open="renameConversationOpen"
      :conversation="actionConversation"
      @updated="handleConversationUpdated"
    />

    <AlertDialog v-model:open="deleteConversationOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('assistant.conversation.delete.title') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('assistant.conversation.delete.desc', { name: actionConversation?.name }) }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>
            {{ t('assistant.conversation.delete.cancel') }}
          </AlertDialogCancel>
          <AlertDialogAction
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            @click.prevent="confirmDeleteConversation"
          >
            {{ t('assistant.conversation.delete.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
