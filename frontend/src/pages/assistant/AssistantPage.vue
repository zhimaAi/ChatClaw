<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import IconAssistant from '@/assets/icons/assistant.svg'
import IconSidebarCollapse from '@/assets/icons/sidebar-collapse.svg'
import IconSidebarExpand from '@/assets/icons/sidebar-expand.svg'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { getLogoDataUrl } from '@/composables/useLogo'
import CreateAgentDialog from './components/CreateAgentDialog.vue'
import AgentSettingsDialog from './components/AgentSettingsDialog.vue'
import RenameConversationDialog from './components/RenameConversationDialog.vue'
import ChatMessageList from './components/ChatMessageList.vue'
import AgentSidebar from './components/AgentSidebar.vue'
import ChatInputArea from './components/ChatInputArea.vue'
import SnapModeHeader from './components/SnapModeHeader.vue'
import { useNavigationStore, useChatStore } from '@/stores'
import { type Agent } from '@bindings/willchat/internal/services/agents'
import { Events } from '@wailsio/runtime'
import {
  ConversationsService,
  type Conversation,
  CreateConversationInput,
  UpdateConversationInput,
} from '@bindings/willchat/internal/services/conversations'
import { SnapService } from '@bindings/willchat/internal/services/windows'
import { TextSelectionService } from '@bindings/willchat/internal/services/textselection'
import { LibraryService, type Library } from '@bindings/willchat/internal/services/library'
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
import { useAgents } from './composables/useAgents'
import { useConversations } from './composables/useConversations'
import { useModelSelection } from './composables/useModelSelection'
import { useSnapMode } from './composables/useSnapMode'

/**
 * Props - 每个标签页实例都有自己独立的 tabId
 * 通过 v-show 控制显示/隐藏，组件实例不会被销毁，状态自然保留
 * mode: 'main' 主窗体模式（默认），'snap' 吸附窗体模式
 */
const props = withDefaults(
  defineProps<{
    tabId: string
    mode?: 'main' | 'snap'
  }>(),
  {
    mode: 'main',
  }
)

// Computed for mode checks
const isSnapMode = computed(() => props.mode === 'snap')

type ListMode = 'personal' | 'team'

const { t } = useI18n()
const navigationStore = useNavigationStore()
const chatStore = useChatStore()

// Use composables
const {
  agents,
  activeAgentId,
  loading,
  loadAgents,
  createAgent,
  updateAgent,
  deleteAgent,
} = useAgents()

const {
  conversationsByAgent,
  activeConversationId,
  activeConversation,
  loadConversations,
  ensureConversationsLoaded,
  markConversationsStale,
  getAgentConversations,
  getAllAgentConversations,
  createConversation,
  updateConversation: updateConversationState,
  deleteConversation,
  togglePin,
  clearAgentCache,
} = useConversations(props.tabId)

const {
  providersWithModels,
  selectedModelKey,
  hasModels,
  selectedModelInfo,
  loadModels,
  selectDefaultModel,
  saveModelToConversationIfNeeded,
} = useModelSelection()

const {
  hasAttachedTarget,
  showAiSendButton,
  showAiEditButton,
  checkSnapStatus,
  loadSnapSettings,
  cancelSnap,
  findAndAttach,
  handleSendAndTrigger,
  handleSendToEdit,
  handleCopyToClipboard,
} = useSnapMode()

// Local state
const listMode = ref<ListMode>('personal')
const createOpen = ref(false)
const settingsOpen = ref(false)
const settingsAgent = ref<Agent | null>(null)
const sidebarCollapsed = ref(false)
const chatInput = ref('')
const enableThinking = ref(false)
const libraries = ref<Library[]>([])
const selectedLibraryIds = ref<number[]>([])
const renameConversationOpen = ref(false)
const deleteConversationOpen = ref(false)
const actionConversation = ref<Conversation | null>(null)

// Computed
const activeAgent = computed(() => {
  if (activeAgentId.value == null) return null
  return agents.value.find((a) => a.id === activeAgentId.value) ?? null
})

// Whether the agent list is empty (loaded & no agents)
const isAgentEmpty = computed(
  () => !loading.value && agents.value.length === 0
)

// Helper function to clear knowledge base selection
const clearKnowledgeSelection = () => {
  selectedLibraryIds.value = []
}

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
  return (
    !!activeAgentId.value &&
    chatInput.value.trim() !== '' &&
    !!selectedModelInfo.value &&
    !isGenerating.value
  )
})

