<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import IconAgentAdd from '@/assets/icons/agent-add.svg'
import IconNewConversation from '@/assets/icons/new-conversation.svg'
import IconSidebarCollapse from '@/assets/icons/sidebar-collapse.svg'
import IconSidebarExpand from '@/assets/icons/sidebar-expand.svg'
import IconSettings from '@/assets/icons/settings.svg'
import IconSendDisabled from '@/assets/icons/send-disabled.svg'
import IconSendActive from '@/assets/icons/send-active.svg'
import IconSelectKnowledge from '@/assets/icons/select-knowledge.svg'
import IconSelectImage from '@/assets/icons/select-image.svg'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import LogoIcon from '@/assets/images/logo.svg'
import CreateAgentDialog from './components/CreateAgentDialog.vue'
import AgentSettingsDialog from './components/AgentSettingsDialog.vue'
import { AgentsService, type Agent } from '@bindings/willchat/internal/services/agents'
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
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

type ListMode = 'personal' | 'team'

// Mock chat history data
interface ChatHistory {
  id: number
  title: string
  createdAt: string
}

const { t } = useI18n()

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

// Mock chat history for each agent
const mockChatHistories: Record<number, ChatHistory[]> = {
  1: [
    { id: 1, title: '我的优惠券为什么无法使用？', createdAt: '2024-01-15' },
    { id: 2, title: '账户被锁定可能是由于多次输...', createdAt: '2024-01-14' },
    { id: 3, title: '账户被锁定可能是由于多次输...', createdAt: '2024-01-13' },
    { id: 4, title: '非常抱歉给您带来不便，请...', createdAt: '2024-01-12' },
  ],
}

const activeAgent = computed(() => {
  if (activeAgentId.value == null) return null
  return agents.value.find((a) => a.id === activeAgentId.value) ?? null
})

const canSend = computed(() => {
  return chatInput.value.trim() !== '' && selectedModelKey.value !== ''
})

const hasModels = computed(() => {
  return providersWithModels.value.some((pw) =>
    pw.model_groups.some((g) => g.type === 'llm' && g.models.length > 0)
  )
})

// Get chat histories for an agent (max 3)
const getAgentChatHistories = (agentId: number): ChatHistory[] => {
  return (mockChatHistories[agentId] || []).slice(0, 3)
}

// Get all chat histories for an agent (for the dropdown menu)
const getAllAgentChatHistories = (agentId: number): ChatHistory[] => {
  return mockChatHistories[agentId] || []
}

const loadAgents = async () => {
  loading.value = true
  try {
    const list = await AgentsService.ListAgents()
    agents.value = list
    if (activeAgentId.value == null && list.length > 0) {
      activeAgentId.value = list[0].id
    }
  } catch (error: unknown) {
    toast.error(getErrorMessage(error) || t('assistant.errors.loadFailed'))
  } finally {
    loading.value = false
  }
}

