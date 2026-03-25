<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Dialogs } from '@wailsio/runtime'
import {
  Check,
  Link2,
  LoaderCircle,
  MoreHorizontal,
  Plus,
  ShieldCheck,
  Unlink,
  Edit,
} from 'lucide-vue-next'
import {
  OpenClawAgentsService,
  type OpenClawAgent,
} from '@bindings/chatclaw/internal/services/openclawagents'
import { ChannelService, UpdateChannelInput } from '@bindings/chatclaw/internal/services/channels'
import type { Channel, PlatformMeta } from '@bindings/chatclaw/internal/services/channels'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import ConfigChannelDialog from '@/pages/channels/components/ConfigChannelDialog.vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { platformIconMap } from '@/assets/icons/snap/platformIcons'
import { getPlatformDocsUrl, openExternalLink } from '@/pages/channels/platformDocs'

const props = defineProps<{
  agent: OpenClawAgent | null
}>()

const open = defineModel<boolean>('open', { required: true })

const { t, te } = useI18n()

/** Platforms that support create/bind in UI (feishu + wecom + dingtalk). */
function isChannelPlatformSelectable(platformId: string) {
  return platformId === 'feishu' || platformId === 'wecom' || platformId === 'dingtalk'
}

const channels = ref<Channel[]>([])
const platforms = ref<PlatformMeta[]>([])
const agents = ref<OpenClawAgent[]>([])
const loading = ref(false)
const actionLoadingId = ref<number | null>(null)
const selectedPlatformId = ref('')
const showCreateForm = ref(false)

const inlineFormName = ref('')
const inlineFormAvatar = ref('')
const inlineFormAppId = ref('')
const inlineFormAppSecret = ref('')
const inlineFormToken = ref('')
const inlineFormSaving = ref(false)
const inlineFormVerifying = ref(false)

const showAddBotDialog = ref(false)
const selectedBotId = ref<number | null>(null)
const addBotLoading = ref(false)
const showConfigChannelDialog = ref(false)
const channelToEdit = ref<Channel | null>(null)

const currentAgentId = computed(() => props.agent?.id ?? 0)
const currentAgentName = computed(() => props.agent?.name || t('assistant.channels.currentAgent'))

const selectedPlatformMeta = computed(() => {
  return platforms.value.find((platform) => platform.id === selectedPlatformId.value) ?? null
})

const selectedPlatformChannels = computed(() => {
  if (!selectedPlatformId.value) return []

  const currentId = currentAgentId.value
  // Only show channels bound to the current agent
  return channels.value.filter(
    (channel) => channel.platform === selectedPlatformId.value && channel.agent_id === currentId
  )
})

const isFeishu = computed(() => selectedPlatformMeta.value?.id === 'feishu')
const isDingTalk = computed(() => selectedPlatformMeta.value?.id === 'dingtalk')
const isWeCom = computed(() => selectedPlatformMeta.value?.id === 'wecom')
const inlineAppIdLabel = computed(() => (isWeCom.value ? 'Bot ID' : t('channels.config.appId')))
const inlineAppSecretLabel = computed(() =>
  isWeCom.value ? 'Secret' : t('channels.config.appSecret')
)
const inlineAppIdPlaceholder = computed(() =>
  isWeCom.value ? t('channels.config.wecomAppIdPlaceholder') : t('channels.config.appIdPlaceholder')
)
const inlineAppSecretPlaceholder = computed(() =>
  isWeCom.value
    ? t('channels.config.wecomAppSecretPlaceholder')
    : t('channels.config.appSecretPlaceholder')
)

const isInlineFormValid = computed(() => {
  if (!inlineFormName.value.trim()) return false
  return !!(inlineFormAppId.value.trim() && inlineFormAppSecret.value.trim())
})

const shouldShowCreateForm = computed(() => {
  if (!selectedPlatformMeta.value) return false
  return showCreateForm.value || selectedPlatformChannels.value.length === 0
})

