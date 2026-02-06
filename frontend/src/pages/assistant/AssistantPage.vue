<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowUp, Check, Copy, MoreHorizontal, Pin, PinOff, Plus, Lightbulb, SendHorizontal, Square, Type } from 'lucide-vue-next'
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
import { SnapService } from '@bindings/willchat/internal/services/windows'
import { SettingsService, Category } from '@bindings/willchat/internal/services/settings'
import { TextSelectionService } from '@bindings/willchat/internal/services/textselection'
import { Clipboard } from '@wailsio/runtime'
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

const listMode = ref<ListMode>('personal')

const agents = ref<Agent[]>([])
const activeAgentId = ref<number | null>(null)

const createOpen = ref(false)
const settingsOpen = ref(false)
const settingsAgent = ref<Agent | null>(null)
const loading = ref(false)

// Sidebar collapse state - default collapsed in snap mode
const sidebarCollapsed = ref(false)

// ==================== Snap mode specific state ====================
// Track if there's an attached target (for snap mode)
const hasAttachedTarget = ref(false)

// Settings for button visibility (snap mode)
const showAiSendButton = ref(true)
const showAiEditButton = ref(true)

// Map from target process name to settings key
const processToSettingsKey: Record<string, string> = {
  // Windows
  'Weixin.exe': 'snap_wechat',
  'WeChat.exe': 'snap_wechat',
  'WeChatApp.exe': 'snap_wechat',
  'WeChatAppEx.exe': 'snap_wechat',
  'WXWork.exe': 'snap_wecom',
  'QQ.exe': 'snap_qq',
  'QQNT.exe': 'snap_qq',
  'DingTalk.exe': 'snap_dingtalk',
  'Feishu.exe': 'snap_feishu',
  'Lark.exe': 'snap_feishu',
  'Douyin.exe': 'snap_douyin',
  // macOS
  '微信': 'snap_wechat',
  'Weixin': 'snap_wechat',
  'weixin': 'snap_wechat',
  'WeChat': 'snap_wechat',
  'wechat': 'snap_wechat',
  'com.tencent.xinWeChat': 'snap_wechat',
  '企业微信': 'snap_wecom',
  'WeCom': 'snap_wecom',
  'wecom': 'snap_wecom',
  'WeWork': 'snap_wecom',
  'wework': 'snap_wecom',
  'WXWork': 'snap_wecom',
  'wxwork': 'snap_wecom',
  'qiyeweixin': 'snap_wecom',
  'com.tencent.WeWorkMac': 'snap_wecom',
  'QQ': 'snap_qq',
  'qq': 'snap_qq',
  'com.tencent.qq': 'snap_qq',
  '钉钉': 'snap_dingtalk',
  'DingTalk': 'snap_dingtalk',
  'dingtalk': 'snap_dingtalk',
  'com.alibaba.DingTalkMac': 'snap_dingtalk',
  '飞书': 'snap_feishu',
  'Feishu': 'snap_feishu',
  'feishu': 'snap_feishu',
  'Lark': 'snap_feishu',
  'lark': 'snap_feishu',
  'com.bytedance.feishu': 'snap_feishu',
  'com.bytedance.Lark': 'snap_feishu',
  '抖音': 'snap_douyin',
  'Douyin': 'snap_douyin',
  'douyin': 'snap_douyin',
}

// Check snap status and update hasAttachedTarget
const checkSnapStatus = async () => {
  if (!isSnapMode.value) return
  try {
    const status = await SnapService.GetStatus()
    hasAttachedTarget.value = status.state === 'attached' && !!status.targetProcess
  } catch (error) {
    console.error('Failed to check snap status:', error)
    hasAttachedTarget.value = false
  }
}

// Load snap settings
const loadSnapSettings = async () => {
  if (!isSnapMode.value) return
  try {
    const settings = await SettingsService.List(Category.CategorySnap)
    settings.forEach((setting) => {
      if (setting.key === 'show_ai_send_button') {
        showAiSendButton.value = setting.value === 'true'
      }
      if (setting.key === 'show_ai_edit_button') {
        showAiEditButton.value = setting.value === 'true'
      }
    })
  } catch (error) {
    console.error('Failed to load snap settings:', error)
  }
}

// Cancel snap (disable the attached app's toggle)
const cancelSnap = async () => {
  try {
    const status = await SnapService.GetStatus()
    if (status.state === 'attached' && status.targetProcess) {
      const settingsKey = processToSettingsKey[status.targetProcess]
      if (settingsKey) {
        await SettingsService.SetValue(settingsKey, 'false')
        await SnapService.SyncFromSettings()
        hasAttachedTarget.value = false
      }
    }
  } catch (error) {
    console.error('Failed to cancel snap:', error)
  }
}

// Action handlers for AI response buttons (snap mode)
const handleSendAndTrigger = async (content: string) => {
  if (!content) return
  try {
    await SnapService.SendTextToTarget(content, true)
    toast.success(t('winsnap.toast.sent'))
  } catch (error) {
    console.error('Failed to send and trigger:', error)
    await checkSnapStatus()
    toast.error(hasAttachedTarget.value ? t('winsnap.toast.sendFailed') : t('winsnap.toast.noTarget'))
  }
}

