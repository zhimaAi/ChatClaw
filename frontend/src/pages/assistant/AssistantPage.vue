<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowUp, MoreHorizontal, Pin, PinOff } from 'lucide-vue-next'
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
import { useNavigationStore } from '@/stores'
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
} from '@/components/ui/select'
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

// Chat message interface
interface ChatMessage {
  id: number
  role: 'user' | 'assistant'
  content: string
  createdAt: Date
}

const { t } = useI18n()
const navigationStore = useNavigationStore()

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

// Conversations state
const conversations = ref<Conversation[]>([])
const activeConversationId = ref<number | null>(null)
const conversationsLoading = ref(false)

// Chat messages (for display)
const chatMessages = ref<ChatMessage[]>([])
let messageIdCounter = 0

// Conversation dialogs
const renameConversationOpen = ref(false)
const deleteConversationOpen = ref(false)
const actionConversation = ref<Conversation | null>(null)

const activeAgent = computed(() => {
  if (activeAgentId.value == null) return null
  return agents.value.find((a) => a.id === activeAgentId.value) ?? null
})

const activeConversation = computed(() => {
  if (activeConversationId.value == null) return null
  return conversations.value.find((c) => c.id === activeConversationId.value) ?? null
})

const canSend = computed(() => {
  return chatInput.value.trim() !== '' && selectedModelKey.value !== ''
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
  return conversations.value.filter((c) => c.agent_id === agentId).slice(0, 3)
}

/**
 * Get all conversations for a specific agent (for dropdown menu)
 */
const getAllAgentConversations = (agentId: number): Conversation[] => {
  return conversations.value.filter((c) => c.agent_id === agentId)
}

const loadAgents = async () => {
  loading.value = true
  try {
    const list = await AgentsService.ListAgents()
    agents.value = list

    // 默认选中第一个助手
    if (list.length > 0) {
      activeAgentId.value = list[0].id
    }

    // Update tab icon and title
    updateCurrentTab()
  } catch (error: unknown) {
    toast.error(getErrorMessage(error) || t('assistant.errors.loadFailed'))
  } finally {
    loading.value = false
  }
}

const loadConversations = async (agentId: number, preserveSelection = false) => {
  conversationsLoading.value = true
  const previousConversationId = activeConversationId.value
  try {
    const list = await ConversationsService.ListConversations(agentId)
    conversations.value = list || []
    if (preserveSelection && previousConversationId !== null) {
      // 保持当前选中状态（如果会话仍存在）
      const stillExists = list?.some((c) => c.id === previousConversationId)
      if (!stillExists) {
        activeConversationId.value = null
        chatMessages.value = []
      }
    } else {
      // Don't auto-select any conversation when loading
      activeConversationId.value = null
      chatMessages.value = []
    }
  } catch (error: unknown) {
    toast.error(getErrorMessage(error) || t('assistant.errors.loadConversationsFailed'))
    conversations.value = []
  } finally {
    conversationsLoading.value = false
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
}

const handleNewConversation = () => {
  // Just clear the current conversation selection and chat messages
  // Don't create a conversation record until user sends first message
  activeConversationId.value = null
  chatMessages.value = []
  chatInput.value = ''
}

const handleSend = async () => {
  if (!canSend.value || !activeAgentId.value) return

  const messageContent = chatInput.value.trim()
  chatInput.value = ''

  // Add user message to chat display
  const userMessage: ChatMessage = {
    id: ++messageIdCounter,
    role: 'user',
    content: messageContent,
    createdAt: new Date(),
  }
  chatMessages.value.push(userMessage)

  // If no active conversation, create one
  if (!activeConversationId.value) {
    try {
      const newConversation = await ConversationsService.CreateConversation(
        new CreateConversationInput({
          agent_id: activeAgentId.value,
          name: messageContent, // First message becomes conversation name
          last_message: messageContent,
        })
      )
      if (newConversation) {
        // 添加新会话并排序（置顶优先）
        conversations.value = [newConversation, ...conversations.value].sort((a, b) => {
          if (a.is_pinned !== b.is_pinned) return a.is_pinned ? -1 : 1
          return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
        })
        activeConversationId.value = newConversation.id
      }
    } catch (error: unknown) {
      toast.error(getErrorMessage(error) || t('assistant.errors.createConversationFailed'))
    }
  } else {
    // Update existing conversation's last_message and updated_at
    try {
      const updated = await ConversationsService.UpdateConversation(
        activeConversationId.value,
        new UpdateConversationInput({
          last_message: messageContent,
        })
      )
      if (updated) {
        // 更新并重新排序（置顶优先，然后按更新时间倒序）
        handleConversationUpdated(updated)
      }
    } catch (error: unknown) {
      console.error('Failed to update conversation:', error)
      // Non-critical error, don't show toast
    }
  }

  // TODO: Here you would call the LLM API to get a response
  // For now, just display the user message
}

const handleSelectConversation = (conversation: Conversation) => {
  activeConversationId.value = conversation.id
  // TODO: Load conversation messages from backend
  // For now, just clear messages
  chatMessages.value = []
}

const handleSelectKnowledge = () => {
  // TODO: Implement knowledge selection
}

const handleSelectImage = () => {
  // TODO: Implement image selection
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
      await loadConversations(activeAgentId.value, true)
    }
  } catch (error) {
    console.error('Failed to toggle pin:', error)
    toast.error(getErrorMessage(error) || t('assistant.errors.updateConversationFailed'))
  }
}