const unboundPlatformChannels = computed(() => {
  if (!selectedPlatformId.value) return []
  return channels.value.filter(
    (channel) => channel.platform === selectedPlatformId.value && channel.agent_id === 0
  )
})

function resetInlineForm() {
  inlineFormName.value = ''
  inlineFormAvatar.value = ''
  inlineFormAppId.value = ''
  inlineFormAppSecret.value = ''
  inlineFormToken.value = ''
}

function getPlatformIcon(platformId: string): string | null {
  return platformIconMap[platformId] || null
}

function getPlatformDisplayName(platformId: string, fallbackName?: string): string {
  const key = `channels.platforms.${platformId}`
  if (te(key)) return t(key)
  return fallbackName || platformId
}

function getPlatformDescription(platformId: string): string {
  const key = `channels.meta.${platformId}.description`
  if (te(key)) {
    return t(key)
  }
  return t('assistant.channels.createDesc')
}

function getAppId(extraConfig: string): string {
  try {
    const config = JSON.parse(extraConfig)
    return config.app_id || config.token || 'N/A'
  } catch {
    return 'N/A'
  }
}

function getAgentName(agentId: number): string {
  if (!agentId) return t('assistant.channels.unbound')
  return (
    agents.value.find((agent) => agent.id === agentId)?.name || t('assistant.channels.unknownAgent')
  )
}

function getBindStatusText(channel: Channel): string {
  if (channel.agent_id === currentAgentId.value) return t('assistant.channels.boundCurrent')
  if (channel.agent_id === 0) return t('assistant.channels.unbound')
  return t('assistant.channels.boundOther')
}

function getBindActionText(channel: Channel): string {
  if (channel.agent_id === 0) return t('assistant.channels.bindCurrent')
  if (channel.agent_id === currentAgentId.value) return t('assistant.channels.unbindCurrent')
  return t('assistant.channels.rebindCurrent')
}

function getChannelStatusText(channel: Channel): string {
  if (channel.status === 'online') return t('assistant.channels.statusOnline', '已连接')
  if (channel.status === 'error') return t('assistant.channels.statusError', '错误')
  return t('assistant.channels.statusOffline', '未连接')
}

function syncCreateFormVisibility(platformId: string) {
  // Show create form if current agent has no bound channels on this platform
  const hasBoundChannels = channels.value.some(
    (channel) => channel.platform === platformId && channel.agent_id === currentAgentId.value
  )
  showCreateForm.value = !hasBoundChannels
}

function handleSelectPlatform(platformId: string) {
  selectedPlatformId.value = platformId
  syncCreateFormVisibility(platformId)
}

async function loadData() {
  if (!open.value || !props.agent) return

  loading.value = true
  try {
    const [channelList, platformList, agentList] = await Promise.all([
      ChannelService.ListChannels(),
      ChannelService.GetSupportedPlatforms(),
      OpenClawAgentsService.ListAgents(),
    ])

    channels.value = channelList || []
    platforms.value = platformList || []
    agents.value = agentList || []

    const selectableIds = ['feishu', 'wecom', 'dingtalk', 'qq']
    const hasSelectedPlatform = platforms.value.some(
      (platform) => platform.id === selectedPlatformId.value
    )
    const currentIsSelectable = selectableIds.includes(selectedPlatformId.value)
    if (!hasSelectedPlatform || !currentIsSelectable) {
      selectedPlatformId.value =
        platforms.value.find((p) => selectableIds.includes(p.id))?.id ||
        platforms.value[0]?.id ||
        ''
    }
    if (selectedPlatformId.value) {
      syncCreateFormVisibility(selectedPlatformId.value)
    }
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

watch(open, (value) => {
  if (value) {
    resetInlineForm()
    void loadData()
    return
  }

  showCreateForm.value = false
  actionLoadingId.value = null
  resetInlineForm()
})

watch(
  () => props.agent?.id,
  () => {
    if (open.value) {
      void loadData()
    }
  }
)

async function handlePickAvatar() {
  if (inlineFormSaving.value) return

  try {
    const path = await Dialogs.OpenFile({
      CanChooseFiles: true,
      CanChooseDirectories: false,
      AllowsMultipleSelection: false,
      Title: t('channels.config.pickAvatar', '选择头像'),
      Filters: [
        {
          DisplayName: t('channels.config.filterImages', '图片文件'),
          Pattern: '*.png;*.jpg;*.jpeg;*.gif;*.webp;*.svg',
        },
      ],
    })

    if (!path) return
    inlineFormAvatar.value = await OpenClawAgentsService.ReadIconFile(path)
  } catch (error) {
    if (String(error).includes('cancelled by user')) return
    console.error('Failed to pick icon:', error)
  }
}

async function handleCreateChannel() {
  if (!props.agent || !selectedPlatformMeta.value || !isInlineFormValid.value) return

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
      openclaw_scope: false,
    })

    if (channel) {
      await ChannelService.BindAgent(channel.id, props.agent.id)
    }

    toast.success(t('assistant.channels.createAndBindSuccess'))
    resetInlineForm()
    await loadData()
    showCreateForm.value = false
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    inlineFormSaving.value = false
  }
}

