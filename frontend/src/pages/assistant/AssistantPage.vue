<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { PanelRight } from 'lucide-vue-next'
import IconAssistant from '@/assets/icons/assistant.svg'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { getLogoDataUrl } from '@/composables/useLogo'
import CreateAgentDialog from './components/CreateAgentDialog.vue'
import AgentSettingsDialog from './components/AgentSettingsDialog.vue'
import AgentChannelsDialog from './components/AgentChannelsDialog.vue'
import RenameConversationDialog from './components/RenameConversationDialog.vue'
import ChatMessageList from './components/ChatMessageList.vue'
import AgentSidebar from './components/AgentSidebar.vue'
import ChatInputArea from './components/ChatInputArea.vue'
import WorkspaceDrawer from './components/WorkspaceDrawer.vue'
import SnapModeHeader from './components/SnapModeHeader.vue'
import { useNavigationStore, useChatStore, useSettingsStore } from '@/stores'
import type { PendingChatImage } from '@/stores/navigation'
import { type Agent } from '@bindings/chatclaw/internal/services/agents'
import type { ImagePayload } from '@bindings/chatclaw/internal/services/chat'
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
  ChatWikiService,
  TeamChatInput,
  type Robot,
} from '@bindings/chatclaw/internal/services/chatwiki'
import { SettingsService } from '@bindings/chatclaw/internal/services/settings'
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
import { useTeamRobots } from './composables/useTeamRobots'
import { supportsMultimodal } from '@/composables/useMultimodal'
import { getBinding as getChatwikiBinding } from '@/lib/chatwikiCache'
import { clearUnavailableChatwikiSelection } from '@/lib/chatwikiModelAvailability'

/**
 * Props - 每个标签页实例都有自己独立的 tabId
 * 通过 v-show 控制显示/隐藏，组件实例不会被销毁，状态自然保留
 * mode: 'main' 主窗体模式（默认），'snap' 吸附窗体模式
 */
const props = withDefaults(
  defineProps<{
    tabId: string
    mode?: 'main' | 'snap' | 'embedded' | 'history-iframe'
    initialConversationId?: number | null
    initialAgentId?: number | null
  }>(),
  {
    mode: 'main',
    initialConversationId: null,
    initialAgentId: null,
  }
)

// Computed for mode checks
const isSnapMode = computed(() => props.mode === 'snap')
const isEmbeddedMode = computed(() => props.mode === 'embedded')
const isHistoryIframeMode = computed(() => props.mode === 'history-iframe')
const isIsolatedIframeMode = computed(() => isEmbeddedMode.value || isHistoryIframeMode.value)
const hideAssistantSidebar = computed(() => isEmbeddedMode.value || isHistoryIframeMode.value)
const showHistoryConversationShell = computed(
  () => isHistoryIframeMode.value && activeDisplayConversationId.value != null
)

type ListMode = 'personal' | 'team'

// Snap mode: cache keys for list mode and assistant selection (restore on next open, default personal)
const SNAP_CACHE_LIST_MODE = 'snap_list_mode'
const SNAP_CACHE_AGENT_ID = 'snap_agent_id'
const SNAP_CACHE_TEAM_ROBOT_ID = 'snap_team_robot_id'

const { t } = useI18n()
const navigationStore = useNavigationStore()
const chatStore = useChatStore()
const settingsStore = useSettingsStore()

// Use composables
const { agents, activeAgentId, loading, loadAgents, createAgent, updateAgent, deleteAgent } =
  useAgents()

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

const {
  teamRobots,
  activeTeamRobotId,
  activeRobot,
  binding,
  teamLoading,
  teamBindingChecked,
  teamBound,
  loadTeamRobots,
  ensureBindingLoaded,
} = useTeamRobots()

function goToChatwikiBindingSettings() {
  settingsStore.setActiveMenu('chatwiki')
  navigationStore.navigateToModule('settings')
}

// Local state
const listMode = ref<ListMode>('personal')
const createOpen = ref(false)
const settingsOpen = ref(false)
const settingsAgent = ref<Agent | null>(null)
const channelsOpen = ref(false)
const channelsAgent = ref<Agent | null>(null)
const settingsInitialTab = ref<string>('')
const sidebarCollapsed = ref(false)
const workspaceDrawerOpen = ref(false)
/** dialogue_id from SSE per team conversation id, for next request */
const teamDialogueIdByConversation = ref<Record<number, string>>({})
let teamAssistantMessageCounter = -1000000

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
/** ChatWiki team libraries (all types) when bound; used in personal assistant knowledge selector */
const assistantTeamLibraries = ref<{ id: string; name: string }[]>([])
/** Selected team library ids for recall (comma-separated in conversation.team_library_id; distinct from personal library_ids) */
const assistantSelectedTeamLibraryIds = ref<string[]>([])
const selectedLibraryIds = ref<number[]>([])
/** When opening from knowledge team tab: team library id(s) for recall, comma-separated (consumed on first conversation create) */
const pendingTeamLibraryId = ref<string | null>(null)

function parseTeamLibraryIds(s: string | null | undefined): string[] {
  if (!s || !String(s).trim()) return []
  return String(s)
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean)
}
function teamLibraryIdsToString(ids: string[]): string {
  return [...new Set(ids.map((x) => String(x).trim()).filter(Boolean))].join(',')
}
const renameConversationOpen = ref(false)
const deleteConversationOpen = ref(false)
const actionConversation = ref<Conversation | null>(null)
const isTeamMode = computed(() => listMode.value === 'team')
const activeTeamRobot = computed(() => activeRobot.value)
const activeTeamConversationId = ref<number | null>(null)

const getTeamConversationAgentId = (robotId: string) => {
  let hash = 0
  for (let i = 0; i < robotId.length; i += 1) {
    hash = (hash * 31 + robotId.charCodeAt(i)) % 1000000007
  }
  return hash + 1000
}

const getActiveTeamConversations = () => {
  if (!activeTeamRobotId.value) return []
  return getAllAgentConversations(getTeamConversationAgentId(activeTeamRobotId.value))
}

const activeTeamConversation = computed<Conversation | null>(() => {
  if (!activeTeamConversationId.value) return null
  const list = getActiveTeamConversations()
  return list.find((c) => c.id === activeTeamConversationId.value) || null
})