const handleConversationUpdated = (updated: Conversation) => {
  // Update and re-sort (pinned first, then by updated_at desc)
  conversations.value = conversations.value
    .map((c) => (c.id === updated.id ? updated : c))
    .sort((a, b) => {
      // Pinned items first
      if (a.is_pinned !== b.is_pinned) {
        return a.is_pinned ? -1 : 1
      }
      // Then by updated_at desc
      return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
    })
}

const confirmDeleteConversation = async () => {
  if (!actionConversation.value) return
  try {
    await ConversationsService.DeleteConversation(actionConversation.value.id)
    conversations.value = conversations.value.filter((c) => c.id !== actionConversation.value?.id)
    if (activeConversationId.value === actionConversation.value.id) {
      activeConversationId.value = null
      chatMessages.value = []
    }
    toast.success(t('assistant.conversation.delete.success'))
    deleteConversationOpen.value = false
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
    conversations.value = []
    activeConversationId.value = null
    chatMessages.value = []
  }
})

// Watch for models loaded
watch(providersWithModels, () => {
  selectDefaultModel()
})

// 当前标签页是否激活
const isTabActive = computed(() => navigationStore.activeTabId === props.tabId)

// 监听标签页激活状态，激活时刷新模型列表
// 这样用户在设置页面启用模型后，切换回聊天页面时能看到最新的模型
watch(isTabActive, (active) => {
  if (active) {
    loadModels()
  }
})

// Listen for text selection events
let unsubscribeTextSelection: (() => void) | null = null

onMounted(() => {
  loadAgents()
  loadModels()

  // Listen for text selection to send to assistant
  unsubscribeTextSelection = Events.On('text-selection:send-to-assistant', (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : event?.data ?? event
    const text = payload?.text ?? ''
    if (text) {
      chatInput.value = text
      // Auto-send after a short delay (if model is selected)
      if (canSend.value) {
        setTimeout(() => {
          handleSend()
        }, 100)
      }
    }
  })
})

onUnmounted(() => {
  unsubscribeTextSelection?.()
  unsubscribeTextSelection = null
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
                  @click.stop="activeAgentId = a.id; handleNewConversation()"
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
                      <DropdownMenuSubTrigger>
                        {{ t('assistant.menu.history') }}
                      </DropdownMenuSubTrigger>
                      <DropdownMenuSubContent class="w-56">
                        <template v-if="getAllAgentConversations(a.id).length > 0">
                          <DropdownMenuItem
                            v-for="conv in getAllAgentConversations(a.id)"
                            :key="conv.id"
                            @click="activeAgentId = a.id; handleSelectConversation(conv)"
                          >
                            <span class="truncate">{{ conv.name }}</span>
                          </DropdownMenuItem>
                        </template>
                        <DropdownMenuItem v-else disabled>
                          {{ t('assistant.empty') }}
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
    <section class="flex flex-1 flex-col overflow-hidden">
      <!-- Chat messages area -->
      <div
        v-if="chatMessages.length > 0"
        class="flex-1 overflow-auto px-6 py-4"
      >
        <div class="mx-auto max-w-[800px] flex flex-col gap-4">
          <div
            v-for="msg in chatMessages"
            :key="msg.id"
            :class="cn(
              'flex',
              msg.role === 'user' ? 'justify-end' : 'justify-start'
            )"
          >
            <div
              :class="cn(
                'max-w-[80%] rounded-2xl px-4 py-3 text-sm',
                msg.role === 'user'
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-muted text-foreground'
              )"
            >
              <p class="whitespace-pre-wrap wrap-break-word">{{ msg.content }}</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Empty state / Input area -->
      <div
        :class="cn(
          'flex px-6',
          chatMessages.length > 0
            ? 'pb-4'
            : 'flex-1 items-center justify-center'
        )"
      >
        <div
          :class="cn(
            'flex w-full flex-col items-center gap-10',
            chatMessages.length === 0 && '-translate-y-10'
          )"
        >
          <div v-if="chatMessages.length === 0" class="flex items-center gap-3">
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
              @keydown.enter.exact.prevent="handleSend"
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

                <Button
                  size="icon"
                  variant="ghost"
                  class="size-8 rounded-full border border-border bg-background hover:bg-muted/40"
                  :title="t('assistant.chat.selectKnowledge')"
                  @click="handleSelectKnowledge"
                >
                  <IconSelectKnowledge class="size-4 text-muted-foreground" />
                </Button>

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