async function handleBindChannel(channel: Channel) {
  if (!props.agent) return

  actionLoadingId.value = channel.id
  try {
    await ChannelService.BindAgent(channel.id, props.agent.id)
    toast.success(t('assistant.channels.bindSuccess'))
    await loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    actionLoadingId.value = null
  }
}

async function handleUnbindChannel(channel: Channel) {
  actionLoadingId.value = channel.id
  try {
    await ChannelService.UnbindAgent(channel.id)
    toast.success(t('assistant.channels.unbindSuccess'))
    await loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    actionLoadingId.value = null
  }
}

async function handleToggleChannel(channel: Channel, enabled: boolean) {
  actionLoadingId.value = channel.id
  try {
    await ChannelService.UpdateChannel(channel.id, new UpdateChannelInput({ enabled }))
    if (enabled) {
      await ChannelService.ConnectChannel(channel.id)
      toast.success(t('channels.connect.success', '连接成功'))
    } else {
      await ChannelService.DisconnectChannel(channel.id)
      toast.success(t('channels.disconnect.success', '已断开连接'))
    }
    await loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    actionLoadingId.value = null
  }
}

function isSelectableChannelPlatform(platformId: string) {
  return (
    platformId === 'feishu' ||
    platformId === 'wecom' ||
    platformId === 'dingtalk' ||
    platformId === 'qq'
  )
}

function openPlatformDocs() {
  const url = getPlatformDocsUrl(selectedPlatformMeta.value?.id)
  void openExternalLink(url)
}