// Reason why send is disabled (for tooltip)
const sendDisabledReason = computed(() => {
  if (isGenerating.value) return ''
  if (!activeAgentId.value) return t('assistant.placeholders.createAgentFirst')
  if (!selectedModelKey.value) return t('assistant.placeholders.selectModelFirst')
  if (!chatInput.value.trim()) return t('assistant.placeholders.enterToSend')
  return ''
})

// Load knowledge base list
const loadLibrariesFn = async () => {
  try {
    const list = await LibraryService.ListLibraries()
    libraries.value = list || []
  } catch (error: unknown) {
    console.error('Failed to load libraries:', error)
  }
}

const handleCreate = async (data: { name: string; prompt: string; icon: string }) => {
  try {
    await createAgent(data)
    createOpen.value = false
    updateCurrentTab()
    
    // Notify other windows/tabs to refresh agents list
    Events.Emit('agents:changed', {
      sourceTabId: props.tabId,
      action: 'created',
    })
  } catch {
    // Error already handled in composable
  }
}

const openSettings = (agent: Agent) => {
  settingsAgent.value = agent
  settingsOpen.value = true
}

const handleUpdated = (updated: Agent) => {
  // Get old agent info to check if default model changed
  const oldAgent = agents.value.find((a) => a.id === updated.id)
  const hadDefaultModel = oldAgent?.default_llm_provider_id && oldAgent?.default_llm_model_id
  const hasDefaultModel = updated.default_llm_provider_id && updated.default_llm_model_id

  updateAgent(updated)

  // If updating the currently selected agent
  if (activeAgentId.value === updated.id) {
    updateCurrentTab()
    // Sync model selection if default model changed
    if (hasDefaultModel || hadDefaultModel) {
      selectDefaultModel(activeAgent.value, activeConversation.value)
    }
  }

  // Notify other windows/tabs to refresh agents list
  Events.Emit('agents:changed', {
    sourceTabId: props.tabId,
    action: 'updated',
  })
}