const loadModels = async () => {
  try {
    const providers = await ProvidersService.ListProviders()
    const enabled = providers.filter((p) => p.enabled)
    const results: ProviderWithModels[] = []
    for (const p of enabled) {
      const withModels = await ProvidersService.GetProviderWithModels(p.provider_id)
      if (withModels) results.push(withModels)
    }
    providersWithModels.value = results
  } catch (error: unknown) {
    console.warn('Failed to load models:', error)
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
  if (idx >= 0) agents.value[idx] = updated
  if (activeAgentId.value === updated.id) activeAgentId.value = updated.id
}

const handleDeleted = (id: number) => {
  agents.value = agents.value.filter((a) => a.id !== id)
  if (activeAgentId.value === id) {
    activeAgentId.value = agents.value.length > 0 ? agents.value[0].id : null
  }
}

const handleNewConversation = () => {
  // TODO: Implement new conversation logic
  console.log('New conversation clicked')
}

const handleSend = () => {
  if (!canSend.value) return
  // TODO: Implement send logic
  console.log('Send:', chatInput.value, 'Model:', selectedModelKey.value)
}

const handleSelectKnowledge = () => {
  // TODO: Implement knowledge selection
  console.log('Select knowledge clicked')
}

const handleSelectImage = () => {
  // TODO: Implement image selection
  console.log('Select image clicked')
}

const handleSelectHistory = (history: ChatHistory) => {
  // TODO: Implement history selection
  console.log('Select history:', history)
}

// Watch for active agent changes to update selected model
watch(activeAgentId, () => {
  selectDefaultModel()
})

// Watch for models loaded
watch(providersWithModels, () => {
  selectDefaultModel()
})

onMounted(() => {
  loadAgents()
  loadModels()
})
</script>

<template>
  <div class="flex h-full w-full overflow-hidden bg-background">
    <!-- 左侧：助手列表 -->
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
            <button
              :class="
                cn(
                  'group flex h-11 w-full items-center gap-2 rounded px-2 text-left outline-none transition-colors',
                  a.id === activeAgentId
                    ? 'bg-zinc-100 text-foreground dark:bg-accent'
                    : 'bg-white text-muted-foreground shadow-[0px_1px_4px_0px_rgba(0,0,0,0.1)] hover:bg-accent/50 hover:text-foreground dark:bg-zinc-800/50 dark:shadow-[0px_1px_4px_0px_rgba(255,255,255,0.05)]'
                )
              "
              @click="activeAgentId = a.id"
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

              <!-- 新会话按钮 -->
              <Button
                size="icon"
                variant="ghost"
                class="size-7 opacity-0 group-hover:opacity-100"
                :title="t('assistant.sidebar.newConversation')"
                @click.stop="handleNewConversation"
              >
                <IconNewConversation class="size-4 text-muted-foreground" />
              </Button>

              <!-- 设置下拉菜单 -->
              <DropdownMenu>
                <DropdownMenuTrigger as-child>
                  <Button
                    size="icon"
                    variant="ghost"
                    class="size-7 opacity-0 group-hover:opacity-100"
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
                      <template v-if="getAllAgentChatHistories(a.id).length > 0">
                        <DropdownMenuItem
                          v-for="history in getAllAgentChatHistories(a.id)"
                          :key="history.id"
                          @click="handleSelectHistory(history)"
                        >
                          <span class="truncate">{{ history.title }}</span>
                        </DropdownMenuItem>
                      </template>
                      <DropdownMenuItem v-else disabled>
                        {{ t('assistant.empty') }}
                      </DropdownMenuItem>
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                </DropdownMenuContent>
              </DropdownMenu>
            </button>

            <!-- Chat history list (max 3 items) - only show for active agent -->
            <div
              v-if="a.id === activeAgentId && getAgentChatHistories(a.id).length > 0"
              class="ml-10 mt-1 flex flex-col gap-0.5"
            >
              <button
                v-for="history in getAgentChatHistories(a.id)"
                :key="history.id"
                class="truncate rounded px-2 py-1 text-left text-xs text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground"
                @click="handleSelectHistory(history)"
              >
                {{ history.title }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </aside>

    <!-- 收起/展开按钮 -->
    <div class="flex items-start pt-3">
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

    <!-- 右侧：聊天区 -->
    <section class="flex flex-1 flex-col overflow-hidden">
      <!-- 聊天内容区域 -->
      <div class="flex flex-1 flex-col items-center justify-center gap-2 overflow-auto">
        <div class="text-lg font-semibold text-foreground">
          {{ activeAgent?.name ?? t('assistant.placeholders.noAgentSelected') }}
        </div>
        <div class="max-w-dialog-md px-6 text-center text-sm text-muted-foreground">
          {{ t('assistant.placeholders.chatComingSoon') }}
        </div>
      </div>

      <!-- 聊天输入区域 -->
      <div class="border-t border-border p-4">
        <div
          class="mx-auto max-w-3xl rounded-xl border border-border bg-card p-3 shadow-sm dark:border-white/10"
        >
          <!-- 输入框 -->
          <textarea
            v-model="chatInput"
            :placeholder="t('assistant.placeholders.inputPlaceholder')"
            class="min-h-[60px] w-full resize-none bg-transparent text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
            rows="2"
            @keydown.enter.exact.prevent="handleSend"
          />

          <!-- 底部工具栏 -->
          <div class="mt-2 flex items-center justify-between">
            <div class="flex items-center gap-2">
              <!-- 模型选择 -->
              <Select
                v-model="selectedModelKey"
                :disabled="!hasModels"
              >
                <SelectTrigger
                  class="h-8 w-auto min-w-[140px] max-w-[200px] rounded-md border-0 bg-muted/50 px-2 text-xs hover:bg-muted"
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

              <!-- 选择知识库 -->
              <Button
                size="icon"
                variant="ghost"
                class="size-8"
                :title="t('assistant.chat.selectKnowledge')"
                @click="handleSelectKnowledge"
              >
                <IconSelectKnowledge class="size-4 text-muted-foreground" />
              </Button>

              <!-- 选择图片 -->
              <Button
                size="icon"
                variant="ghost"
                class="size-8"
                :title="t('assistant.chat.selectImage')"
                @click="handleSelectImage"
              >
                <IconSelectImage class="size-4 text-muted-foreground" />
              </Button>
            </div>

            <!-- 发送按钮 -->
            <TooltipProvider v-if="!canSend">
              <Tooltip>
                <TooltipTrigger as-child>
                  <Button
                    size="icon"
                    class="size-9 rounded-lg bg-muted text-muted-foreground hover:bg-muted"
                    disabled
                  >
                    <IconSendDisabled class="size-5" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>{{ t('assistant.placeholders.enterToSend') }}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <Button
              v-else
              size="icon"
              class="size-9 rounded-lg bg-primary text-primary-foreground hover:bg-primary/90"
              :title="t('assistant.chat.send')"
              @click="handleSend"
            >
              <IconSendActive class="size-5" />
            </Button>
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
  </div>
</template>