async function handleInlineVerify() {
  if (!isInlineFormValid.value) {
    toast.error(t('channels.inline.fillRequired', '请先填写必填项'))
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
    toast.success(t('channels.inline.verifySuccess', '验证通过'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('channels.inline.verifyFailed', '验证失败'))
  } finally {
    inlineFormVerifying.value = false
  }
}

function handleOpenAddBotDialog() {
  selectedBotId.value = null
  showAddBotDialog.value = true
}

function handleEditChannel(channel: Channel) {
  channelToEdit.value = channel
  showConfigChannelDialog.value = true
}

async function handleAddBotConfirm() {
  if (!props.agent || !selectedBotId.value) {
    toast.error(t('assistant.channels.selectBot'))
    return
  }

  addBotLoading.value = true
  try {
    await ChannelService.BindAgent(selectedBotId.value, props.agent.id)
    toast.success(t('assistant.channels.bindSuccess'))
    showAddBotDialog.value = false
    selectedBotId.value = null
    await loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    addBotLoading.value = false
  }
}

async function handleConfigChannelSaved(channel: Channel, isEdit: boolean) {
  if (!props.agent) return

  try {
    if (!isEdit) {
      await ChannelService.BindAgent(channel.id, props.agent.id)
      toast.success(t('assistant.channels.createAndBindSuccess'))
    }
    channelToEdit.value = null
    await loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent
      class="h-[654px] w-[calc(100%-2rem)] max-w-[calc(100%-2rem)] gap-0 overflow-hidden p-0 sm:w-[720px] sm:max-w-[720px]"
    >
      <DialogHeader class="sr-only">
        <DialogTitle>{{ t('assistant.channels.title') }}</DialogTitle>
        <DialogDescription>
          {{ t('assistant.channels.subtitle', { name: currentAgentName }) }}
        </DialogDescription>
      </DialogHeader>

      <div class="flex h-full flex-col bg-white dark:bg-background">
        <div
          class="flex h-14 shrink-0 items-center justify-between border-b border-[#d4d4d4] px-4 dark:border-border"
        >
          <h2 class="text-xl font-semibold leading-6 text-[#0a0a0a] dark:text-foreground">
            {{ t('assistant.menu.channels') }}
          </h2>
        </div>

        <div class="flex min-h-0 flex-1">
          <aside class="flex w-40 shrink-0 flex-col border-r border-[#d4d4d4] dark:border-border">
            <div class="flex flex-col gap-1 px-4 py-2">
              <button
                v-for="platform in platforms"
                :key="platform.id"
                type="button"
                :class="
                  cn(
                    'flex h-8 items-center rounded-md px-3 text-left text-sm text-[#404040] transition-colors dark:text-muted-foreground',
                    selectedPlatformId === platform.id &&
                      'bg-[#f5f5f5] text-[#171717] dark:bg-muted dark:text-foreground',
                    selectedPlatformId !== platform.id &&
                      isSelectableChannelPlatform(platform.id) &&
                      'hover:bg-[#f5f5f5]/70 dark:hover:bg-muted/60',
                    !isSelectableChannelPlatform(platform.id) && 'opacity-50 cursor-not-allowed'
                  )
                "
                @click="
                  isSelectableChannelPlatform(platform.id)
                    ? handleSelectPlatform(platform.id)
                    : toast.default(t('channels.comingSoon'))
                "
              >
                <span class="truncate">{{
                  getPlatformDisplayName(platform.id, platform.name)
                }}</span>
              </button>
            </div>
          </aside>

          <section class="flex min-w-0 flex-1 flex-col">
            <div class="border-[#d9d9d9] px-4 pt-4 pb-0 dark:border-border">
              <div
                v-if="selectedPlatformMeta"
                class="rounded-[16px] border border-[#d9d9d9] bg-white p-4 shadow-sm dark:border-border dark:bg-card dark:shadow-none dark:ring-1 dark:ring-white/10"
              >
                <div class="flex items-start justify-between gap-4">
                  <div class="min-w-0 flex-1">
                    <h3
                      class="text-base font-semibold leading-6 text-[#262626] dark:text-foreground"
                    >
                      {{
                        getPlatformDisplayName(selectedPlatformMeta.id, selectedPlatformMeta.name)
                      }}
                    </h3>
                    <p class="mt-1 text-sm leading-5 text-[#737373] dark:text-muted-foreground">
                      {{ getPlatformDescription(selectedPlatformMeta.id) }}
                    </p>
                  </div>

                  <div class="flex shrink-0 items-center gap-2">
                    <Button
                      size="icon"
                      variant="ghost"
                      class="h-6 w-6 rounded-sm p-0 text-[#171717] hover:bg-[#f5f5f5] dark:text-foreground dark:hover:bg-muted"
                      :disabled="!selectedPlatformMeta"
                      @click="handleOpenAddBotDialog"
                    >
                      <Plus class="size-4" />
                    </Button>
                  </div>
                </div>
              </div>
            </div>

            <div class="flex-1 overflow-y-auto px-4 py-4">
              <div
                v-if="loading"
                class="flex items-center justify-center py-16 text-sm text-muted-foreground"
              >
                {{ t('common.loading', '加载中...') }}
              </div>

              <div
                v-else-if="!selectedPlatformMeta"
                class="flex items-center justify-center rounded-[16px] border border-dashed border-border px-6 py-12 text-sm text-muted-foreground"
              >
                {{ t('assistant.channels.noPlatforms') }}
              </div>

              <div v-else-if="shouldShowCreateForm">
                <div
                  class="rounded-[16px] border border-[#d9d9d9] bg-white p-4 shadow-sm dark:border-border dark:bg-card dark:shadow-none dark:ring-1 dark:ring-white/10"
                >
                  <div class="mb-6 flex flex-col items-center gap-2 pt-2">
                    <button
                      type="button"
                      class="flex h-[62px] w-[62px] items-center justify-center overflow-hidden rounded-[14px] border border-[#d9d9d9] bg-white shadow-sm transition-opacity hover:opacity-80 dark:border-border dark:bg-muted dark:shadow-none dark:ring-1 dark:ring-white/10"
                      @click="handlePickAvatar"
                    >
                      <img
                        v-if="inlineFormAvatar"
                        :src="inlineFormAvatar"
                        :alt="
                          inlineFormName ||
                          getPlatformDisplayName(selectedPlatformMeta.id, selectedPlatformMeta.name)
                        "
                        class="h-full w-full object-cover"
                      />
                      <img
                        v-else-if="getPlatformIcon(selectedPlatformMeta.id)"
                        :src="getPlatformIcon(selectedPlatformMeta.id)!"
                        :alt="
                          getPlatformDisplayName(selectedPlatformMeta.id, selectedPlatformMeta.name)
                        "
                        class="h-8 w-8 object-contain"
                      />
                      <Plus v-else class="size-6 text-[#595959] dark:text-muted-foreground" />
                    </button>
                    <p
                      class="text-center text-sm leading-[22px] text-[#8c8c8c] dark:text-muted-foreground"
                    >
                      {{ t('assistant.icon.hint') }}
                    </p>
                  </div>

                  <div class="space-y-4">
                    <div class="flex flex-col gap-1">
                      <label
                        class="text-sm font-medium leading-5 text-[#0a0a0a] dark:text-foreground"
                      >
                        * {{ t('assistant.fields.name', '名称') }}
                      </label>
                      <Input
                        v-model="inlineFormName"
                        class="h-9 rounded-lg border-[#e5e5e5] px-3 shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] dark:border-border dark:shadow-none dark:ring-1 dark:ring-white/10"
                        :placeholder="t('channels.inline.namePlaceholder', '请输入')"
                      />
                    </div>

                    <div class="flex flex-col gap-1">
                      <label
                        class="text-sm font-medium leading-5 text-[#0a0a0a] dark:text-foreground"
                      >
                        * {{ inlineAppIdLabel }}
                      </label>
                      <Input
                        v-model="inlineFormAppId"
                        class="h-9 rounded-lg border-[#e5e5e5] px-3 shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] dark:border-border dark:shadow-none dark:ring-1 dark:ring-white/10"
                        :placeholder="inlineAppIdPlaceholder"
                      />
                    </div>

                    <div class="flex flex-col gap-1">
                      <label
                        class="text-sm font-medium leading-5 text-[#0a0a0a] dark:text-foreground"
                      >
                        * {{ inlineAppSecretLabel }}
                      </label>
                      <Input
                        v-model="inlineFormAppSecret"
                        type="password"
                        class="h-9 rounded-lg border-[#e5e5e5] px-3 shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] dark:border-border dark:shadow-none dark:ring-1 dark:ring-white/10"
                        :placeholder="inlineAppSecretPlaceholder"
                      />
                    </div>
                  </div>

                  <div class="mt-4 flex items-center gap-2">
                    <Button
                      type="button"
                      class="h-10 gap-2 rounded-lg bg-[#f5f5f5] px-6 text-[#171717] hover:bg-[#e5e5e5] dark:bg-muted dark:text-foreground dark:hover:bg-muted/80"
                      :disabled="inlineFormSaving || inlineFormVerifying || !isInlineFormValid"
                      @click="handleInlineVerify"
                    >
                      <LoaderCircle
                        v-if="inlineFormVerifying"
                        class="size-4 shrink-0 animate-spin"
                      />
                      <ShieldCheck v-else class="size-4 shrink-0" />
                      {{
                        inlineFormVerifying
                          ? t('channels.inline.verifying', '验证中…')
                          : t('channels.inline.verifyConfig', '验证配置')
                      }}
                    </Button>

                    <Button
                      class="h-10 gap-2 rounded-lg bg-[#171717] px-6 text-white hover:bg-[#171717]/90 dark:bg-primary dark:text-primary-foreground dark:hover:bg-primary/90"
                      :disabled="inlineFormSaving || inlineFormVerifying || !isInlineFormValid"
                      @click="handleCreateChannel"
                    >
                      <Plus class="size-4 shrink-0" />
                      {{ t('channels.inline.save', '保存添加') }}
                    </Button>

                    <Button
                      variant="outline"
                      class="h-10 rounded-lg border-[#d4d4d4] bg-white px-6 shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] dark:border-border dark:bg-transparent dark:shadow-none dark:ring-1 dark:ring-white/10"
                      @click="openPlatformDocs"
                    >
                      {{ t('channels.inline.configSteps', '配置步骤') }}
                    </Button>
                  </div>
                </div>
              </div>

              <div v-else class="space-y-4">
                <div
                  v-for="channel in selectedPlatformChannels"
                  :key="channel.id"
                  class="rounded-[16px] border border-[#d9d9d9] bg-white p-4 shadow-sm dark:border-border dark:bg-card dark:shadow-none dark:ring-1 dark:ring-white/10"
                >
                  <div class="flex items-start justify-between gap-3">
                    <div class="min-w-0 flex-1">
                      <div class="flex min-w-0 items-start gap-2">
                        <div
                          class="flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded-[4px] border border-[#d9d9d9] bg-white dark:border-border dark:bg-muted"
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
                          <span v-else class="text-[10px] text-muted-foreground">AI</span>
                        </div>
                        <p
                          class="min-w-0 truncate text-sm leading-[22px] text-[#171717] dark:text-foreground"
                        >
                          {{ channel.name }}
                        </p>
                      </div>

                      <p class="mt-2 text-xs leading-5 text-[#8c8c8c] dark:text-muted-foreground">
                        Appid: {{ getAppId(channel.extra_config) }}
                      </p>
                    </div>

                    <div class="flex shrink-0 items-center gap-2">
                      <Switch
                        :model-value="channel.enabled"
                        :disabled="actionLoadingId === channel.id || channel.agent_id === 0"
                        @update:model-value="
                          (value: boolean) => handleToggleChannel(channel, value)
                        "
                      />

                      <DropdownMenu>
                        <DropdownMenuTrigger as-child>
                          <Button
                            variant="ghost"
                            size="icon"
                            class="h-6 w-6 rounded-sm bg-transparent p-0 text-[#171717] hover:bg-[#f5f5f5] dark:text-foreground dark:hover:bg-muted"
                            :disabled="actionLoadingId === channel.id"
                          >
                            <MoreHorizontal class="size-4" />
                          </Button>
                        </DropdownMenuTrigger>

                        <DropdownMenuContent
                          align="end"
                          class="min-w-32 rounded-md bg-white p-0.5 shadow-[0_8px_10px_-5px_rgba(0,0,0,0.08),0_16px_24px_2px_rgba(0,0,0,0.04),0_6px_30px_5px_rgba(0,0,0,0.05)] dark:bg-popover"
                        >
                          <DropdownMenuItem
                            class="gap-2 rounded px-4 py-[5px]"
                            @click="handleEditChannel(channel)"
                          >
                            <Edit class="size-4" />
                            {{ t('common.edit', '编辑') }}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            class="gap-2 rounded px-4 py-[5px]"
                            @click="
                              channel.agent_id === currentAgentId
                                ? handleUnbindChannel(channel)
                                : handleBindChannel(channel)
                            "
                          >
                            <Unlink v-if="channel.agent_id === currentAgentId" class="size-4" />
                            <Link2 v-else class="size-4" />
                            {{ getBindActionText(channel) }}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </div>

                  <div class="mt-2 flex flex-wrap items-center gap-2">
                    <div
                      class="inline-flex items-center gap-1.5 rounded-full bg-[#f0f0f0] px-2 py-0.5 dark:bg-muted"
                    >
                      <div
                        class="h-2 w-2 rounded-full"
                        :class="{
                          'bg-green-500': channel.status === 'online',
                          'bg-red-500': channel.status === 'error',
                          'bg-gray-400': channel.status === 'offline' || !channel.status,
                        }"
                      />
                      <span class="text-xs leading-4 text-[#595959] dark:text-muted-foreground">
                        {{ getChannelStatusText(channel) }}
                      </span>
                    </div>

                    <div
                      class="inline-flex items-center gap-1 rounded-full bg-[#f0f0f0] px-2 py-0.5 dark:bg-muted"
                    >
                      <Check
                        v-if="channel.agent_id !== 0"
                        class="size-3.5 text-[#595959] dark:text-muted-foreground"
                      />
                      <Unlink v-else class="size-3.5 text-[#595959] dark:text-muted-foreground" />
                      <span class="text-xs leading-4 text-[#595959] dark:text-muted-foreground">
                        {{ getBindStatusText(channel) }}
                      </span>
                    </div>

                    <div
                      v-if="channel.agent_id !== 0"
                      class="inline-flex items-center rounded-full bg-[#f0f0f0] px-2 py-0.5 dark:bg-muted"
                    >
                      <span class="text-xs leading-4 text-[#595959] dark:text-muted-foreground">
                        {{ getAgentName(channel.agent_id) }}
                      </span>
                    </div>
                  </div>
                </div>

                <div
                  v-if="selectedPlatformChannels.length === 0"
                  class="flex items-center justify-center rounded-[16px] border border-dashed border-border px-6 py-12 text-sm text-muted-foreground"
                >
                  {{ t('assistant.channels.emptyPlatform') }}
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>
    </DialogContent>
  </Dialog>

  <!-- Add Bot Dialog -->
  <Dialog v-model:open="showAddBotDialog">
    <DialogContent
      class="flex h-auto max-h-[85vh] w-[calc(100%-2rem)] max-w-[calc(100%-2rem)] flex-col gap-0 overflow-hidden p-0 sm:w-[480px] sm:max-w-[480px]"
    >
      <DialogHeader class="sr-only">
        <DialogDescription>{{ t('assistant.channels.addBotHint') }}</DialogDescription>
      </DialogHeader>

      <!-- Header -->
      <div
        class="flex h-14 shrink-0 items-center justify-between border-b border-[#d4d4d4] px-6 dark:border-border"
      >
        <DialogTitle class="text-xl font-semibold leading-6 text-[#0a0a0a] dark:text-foreground">
          {{ t('assistant.channels.addBot') }}
        </DialogTitle>
      </div>

      <!-- Content -->
      <div class="flex min-h-0 flex-1 flex-col overflow-hidden bg-white p-6 dark:bg-background">
        <!-- Hint Alert -->
        <div
          class="mb-4 shrink-0 rounded-lg border border-[#e5e5e5] bg-white px-4 py-3 dark:border-border dark:bg-card"
        >
          <p class="text-sm font-medium leading-5 text-[#0a0a0a] dark:text-foreground">
            {{ t('assistant.channels.addBotHint') }}
          </p>
        </div>

        <!-- Add Bot Button -->
        <Button
          class="mb-4 h-10 w-full shrink-0 gap-2 rounded-lg bg-[#171717] text-white hover:bg-[#171717]/90 dark:bg-primary dark:text-primary-foreground dark:hover:bg-primary/90"
          :disabled="addBotLoading"
          @click="showConfigChannelDialog = true; showAddBotDialog = false"
        >
          <Plus class="size-4 shrink-0" />
          {{ t('assistant.channels.addBot') }}
        </Button>

        <!-- Bot List -->
        <div class="min-h-0 flex-1 space-y-3 overflow-y-auto">
          <div
            v-if="unboundPlatformChannels.length === 0"
            class="flex items-center justify-center rounded-[16px] border border-dashed border-border px-6 py-12 text-sm text-muted-foreground"
          >
            {{ t('assistant.channels.noUnboundBot') }}
          </div>

          <button
            v-for="channel in unboundPlatformChannels"
            :key="channel.id"
            type="button"
            :class="[
              'flex w-full items-center justify-between rounded-[16px] border p-4 text-left transition-colors',
              selectedBotId === channel.id
                ? 'border-[#171717] bg-white shadow-sm dark:border-foreground dark:bg-card'
                : 'border-[#d4d4d4] bg-white shadow-sm hover:border-[#a3a3a3] dark:border-border dark:bg-card dark:hover:border-muted-foreground',
            ]"
            @click="selectedBotId = channel.id"
          >
            <div class="flex min-w-0 flex-1 items-center gap-3">
              <div
                class="flex h-[62px] w-[62px] shrink-0 items-center justify-center overflow-hidden rounded-[14px] border border-[#d9d9d9] bg-white dark:border-border dark:bg-muted"
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
                  class="h-11 w-11 object-contain"
                />
                <span v-else class="text-lg text-muted-foreground">AI</span>
              </div>
              <div class="min-w-0 flex-1">
                <p class="truncate text-sm leading-[22px] text-[#171717] dark:text-foreground">
                  {{ channel.name }}
                </p>
                <p class="text-xs leading-5 text-[#8c8c8c] dark:text-muted-foreground">
                  Appid: {{ getAppId(channel.extra_config) }}
                </p>
              </div>
            </div>
            <div
              :class="[
                'flex h-4 w-4 shrink-0 items-center justify-center rounded-full border-2',
                selectedBotId === channel.id
                  ? 'border-[#171717] bg-[#171717] dark:border-foreground dark:bg-foreground'
                  : 'border-[#d9d9d9] bg-white dark:border-border dark:bg-transparent',
              ]"
            >
              <div
                v-if="selectedBotId === channel.id"
                class="h-1.5 w-1.5 rounded-full bg-white dark:bg-background"
              />
            </div>
          </button>
        </div>
      </div>

      <!-- Footer -->
      <div
        class="flex shrink-0 items-center justify-end gap-3 border-t border-[#d4d4d4] bg-white px-6 py-4 dark:border-border dark:bg-background"
      >
        <Button
          variant="outline"
          class="h-10 rounded-lg border-[#d4d4d4] bg-white px-6 shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] dark:border-border dark:bg-transparent dark:shadow-none dark:ring-1 dark:ring-white/10"
          @click="showAddBotDialog = false"
        >
          {{ t('common.cancel') }}
        </Button>
        <Button
          class="h-10 rounded-lg bg-[#171717] px-6 text-white hover:bg-[#171717]/90 dark:bg-primary dark:text-primary-foreground dark:hover:bg-primary/90"
          :disabled="addBotLoading || !selectedBotId"
          @click="handleAddBotConfirm"
        >
          {{ t('common.confirm') }}
        </Button>
      </div>
    </DialogContent>
  </Dialog>

  <!-- Config Channel Dialog -->
  <ConfigChannelDialog
    v-model:open="showConfigChannelDialog"
    :platform="selectedPlatformMeta"
    :channel="channelToEdit"
    @saved="handleConfigChannelSaved"
    @update:open="
      (val) => {
        if (!val) channelToEdit = null
      }
    "
  />
</template>
