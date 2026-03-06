<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { PanelRight } from 'lucide-vue-next'
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
import WorkspaceDrawer from './components/WorkspaceDrawer.vue'
import SnapModeHeader from './components/SnapModeHeader.vue'
import { useNavigationStore, useChatStore } from '@/stores'
import type { PendingChatImage } from '@/stores/navigation'
import { type Agent } from '@bindings/chatclaw/internal/services/agents'
import { Events } from '@wailsio/runtime'
import {
  ConversationsService,
  type Conversation,
  CreateConversationInput,
  UpdateConversationInput,
} from '@bindings/chatclaw/internal/services/conversations'
import { SnapService } from '@bindings/chatclaw/internal/services/windows'
import { TextSelectionService } from '@bindings/chatclaw/internal/services/textselection'
import { LibraryService, type Library } from '@bindings/chatclaw/internal/services/library'
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
import { supportsMultimodal } from '@/composables/useMultimodal'

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
  closeSnapWindow,
  handleSendAndTrigger,
  handleSendToEdit,
  handleCopyToClipboard,
} = useSnapMode()

// Local state
const listMode = ref<ListMode>('personal')
const createOpen = ref(false)
const settingsOpen = ref(false)
const settingsAgent = ref<Agent | null>(null)
const settingsInitialTab = ref<string>('')
const sidebarCollapsed = ref(false)
const workspaceDrawerOpen = ref(false)

// Snap mode: draggable floating expand button
const snapBtnTop = ref(8) // initial top offset in px
const snapBtnDragging = ref(false)
let snapBtnStartY = 0
let snapBtnStartTop = 0
let snapBtnDidDrag = false // true if moved beyond threshold

function onSnapBtnPointerDown(e: PointerEvent) {
  snapBtnDragging.value = true
  snapBtnDidDrag = false
  snapBtnStartY = e.clientY
  snapBtnStartTop = snapBtnTop.value
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
}
function onSnapBtnPointerMove(e: PointerEvent) {
  if (!snapBtnDragging.value) return
  const dy = e.clientY - snapBtnStartY
  if (Math.abs(dy) > 3) snapBtnDidDrag = true
  const parent = (e.currentTarget as HTMLElement).parentElement
  if (!parent) return
  const btnH = 28
  // When no messages the input is centered inside the same container,
  // so limit the button to the upper 40% to avoid overlapping the input box
  const hasMessages = chatMessages.value.length > 0 || isGenerating.value
  const maxTop = hasMessages
    ? parent.clientHeight - btnH
    : Math.min(parent.clientHeight - btnH, parent.clientHeight * 0.4)
  const newTop = Math.max(0, Math.min(maxTop, snapBtnStartTop + dy))
  snapBtnTop.value = newTop
}
function onSnapBtnPointerUp(e: PointerEvent) {
  snapBtnDragging.value = false
  ;(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId)
  // Toggle sidebar on tap (no drag)
  if (!snapBtnDidDrag) sidebarCollapsed.value = !sidebarCollapsed.value
}
const chatInput = ref('')
const chatMode = ref('task')
const enableThinking = ref(false)
const libraries = ref<Library[]>([])
const selectedLibraryIds = ref<number[]>([])
const renameConversationOpen = ref(false)
const deleteConversationOpen = ref(false)
const actionConversation = ref<Conversation | null>(null)

// Pending images for message
interface PendingImage {
  id: string
  file: File
  mimeType: string
  base64: string
  dataUrl: string
  fileName: string
  size: number
}
const pendingImages = ref<PendingImage[]>([])

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
  const hasContent = chatInput.value.trim() !== '' || pendingImages.value.length > 0
  return (
    !!activeAgentId.value &&
    hasContent &&
    !!selectedModelInfo.value &&
    !isGenerating.value
  )
})