const handleSendToEdit = async (content: string) => {
  if (!content) return
  try {
    await SnapService.PasteTextToTarget(content)
    toast.success(t('winsnap.toast.pasted'))
  } catch (error) {
    console.error('Failed to paste to edit:', error)
    await checkSnapStatus()
    toast.error(hasAttachedTarget.value ? t('winsnap.toast.pasteFailed') : t('winsnap.toast.noTarget'))
  }
}

const handleCopyToClipboard = async (content: string) => {
  if (!content) return
  try {
    await Clipboard.SetText(content)
    toast.success(t('winsnap.toast.copied'))
  } catch (error) {
    console.error('Failed to copy to clipboard:', error)
  }
}
// ==================== End snap mode specific state ====================

// Chat input
const chatInput = ref('')

// Model selection
const providersWithModels = ref<ProviderWithModels[]>([])
const selectedModelKey = ref('')

// Thinking mode
const enableThinking = ref(false)

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

const activeConversation = computed<Conversation | null>(() => {
  if (!activeConversationId.value || !activeAgentId.value) return null
  const list = conversationsByAgent.value[activeAgentId.value] ?? []
  return list.find((c) => c.id === activeConversationId.value) ?? null
})

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

  // Prefer the active conversation's model if available.
  {
    const conv = activeConversation.value
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

const parseSelectedModelKey = (key: string): { providerId: string; modelId: string } | null => {
  if (!key) return null
  const [providerId, modelId] = key.split('::')
  if (!providerId || !modelId) return null
  return { providerId, modelId }
}

const saveModelToConversationIfNeeded = async (opts?: { silent?: boolean }) => {
  const silent = opts?.silent ?? true
  if (!activeConversationId.value) return

  const parsed = parseSelectedModelKey(selectedModelKey.value)
  if (!parsed) return

  // Avoid redundant updates when switching conversations (we already read from DB into selectedModelKey).
  const current = activeConversation.value
  if (
    current &&
    current.llm_provider_id === parsed.providerId &&
    current.llm_model_id === parsed.modelId
  ) {
    return
  }

  try {
    const updated = await ConversationsService.UpdateConversation(
      activeConversationId.value,
      new UpdateConversationInput({
        llm_provider_id: parsed.providerId,
        llm_model_id: parsed.modelId,
      })
    )
    if (updated) {
      handleConversationUpdated(updated)
    }
  } catch (error: unknown) {
    // Non-critical: if this fails, backend will continue using the previously saved model.
    if (!silent) {
      toast.error(getErrorMessage(error) || t('assistant.errors.updateConversationFailed'))
    } else {
      console.warn('Failed to save model to conversation:', error)
    }
  }
}

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

    // Notify other windows/tabs to refresh agents list
    Events.Emit('agents:changed', {
      sourceTabId: props.tabId,
      action: 'created',
    })
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

  // Notify other windows/tabs to refresh agents list
  Events.Emit('agents:changed', {
    sourceTabId: props.tabId,
    action: 'updated',
  })
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
          enable_thinking: enableThinking.value,
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
      // Ensure conversation model is synced before sending to backend.
      // Backend send API only takes conversation_id, so it relies on the saved model in conversation record.
      await saveModelToConversationIfNeeded({ silent: true })
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

  // Set thinking mode from conversation
  enableThinking.value = conversation.enable_thinking || false
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

function handleConversationUpdated(updated: Conversation) {
  const agentId = updated.agent_id
  const current = conversationsByAgent.value[agentId] ?? []
  const exists = current.some((c) => c.id === updated.id)
  // Update and re-sort (pinned first, then by updated_at desc)
  const next = (exists ? current.map((c) => (c.id === updated.id ? updated : c)) : [updated, ...current])
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

// Persist model switching to current conversation so backend uses the new model.
watch(selectedModelKey, () => {
  // Fire-and-forget; handleSend will also await a sync to avoid race conditions.
  void saveModelToConversationIfNeeded({ silent: true })
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
let unsubscribeAgentsChanged: (() => void) | null = null
// Snap mode event listeners
let unsubscribeSnapSettings: (() => void) | null = null
let unsubscribeSnapStateChanged: (() => void) | null = null
let unsubscribeTextSelectionSnap: (() => void) | null = null
let onPointerDown: ((e: PointerEvent) => void) | null = null

onMounted(() => {
  // In snap mode, default sidebar to collapsed
  if (isSnapMode.value) {
    sidebarCollapsed.value = true
  }

  void (async () => {
    await loadAgents()
    await loadModels()
    await loadLibraries()

    if (activeAgentId.value != null) {
      await loadConversations(activeAgentId.value, { force: true })
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
    onPointerDown = (e: PointerEvent) => {
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
      void loadConversations(agentId, { preserveSelection: true, force: true })
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
    <div
      v-if="isSnapMode"
      class="flex h-10 shrink-0 items-center justify-between border-b border-border bg-background px-3"
      style="--wails-draggable: drag"
    >
      <!-- Left: Agent selector -->
      <div class="flex items-center gap-1" style="--wails-draggable: no-drag">
        <Select
          :model-value="activeAgentId?.toString() ?? ''"
          @update:model-value="(v) => { if (v) { activeAgentId = Number(v); handleNewConversation(); } }"
        >
          <SelectTrigger class="h-7 w-auto min-w-[120px] max-w-[180px] border-0 bg-transparent px-2 text-sm font-medium shadow-none hover:bg-muted/50">
            <div v-if="activeAgent" class="flex items-center gap-1.5">
              <img v-if="activeAgent.icon" :src="activeAgent.icon" class="size-4 rounded object-contain" />
              <LogoIcon v-else class="size-4" />
              <span class="truncate">{{ activeAgent.name }}</span>
            </div>
            <span v-else class="text-muted-foreground">{{ t('assistant.placeholders.noAgentSelected') }}</span>
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem v-for="a in agents" :key="a.id" :value="a.id.toString()">
                <div class="flex items-center gap-2">
                  <img v-if="a.icon" :src="a.icon" class="size-4 rounded object-contain" />
                  <LogoIcon v-else class="size-4" />
                  <span>{{ a.name }}</span>
                </div>
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>

      <!-- Right: New conversation + Cancel snap -->
      <div class="flex items-center gap-1.5" style="--wails-draggable: no-drag">
        <button
          class="rounded-md p-1 hover:bg-muted"
          :title="t('assistant.sidebar.newConversation')"
          type="button"
          @click="handleNewConversation"
        >
          <Plus class="size-4 text-muted-foreground" />
        </button>
        <button
          class="ml-1 inline-flex items-center gap-1.5 rounded-md bg-muted/50 px-2 py-1 text-xs hover:bg-muted"
          type="button"
          @click="cancelSnap"
        >
          <PinOff class="size-3.5 text-muted-foreground" />
          <span class="text-muted-foreground">{{ t('winsnap.cancelSnap') }}</span>
        </button>
      </div>
    </div>

    <!-- Main content wrapper -->
    <div class="relative flex min-h-0 flex-1 overflow-hidden">
    <!-- Overlay backdrop for snap mode sidebar -->
    <div
      v-if="isSnapMode && !sidebarCollapsed"
      class="absolute inset-0 z-10 bg-black/20"
      @click="sidebarCollapsed = true"
    />

    <!-- Left side: Agent list (collapsible, overlay in snap mode when expanded) -->
    <aside
      v-if="!isSnapMode || !sidebarCollapsed"
      :class="
        cn(
          'flex shrink-0 flex-col border-r border-border bg-background transition-all duration-200',
          sidebarCollapsed ? 'w-0 overflow-hidden' : 'w-sidebar',
          // In snap mode, sidebar is an overlay
          isSnapMode && !sidebarCollapsed && 'absolute inset-y-0 left-0 z-20 shadow-lg'
        )
      "
    >
      <!-- Snap mode: close button at top -->
      <div v-if="isSnapMode && !sidebarCollapsed" class="flex items-center justify-end border-b border-border px-2 py-1.5">
        <Button
          size="icon"
          variant="ghost"
          class="size-6"
          :title="t('assistant.sidebar.collapse')"
          @click="sidebarCollapsed = true"
        >
          <IconSidebarCollapse class="size-4 text-muted-foreground" />
        </Button>
      </div>

      <div class="flex items-center justify-between gap-2 p-3">
        <div class="inline-flex rounded-md bg-muted p-1">
          <button
            :class="
              cn(
                'rounded px-3 py-1 text-sm transition-colors',
                listMode === 'personal'
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              )
            "
            @click="listMode = 'personal'"
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
        :mode="props.mode"
        :has-attached-target="hasAttachedTarget"
        :show-ai-send-button="showAiSendButton"
        :show-ai-edit-button="showAiEditButton"
        class="min-w-0 flex-1 overflow-hidden"
        @edit-message="handleEditMessage"
        @snap-send-and-trigger="handleSendAndTrigger"
        @snap-send-to-edit="handleSendToEdit"
        @snap-copy="handleCopyToClipboard"
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

                <!-- Thinking mode toggle -->
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <Button
                        size="icon"
                        variant="ghost"
                        class="size-8 rounded-full border border-border bg-background"
                        :class="
                          enableThinking
                            ? 'border-primary/50 bg-primary/10 hover:bg-primary/10'
                            : 'hover:bg-muted/40'
                        "
                        @click="enableThinking = !enableThinking"
                      >
                        <Lightbulb
                          class="size-4 pointer-events-none"
                          :class="enableThinking ? 'text-primary' : 'text-muted-foreground'"
                        />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>{{ enableThinking ? t('assistant.chat.thinkingOn') : t('assistant.chat.thinkingOff') }}</p>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>

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
                      class="size-8 rounded-full border border-border bg-background"
                      :class="
                        selectedLibraryIds.length > 0
                          ? 'border-primary/50 bg-primary/10 hover:bg-primary/10'
                          : 'hover:bg-muted/40'
                      "
                    >
                      <IconSelectKnowledge
                        class="size-4 pointer-events-none"
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
                      <p>{{ sendDisabledReason || t('assistant.placeholders.enterToSend') }}</p>
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
