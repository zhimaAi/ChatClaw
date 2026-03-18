<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Plus,
  Trash2,
  MoreHorizontal,
  Unlink,
  Link,
  BadgeCheck,
  RouteOff,
  SquareDashed,
  Check,
  LoaderCircle,
  Edit,
} from 'lucide-vue-next'
import IconChannels from '@/assets/icons/channelsMax.svg'
import IconCheck from '@/assets/icons/check-icon.svg'
import IconClose from '@/assets/icons/close-icon.svg'
import { platformIconMap } from '@/assets/icons/snap/platformIcons'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { Dialogs } from '@wailsio/runtime'
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import AddChannelDialog from './components/AddChannelDialog.vue'
import ConfigChannelDialog from './components/ConfigChannelDialog.vue'
import BindAgentDialog from './components/BindAgentDialog.vue'
import { getPlatformDocsUrl, openExternalLink } from './platformDocs'
import { ChannelService, UpdateChannelInput } from '@bindings/chatclaw/internal/services/channels'
import type {
  Channel,
  ChannelStats,
  PlatformMeta,
} from '@bindings/chatclaw/internal/services/channels'
import { AgentsService, type Agent } from '@bindings/chatclaw/internal/services/agents'

defineProps<{ tabId: string }>()

const { t, te } = useI18n()

/** Platforms that support add/filter in UI (feishu + wecom + qq). */
function isChannelPlatformSelectable(platformId: string) {
  return platformId === 'feishu' || platformId === 'wecom' || platformId === 'qq'
}

const channels = ref<Channel[]>([])
const stats = ref<ChannelStats>({ total: 0, connected: 0, disconnected: 0 })
const platforms = ref<PlatformMeta[]>([])
const agents = ref<Agent[]>([])
const loading = ref(false)
const addDialogOpen = ref(false)
const configDialogOpen = ref(false)
const selectedPlatform = ref<PlatformMeta | null>(null)
const channelToEdit = ref<Channel | null>(null)
const deleteDialogOpen = ref(false)
const channelToDelete = ref<Channel | null>(null)
const bindDialogOpen = ref(false)
const channelToBind = ref<Channel | null>(null)
/** True when bind dialog was opened right after creating a channel (show auto-generate option) */
const bindFromCreate = ref(false)
const toggleDialogOpen = ref(false)
const channelToToggle = ref<{ channel: Channel; val: boolean } | null>(null)

const selectedFilter = ref<string>('all')

// Inline add form state
const inlineFormName = ref('')
const inlineFormAvatar = ref('')
const inlineFormAppId = ref('')
const inlineFormAppSecret = ref('')
const inlineFormSaving = ref(false)
const inlineFormVerifying = ref(false)

const filteredChannels = computed(() => {
  if (selectedFilter.value === 'all') return channels.value
  return channels.value.filter((ch) => ch.platform === selectedFilter.value)
})

const selectedPlatformMeta = computed(() => {
  if (selectedFilter.value === 'all') return null
  return platforms.value.find((p) => p.id === selectedFilter.value) || null
})

const isInlineWeCom = computed(() => selectedPlatformMeta.value?.id === 'wecom')
const inlineAppIdLabel = computed(() =>
  isInlineWeCom.value ? t('channels.config.wecomBotId') : t('channels.config.appId')
)
const inlineAppSecretLabel = computed(() =>
  isInlineWeCom.value ? t('channels.config.wecomSecret') : t('channels.config.appSecret')
)
const inlineAppIdPlaceholder = computed(() =>
  isInlineWeCom.value
    ? t('channels.config.wecomAppIdPlaceholder')
    : t('channels.config.appIdPlaceholder')
)
const inlineAppSecretPlaceholder = computed(() =>
  isInlineWeCom.value
    ? t('channels.config.wecomAppSecretPlaceholder')
    : t('channels.config.appSecretPlaceholder')
)