// Reason why send is disabled (for tooltip)
const sendDisabledReason = computed(() => {
  if (isGenerating.value) return ''
  if (!activeAgentId.value) return t('assistant.placeholders.createAgentFirst')
  if (!selectedModelKey.value) return t('assistant.placeholders.selectModelFirst')
  const hasContent = chatInput.value.trim() !== '' || pendingImages.value.length > 0
  if (!hasContent) return t('assistant.placeholders.enterToSend')
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

const openSettings = (agent: Agent, initialTab?: string) => {
  settingsAgent.value = agent
  settingsInitialTab.value = initialTab || ''
  settingsOpen.value = true
}

const handleOpenWorkspaceSettings = () => {
  if (activeAgent.value) {
    openSettings(activeAgent.value, 'workspace')
  }
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
  // Clear selection; only purge cached messages if the conversation is not actively streaming
  // (another tab may still be using it).
  if (activeConversationId.value && !chatStore.isGenerating(activeConversationId.value).value) {
    chatStore.clearMessages(activeConversationId.value)
  }
  activeConversationId.value = null
  chatInput.value = ''
  pendingImages.value = []
  // Clear knowledge base selection for new conversation
  clearKnowledgeSelection()
  // Reset thinking mode to default (off) for new conversation
  enableThinking.value = false
  // Reset chat mode to default (task) for new conversation
  chatMode.value = 'task'
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

const handleSelectConversation = async (conversation: Conversation) => {
  activeConversationId.value = conversation.id
  // Load messages from backend via chatStore
  chatStore.loadMessages(conversation.id)

  // Set model selection from conversation's saved model
  if (conversation.llm_provider_id && conversation.llm_model_id) {
    selectedModelKey.value = `${conversation.llm_provider_id}::${conversation.llm_model_id}`
  }

  // Set knowledge base selection from conversation
  selectedLibraryIds.value = conversation.library_ids || []

  // Set thinking mode from conversation (skip toast notification)
  isRestoringConversation = true
  enableThinking.value = conversation.enable_thinking || false
  await nextTick()
  isRestoringConversation = false

  // Set chat mode from conversation
  chatMode.value = conversation.chat_mode || 'task'
}

const handleSend = async () => {
  if (!canSend.value || !activeAgentId.value) return

  const messageContent = chatInput.value.trim()
  const imagesToSend = [...pendingImages.value]
  chatInput.value = ''
  pendingImages.value = []

  // If no active conversation, create one first
  if (!activeConversationId.value) {
    try {
      // Get current model selection
      const [providerId, modelId] = selectedModelKey.value.split('::')

      await createConversation(
        new CreateConversationInput({
          agent_id: activeAgentId.value,
          name: messageContent.slice(0, 50) || (imagesToSend.length > 0 ? t('assistant.imageMessage') : ''), // First message becomes conversation name (truncated)
          last_message: messageContent || (imagesToSend.length > 0 ? t('assistant.imageMessage') : ''),
          llm_provider_id: providerId || '',
          llm_model_id: modelId || '',
          library_ids: selectedLibraryIds.value,
          enable_thinking: enableThinking.value,
          chat_mode: chatMode.value,
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

      await chatStore.sendMessage(activeConversationId.value, messageContent, props.tabId, imagesToSend)

      // Update conversation's last_message
      try {
        const updated2 = await ConversationsService.UpdateConversation(
          activeConversationId.value,
          new UpdateConversationInput({
            last_message: messageContent || (imagesToSend.length > 0 ? t('assistant.imageMessage') : ''),
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

// Remove a single library from selection
const handleRemoveLibrary = async (id: number) => {
  selectedLibraryIds.value = selectedLibraryIds.value.filter((lid) => lid !== id)
  await saveLibraryIdsToConversation()
}

// Handle image selection
const handleAddImages = async (files: FileList | File[]) => {
  // Check if current model supports multimodal (vision)
  const modelInfo = selectedModelInfo.value
  if (modelInfo && !supportsMultimodal(modelInfo.providerId, modelInfo.modelId, modelInfo.capabilities)) {
    toast.error(t('assistant.errors.modelNotSupportVision'))
    return
  }

  const fileArray = Array.from(files)
  const MAX_IMAGES = 4
  const currentCount = pendingImages.value.length

  if (currentCount + fileArray.length > MAX_IMAGES) {
    toast.error(t('assistant.errors.tooManyImages', { max: MAX_IMAGES }))
    return
  }

  for (const file of fileArray) {
    try {
      const base64 = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader()
        reader.onload = () => {
          const dataUrl = reader.result as string
          // Extract base64 without data: prefix
          const base64Match = dataUrl.match(/^data:image\/[^;]+;base64,(.+)$/)
          if (base64Match) {
            resolve(base64Match[1])
          } else {
            reject(new Error('Invalid image data'))
          }
        }
        reader.onerror = reject
        reader.readAsDataURL(file)
      })

      const dataUrl = `data:${file.type};base64,${base64}`
      pendingImages.value.push({
        id: `${Date.now()}-${Math.random()}`,
        file,
        mimeType: file.type,
        base64,
        dataUrl,
        fileName: file.name,
        size: file.size,
      })
    } catch (error) {
      console.error('Failed to read image:', error)
      toast.error(t('assistant.errors.imageReadFailed'))
    }
  }
}

const handleRemoveImage = (id: string) => {
  pendingImages.value = pendingImages.value.filter((img) => img.id !== id)
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

// Track if thinking mode watch should show toast (skip initial mount and conversation restoration)
let isInitialMount = true
let isRestoringConversation = false

// Watch thinking mode changes and save to conversation
watch(enableThinking, (newValue) => {
  void saveThinkingToConversation()
  // Show toast notification when thinking mode changes (skip initial mount and conversation restoration)
  if (!isInitialMount && !isRestoringConversation) {
    toast.default(newValue ? t('assistant.chat.thinkingOn') : t('assistant.chat.thinkingOff'))
  }
})

// Save chat mode to current conversation
const saveChatModeToConversation = async () => {
  if (!activeConversationId.value) return
  try {
    await ConversationsService.UpdateConversation(
      activeConversationId.value,
      new UpdateConversationInput({
        chat_mode: chatMode.value,
      })
    )
  } catch (err) {
    console.error('Failed to save chat mode to conversation:', err)
  }
}

// Watch chat mode changes and save to conversation
watch(chatMode, () => {
  void saveChatModeToConversation()
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
watch(activeAgentId, async (newAgentId, oldAgentId) => {
  selectDefaultModel(activeAgent.value, activeConversation.value)
  updateCurrentTab()
  if (newAgentId && oldAgentId !== undefined) {
    const isRealSwitch = oldAgentId !== null && oldAgentId !== newAgentId
    await loadConversations(newAgentId, {
      preserveSelection: !isRealSwitch,
      activeAgentId: newAgentId,
    })
    if (isRealSwitch && agents.value.length > 1) {
      const conversations = getAllAgentConversations(newAgentId)
      if (conversations.length > 0) {
        handleSelectConversation(conversations[0])
      }
    }
  } else if (!newAgentId) {
    if (activeConversationId.value) {
      chatStore.clearMessages(activeConversationId.value)
    }
    activeConversationId.value = null
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
      const agentIdBefore = activeAgentId.value
      await loadAgents()
      await loadLibrariesFn()

      // Only refresh conversations here if loadAgents didn't change the active agent.
      // If it did change, watch(activeAgentId) already handled the conversation reload.
      if (activeAgentId.value != null && activeAgentId.value === agentIdBefore) {
        await loadConversations(activeAgentId.value, {
          preserveSelection: true,
          force: true,
          activeAgentId: activeAgentId.value,
        })
        await syncLibraryIdsFromConversation()
      }
    })()
  }
})

// Listen for text selection events
let unsubscribeTextSelection: (() => void) | null = null
let unsubscribeConversationsChanged: (() => void) | null = null
let unsubscribeAgentsChanged: (() => void) | null = null
let unsubscribeModelsChanged: (() => void) | null = null
// Snap mode event listeners
let unsubscribeSnapSettings: (() => void) | null = null
let unsubscribeSnapStateChanged: (() => void) | null = null
let unsubscribeTextSelectionSnap: (() => void) | null = null

const wakeAttachedSkipSelector =
  '[data-snap-block-focus], [data-snap-drag-zone], [data-snap-no-wake], [data-snap-action], [data-radix-popper-content-wrapper], [data-radix-select-viewport], [data-radix-menu-content], [role="listbox"], [role="dialog"]'

const shouldSkipWakeAttached = (e: globalThis.PointerEvent): boolean => {
  if (e.button !== 0) return true
  if (!hasAttachedTarget.value) return true
  const target = e.target instanceof Element ? e.target : null
  if (!target) return true
  return !!target.closest(wakeAttachedSkipSelector)
}

const handleWakeAttachedPointerDown = (e: globalThis.PointerEvent) => {
  if (shouldSkipWakeAttached(e)) return
  void SnapService.WakeAttached()
}

onMounted(() => {
  // In snap mode, default sidebar to collapsed
  if (isSnapMode.value) {
    sidebarCollapsed.value = true
  }

  void (async () => {
    await loadAgents()
    await loadModels()
    await loadLibrariesFn()

    // Check for pending chat data (e.g. from knowledge page shortcut)
    const pendingData = navigationStore.consumePendingChatData(props.tabId)
    
    if (activeAgentId.value != null) {
      // Preserve current selection to avoid wiping the first message on cold start.
      await loadConversations(activeAgentId.value, {
        preserveSelection: true,
        force: true,
        activeAgentId: activeAgentId.value,
      })
    }

    if (pendingData) {
      // Apply pre-selected agent
      if (pendingData.agentId && agents.value.some((a) => a.id === pendingData.agentId)) {
        activeAgentId.value = pendingData.agentId
        // Load conversations for the selected agent
        await loadConversations(pendingData.agentId, {
          preserveSelection: false,
          force: true,
          activeAgentId: pendingData.agentId,
        })
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
      // Apply chat mode
      if (pendingData.chatMode) {
        chatMode.value = pendingData.chatMode
      }
      // Apply chat input
      if (pendingData.chatInput) {
        chatInput.value = pendingData.chatInput
      }
      // Apply pending images (convert from serializable format to PendingImage)
      if (pendingData.pendingImages && pendingData.pendingImages.length > 0) {
        const converted: PendingImage[] = []
        for (const img of pendingData.pendingImages as PendingChatImage[]) {
          try {
            const blob = await (await fetch(img.dataUrl)).blob()
            const file = new File([blob], img.fileName, { type: img.mimeType })
            converted.push({
              id: img.id,
              file,
              mimeType: img.mimeType,
              base64: img.base64,
              dataUrl: img.dataUrl,
              fileName: img.fileName,
              size: img.size,
            })
          } catch (e) {
            console.warn('Failed to convert pending image:', img.fileName, e)
          }
        }
        pendingImages.value = converted
      }
      // Ensure we start with a new conversation
      activeConversationId.value = null

      // Auto-send after a short delay to let Vue reactivity settle (text or images)
      const hasContent = (pendingData.chatInput?.trim() ?? '') !== '' || (pendingData.pendingImages?.length ?? 0) > 0
      if (hasContent) {
        window.setTimeout(() => {
          if (canSend.value) {
            handleSend()
          }
        }, 200)
      }
    } else {
      // New tab starts with a fresh conversation (no auto-select).
      // The user can pick an existing conversation from the sidebar.
    }

    // Snap mode initialization
    if (isSnapMode.value) {
      await loadSnapSettings()
      await checkSnapStatus()
    }

    // Mark initial mount as complete (enable toast notifications for thinking mode changes)
    isInitialMount = false
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

    // WakeAttached is now explicitly bound only on data-snap-wake regions
    // (message area / input area), avoiding global pointer capture side effects.
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

  // Listen for model/provider changes from settings page (e.g., add/delete model, enable/disable provider)
  unsubscribeModelsChanged = Events.On('models:changed', () => {
    void loadModels()
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
  unsubscribeModelsChanged?.()
  unsubscribeModelsChanged = null

  // Snap mode cleanup
  unsubscribeSnapSettings?.()
  unsubscribeSnapSettings = null
  unsubscribeSnapStateChanged?.()
  unsubscribeSnapStateChanged = null
  unsubscribeTextSelectionSnap?.()
  unsubscribeTextSelectionSnap = null
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
      @close-window="closeSnapWindow"
    />

    <!-- Main content wrapper (snap mode: vertical layout so input spans full width) -->
    <div :class="cn('relative flex min-h-0 flex-1 overflow-hidden', isSnapMode && 'flex-col')">

    <!-- Overlay backdrop for snap mode sidebar (covers entire area including input) -->
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
      :on-wake-attached="handleWakeAttachedPointerDown"
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

    <!-- Upper row: expand button + messages -->
    <div class="relative flex min-h-0 flex-1 overflow-hidden">

    <!-- Collapse/Expand button (hidden when empty; snap mode: floating, draggable) -->
    <div
      v-if="!isAgentEmpty && isSnapMode"
      class="absolute left-0.5 z-[5] cursor-grab active:cursor-grabbing"
      :style="{ top: snapBtnTop + 'px' }"
      @pointerdown="onSnapBtnPointerDown"
      @pointermove="onSnapBtnPointerMove"
      @pointerup="onSnapBtnPointerUp"
    >
      <Button
        size="icon"
        variant="ghost"
        class="size-7 pointer-events-none"
        :title="sidebarCollapsed ? t('assistant.sidebar.expand') : t('assistant.sidebar.collapse')"
      >
        <IconSidebarExpand v-if="sidebarCollapsed" class="size-5 text-muted-foreground" />
        <IconSidebarCollapse v-else class="size-5 text-muted-foreground" />
      </Button>
    </div>
    <!-- Collapse/Expand button (non-snap mode: in-flow) -->
    <div
      v-if="!isAgentEmpty && !isSnapMode"
      class="flex w-8 shrink-0 items-center justify-center"
    >
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
      <!-- Top toolbar: workspace drawer toggle (task mode + active conversation only) -->
      <div
        v-if="!isAgentEmpty && !isSnapMode && activeConversationId && chatMode === 'task'"
        class="flex shrink-0 items-center justify-end px-2 pt-1"
      >
        <Button
          size="icon"
          variant="ghost"
          class="size-7"
          :title="t('assistant.workspaceDrawer.title')"
          @click="workspaceDrawerOpen = !workspaceDrawerOpen"
        >
          <PanelRight :class="cn('size-4', workspaceDrawerOpen ? 'text-foreground' : 'text-muted-foreground')" />
        </Button>
      </div>

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
        data-snap-wake="true"
        :conversation-id="activeConversationId"
        :tab-id="tabId"
        :mode="props.mode"
        :agent-name="activeAgent?.name"
        :agent-icon="activeAgent?.icon"
        :sandbox-mode="activeAgent?.sandbox_mode"
        :has-attached-target="hasAttachedTarget"
        :show-ai-send-button="showAiSendButton"
        :show-ai-edit-button="showAiEditButton"
        class="min-w-0 flex-1 overflow-hidden"
        @pointerdown.capture="handleWakeAttachedPointerDown"
        @edit-message="handleEditMessage"
        @snap-send-and-trigger="handleSendAndTrigger"
        @snap-send-to-edit="handleSendToEdit"
        @snap-copy="handleCopyToClipboard"
      />

      <!-- Input area: non-snap mode OR snap mode with no messages (centered empty state) -->
      <ChatInputArea
        v-if="!isAgentEmpty && (!isSnapMode || (chatMessages.length === 0 && !isGenerating))"
        data-snap-wake="true"
        :chat-input="chatInput"
        :chat-mode="chatMode"
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
        :active-agent="activeAgent"
        :agents="agents"
        :is-snap-mode="isSnapMode"
        :pending-images="pendingImages"
        @pointerdown.capture="handleWakeAttachedPointerDown"
        @update:chat-input="chatInput = $event"
        @update:chat-mode="chatMode = $event"
        @update:selected-model-key="selectedModelKey = $event"
        @update:enable-thinking="enableThinking = $event"
        @update:selected-library-ids="selectedLibraryIds = $event"
        @send="handleSend"
        @stop="handleStop"
        @library-selection-change="handleLibrarySelectionChange"
        @clear-library-selection="clearLibrarySelection"
        @load-libraries="loadLibrariesFn"
        @remove-library="handleRemoveLibrary"
        @add-images="handleAddImages"
        @remove-image="handleRemoveImage"
        @clear-images="pendingImages = []"
      />
    </section>

    <!-- Workspace drawer panel (task mode only) -->
    <WorkspaceDrawer
      v-if="!isSnapMode && activeConversationId && chatMode === 'task'"
      :open="workspaceDrawerOpen"
      :agent="activeAgent"
      :conversation-id="activeConversationId"
      @update:open="workspaceDrawerOpen = $event"
      @open-workspace-settings="handleOpenWorkspaceSettings"
    />
    </div><!-- End upper row -->

    <!-- Input area (snap mode with messages: full-width at bottom) -->
    <ChatInputArea
      v-if="!isAgentEmpty && isSnapMode && (chatMessages.length > 0 || isGenerating)"
      data-snap-wake="true"
      :chat-input="chatInput"
      :chat-mode="chatMode"
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
      :active-agent="activeAgent"
      :agents="agents"
      :is-snap-mode="isSnapMode"
      :pending-images="pendingImages"
      @pointerdown.capture="handleWakeAttachedPointerDown"
      @update:chat-input="chatInput = $event"
      @update:chat-mode="chatMode = $event"
      @update:selected-model-key="selectedModelKey = $event"
      @update:enable-thinking="enableThinking = $event"
      @update:selected-library-ids="selectedLibraryIds = $event"
      @send="handleSend"
      @stop="handleStop"
      @library-selection-change="handleLibrarySelectionChange"
      @clear-library-selection="clearLibrarySelection"
      @load-libraries="loadLibrariesFn"
      @remove-library="handleRemoveLibrary"
      @add-images="handleAddImages"
      @remove-image="handleRemoveImage"
      @clear-images="pendingImages = []"
    />
    </div><!-- End main content wrapper -->

    <!-- Dialogs (rendered outside main content wrapper for proper z-index) -->
    <CreateAgentDialog v-model:open="createOpen" :loading="loading" @create="handleCreate" />
    <AgentSettingsDialog
      v-model:open="settingsOpen"
      :agent="settingsAgent"
      :initial-tab="settingsInitialTab"
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
            class="bg-foreground text-background hover:bg-foreground/90"
            @click.prevent="confirmDeleteConversation"
          >
            {{ t('assistant.conversation.delete.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