const handleDeleted = (id: number) => {
  deleteAgent(id)
  clearAgentCache(id)
  
  // Notify other windows/tabs to refresh agents list
  Events.Emit('agents:changed', {
    sourceTabId: props.tabId,
    action: 'deleted',
  })
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
  // Reset thinking mode to default (off) for new conversation
  enableThinking.value = false
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

  // Set thinking mode from conversation
  enableThinking.value = conversation.enable_thinking || false
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

      await createConversation(
        new CreateConversationInput({
          agent_id: activeAgentId.value,
          name: messageContent.slice(0, 50), // First message becomes conversation name (truncated)
          last_message: messageContent,
          llm_provider_id: providerId || '',
          llm_model_id: modelId || '',
          library_ids: selectedLibraryIds.value,
          enable_thinking: enableThinking.value,
        })
      )
    } catch {
      // Error already handled in composable
      return
    }
  }

  // Now send the message via ChatService
  if (activeConversationId.value) {
    try {
      // Ensure conversation model is synced before sending to backend.
      // Backend send API only takes conversation_id, so it relies on the saved model in conversation record.
      const updated = await saveModelToConversationIfNeeded(
        activeConversationId.value,
        activeConversation.value,
        { silent: true }
      )
      if (updated) {
        handleConversationUpdated(updated)
      }

      await chatStore.sendMessage(activeConversationId.value, messageContent, props.tabId)

      // Update conversation's last_message
      try {
        const updated2 = await ConversationsService.UpdateConversation(
          activeConversationId.value,
          new UpdateConversationInput({
            last_message: messageContent,
          })
        )
        if (updated2) {
          handleConversationUpdated(updated2)
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

// Save thinking mode to current conversation
const saveThinkingToConversation = async () => {
  if (!activeConversationId.value) return
  try {
    await ConversationsService.UpdateConversation(
      activeConversationId.value,
      new UpdateConversationInput({
        enable_thinking: enableThinking.value,
      })
    )
  } catch (err) {
    console.error('Failed to save thinking mode to conversation:', err)
  }
}

// Watch thinking mode changes and save to conversation
watch(enableThinking, () => {
  void saveThinkingToConversation()
})

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
  try {
    await togglePin(conv, activeAgentId.value)
  } catch {
    // Error already handled in composable
  }
}

function handleConversationUpdated(updated: Conversation) {
  updateConversationState(updated)
}

const confirmDeleteConversation = async () => {
  if (!actionConversation.value) return
  try {
    await deleteConversation(actionConversation.value)
    // Clear knowledge base selection when active conversation is deleted
    if (activeConversationId.value === actionConversation.value.id) {
      clearKnowledgeSelection()
    }
    deleteConversationOpen.value = false
  } catch {
    // Error already handled in composable
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
  selectDefaultModel(activeAgent.value, activeConversation.value)
  updateCurrentTab()
  // 切换助手时加载新助手的会话列表
  if (newAgentId && oldAgentId !== undefined) {
    loadConversations(newAgentId, { activeAgentId: newAgentId })
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
  selectDefaultModel(activeAgent.value, activeConversation.value)
})

// Persist model switching to current conversation so backend uses the new model.
watch(selectedModelKey, () => {
  // Fire-and-forget; handleSend will also await a sync to avoid race conditions.
  void (async () => {
    const updated = await saveModelToConversationIfNeeded(
      activeConversationId.value,
      activeConversation.value,
      { silent: true }
    )
    if (updated) {
      handleConversationUpdated(updated)
    }
  })()
})

// 当前标签页是否激活
// For snap mode, always consider it active since it's a standalone window
const isTabActive = computed(() => isSnapMode.value || navigationStore.activeTabId === props.tabId)


// 监听标签页激活状态，激活时刷新模型/助手列表
// - 模型：用户可能在设置页启用/禁用 provider/model
// - 助手：其它标签页可能创建/更新/删除了助手
watch(isTabActive, (active) => {
  if (active) {
    void (async () => {
      await loadModels()
      await loadAgents()
      // Refresh knowledge base list (user may have created/deleted libraries in other pages)
      await loadLibrariesFn()

      // Multi-tab reliability: always refresh conversations from DB when this tab becomes active.
      if (activeAgentId.value != null) {
        await loadConversations(activeAgentId.value, {
          preserveSelection: true,
          force: true,
          activeAgentId: activeAgentId.value,
        })
        // Sync library_ids for current conversation (may have changed from other tabs)
        await syncLibraryIdsFromConversation()
      }
    })()
  }
})

// Listen for text selection events
let unsubscribeTextSelection: (() => void) | null = null
let unsubscribeConversationsChanged: (() => void) | null = null
let unsubscribeAgentsChanged: (() => void) | null = null
// Snap mode event listeners
let unsubscribeSnapSettings: (() => void) | null = null
let unsubscribeSnapStateChanged: (() => void) | null = null
let unsubscribeTextSelectionSnap: (() => void) | null = null
let onPointerDown: ((e: globalThis.PointerEvent) => void) | null = null

onMounted(() => {
  // In snap mode, default sidebar to collapsed
  if (isSnapMode.value) {
    sidebarCollapsed.value = true
  }

  void (async () => {
    await loadAgents()
    await loadModels()
    await loadLibrariesFn()

    if (activeAgentId.value != null) {
      // Preserve current selection to avoid wiping the first message on cold start.
      await loadConversations(activeAgentId.value, {
        preserveSelection: true,
        force: true,
        activeAgentId: activeAgentId.value,
      })
    }

    // Check for pending chat data (e.g. from knowledge page shortcut)
    const pendingData = navigationStore.consumePendingChatData(props.tabId)
    if (pendingData) {
      // Apply pre-selected agent
      if (pendingData.agentId && agents.value.some((a) => a.id === pendingData.agentId)) {
        activeAgentId.value = pendingData.agentId
      }
      // Apply pre-selected model
      if (pendingData.selectedModelKey) {
        selectedModelKey.value = pendingData.selectedModelKey
      }
      // Apply library selection
      if (pendingData.libraryIds && pendingData.libraryIds.length > 0) {
        selectedLibraryIds.value = pendingData.libraryIds
      }
      // Apply thinking mode
      if (pendingData.enableThinking != null) {
        enableThinking.value = pendingData.enableThinking
      }
      // Apply chat input and auto-send
      if (pendingData.chatInput) {
        chatInput.value = pendingData.chatInput
      }
      // Ensure we start with a new conversation
      activeConversationId.value = null

      // Auto-send after a short delay to let Vue reactivity settle
      if (pendingData.chatInput) {
        window.setTimeout(() => {
          if (canSend.value) {
            handleSend()
          }
        }, 200)
      }
    }

    // Snap mode initialization
    if (isSnapMode.value) {
      await loadSnapSettings()
      await checkSnapStatus()
    }
  })()

  // Subscribe to chat events (important: do this at page level, not in ChatMessageList)
  chatStore.subscribe()

  // Listen for text selection to send to assistant (main mode only)
  if (!isSnapMode.value) {
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
  }

  // Snap mode event listeners
  if (isSnapMode.value) {
    // Listen for settings changes
    unsubscribeSnapSettings = Events.On('snap:settings-changed', () => {
      void loadSnapSettings()
      void checkSnapStatus()
    })

    // Listen for snap state changes
    unsubscribeSnapStateChanged = Events.On('snap:state-changed', (event: any) => {
      const payload = Array.isArray(event?.data) ? event.data[0] : event?.data ?? event
      const state = payload?.state
      const targetProcess = payload?.targetProcess
      hasAttachedTarget.value = state === 'attached' && !!targetProcess
    })

    // Listen for text selection to send to snap
    unsubscribeTextSelectionSnap = Events.On('text-selection:send-to-snap', (event: any) => {
      const payload = Array.isArray(event?.data) ? event.data[0] : event?.data ?? event
      const text = payload?.text ?? ''
      if (text) {
        chatInput.value = text
        // Auto-send after a short delay
        window.setTimeout(() => {
          if (canSend.value) {
            handleSend()
          }
        }, 100)
      }
    })

    // Fallback: pull the latest selection action once on startup
    void (async () => {
      try {
        const action = await TextSelectionService.GetLastButtonAction()
        const text = String((action as any)?.text ?? '')
        if (text) {
          chatInput.value = text
          window.setTimeout(() => {
            if (canSend.value) {
              handleSend()
            }
          }, 100)
        }
      } catch (error) {
        console.error('Failed to get last selection action:', error)
      }
    })()

    // When interacting with snap window while attached, bring both windows to front
    onPointerDown = (e: globalThis.PointerEvent) => {
      if (e.button !== 0) return
      void SnapService.WakeAttached()
    }
    window.addEventListener('pointerdown', onPointerDown, true)
  }

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
      void loadConversations(agentId, {
        preserveSelection: true,
        force: true,
        activeAgentId: activeAgentId.value,
      })
    }
  })

  // Listen for agent changes from other windows/tabs (e.g., main window <-> snap window)
  unsubscribeAgentsChanged = Events.On('agents:changed', (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    const sourceTabId = String(payload?.sourceTabId ?? '')
    // Ignore events from self
    if (sourceTabId && sourceTabId === props.tabId) return

    // Refresh agents list
    void loadAgents()
  })
})

onUnmounted(() => {
  // Unsubscribe from chat events
  chatStore.unsubscribe()

  unsubscribeTextSelection?.()
  unsubscribeTextSelection = null
  unsubscribeConversationsChanged?.()
  unsubscribeConversationsChanged = null
  unsubscribeAgentsChanged?.()
  unsubscribeAgentsChanged = null

  // Snap mode cleanup
  unsubscribeSnapSettings?.()
  unsubscribeSnapSettings = null
  unsubscribeSnapStateChanged?.()
  unsubscribeSnapStateChanged = null
  unsubscribeTextSelectionSnap?.()
  unsubscribeTextSelectionSnap = null
  if (onPointerDown) {
    window.removeEventListener('pointerdown', onPointerDown, true)
    onPointerDown = null
  }
})
</script>

<template>
  <div :class="cn('flex h-full w-full overflow-hidden bg-background', isSnapMode && 'flex-col')">
    <!-- Snap mode header toolbar -->
    <SnapModeHeader
      v-if="isSnapMode"
      :agents="agents"
      :active-agent="activeAgent"
      :active-agent-id="activeAgentId"
      :has-attached-target="hasAttachedTarget"
      @update:active-agent-id="activeAgentId = $event"
      @new-conversation="handleNewConversation"
      @cancel-snap="cancelSnap"
      @find-and-attach="findAndAttach"
    />

    <!-- Main content wrapper -->
    <div class="relative flex min-h-0 flex-1 overflow-hidden">
    <!-- Overlay backdrop for snap mode sidebar -->
    <div
      v-if="isSnapMode && !sidebarCollapsed"
      class="absolute inset-0 z-10 bg-black/20"
      @click="sidebarCollapsed = true"
    />

    <!-- Left side: Agent list (collapsible, overlay in snap mode when expanded, hidden when empty) -->
    <AgentSidebar
      v-if="!sidebarCollapsed && !isAgentEmpty"
      :agents="agents"
      :active-agent-id="activeAgentId"
      :active-conversation-id="activeConversationId"
      :loading="loading"
      :list-mode="listMode"
      :is-snap-mode="isSnapMode"
      :get-agent-conversations="getAgentConversations"
      :get-all-agent-conversations="getAllAgentConversations"
      :ensure-conversations-loaded="ensureConversationsLoaded"
      @update:active-agent-id="activeAgentId = $event"
      @update:list-mode="listMode = $event"
      @create="createOpen = true"
      @open-settings="openSettings"
      @new-conversation="handleNewConversation"
      @new-conversation-for-agent="handleNewConversationForAgent"
      @select-conversation="handleSelectConversation"
      @select-conversation-for-agent="handleSelectConversationForAgent"
      @toggle-pin="handleTogglePin"
      @open-rename="handleOpenRenameConversation"
      @open-delete="handleOpenDeleteConversation"
      @close-sidebar="sidebarCollapsed = true"
    />

    <!-- Collapse/Expand button (hidden when empty) -->
    <div v-if="!isAgentEmpty" class="flex w-8 shrink-0 items-center justify-center">
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
      <!-- Agent list empty state -->
      <div v-if="isAgentEmpty" class="flex h-full items-center justify-center px-8">
        <div class="flex flex-col items-center gap-4">
          <div class="grid size-10 place-items-center rounded-lg bg-muted">
            <IconAssistant class="size-4 text-muted-foreground" />
          </div>
          <div class="flex flex-col items-center gap-1.5">
            <h3 class="text-base font-medium text-foreground">
              {{ t('assistant.emptyState.title') }}
            </h3>
            <p class="text-sm text-muted-foreground">
              {{ t('assistant.emptyState.desc') }}
            </p>
          </div>
          <Button class="mt-1" @click="createOpen = true">
            {{ t('assistant.emptyState.createBtn') }}
          </Button>
        </div>
      </div>

      <!-- Chat messages area - show when we have an active conversation -->
      <ChatMessageList
        v-if="!isAgentEmpty && activeConversationId"
        :conversation-id="activeConversationId"
        :tab-id="tabId"
        :mode="props.mode"
        :agent-name="activeAgent?.name"
        :agent-icon="activeAgent?.icon"
        :has-attached-target="hasAttachedTarget"
        :show-ai-send-button="showAiSendButton"
        :show-ai-edit-button="showAiEditButton"
        class="min-w-0 flex-1 overflow-hidden"
        @edit-message="handleEditMessage"
        @snap-send-and-trigger="handleSendAndTrigger"
        @snap-send-to-edit="handleSendToEdit"
        @snap-copy="handleCopyToClipboard"
      />

      <!-- Empty state / Input area (hidden when no agents) -->
      <ChatInputArea
        v-if="!isAgentEmpty"
        :chat-input="chatInput"
        :selected-model-key="selectedModelKey"
        :selected-model-info="selectedModelInfo"
        :providers-with-models="providersWithModels"
        :has-models="hasModels"
        :enable-thinking="enableThinking"
        :selected-library-ids="selectedLibraryIds"
        :libraries="libraries"
        :is-generating="isGenerating"
        :can-send="canSend"
        :send-disabled-reason="sendDisabledReason"
        :chat-messages="chatMessages"
        :active-agent-id="activeAgentId"
        :is-snap-mode="isSnapMode"
        @update:chat-input="chatInput = $event"
        @update:selected-model-key="selectedModelKey = $event"
        @update:enable-thinking="enableThinking = $event"
        @update:selected-library-ids="selectedLibraryIds = $event"
        @send="handleSend"
        @stop="handleStop"
        @library-selection-change="handleLibrarySelectionChange"
        @clear-library-selection="clearLibrarySelection"
        @load-libraries="loadLibrariesFn"
      />
    </section>
    </div><!-- End main content wrapper -->

    <!-- Dialogs (rendered outside main content wrapper for proper z-index) -->
    <CreateAgentDialog v-model:open="createOpen" :loading="loading" @create="handleCreate" />
    <AgentSettingsDialog
      v-model:open="settingsOpen"
      :agent="settingsAgent"
      @updated="handleUpdated"
      @deleted="handleDeleted"
    />

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