const activeDisplayConversationId = computed(() =>
  isTeamMode.value ? activeTeamConversationId.value : activeConversationId.value
)

const logTeam = (stage: string, payload?: Record<string, any>) => {
  if (payload) {
    console.warn(`[assistant][team] ${stage}`, payload)
    return
  }
  console.warn(`[assistant][team] ${stage}`)
}

const logTeamConversationSnapshot = (
  stage: string,
  robotId: string | null = activeTeamRobotId.value
) => {
  if (!robotId) {
    logTeam(stage, {
      robot_id: robotId,
      team_agent_id: null,
      conversation_count: 0,
      active_team_conversation_id: activeTeamConversationId.value,
    })
    return
  }
  const teamAgentId = getTeamConversationAgentId(robotId)
  const list = getAllAgentConversations(teamAgentId)
  logTeam(stage, {
    robot_id: robotId,
    team_agent_id: teamAgentId,
    conversation_count: list.length,
    conversation_ids: list.map((c) => c.id),
    conversation_names: list.map((c) => c.name),
    active_team_conversation_id: activeTeamConversationId.value,
  })
}

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

// Pending files for message
interface PendingFile {
  id: string
  file: File
  mimeType: string
  base64: string
  fileName: string
  size: number
}
const pendingFiles = ref<PendingFile[]>([])

// Computed
const activeAgent = computed(() => {
  if (activeAgentId.value == null) return null
  return agents.value.find((a) => a.id === activeAgentId.value) ?? null
})

// Whether the agent list is empty (loaded & no agents). In team mode we show sidebar when team has content or loading.
const isAgentEmpty = computed(() => {
  if (listMode.value === 'team') return false
  return !loading.value && agents.value.length === 0
})

// Helper function to clear knowledge base selection (personal + team)
const clearKnowledgeSelection = () => {
  selectedLibraryIds.value = []
  assistantSelectedTeamLibraryIds.value = []
}

// Check if currently generating
const isGenerating = computed(() => {
  if (!activeDisplayConversationId.value) return false
  return chatStore.isGenerating(activeDisplayConversationId.value).value
})

// Get messages for current conversation
const chatMessages = computed(() => {
  if (!activeDisplayConversationId.value) return []
  return chatStore.getMessages(activeDisplayConversationId.value).value
})

const canSend = computed(() => {
  const hasContent =
    chatInput.value.trim() !== '' || pendingImages.value.length > 0 || pendingFiles.value.length > 0
  if (isTeamMode.value) {
    return !!activeTeamRobotId.value && chatInput.value.trim() !== '' && !isGenerating.value
  }
  return !!activeAgentId.value && hasContent && !!selectedModelInfo.value && !isGenerating.value
})

// Reason why send is disabled (for tooltip)
const sendDisabledReason = computed(() => {
  if (isGenerating.value) return ''
  if (isTeamMode.value) {
    if (!activeTeamRobotId.value) return t('assistant.errors.selectTeamRobotFirst')
    if (pendingImages.value.length > 0) return t('assistant.errors.teamImageNotSupported')
    if (!chatInput.value.trim()) return t('assistant.placeholders.enterToSend')
    return ''
  }
  if (!activeAgentId.value) return t('assistant.placeholders.createAgentFirst')
  if (!selectedModelKey.value) return t('assistant.placeholders.selectModelFirst')
  const hasContent =
    chatInput.value.trim() !== '' || pendingImages.value.length > 0 || pendingFiles.value.length > 0
  if (!hasContent) return t('assistant.placeholders.enterToSend')
  return ''
})

// Load knowledge base list (personal) + team list when ChatWiki is bound (getLibraryList without type = full list)
const loadLibrariesFn = async () => {
  try {
    // Binding is normally set only by loadTeamRobots (team tab). Personal tab
    // must load binding first or teamBound stays false and team list never loads.
    await ensureBindingLoaded()
    const list = await LibraryService.ListLibraries()
    libraries.value = list || []
    assistantTeamLibraries.value = []
    if (teamBound.value) {
      try {
        const teamList = await ChatWikiService.GetLibraryListOnlyOpenAll()
        assistantTeamLibraries.value = (teamList || [])
          .map((lib: { id?: string; name?: string }) => ({
            id: String(lib?.id ?? ''),
            name: String(lib?.name ?? ''),
          }))
          .filter((x) => x.id)
      } catch {
        // binding expired or network
      }
    }
  } catch (error: unknown) {
    console.error('Failed to load libraries:', error)
  }
}