const isInlineFormValid = computed(() => {
  if (!inlineFormName.value.trim()) return false
  return !!(inlineFormAppId.value.trim() && inlineFormAppSecret.value.trim())
})

async function loadData() {
  loading.value = true
  try {
    const [channelList, channelStats, platformList, agentsList] = await Promise.all([
      ChannelService.ListChannels(),
      ChannelService.GetChannelStats(),
      ChannelService.GetSupportedPlatforms(),
      AgentsService.ListAgents(),
    ])
    channels.value = channelList || []
    stats.value = channelStats || { total: 0, connected: 0, disconnected: 0 }
    platforms.value = platformList || []
    agents.value = agentsList || []
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

function getAgentName(agentId: number): string {
  if (!agentId) return t('channels.agentFallback')
  const agent = agents.value.find((a) => a.id === agentId)
  return agent ? agent.name : t('channels.agentFallback')
}

function handleAddChannel() {
  addDialogOpen.value = true
}

function handleSelectPlatform(platform: PlatformMeta) {
  selectedPlatform.value = platform
  channelToEdit.value = null
  addDialogOpen.value = false
  configDialogOpen.value = true
}

function handleEditChannel(channel: Channel) {
  channelToEdit.value = channel
  selectedPlatform.value = platforms.value.find((p) => p.id === channel.platform) || null
  configDialogOpen.value = true
}

function handleConfigSaved(channel: Channel, isEdit: boolean) {
  configDialogOpen.value = false
  selectedPlatform.value = null
  channelToEdit.value = null
  loadData().then(() => {
    if (!isEdit) {
      channelToBind.value = channel
      bindFromCreate.value = true
      bindDialogOpen.value = true
    }
  })
}

function confirmDelete(channel: Channel) {
  channelToDelete.value = channel
  deleteDialogOpen.value = true
}

async function handleDelete() {
  if (!channelToDelete.value) return
  try {
    await ChannelService.DeleteChannel(channelToDelete.value.id)
    toast.success(t('channels.delete.success'))
    loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    deleteDialogOpen.value = false
    channelToDelete.value = null
  }
}

async function handleEnableChannel(channel: Channel) {
  try {
    await ChannelService.UpdateChannel(channel.id, new UpdateChannelInput({ enabled: true }))
    await ChannelService.ConnectChannel(channel.id)
    toast.success(t('channels.toggle.enableSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    await loadData()
  }
}

async function handleDisableChannel(channel: Channel) {
  try {
    await ChannelService.UpdateChannel(channel.id, new UpdateChannelInput({ enabled: false }))
    await ChannelService.DisconnectChannel(channel.id)
    toast.success(t('channels.toggle.disableSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    await loadData()
  }
}

async function handleToggleConnection(channel: Channel, val: boolean) {
  channelToToggle.value = { channel, val }
  toggleDialogOpen.value = true
}

function cancelToggle() {
  toggleDialogOpen.value = false
  channelToToggle.value = null
  // Just in case the UI switch visually toggled, reload data to revert it to DB state.
  loadData()
}

async function confirmToggle() {
  if (!channelToToggle.value) return
  const { channel, val } = channelToToggle.value
  toggleDialogOpen.value = false
  channelToToggle.value = null

  if (val) {
    await handleEnableChannel(channel)
  } else {
    await handleDisableChannel(channel)
  }
}

async function handleUnbind(channel: Channel) {
  try {
    await ChannelService.UnbindAgent(channel.id)
    toast.success(t('channels.unbindSuccess'))
    loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  }
}

function handleOpenBind(channel: Channel) {
  channelToBind.value = channel
  bindFromCreate.value = false
  bindDialogOpen.value = true
}

async function handleBindAgent(agentId: number) {
  if (!channelToBind.value) return
  try {
    await ChannelService.BindAgent(channelToBind.value.id, agentId)
    toast.success(t('channels.bindSuccess'))
    loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    bindDialogOpen.value = false
    channelToBind.value = null
    bindFromCreate.value = false
  }
}

async function handleAutoGenerate() {
  if (!channelToBind.value) return
  try {
    await ChannelService.EnsureAgentForChannel(channelToBind.value.id)
    await ChannelService.ConnectChannel(channelToBind.value.id)
    toast.success(t('channels.bindAgent.autoGenerateSuccess'))
    loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    bindDialogOpen.value = false
    channelToBind.value = null
    bindFromCreate.value = false
  }
}

function getPlatformIcon(platformId: string): string | null {
  return platformIconMap[platformId] || null
}

function getPlatformName(platformId: string): string {
  const key = `channels.platforms.${platformId}`
  if (te(key)) return t(key)
  const platform = platforms.value.find((p) => p.id === platformId)
  return platform?.name || platformId
}

function resetInlineForm() {
  inlineFormName.value = ''
  inlineFormAvatar.value = ''
  inlineFormAppId.value = ''
  inlineFormAppSecret.value = ''
}

async function handleInlinePickAvatar() {
  if (inlineFormSaving.value) return
  try {
    const path = await Dialogs.OpenFile({
      CanChooseFiles: true,
      CanChooseDirectories: false,
      AllowsMultipleSelection: false,
      Title: t('channels.config.pickAvatar'),
      Filters: [
        {
          DisplayName: t('channels.config.filterImages'),
          Pattern: '*.png;*.jpg;*.jpeg;*.gif;*.webp;*.svg',
        },
      ],
    })
    if (!path) return
    inlineFormAvatar.value = await AgentsService.ReadIconFile(path)
  } catch (error) {
    if (String(error).includes('cancelled by user')) return
    console.error('Failed to pick icon:', error)
  }
}

async function handleInlineSave() {
  if (!selectedPlatformMeta.value) return
  if (!isInlineFormValid.value) return

  inlineFormSaving.value = true
  try {
    const extraConfig = JSON.stringify({
      app_id: inlineFormAppId.value.trim(),
      app_secret: inlineFormAppSecret.value.trim(),
    })

    const channel = await ChannelService.CreateChannel({
      platform: selectedPlatformMeta.value.id,
      name: inlineFormName.value.trim(),
      avatar: inlineFormAvatar.value,
      connection_type: 'gateway',
      extra_config: extraConfig,
    })

    toast.success(t('channels.config.success'))
    resetInlineForm()
    await loadData()
    if (channel) {
      channelToBind.value = channel
      bindFromCreate.value = true
      bindDialogOpen.value = true
    }
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    inlineFormSaving.value = false
  }
}

function openPlatformDocs() {
  const url = getPlatformDocsUrl(selectedPlatformMeta.value?.id)
  void openExternalLink(url)
}

async function handleInlineVerify() {
  if (!isInlineFormValid.value) {
    toast.error(t('channels.inline.fillRequired'))
    return
  }
  if (!selectedPlatformMeta.value) return
  const extraConfig = JSON.stringify({
    app_id: inlineFormAppId.value.trim(),
    app_secret: inlineFormAppSecret.value.trim(),
  })
  inlineFormVerifying.value = true
  try {
    await ChannelService.VerifyChannelConfig(selectedPlatformMeta.value.id, extraConfig)
    toast.success(t('channels.inline.verifySuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('channels.inline.verifyFailed'))
  } finally {
    inlineFormVerifying.value = false
  }
}

function getAppId(extraConfig: string): string {
  try {
    const config = JSON.parse(extraConfig)
    return config.app_id || config.token || t('common.na')
  } catch {
    return t('common.na')
  }
}

onMounted(loadData)
</script>

<template>
  <div class="flex h-full flex-col overflow-y-auto bg-white dark:bg-background">
    <!-- Page Header -->
    <div class="flex h-20 shrink-0 items-center justify-between px-6">
      <div class="flex flex-col gap-1">
        <h1 class="text-base font-semibold text-[#262626] dark:text-foreground">
          {{ t('channels.title') }}
        </h1>
        <p class="text-sm text-[#737373] dark:text-muted-foreground">
          {{ t('channels.subtitle') }}
        </p>
      </div>
      <Button
        class="h-9 gap-1 bg-[#f5f5f5] text-[#171717] hover:bg-[#e5e5e5] border-none shadow-none dark:bg-muted dark:text-foreground dark:hover:bg-muted/80"
        @click="handleAddChannel"
      >
        <Plus class="h-4 w-4 shrink-0" />
        {{ t('channels.addChannel') }}
      </Button>
    </div>

    <div class="flex-1 overflow-y-auto px-6 pb-6">
      <!-- Stats Cards Row -->
      <div class="mb-6 flex flex-wrap gap-4">
        <!-- Card 1: Total -->
        <div
          class="flex h-[102px] w-[222px] items-center gap-4 rounded-[16px] border border-[#d9d9d9] bg-white px-6 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10 dark:border-border dark:bg-card"
        >
          <div
            class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-[#f5f5f5] dark:bg-muted"
          >
            <IconChannels class="h-6 w-6 text-[#171717] dark:text-foreground" />
          </div>
          <div class="flex flex-col gap-1">
            <span
              class="text-2xl font-semibold leading-none tracking-tight text-[#171717] dark:text-foreground"
              >{{ stats.total }}</span
            >
            <span class="text-sm text-[#737373] dark:text-muted-foreground">{{
              t('channels.stats.total')
            }}</span>
          </div>
        </div>
        <!-- Card 2: Connected -->
        <div
          class="flex h-[102px] w-[222px] items-center gap-4 rounded-[16px] border border-[#d9d9d9] bg-white px-6 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10 dark:border-border dark:bg-card"
        >
          <div
            class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-[#f5f5f5] dark:bg-muted"
          >
            <BadgeCheck class="h-6 w-6 text-[#171717] dark:text-foreground" />
          </div>
          <div class="flex flex-col gap-1">
            <span
              class="text-2xl font-semibold leading-none tracking-tight text-[#171717] dark:text-foreground"
              >{{ stats.connected }}</span
            >
            <span class="text-sm text-[#737373] dark:text-muted-foreground">{{
              t('channels.stats.connected')
            }}</span>
          </div>
        </div>
        <!-- Card 3: Disconnected -->
        <div
          class="flex h-[102px] w-[222px] items-center gap-4 rounded-[16px] border border-[#d9d9d9] bg-white px-6 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10 dark:border-border dark:bg-card"
        >
          <div
            class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-[#f5f5f5] dark:bg-muted"
          >
            <RouteOff class="h-6 w-6 text-[#171717] dark:text-foreground" />
          </div>
          <div class="flex flex-col gap-1">
            <span
              class="text-2xl font-semibold leading-none tracking-tight text-[#171717] dark:text-foreground"
              >{{ stats.disconnected }}</span
            >
            <span class="text-sm text-[#737373] dark:text-muted-foreground">{{
              t('channels.stats.disconnected')
            }}</span>
          </div>
        </div>
      </div>

      <!-- Section Header -->
      <h2 class="mb-2 text-base font-semibold text-[#262626] dark:text-foreground">
        {{ t('channels.available.title') }}
      </h2>

      <!-- Platform Filter Tabs -->
      <div
        class="mb-4 inline-flex overflow-x-auto rounded-lg border border-[#e5e5e5] bg-[rgba(0,0,0,0.05)] shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] dark:border-border dark:bg-muted/50"
      >
        <button
          class="px-3 py-[7.5px] text-sm font-medium transition-colors first:rounded-l-lg last:rounded-r-lg"
          :class="
            selectedFilter === 'all'
              ? 'bg-white text-[#0a0a0a] dark:bg-background dark:text-foreground'
              : 'text-[#0a0a0a] hover:bg-white/50 dark:text-foreground dark:hover:bg-background/50'
          "
          @click="selectedFilter = 'all'"
        >
          {{ t('common.all') }}
        </button>
        <button
          v-for="platform in platforms"
          :key="platform.id"
          class="px-3 py-[7.5px] text-sm font-medium transition-colors first:rounded-l-lg last:rounded-r-lg border-l border-[#e5e5e5] dark:border-border"
          :class="[
            selectedFilter === platform.id
              ? 'bg-white text-[#0a0a0a] dark:bg-background dark:text-foreground'
              : 'text-[#0a0a0a] hover:bg-white/50 dark:text-foreground dark:hover:bg-background/50',
            !isChannelPlatformSelectable(platform.id) ? 'opacity-50 cursor-not-allowed' : '',
          ]"
          @click="
            isChannelPlatformSelectable(platform.id)
              ? (selectedFilter = platform.id)
              : toast.default(t('channels.comingSoon'))
          "
        >
          {{ getPlatformName(platform.id) }}
        </button>
      </div>

      <!-- Channels Grid -->
      <div v-if="filteredChannels.length > 0" class="flex flex-wrap gap-4">
        <div
          v-for="channel in filteredChannels"
          :key="channel.id"
          class="flex w-[300px] min-w-0 flex-col gap-2 rounded-[16px] border border-[#d9d9d9] bg-white p-4 shadow-sm transition-all hover:border-[#171717] dark:shadow-none dark:ring-1 dark:ring-white/10 dark:border-border dark:bg-card dark:hover:border-primary/50"
        >
          <!-- Card Header -->
          <div class="flex items-center justify-between">
            <div class="flex flex-1 items-center gap-2 min-w-0">
              <div
                class="flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded border border-[#d9d9d9] bg-white dark:border-border dark:bg-muted"
              >
                <img
                  v-if="channel.avatar"
                  :src="channel.avatar"
                  :alt="channel.name"
                  class="h-full w-full object-cover"
                />
                <img
                  v-else-if="getPlatformIcon(channel.platform)"
                  :src="getPlatformIcon(channel.platform)!"
                  :alt="channel.platform"
                  class="h-3.5 w-3.5 object-contain"
                />
                <span v-else class="text-xs">🤖</span>
              </div>
              <span class="truncate text-sm text-[#171717] dark:text-foreground">{{
                channel.name
              }}</span>
            </div>

            <div class="flex items-center gap-2 shrink-0">
              <!-- Enable switch: turning on enables the channel and then auto-connects it. -->
              <Switch
                :model-value="channel.enabled"
                :disabled="channel.agent_id === 0"
                @update:model-value="(v: boolean) => handleToggleConnection(channel, v)"
              />
              <DropdownMenu>
                <DropdownMenuTrigger as-child>
                  <Button
                    variant="ghost"
                    size="icon"
                    class="h-6 w-6 rounded bg-[#f5f5f5] dark:bg-muted hover:bg-[#e5e5e5] dark:hover:bg-muted/80"
                  >
                    <MoreHorizontal class="h-4 w-4 text-[#171717] dark:text-foreground" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  align="end"
                  class="min-w-24 rounded-md bg-white p-0.5 shadow-[0_8px_10px_-5px_rgba(0,0,0,0.08),0_16px_24px_2px_rgba(0,0,0,0.04),0_6px_30px_5px_rgba(0,0,0,0.05)] dark:bg-popover"
                >
                  <DropdownMenuItem
                    class="gap-2 rounded px-4 py-[5px]"
                    @click="handleEditChannel(channel)"
                  >
                    <Edit class="h-4 w-4" />
                    {{ t('common.edit') }}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    class="gap-2 rounded px-4 py-[5px]"
                    @click="handleOpenBind(channel)"
                  >
                    <Link class="h-4 w-4" />
                    {{
                      channel.agent_id === 0
                        ? t('channels.card.bind')
                        : t('channels.card.switchBind')
                    }}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    class="gap-2 rounded px-4 py-[5px]"
                    :disabled="channel.agent_id === 0"
                    @click="handleUnbind(channel)"
                  >
                    <Unlink class="h-4 w-4" />
                    {{ t('channels.card.unbind') }}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator class="my-0.5 bg-[#f0f0f0] dark:bg-border" />
                  <DropdownMenuItem
                    class="gap-2 rounded px-4 py-[5px] text-destructive focus:bg-destructive/10 focus:text-destructive"
                    @click="confirmDelete(channel)"
                  >
                    <Trash2 class="h-4 w-4" />
                    {{ t('common.delete') }}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>

          <!-- Appid -->
          <p class="text-xs leading-5 text-[#8c8c8c] dark:text-muted-foreground">
            {{ t('channels.card.appId') }}: {{ getAppId(channel.extra_config) }}
          </p>

          <!-- Status tags: wrap on narrow card / long EN copy; pills truncate with title for full text -->
          <div class="flex min-w-0 flex-wrap items-center gap-2">
            <!-- Connection Status -->
            <div
              class="inline-flex max-w-full min-w-0 items-center gap-1.5 rounded-full bg-[#f0f0f0] px-2 py-0.5 dark:bg-muted"
            >
              <div
                class="h-2 w-2 shrink-0 rounded-full"
                :class="{
                  'bg-green-500': channel.status === 'online',
                  'bg-red-500': channel.status === 'error',
                  'bg-gray-400': channel.status === 'offline' || !channel.status,
                }"
              />
              <span
                class="min-w-0 truncate text-xs leading-4 text-[#595959] dark:text-muted-foreground"
                :title="
                  channel.status === 'online'
                    ? t('channels.status.online')
                    : channel.status === 'error'
                      ? t('channels.status.error')
                      : t('channels.status.offline')
                "
              >
                {{
                  channel.status === 'online'
                    ? t('channels.status.online')
                    : channel.status === 'error'
                      ? t('channels.status.error')
                      : t('channels.status.offline')
                }}
              </span>
            </div>
            <!-- Bind Status -->
            <div
              class="inline-flex max-w-full min-w-0 items-center gap-1 rounded-full bg-[#f0f0f0] px-2 py-0.5 dark:bg-muted"
              :class="{
                'cursor-pointer hover:bg-[#e5e5e5] dark:hover:bg-muted/80 transition-colors':
                  channel.agent_id === 0,
              }"
              @click="channel.agent_id === 0 ? handleOpenBind(channel) : undefined"
            >
              <IconCheck
                v-if="channel.agent_id !== 0"
                class="h-3.5 w-3.5 shrink-0 text-[#595959] dark:text-muted-foreground"
              />
              <IconClose
                v-else
                class="h-3.5 w-3.5 shrink-0 text-[#595959] dark:text-muted-foreground"
              />
              <span
                class="min-w-0 truncate text-xs leading-4 text-[#595959] dark:text-muted-foreground"
                :title="
                  channel.agent_id !== 0 ? t('channels.card.bound') : t('channels.card.unbound')
                "
              >
                {{ channel.agent_id !== 0 ? t('channels.card.bound') : t('channels.card.unbound') }}
              </span>
            </div>
            <!-- Agent name: background wraps text only; long names truncate with max-width -->
            <div
              v-if="channel.agent_id !== 0"
              class="inline-flex min-w-0 max-w-[12rem] w-fit items-center rounded-full bg-[#f0f0f0] px-2 py-0.5 dark:bg-muted"
            >
              <span
                class="min-w-0 truncate text-xs leading-4 text-[#595959] dark:text-muted-foreground"
                :title="getAgentName(channel.agent_id)"
              >
                {{ getAgentName(channel.agent_id) }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- Empty State - All platforms -->
      <div
        v-else-if="selectedFilter === 'all'"
        class="flex flex-col items-center justify-center py-12 text-center"
      >
        <div
          class="flex h-12 w-12 items-center justify-center rounded-full bg-[#f5f5f5] dark:bg-muted mb-4"
        >
          <SquareDashed class="h-6 w-6 text-[#737373] dark:text-muted-foreground" />
        </div>
        <h3 class="text-base font-medium text-[#262626] dark:text-foreground">
          {{ t('channels.empty.title') }}
        </h3>
        <p class="mt-2 max-w-sm text-sm text-[#737373] dark:text-muted-foreground">
          {{ t('channels.empty.desc') }}
        </p>
        <Button
          class="mt-6 gap-1 bg-[#171717] text-white hover:bg-[#171717]/90 dark:bg-primary dark:text-primary-foreground dark:hover:bg-primary/90"
          @click="handleAddChannel"
        >
          <Plus class="h-4 w-4 shrink-0" />
          {{ t('channels.addChannel') }}
        </Button>
      </div>

      <!-- Inline Add Form - Specific platform selected (per Figma: Inline Add Form) -->
      <div v-else class="space-y-6">
        <!-- Form Row: three vertical fields, labels with * and 4px gap to input -->
        <div class="flex items-end gap-4">
          <!-- * 机器人头像/名称: 262px width, avatar 40x40 + input flex-1, gap 8px -->
          <div class="flex w-[262px] shrink-0 flex-col gap-1">
            <label
              class="flex items-center gap-1 text-sm font-medium leading-5 text-[#0a0a0a] dark:text-foreground"
            >
              <span>*</span>
              <span>{{ t('channels.inline.avatarName') }}</span>
            </label>
            <div class="flex min-w-0 gap-2">
              <button
                type="button"
                class="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-[#e5e5e5] bg-white shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] transition-opacity hover:opacity-80 dark:border-border dark:bg-input dark:shadow-none dark:ring-1 dark:ring-white/10"
                @click="handleInlinePickAvatar"
              >
                <img
                  v-if="inlineFormAvatar"
                  :src="inlineFormAvatar"
                  class="h-full w-full object-cover"
                />
                <img
                  v-else-if="getPlatformIcon(selectedFilter)"
                  :src="getPlatformIcon(selectedFilter)!"
                  class="h-5 w-5 object-contain"
                />
                <span v-else class="text-lg text-[#737373] dark:text-muted-foreground">🤖</span>
              </button>
              <Input
                v-model="inlineFormName"
                class="h-10 min-w-0 flex-1 rounded-lg border-[#e5e5e5] px-4 py-[9.5px] shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] dark:border-border dark:shadow-none dark:ring-1 dark:ring-white/10"
                :placeholder="t('channels.inline.namePlaceholder')"
                maxlength="60"
              />
            </div>
          </div>

          <!-- * APPID: 260px -->
          <div class="flex w-[260px] shrink-0 flex-col gap-1">
            <label
              class="flex items-center gap-1 text-sm font-medium leading-5 text-[#0a0a0a] dark:text-foreground"
            >
              <span>*</span>
              <span>{{ inlineAppIdLabel }}</span>
            </label>
            <Input
              v-model="inlineFormAppId"
              class="h-10 w-full rounded-lg border-[#e5e5e5] px-4 py-[9.5px] shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] dark:border-border dark:shadow-none dark:ring-1 dark:ring-white/10"
              :placeholder="inlineAppIdPlaceholder"
              maxlength="60"
            />
          </div>

          <!-- * APP Secret: 260px -->
          <div class="flex w-[260px] shrink-0 flex-col gap-1">
            <label
              class="flex items-center gap-1 text-sm font-medium leading-5 text-[#0a0a0a] dark:text-foreground"
            >
              <span>*</span>
              <span>{{ inlineAppSecretLabel }}</span>
            </label>
            <Input
              v-model="inlineFormAppSecret"
              type="password"
              class="h-10 w-full rounded-lg border-[#e5e5e5] px-4 py-[9.5px] shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] dark:border-border dark:shadow-none dark:ring-1 dark:ring-white/10"
              :placeholder="inlineAppSecretPlaceholder"
              maxlength="60"
            />
          </div>
        </div>

        <!-- Button row: 验证配置 | 保存添加 | 配置步骤, gap 12px -->
        <div class="flex items-center gap-3">
          <Button
            type="button"
            class="h-10 gap-2 bg-[#f5f5f5] px-6 text-[#171717] hover:bg-[#e5e5e5] dark:bg-muted dark:text-foreground dark:hover:bg-muted/80"
            :disabled="inlineFormSaving || inlineFormVerifying || !isInlineFormValid"
            @click="handleInlineVerify"
          >
            <LoaderCircle v-if="inlineFormVerifying" class="h-4 w-4 shrink-0 animate-spin" />
            <Check v-else class="h-4 w-4 shrink-0" />
            {{
              inlineFormVerifying
                ? t('channels.inline.verifying')
                : t('channels.inline.verifyConfig')
            }}
          </Button>
          <Button
            class="h-10 gap-2 bg-[#171717] px-6 text-white hover:bg-[#171717]/90 dark:bg-primary dark:text-primary-foreground dark:hover:bg-primary/90"
            :disabled="inlineFormSaving || inlineFormVerifying || !isInlineFormValid"
            @click="handleInlineSave"
          >
            <Plus class="h-4 w-4 shrink-0" />
            {{ t('channels.inline.save') }}
          </Button>
          <Button
            variant="outline"
            class="h-10 border-[#d4d4d4] px-6 shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] dark:border-border dark:shadow-none dark:ring-1 dark:ring-white/10"
            @click="openPlatformDocs"
          >
            {{ t('channels.inline.configSteps') }}
          </Button>
        </div>
      </div>
    </div>

    <!-- Add Channel Dialog -->
    <AddChannelDialog
      v-model:open="addDialogOpen"
      :platforms="platforms"
      @select="handleSelectPlatform"
    />

    <!-- Config Channel Dialog -->
    <ConfigChannelDialog
      v-model:open="configDialogOpen"
      :platform="selectedPlatform"
      :channel="channelToEdit"
      @saved="handleConfigSaved"
      @update:open="
        (val) => {
          if (!val) channelToEdit = null
        }
      "
    />

    <!-- Bind Agent Dialog -->
    <BindAgentDialog
      v-model:open="bindDialogOpen"
      :from-create="bindFromCreate"
      :current-agent-id="channelToBind?.agent_id ?? null"
      @bind="handleBindAgent"
      @auto-generate="handleAutoGenerate"
    />

    <!-- Toggle Confirmation -->
    <AlertDialog
      :open="toggleDialogOpen"
      @update:open="
        (val) => {
          if (!val) cancelToggle()
        }
      "
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{
            channelToToggle?.val
              ? t('channels.toggle.enableTitle')
              : t('channels.toggle.disableTitle')
          }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{
              channelToToggle?.val
                ? t('channels.toggle.enableDesc')
                : t('channels.toggle.disableDesc')
            }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel @click="cancelToggle">{{ t('common.cancel') }}</AlertDialogCancel>
          <Button
            class="bg-primary text-primary-foreground hover:bg-primary/90"
            @click="confirmToggle"
          >
            {{ t('common.confirm') }}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <!-- Delete Confirmation -->
    <AlertDialog v-model:open="deleteDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('channels.delete.title') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('channels.delete.desc', { name: channelToDelete?.name }) }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction
            class="bg-foreground text-background hover:bg-foreground/90"
            @click="handleDelete"
          >
            {{ t('common.delete') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