const handleCreate = async (data: { name: string; prompt: string; icon: string }) => {
  try {
    await createAgent(data)
    createOpen.value = false
    if (!isIsolatedIframeMode.value) {
      updateCurrentTab()
    }

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

const openChannels = (agent: Agent) => {
  channelsAgent.value = agent
  channelsOpen.value = true
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
    if (!isIsolatedIframeMode.value) {
      updateCurrentTab()
    }
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
  pendingFiles.value = []
  // Clear knowledge base selection for new conversation
  clearKnowledgeSelection()
  // Reset thinking mode to default (off) for new conversation
  enableThinking.value = false
  // Reset chat mode to default (task) for new conversation
  chatMode.value = 'task'
}

const handleListModeChange = (mode: ListMode) => {
  logTeam('switch list mode', { from: listMode.value, to: mode })
  listMode.value = mode
  if (mode === 'team') {
    chatMode.value = 'chat'
    enableThinking.value = false
    pendingImages.value = []
    pendingFiles.value = []
    clearKnowledgeSelection()
    void (async () => {
      await loadTeamRobots()
      logTeamConversationSnapshot('after loadTeamRobots')
    })()
  }
}

// Snap mode: persist list mode and assistant selection to cache
function persistSnapCache() {
  void SettingsService.SetValue(SNAP_CACHE_LIST_MODE, listMode.value).catch(() => {})
  void SettingsService.SetValue(
    SNAP_CACHE_AGENT_ID,
    activeAgentId.value != null ? String(activeAgentId.value) : ''
  ).catch(() => {})
  void SettingsService.SetValue(SNAP_CACHE_TEAM_ROBOT_ID, activeTeamRobotId.value ?? '').catch(
    () => {}
  )
}

// Snap mode: restore list mode and assistant selection from cache (default: personal)
async function restoreSnapCache() {
  try {
    const cachedMode = (await SettingsService.Get(SNAP_CACHE_LIST_MODE))?.value ?? 'personal'
    listMode.value = cachedMode === 'team' ? 'team' : 'personal'
    if (listMode.value === 'team') {
      await loadTeamRobots()
      const cachedRobotId = (await SettingsService.Get(SNAP_CACHE_TEAM_ROBOT_ID))?.value?.trim()
      if (cachedRobotId && teamRobots.value.some((r) => r.id === cachedRobotId)) {
        activeTeamRobotId.value = cachedRobotId
      }
    } else {
      const cachedAgentId = (await SettingsService.Get(SNAP_CACHE_AGENT_ID))?.value?.trim()
      if (cachedAgentId) {
        const id = Number(cachedAgentId)
        if (Number.isFinite(id) && agents.value.some((a) => a.id === id)) {
          activeAgentId.value = id
        }
      }
    }
  } catch {
    listMode.value = 'personal'
  }
}

const handleNewConversationForAgent = (agentId: number) => {
  if (activeAgentId.value !== agentId) {
    activeAgentId.value = agentId
  }
  handleNewConversation()
}

const handleNewConversationForTeamRobot = (robotId: string) => {
  if (activeTeamRobotId.value !== robotId) {
    activeTeamRobotId.value = robotId
  }
  const conversationId = activeTeamConversationId.value
  if (conversationId != null) {
    delete teamDialogueIdByConversation.value[conversationId]
    if (!chatStore.isGenerating(conversationId).value) {
      chatStore.clearMessages(conversationId)
    }
  }
  activeTeamConversationId.value = null
  chatInput.value = ''
  pendingImages.value = []
  pendingFiles.value = []
}

// Snap header: new conversation (after switching agent or team robot in dropdown)
function handleSnapNewConversation() {
  if (listMode.value === 'team' && activeTeamRobotId.value) {
    handleNewConversationForTeamRobot(activeTeamRobotId.value)
  } else {
    handleNewConversation()
  }
}

const handleSelectConversationForAgent = (agentId: number, conversation: Conversation) => {
  if (activeAgentId.value !== agentId) {
    activeAgentId.value = agentId
  }
  handleSelectConversation(conversation)
}

const handleSelectConversationForTeamRobot = (robotId: string, conversation: Conversation) => {
  logTeam('select team conversation', {
    robot_id: robotId,
    conversation_id: conversation.id,
    dialogue_id: conversation.dialogue_id,
    name: conversation.name,
  })
  if (activeTeamRobotId.value !== robotId) {
    activeTeamRobotId.value = robotId
  }
  activeTeamConversationId.value = conversation.id
  chatStore.loadMessages(conversation.id)
  logTeamConversationSnapshot('after select team conversation', robotId)
}

const handleSelectConversation = async (conversation: Conversation) => {
  activeConversationId.value = conversation.id
  // Load messages from backend via chatStore
  chatStore.loadMessages(conversation.id)

  // Set model selection from conversation's saved model
  if (conversation.llm_provider_id && conversation.llm_model_id) {
    const binding = await getChatwikiBinding().catch(() => null)
    const nextKey = clearUnavailableChatwikiSelection(
      `${conversation.llm_provider_id}::${conversation.llm_model_id}`,
      binding
    )
    if (nextKey) {
      selectedModelKey.value = nextKey
    } else {
      selectedModelKey.value = ''
      try {
        const cleared = await ConversationsService.UpdateConversation(
          conversation.id,
          new UpdateConversationInput({
            llm_provider_id: '',
            llm_model_id: '',
          })
        )
        if (cleared) {
          handleConversationUpdated(cleared)
        }
      } catch (error) {
        console.warn('Failed to clear unavailable Chatwiki conversation model:', error)
      }
      selectDefaultModel(activeAgent.value, conversation)
    }
  }

  // Personal + team knowledge can both be selected; recall runs both paths when set
  selectedLibraryIds.value = conversation.library_ids || []
  assistantSelectedTeamLibraryIds.value = parseTeamLibraryIds(conversation.team_library_id)

  // Set thinking mode from conversation (skip toast notification)
  isRestoringConversation = true
  enableThinking.value = conversation.enable_thinking || false
  await nextTick()
  isRestoringConversation = false

  // Set chat mode from conversation
  chatMode.value = conversation.chat_mode || 'task'
}

const getTeamRobotKey = () => {
  const robot = activeTeamRobot.value as any
  return String(robot?.robot_key ?? robot?.robotKey ?? '').trim()
}

const buildTeamHistoryMessages = (conversationId: number) => {
  const history = chatStore.getMessages(conversationId).value
  return history
    .filter((m) => m.role === 'user' || m.role === 'assistant' || m.role === 'system')
    .map((m) => ({
      role: m.role,
      content: String(m.content ?? ''),
    }))
    .filter((m) => m.content.trim() !== '')
}

const sendTeamMessage = async (messageContent: string) => {
  if (!activeTeamRobotId.value) {
    logTeam('send blocked: no team conversation id', {
      active_team_robot_id: activeTeamRobotId.value,
    })
    return
  }
  let conversationId = activeTeamConversationId.value
  if (!conversationId) {
    const teamAgentId = getTeamConversationAgentId(activeTeamRobotId.value)
    logTeam('team conversation missing, creating new one', {
      robot_id: activeTeamRobotId.value,
      team_agent_id: teamAgentId,
      message_len: messageContent.length,
    })
    const created = await ConversationsService.CreateConversation(
      new CreateConversationInput({
        agent_id: teamAgentId,
        name: messageContent.slice(0, 50),
        last_message: messageContent,
        chat_mode: 'chat',
        team_type: 'team',
      })
    )
    if (!created) {
      logTeam('create team conversation returned empty')
      return
    }
    handleConversationUpdated(created)
    conversationId = created.id
    activeTeamConversationId.value = created.id
    logTeam('created team conversation', {
      robot_id: activeTeamRobotId.value,
      team_agent_id: teamAgentId,
      conversation_id: created.id,
      dialogue_id: created.dialogue_id,
      chat_mode: created.chat_mode,
      team_type: created.team_type,
    })
    logTeamConversationSnapshot('after create team conversation')
  }
  const teamAgentId = getTeamConversationAgentId(activeTeamRobotId.value)

  const currentBinding = binding.value
  if (!currentBinding?.server_url || !currentBinding?.token) {
    logTeam('send blocked: missing binding', {
      has_binding: !!currentBinding,
      server_url: currentBinding?.server_url,
      token_length: String(currentBinding?.token ?? '').length,
    })
    toast.error(t('knowledge.team.needsBindingShort'))
    return
  }

  const robotKey = getTeamRobotKey()
  if (!robotKey) {
    logTeam('send blocked: missing robot_key', {
      active_team_robot_id: activeTeamRobotId.value,
      active_robot: activeTeamRobot.value,
    })
    toast.error(t('assistant.errors.teamRobotMissingKey'))
    return
  }

  const history = buildTeamHistoryMessages(conversationId)
  logTeam('send begin', {
    conversation_id: conversationId,
    robot_id: activeTeamRobotId.value,
    robot_key: robotKey,
    message_len: messageContent.length,
    history_count: history.length,
  })
  chatStore.appendLocalMessage(conversationId, 'user', messageContent)

  const localMessageId = teamAssistantMessageCounter--
  logTeam('request via backend proxy', {
    conversation_id: conversationId,
    local_message_id: localMessageId,
    token_length: String(currentBinding.token).length,
  })
  const conversationDialogueId =
    activeTeamConversation.value?.id === conversationId
      ? activeTeamConversation.value.dialogue_id
      : 0
  const dialogueId =
    conversationDialogueId > 0
      ? String(conversationDialogueId)
      : teamDialogueIdByConversation.value[conversationId]
  const useNewDialogue = dialogueId ? 0 : 1
  logTeam('team send params', {
    conversation_id: conversationId,
    dialogue_id: dialogueId,
    use_new_dialogue: useNewDialogue,
  })
  try {
    const result = await ChatWikiService.SendTeamMessageStream(
      new TeamChatInput({
        conversation_id: conversationId,
        team_agent_id: teamAgentId,
        tab_id: props.tabId,
        robot_key: robotKey,
        content: messageContent,
        messages: history,
        use_new_dialogue: useNewDialogue,
        ...(dialogueId ? { dialogue_id: dialogueId } : {}),
      })
    )
    logTeam('backend proxy started', {
      request_id: result?.request_id,
      message_id: result?.message_id,
    })
    try {
      const updated = await ConversationsService.UpdateConversation(
        conversationId,
        new UpdateConversationInput({
          last_message: messageContent,
        })
      )
      if (updated) {
        handleConversationUpdated(updated)
      }
    } catch {
      // Non-critical error for team mode
    }
  } catch (error: unknown) {
    logTeam('backend proxy failed', {
      error: getErrorMessage(error) || String(error),
    })
    throw error
  }
}

const handleSend = async () => {
  if (!canSend.value) {
    logTeam('handleSend blocked by canSend=false', {
      is_team_mode: isTeamMode.value,
      chat_input_len: chatInput.value.trim().length,
      active_team_robot_id: activeTeamRobotId.value,
      active_agent_id: activeAgentId.value,
      is_generating: isGenerating.value,
      reason: sendDisabledReason.value,
    })
    return
  }

  const messageContent = chatInput.value.trim()
  const imagesToSend = [...pendingImages.value]
  const filesToSend = [...pendingFiles.value]
  chatInput.value = ''
  pendingImages.value = []
  pendingFiles.value = []

  if (isTeamMode.value) {
    try {
      await sendTeamMessage(messageContent)
    } catch (error: unknown) {
      toast.error(getErrorMessage(error) || t('assistant.errors.sendFailed'))
    }
    return
  }

  if (!activeAgentId.value) return

  // If no active conversation, create one first
  if (!activeConversationId.value) {
    try {
      // Get current model selection
      const [providerId, modelId] = selectedModelKey.value.split('::')

      await createConversation(
        new CreateConversationInput({
          agent_id: activeAgentId.value,
          name:
            messageContent.slice(0, 50) ||
            (imagesToSend.length > 0
              ? t('assistant.imageMessage')
              : filesToSend.length > 0
                ? t('assistant.chat.fileMessage')
                : ''),
          last_message:
            messageContent ||
            (imagesToSend.length > 0
              ? t('assistant.imageMessage')
              : filesToSend.length > 0
                ? t('assistant.chat.fileMessage')
                : ''),
          llm_provider_id: providerId || '',
          llm_model_id: modelId || '',
          library_ids: selectedLibraryIds.value,
          team_library_id:
            pendingTeamLibraryId.value ??
            teamLibraryIdsToString(assistantSelectedTeamLibraryIds.value),
          enable_thinking: enableThinking.value,
          chat_mode: chatMode.value,
        })
      )
      pendingTeamLibraryId.value = null
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

      await chatStore.sendMessage(
        activeConversationId.value,
        messageContent,
        props.tabId,
        imagesToSend,
        filesToSend
      )

      // Update conversation's last_message
      try {
        const updated2 = await ConversationsService.UpdateConversation(
          activeConversationId.value,
          new UpdateConversationInput({
            last_message:
              messageContent ||
              (imagesToSend.length > 0
                ? t('assistant.imageMessage')
                : filesToSend.length > 0
                  ? t('assistant.chat.fileMessage')
                  : ''),
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
  if (isTeamMode.value) {
    const conversationId = activeTeamConversationId.value
    if (!conversationId) return
    logTeam('stop team request', { conversation_id: conversationId })
    void ChatWikiService.StopTeamMessageStream(conversationId)
    return
  }
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
  if (activeConversationId.value) {
    try {
      await ConversationsService.UpdateConversation(
        activeConversationId.value,
        new UpdateConversationInput({
          library_ids: [] as number[],
          team_library_id: '',
        })
      )
    } catch {
      // non-critical
    }
  }
}

// Remove a single library from selection
const handleRemoveLibrary = async (id: number) => {
  selectedLibraryIds.value = selectedLibraryIds.value.filter((lid) => lid !== id)
  await saveLibraryIdsToConversation()
}

// Handle image selection
const handleAddImages = async (files: FileList | File[]) => {
  if (isTeamMode.value) {
    toast.error(t('assistant.errors.teamImageNotSupported'))
    return
  }
  // 支持图片识别的模型可以通过调用技能去识别图片，所以不再限制模型能力
  // const modelInfo = selectedModelInfo.value
  // if (modelInfo && !supportsMultimodal(modelInfo.providerId, modelInfo.modelId, modelInfo.capabilities)) {
  //   toast.error(t('assistant.errors.modelNotSupportVision'))
  //   return
  // }

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

const ALLOWED_FILE_EXTENSIONS = [
  '.pdf',
  '.doc',
  '.docx',
  '.xls',
  '.xlsx',
  '.ppt',
  '.pptx',
  '.txt',
  '.csv',
  '.md',
  '.json',
  '.xml',
  '.html',
  '.rtf',
  '.log',
]
const MAX_FILE_SIZE = 20 * 1024 * 1024 // 20MB
const MAX_FILES = 4

const handleAddFiles = async (files: FileList | File[]) => {
  if (isTeamMode.value) {
    toast.error(t('assistant.errors.teamImageNotSupported'))
    return
  }

  const fileArray = Array.from(files)
  const currentCount = pendingFiles.value.length

  if (currentCount + fileArray.length > MAX_FILES) {
    toast.error(t('assistant.errors.tooManyFiles', { max: MAX_FILES }))
    return
  }

  for (const file of fileArray) {
    const ext = '.' + file.name.split('.').pop()?.toLowerCase()
    if (!ALLOWED_FILE_EXTENSIONS.includes(ext)) {
      toast.error(t('assistant.errors.invalidFileType'))
      continue
    }
    if (file.size > MAX_FILE_SIZE) {
      toast.error(t('assistant.errors.fileTooLarge', { max: '20MB' }))
      continue
    }

    try {
      const base64 = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader()
        reader.onload = () => {
          const dataUrl = reader.result as string
          const base64Match = dataUrl.match(/^data:[^;]+;base64,(.+)$/)
          if (base64Match) {
            resolve(base64Match[1])
          } else {
            reject(new Error('Invalid file data'))
          }
        }
        reader.onerror = reject
        reader.readAsDataURL(file)
      })

      pendingFiles.value.push({
        id: `${Date.now()}-${Math.random()}`,
        file,
        mimeType: file.type || 'application/octet-stream',
        base64,
        fileName: file.name,
        size: file.size,
      })
    } catch (error) {
      console.error('Failed to read file:', error)
      toast.error(t('assistant.errors.fileReadFailed'))
    }
  }
}

const handleRemoveFile = (id: string) => {
  pendingFiles.value = pendingFiles.value.filter((f) => f.id !== id)
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

// Save library_ids only; keeps team_library_id unchanged so both can coexist
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

// Team knowledge multi-select: persists team_library_id as comma-separated ids; keeps library_ids
const saveTeamLibraryIdsToConversation = async (ids: string[]) => {
  assistantSelectedTeamLibraryIds.value = [
    ...new Set(ids.map((x) => String(x).trim()).filter(Boolean)),
  ]
  const teamStr = teamLibraryIdsToString(assistantSelectedTeamLibraryIds.value)
  if (!activeConversationId.value) return
  try {
    await ConversationsService.UpdateConversation(
      activeConversationId.value,
      new UpdateConversationInput({
        team_library_id: teamStr,
      })
    )
  } catch (error: unknown) {
    console.error('Failed to save team library selection:', error)
  }
}

/** Toggle one team library id in the selection (same interaction pattern as personal multi-select). */
const toggleAssistantTeamLibraryId = async (id: string) => {
  const set = new Set(assistantSelectedTeamLibraryIds.value)
  if (set.has(id)) set.delete(id)
  else set.add(id)
  await saveTeamLibraryIdsToConversation([...set])
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
    assistantSelectedTeamLibraryIds.value = parseTeamLibraryIds(currentConversation.team_library_id)
  }
}

// Handle message editing (resend from that point)
const handleEditMessage = async (messageId: number, newContent: string, images: ImagePayload[]) => {
  if (!activeConversationId.value) return

  try {
    await chatStore.editAndResend(
      activeConversationId.value,
      messageId,
      newContent,
      props.tabId,
      images
    )
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
    await togglePin(conv, conv.agent_id)
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
    if (activeTeamConversationId.value === actionConversation.value.id) {
      activeTeamConversationId.value = null
      delete teamDialogueIdByConversation.value[actionConversation.value.id]
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
  if (isIsolatedIframeMode.value) return
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
  if (!isIsolatedIframeMode.value) {
    updateCurrentTab()
  }
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

// Snap mode: persist list mode and assistant selection when user changes selection
watch(
  () => [listMode.value, activeAgentId.value, activeTeamRobotId.value] as const,
  () => {
    if (isSnapMode.value) persistSnapCache()
  }
)

watch(activeTeamRobotId, (newId, oldId) => {
  logTeam('active team robot changed', {
    from: oldId,
    to: newId,
  })
  if (!newId) {
    activeTeamConversationId.value = null
    return
  }
  const teamAgentId = getTeamConversationAgentId(newId)
  void (async () => {
    logTeam('loading team conversations by robot', {
      robot_id: newId,
      team_agent_id: teamAgentId,
    })
    await loadConversations(teamAgentId, {
      preserveSelection: false,
      affectActiveSelection: false,
      force: true,
    })
    const list = getAllAgentConversations(teamAgentId)
    activeTeamConversationId.value = list.length > 0 ? list[0].id : null
    logTeam('loaded team conversations by robot', {
      robot_id: newId,
      team_agent_id: teamAgentId,
      conversation_count: list.length,
      picked_conversation_id: activeTeamConversationId.value,
    })
    if (activeTeamConversationId.value) {
      chatStore.loadMessages(activeTeamConversationId.value)
    }
    logTeamConversationSnapshot('after activeTeamRobotId watcher load', newId)
  })()
})

// 当前标签页是否激活
// For snap/embedded mode, always consider it active since it does not follow main tab visibility.
const isTabActive = computed(
  () => isSnapMode.value || isIsolatedIframeMode.value || navigationStore.activeTabId === props.tabId
)

async function initializeEmbeddedConversation() {
  if (!props.initialConversationId) return

  try {
    const conversation = await ConversationsService.GetConversation(props.initialConversationId)
    if (!conversation) return

    const nextAgentId = props.initialAgentId || conversation.agent_id
    activeAgentId.value = nextAgentId

    await loadConversations(nextAgentId, {
      preserveSelection: true,
      force: true,
      activeAgentId: nextAgentId,
    })

    handleSelectConversation(conversation)
  } catch (error) {
    console.error('Failed to initialize embedded conversation:', error)
    activeConversationId.value = null
  }
}

// 监听标签页激活状态，激活时刷新模型/助手列表
watch(isTabActive, (active) => {
  if (active) {
    void (async () => {
      await loadModels()
      const agentIdBefore = activeAgentId.value
      await loadAgents()
      await loadLibrariesFn()

      // When on team tab and unbound, re-check binding so we refresh after user binds in settings
      if (listMode.value === 'team' && !teamBound.value) {
        await loadTeamRobots()
      }

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
let unsubscribeMessagesChanged: (() => void) | null = null
// Snap mode event listeners
let unsubscribeSnapSettings: (() => void) | null = null
let unsubscribeSnapStateChanged: (() => void) | null = null
let unsubscribeTextSelectionSnap: (() => void) | null = null
let unsubscribeChatCompleteTeamDialogue: (() => void) | null = null

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
  if (isSnapMode.value || isEmbeddedMode.value) {
    sidebarCollapsed.value = true
  }

  void (async () => {
    await loadAgents()
    await loadModels()
    await loadLibrariesFn()

    if (isIsolatedIframeMode.value) {
      await initializeEmbeddedConversation()
    } else {
      // Snap mode: restore list mode and assistant selection from cache (default: personal)
      if (isSnapMode.value) {
        await restoreSnapCache()
      }
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
        if (pendingData.teamLibraryId) {
          pendingTeamLibraryId.value = pendingData.teamLibraryId
          assistantSelectedTeamLibraryIds.value = parseTeamLibraryIds(pendingData.teamLibraryId)
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
        const hasContent =
          (pendingData.chatInput?.trim() ?? '') !== '' ||
          (pendingData.pendingImages?.length ?? 0) > 0
        if (hasContent) {
          window.setTimeout(() => {
            if (canSend.value) {
              handleSend()
            }
          }, 200)
        }
      }
      if (!pendingData) {
        // New tab starts with a fresh conversation (no auto-select).
        // The user can pick an existing conversation from the sidebar.
      }
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
  if (!isSnapMode.value && !isIsolatedIframeMode.value) {
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
      const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
      const state = payload?.state
      const targetProcess = payload?.targetProcess
      hasAttachedTarget.value = state === 'attached' && !!targetProcess
    })

    // Listen for text selection to send to snap
    unsubscribeTextSelectionSnap = Events.On('text-selection:send-to-snap', (event: any) => {
      const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
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

    const activeTeamAgentId = activeTeamRobotId.value
      ? getTeamConversationAgentId(activeTeamRobotId.value)
      : null

    // If this tab is active and currently viewing the same agent/team robot, refresh immediately.
    if (
      isTabActive.value &&
      (activeAgentId.value === agentId ||
        (listMode.value === 'team' && activeTeamAgentId === agentId))
    ) {
      logTeam('received conversations:changed for active team scope', {
        agent_id: agentId,
        active_team_agent_id: activeTeamAgentId,
        list_mode: listMode.value,
      })
      void loadConversations(agentId, {
        preserveSelection: true,
        force: true,
        activeAgentId: activeAgentId.value,
        affectActiveSelection: activeAgentId.value === agentId,
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

  // Listen for message changes
  unsubscribeMessagesChanged = Events.On('chat:messages-changed', (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    const conversationId = Number(payload?.conversation_id)
    if (conversationId && conversationId === activeConversationId.value) {
      void chatStore.loadMessages(conversationId)
    }
  })

  // Listen for model/provider changes from settings page (e.g., add/delete model, enable/disable provider)
  unsubscribeModelsChanged = Events.On('models:changed', () => {
    void loadModels()
  })

  // Store dialogue_id from team chat SSE for next request
  unsubscribeChatCompleteTeamDialogue = Events.On('chat:complete', (event: any) => {
    const data = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    const convId = data?.conversation_id
    const dId = data?.dialogue_id
    const isKnownTeamConversation =
      typeof convId === 'number' &&
      (convId === activeTeamConversationId.value ||
        teamRobots.value.some((r) => {
          const teamAgentId = getTeamConversationAgentId(r.id)
          return (conversationsByAgent.value[teamAgentId] || []).some((c) => c.id === convId)
        }))
    if (isKnownTeamConversation && dId != null && String(dId).trim() !== '') {
      teamDialogueIdByConversation.value[convId] = String(dId).trim()
      logTeam('stored dialogue_id from SSE', { conversation_id: convId, dialogue_id: dId })
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
  unsubscribeMessagesChanged?.()
  unsubscribeMessagesChanged = null
  unsubscribeAgentsChanged?.()
  unsubscribeAgentsChanged = null
  unsubscribeModelsChanged?.()
  unsubscribeModelsChanged = null
  unsubscribeChatCompleteTeamDialogue?.()
  unsubscribeChatCompleteTeamDialogue = null

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
      :list-mode="listMode"
      :agents="agents"
      :active-agent="activeAgent"
      :active-agent-id="activeAgentId"
      :team-robots="teamRobots"
      :active-team-robot="activeTeamRobot"
      :active-team-robot-id="activeTeamRobotId"
      :team-loading="teamLoading"
      :has-attached-target="hasAttachedTarget"
      @update:list-mode="handleListModeChange"
      @update:active-agent-id="activeAgentId = $event"
      @update:active-team-robot-id="activeTeamRobotId = $event"
      @new-conversation="handleSnapNewConversation"
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

      <!-- Left side: Agent list (collapsible, overlay in snap mode when expanded; always show so personal/team tab can switch when empty) -->
      <AgentSidebar
        v-if="!hideAssistantSidebar && !sidebarCollapsed"
        :agents="agents"
        :active-agent-id="activeAgentId"
        :active-conversation-id="activeDisplayConversationId"
        :loading="loading"
        :list-mode="listMode"
        :is-snap-mode="isSnapMode"
        :get-agent-conversations="getAgentConversations"
        :get-all-agent-conversations="getAllAgentConversations"
        :ensure-conversations-loaded="ensureConversationsLoaded"
        :get-team-conversation-agent-id="getTeamConversationAgentId"
        :team-robots="teamRobots"
        :active-team-robot-id="activeTeamRobotId"
        :team-loading="teamLoading"
        :team-binding-checked="teamBindingChecked"
        :team-bound="teamBound"
        :on-wake-attached="handleWakeAttachedPointerDown"
        @go-bind="goToChatwikiBindingSettings"
        @update:active-agent-id="activeAgentId = $event"
        @update:active-team-robot-id="activeTeamRobotId = $event"
        @update:list-mode="handleListModeChange"
        @create="createOpen = true"
        @open-settings="openSettings"
        @open-channels="openChannels"
        @new-conversation="handleNewConversation"
        @new-conversation-for-agent="handleNewConversationForAgent"
        @new-conversation-for-team-robot="handleNewConversationForTeamRobot"
        @select-conversation="handleSelectConversation"
        @select-conversation-for-agent="handleSelectConversationForAgent"
        @select-conversation-for-team-robot="handleSelectConversationForTeamRobot"
        @toggle-pin="handleTogglePin"
        @open-rename="handleOpenRenameConversation"
        @open-delete="handleOpenDeleteConversation"
        @close-sidebar="sidebarCollapsed = true"
      />

      <!-- Upper row: expand button + messages -->
      <div class="relative flex min-h-0 flex-1 overflow-hidden">
        <!-- Collapse/Expand handle (snap mode: floating, draggable) -->
        <div
          v-if="!hideAssistantSidebar && isSnapMode"
          class="absolute left-0.5 z-10 cursor-grab active:cursor-grabbing"
          :style="{ top: snapBtnTop + 'px' }"
          @pointerdown="onSnapBtnPointerDown"
          @pointermove="onSnapBtnPointerMove"
          @pointerup="onSnapBtnPointerUp"
        >
          <div
            class="group/handle relative flex h-7 w-6 items-center justify-center pointer-events-none"
          >
            <div
              class="relative flex h-7 w-5 items-center justify-center rounded-md border border-border bg-background/90 shadow-sm backdrop-blur dark:shadow-none dark:ring-1 dark:ring-white/10"
            >
              <span
                class="h-5 w-px bg-muted-foreground/60 transition-all duration-200 group-hover/handle:opacity-0 group-hover/handle:scale-y-75"
              />
              <span
                class="absolute inset-0 flex items-center justify-center text-muted-foreground opacity-0 transition-all duration-200 group-hover/handle:opacity-100"
              >
                <ChevronLeft v-if="!sidebarCollapsed" class="size-4" />
                <ChevronRight v-else class="size-4" />
              </span>
            </div>
            <span
              class="pointer-events-none absolute left-full ml-2 whitespace-nowrap rounded-md border border-border bg-popover px-2 py-1 text-xs text-popover-foreground opacity-0 shadow-sm transition-all duration-200 group-hover/handle:opacity-100 dark:shadow-none dark:ring-1 dark:ring-white/10"
            >
              {{
                sidebarCollapsed ? t('assistant.sidebar.expand') : t('assistant.sidebar.collapse')
              }}
            </span>
          </div>
        </div>
        <!-- Collapse/Expand handle (non-snap mode: in-flow) -->
        <div
          v-if="!hideAssistantSidebar && !isSnapMode"
          class="relative z-10 flex w-8 shrink-0 items-center justify-center"
        >
          <button
            type="button"
            class="group/handle relative flex h-16 w-6 items-center justify-center"
            :aria-label="
              sidebarCollapsed ? t('assistant.sidebar.expand') : t('assistant.sidebar.collapse')
            "
            @click="sidebarCollapsed = !sidebarCollapsed"
          >
            <div
              class="relative flex h-12 w-5 items-center justify-center rounded-md border border-border bg-background/90 shadow-sm backdrop-blur transition-colors dark:shadow-none dark:ring-1 dark:ring-white/10"
            >
              <span
                class="h-6 w-px bg-muted-foreground/60 transition-all duration-200 group-hover/handle:opacity-0 group-hover/handle:scale-y-75"
              />
              <span
                class="absolute inset-0 flex items-center justify-center text-muted-foreground opacity-0 transition-all duration-200 group-hover/handle:opacity-100"
              >
                <ChevronLeft v-if="!sidebarCollapsed" class="size-4" />
                <ChevronRight v-else class="size-4" />
              </span>
            </div>
            <span
              class="pointer-events-none absolute left-full ml-2 whitespace-nowrap rounded-md border border-border bg-popover px-2 py-1 text-xs text-popover-foreground opacity-0 shadow-sm transition-all duration-200 group-hover/handle:opacity-100 dark:shadow-none dark:ring-1 dark:ring-white/10"
            >
              {{
                sidebarCollapsed ? t('assistant.sidebar.expand') : t('assistant.sidebar.collapse')
              }}
            </span>
          </button>
        </div>

        <!-- Right side: Chat area -->
        <section class="flex min-w-0 flex-1 flex-col overflow-hidden">
          <!-- Top toolbar: workspace drawer toggle (task mode + active conversation only; hidden in team mode).
           Always render to avoid layout shift when switching chat mode; hide button via invisible. -->
          <div
            v-if="!isAgentEmpty && !isHistoryIframeMode && !isSnapMode && listMode !== 'team'"
            class="flex shrink-0 items-center justify-end px-2 pt-1"
          >
            <Button
              size="icon"
              variant="ghost"
              :class="cn('size-7', chatMode !== 'task' && 'invisible')"
              :title="t('assistant.workspaceDrawer.title')"
              @click="workspaceDrawerOpen = !workspaceDrawerOpen"
            >
              <PanelRight
                :class="
                  cn('size-4', workspaceDrawerOpen ? 'text-foreground' : 'text-muted-foreground')
                "
              />
            </Button>
          </div>

          <!-- Agent list empty state (personal tab, no agents) -->
          <div v-if="isAgentEmpty && !showHistoryConversationShell" class="flex h-full items-center justify-center px-8">
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

          <!-- Team tab: not bound - same structure as personal empty (icon + title + desc + bind button) -->
          <div
            v-else-if="listMode === 'team' && !teamBound"
            class="flex h-full items-center justify-center px-8"
          >
            <div class="flex flex-col items-center gap-4">
              <div class="grid size-10 place-items-center rounded-lg bg-muted">
                <IconAssistant class="size-4 text-muted-foreground" />
              </div>
              <div class="flex flex-col items-center gap-1.5">
                <h3 class="text-base font-medium text-foreground">
                  {{ t('knowledge.team.notBoundTitle') }}
                </h3>
                <p class="text-sm text-muted-foreground">
                  {{ t('assistant.teamNeedsBindingDesc') }}
                </p>
              </div>
              <Button class="mt-1" @click="goToChatwikiBindingSettings">
                {{ t('knowledge.team.goBind') }}
              </Button>
            </div>
          </div>

          <!-- Team tab: bound but no robots - empty data hint only (sync with personal empty style) -->
          <div
            v-else-if="listMode === 'team' && teamBound && teamRobots.length === 0"
            class="flex h-full items-center justify-center px-8"
          >
            <div class="flex flex-col items-center gap-4">
              <div class="grid size-10 place-items-center rounded-lg bg-muted">
                <IconAssistant class="size-4 text-muted-foreground" />
              </div>
              <p class="text-sm text-muted-foreground">
                {{ t('assistant.teamEmpty') }}
              </p>
            </div>
          </div>

          <!-- Chat messages area - show when we have an active conversation -->
          <ChatMessageList
            v-else-if="activeDisplayConversationId"
            data-snap-wake="true"
            :conversation-id="activeDisplayConversationId"
            :tab-id="tabId"
            :mode="props.mode"
            :agent-name="isTeamMode ? activeTeamRobot?.name : activeAgent?.name"
            :agent-icon="isTeamMode ? activeTeamRobot?.icon : activeAgent?.icon"
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

          <!-- Input area: hide when personal empty, team empty/unbound, or history-iframe (read-only) -->
          <ChatInputArea
            v-if="
              !isHistoryIframeMode &&
              (!isAgentEmpty || showHistoryConversationShell) &&
              !(listMode === 'team' && (!teamBound || teamRobots.length === 0)) &&
              (!isSnapMode || (chatMessages.length === 0 && !isGenerating))
            "
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
            :assistant-team-libraries="
              listMode !== 'team' && teamBound ? assistantTeamLibraries : []
            "
            :assistant-selected-team-library-ids="
              listMode !== 'team' ? assistantSelectedTeamLibraryIds : []
            "
            :is-generating="isGenerating"
            :can-send="canSend"
            :send-disabled-reason="sendDisabledReason"
            :chat-messages="chatMessages"
            :active-agent-id="isTeamMode ? -1 : activeAgentId"
            :active-agent="activeAgent"
            :agents="agents"
            :is-snap-mode="isSnapMode"
            :is-team-mode="listMode === 'team'"
            :pending-images="pendingImages"
            :pending-files="pendingFiles"
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
            @toggle-assistant-team-library="toggleAssistantTeamLibraryId"
            @remove-library="handleRemoveLibrary"
            @add-images="handleAddImages"
            @remove-image="handleRemoveImage"
            @clear-images="pendingImages = []"
            @add-files="handleAddFiles"
            @remove-file="handleRemoveFile"
            @clear-files="pendingFiles = []"
            @new-conversation="handleNewConversation"
          />
        </section>

        <!-- Workspace drawer panel (task mode only; hidden in team mode).
         Always rendered to avoid layout shift; auto-closed when not in task mode. -->
        <WorkspaceDrawer
          v-if="!isSnapMode && !isHistoryIframeMode && listMode !== 'team'"
          :open="workspaceDrawerOpen && chatMode === 'task'"
          :agent="activeAgent"
          :conversation-id="activeConversationId"
          @update:open="workspaceDrawerOpen = $event"
          @open-workspace-settings="handleOpenWorkspaceSettings"
        />
      </div>
      <!-- End upper row -->

      <!-- Input area (snap mode with messages: full-width at bottom) -->
      <ChatInputArea
        v-if="
          (!isAgentEmpty || showHistoryConversationShell) &&
          !(listMode === 'team' && (!teamBound || teamRobots.length === 0)) &&
          isSnapMode &&
          (chatMessages.length > 0 || isGenerating)
        "
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
        :assistant-team-libraries="listMode !== 'team' && teamBound ? assistantTeamLibraries : []"
        :assistant-selected-team-library-ids="
          listMode !== 'team' ? assistantSelectedTeamLibraryIds : []
        "
        :is-generating="isGenerating"
        :can-send="canSend"
        :send-disabled-reason="sendDisabledReason"
        :chat-messages="chatMessages"
        :active-agent-id="isTeamMode ? -1 : activeAgentId"
        :active-agent="activeAgent"
        :agents="agents"
        :is-snap-mode="isSnapMode"
        :is-team-mode="listMode === 'team'"
        :pending-images="pendingImages"
        :pending-files="pendingFiles"
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
        @toggle-assistant-team-library="toggleAssistantTeamLibraryId"
        @remove-library="handleRemoveLibrary"
        @add-images="handleAddImages"
        @remove-image="handleRemoveImage"
        @clear-images="pendingImages = []"
        @add-files="handleAddFiles"
        @remove-file="handleRemoveFile"
        @clear-files="pendingFiles = []"
        @new-conversation="handleSnapNewConversation"
      />
    </div>
    <!-- End main content wrapper -->

    <!-- Dialogs (rendered outside main content wrapper for proper z-index; skip in history-iframe to avoid DismissableLayer interference) -->
    <template v-if="!isHistoryIframeMode">
      <CreateAgentDialog v-model:open="createOpen" :loading="loading" @create="handleCreate" />
      <AgentChannelsDialog v-model:open="channelsOpen" :agent="channelsAgent" />
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
    </template>
  </div>
</template>
