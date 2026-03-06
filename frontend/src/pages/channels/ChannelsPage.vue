<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, Trash2, MoreHorizontal, Unlink, BadgeCheck, RouteOff, SquareDashed } from 'lucide-vue-next'
import IconChannels from '@/assets/icons/channelsMax.svg'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
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
import { ChannelService } from '@bindings/chatclaw/internal/services/channels'
import type { Channel, ChannelStats, PlatformMeta } from '@bindings/chatclaw/internal/services/channels'

defineProps<{ tabId: string }>()

const { t } = useI18n()

const channels = ref<Channel[]>([])
const stats = ref<ChannelStats>({ total: 0, connected: 0, disconnected: 0 })
const platforms = ref<PlatformMeta[]>([])
const loading = ref(false)
const addDialogOpen = ref(false)
const configDialogOpen = ref(false)
const selectedPlatform = ref<PlatformMeta | null>(null)
const deleteDialogOpen = ref(false)
const channelToDelete = ref<Channel | null>(null)

const selectedFilter = ref<string>('all')

const filteredChannels = computed(() => {
  if (selectedFilter.value === 'all') return channels.value
  return channels.value.filter((ch) => ch.platform === selectedFilter.value)
})

const platformIconMap: Record<string, string> = {
  dingtalk: '/src/assets/icons/snap/dingtalk.svg',
  feishu: '/src/assets/icons/snap/feishu.svg',
  wecom: '/src/assets/icons/snap/wechat.svg',
  qq: '/src/assets/icons/snap/qq.svg',
  twitter: '/src/assets/icons/snap/twitter.svg',
}

async function loadData() {
  loading.value = true
  try {
    const [channelList, channelStats, platformList] = await Promise.all([
      ChannelService.ListChannels(),
      ChannelService.GetChannelStats(),
      ChannelService.GetSupportedPlatforms(),
    ])
    channels.value = channelList || []
    stats.value = channelStats || { total: 0, connected: 0, disconnected: 0 }
    platforms.value = platformList || []
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

function handleAddChannel() {
  addDialogOpen.value = true
}

function handleSelectPlatform(platform: PlatformMeta) {
  selectedPlatform.value = platform
  addDialogOpen.value = false
  configDialogOpen.value = true
}

function handleConfigSaved() {
  configDialogOpen.value = false
  selectedPlatform.value = null
  loadData()
}

function confirmDelete(channel: Channel) {
  channelToDelete.value = channel
  deleteDialogOpen.value = true
}

async function handleDelete() {
  if (!channelToDelete.value) return
  try {
    await ChannelService.DeleteChannel(channelToDelete.value.id)
    toast.success(t('channels.delete.success', '删除成功'))
    loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    deleteDialogOpen.value = false
    channelToDelete.value = null
  }
}

async function handleConnect(channel: Channel) {
  try {
    await ChannelService.ConnectChannel(channel.id)
    toast.success(t('channels.connect.success', '连接成功'))
    loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
    // Re-load to reset the switch if failed
    loadData()
  }
}

async function handleDisconnect(channel: Channel) {
  try {
    await ChannelService.DisconnectChannel(channel.id)
    toast.success(t('channels.disconnect.success', '断开成功'))
    loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
    loadData()
  }
}

async function handleToggleConnection(channel: Channel, val: boolean) {
  if (val) {
    await handleConnect(channel)
  } else {
    await handleDisconnect(channel)
  }
}

async function handleUnbind(channel: Channel) {
  try {
    await ChannelService.UnbindAgent(channel.id)
    toast.success('已解绑助手')
    loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  }
}

function getPlatformIcon(platformId: string): string | null {
  return platformIconMap[platformId] || null
}

function getPlatformName(platformId: string): string {
  const platform = platforms.value.find(p => p.id === platformId)
  return platform?.name || platformId
}

function getAppId(extraConfig: string): string {
  try {
    const config = JSON.parse(extraConfig)
    return config.app_id || config.token || 'N/A'
  } catch {
    return 'N/A'
  }
}

onMounted(loadData)
</script>

<template>
  <div class="flex h-full flex-col overflow-y-auto bg-white dark:bg-background">
    <!-- Page Header -->
    <div class="flex h-20 shrink-0 items-center justify-between px-6">
      <div class="flex flex-col gap-1">
        <h1 class="text-base font-semibold text-[#262626] dark:text-foreground">{{ t('channels.title', '频道') }}</h1>
        <p class="text-sm text-[#737373] dark:text-muted-foreground">{{ t('channels.subtitle', '管理您的消息频道和连接') }}</p>
      </div>
      <Button 
        class="h-9 bg-[#f5f5f5] text-[#171717] hover:bg-[#e5e5e5] border-none shadow-none dark:bg-muted dark:text-foreground dark:hover:bg-muted/80" 
        @click="handleAddChannel"
      >
        <Plus class="mr-1.5 h-4 w-4" />
        {{ t('channels.addChannel', '添加频道') }}
      </Button>
    </div>

    <div class="flex-1 overflow-y-auto px-6 pb-6">
      <!-- Stats Cards Row -->
      <div class="mb-6 flex flex-wrap gap-4">
        <!-- Card 1: Total -->
        <div class="flex h-[102px] w-[222px] items-center gap-4 rounded-[16px] border border-[#d9d9d9] bg-white px-6 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10 dark:border-border dark:bg-card">
          <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-[#f5f5f5] dark:bg-muted">
            <IconChannels class="h-6 w-6 text-[#171717] dark:text-foreground" />
          </div>
          <div class="flex flex-col gap-1">
            <span class="text-2xl font-semibold leading-none tracking-tight text-[#171717] dark:text-foreground">{{ stats.total }}</span>
            <span class="text-sm text-[#737373] dark:text-muted-foreground">{{ t('channels.stats.total', '频道总数') }}</span>
          </div>
        </div>
        <!-- Card 2: Connected -->
        <div class="flex h-[102px] w-[222px] items-center gap-4 rounded-[16px] border border-[#d9d9d9] bg-white px-6 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10 dark:border-border dark:bg-card">
          <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-[#f5f5f5] dark:bg-muted">
            <BadgeCheck class="h-6 w-6 text-[#171717] dark:text-foreground" />
          </div>
          <div class="flex flex-col gap-1">
            <span class="text-2xl font-semibold leading-none tracking-tight text-[#171717] dark:text-foreground">{{ stats.connected }}</span>
            <span class="text-sm text-[#737373] dark:text-muted-foreground">{{ t('channels.stats.connected', '已连接') }}</span>
          </div>
        </div>
        <!-- Card 3: Disconnected -->
        <div class="flex h-[102px] w-[222px] items-center gap-4 rounded-[16px] border border-[#d9d9d9] bg-white px-6 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10 dark:border-border dark:bg-card">
          <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-[#f5f5f5] dark:bg-muted">
            <RouteOff class="h-6 w-6 text-[#171717] dark:text-foreground" />
          </div>
          <div class="flex flex-col gap-1">
            <span class="text-2xl font-semibold leading-none tracking-tight text-[#171717] dark:text-foreground">{{ stats.disconnected }}</span>
            <span class="text-sm text-[#737373] dark:text-muted-foreground">{{ t('channels.stats.disconnected', '未连接') }}</span>
          </div>
        </div>
      </div>

      <!-- Section Header -->
      <h2 class="mb-2 text-base font-semibold text-[#262626] dark:text-foreground">{{ t('channels.available', '可用频道') }}</h2>

      <!-- Platform Filter Tabs -->
      <div class="mb-4 inline-flex overflow-x-auto rounded-lg border border-[#e5e5e5] bg-[rgba(0,0,0,0.05)] shadow-[0_1px_2px_0_rgba(0,0,0,0.05)] dark:border-border dark:bg-muted/50">
        <button
          class="px-3 py-[7.5px] text-sm font-medium transition-colors first:rounded-l-lg last:rounded-r-lg"
          :class="selectedFilter === 'all' ? 'bg-white text-[#0a0a0a] dark:bg-background dark:text-foreground' : 'text-[#0a0a0a] hover:bg-white/50 dark:text-foreground dark:hover:bg-background/50'"
          @click="selectedFilter = 'all'"
        >
          {{ t('common.all', '全部') }}
        </button>
        <button
          v-for="platform in platforms"
          :key="platform.id"
          class="px-3 py-[7.5px] text-sm font-medium transition-colors first:rounded-l-lg last:rounded-r-lg border-l border-[#e5e5e5] dark:border-border"
          :class="selectedFilter === platform.id ? 'bg-white text-[#0a0a0a] dark:bg-background dark:text-foreground' : 'text-[#0a0a0a] hover:bg-white/50 dark:text-foreground dark:hover:bg-background/50'"
          @click="selectedFilter = platform.id"
        >
          {{ getPlatformName(platform.id) }}
        </button>
      </div>

      <!-- Channels Grid -->
      <div v-if="filteredChannels.length > 0" class="flex flex-wrap gap-4">
        <div
          v-for="channel in filteredChannels"
          :key="channel.id"
          class="flex w-[300px] flex-col gap-2 rounded-[16px] border border-[#d9d9d9] bg-white p-4 shadow-sm transition-all hover:border-[#171717] dark:shadow-none dark:ring-1 dark:ring-white/10 dark:border-border dark:bg-card dark:hover:border-primary/50"
        >
          <!-- Card Header -->
          <div class="flex items-center justify-between">
            <div class="flex flex-1 items-center gap-2">
              <div class="flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded border border-[#d9d9d9] bg-white dark:border-border dark:bg-muted">
                <img 
                  v-if="getPlatformIcon(channel.platform)" 
                  :src="getPlatformIcon(channel.platform)!" 
                  :alt="channel.platform"
                  class="h-3.5 w-3.5 object-contain"
                />
                <span v-else class="text-xs">🤖</span>
              </div>
              <span class="truncate text-sm text-[#171717] dark:text-foreground">{{ channel.name }}</span>
            </div>
            
            <div class="flex items-center gap-2">
              <Switch 
                :checked="channel.status === 'online'" 
                @update:checked="val => handleToggleConnection(channel, val)" 
              />
              <DropdownMenu>
                <DropdownMenuTrigger as-child>
                  <Button variant="ghost" size="icon" class="h-6 w-6 rounded bg-[#f5f5f5] dark:bg-muted hover:bg-[#e5e5e5] dark:hover:bg-muted/80">
                    <MoreHorizontal class="h-4 w-4 text-[#171717] dark:text-foreground" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" class="min-w-24 rounded-md bg-white p-0.5 shadow-[0_8px_10px_-5px_rgba(0,0,0,0.08),0_16px_24px_2px_rgba(0,0,0,0.04),0_6px_30px_5px_rgba(0,0,0,0.05)] dark:bg-popover">
                  <DropdownMenuItem class="gap-2 rounded px-4 py-[5px]" @click="handleUnbind(channel)" :disabled="channel.agent_id === 0">
                    <Unlink class="h-4 w-4" />
                    解绑
                  </DropdownMenuItem>
                  <DropdownMenuSeparator class="my-0.5 bg-[#f0f0f0] dark:bg-border" />
                  <DropdownMenuItem class="gap-2 rounded px-4 py-[5px] text-destructive focus:bg-destructive/10 focus:text-destructive" @click="confirmDelete(channel)">
                    <Trash2 class="h-4 w-4" />
                    删除
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>

          <!-- Appid -->
          <p class="text-xs leading-5 text-[#8c8c8c] dark:text-muted-foreground">
            Appid: {{ getAppId(channel.extra_config) }}
          </p>

          <!-- Status Tags -->
          <div class="flex items-center gap-2">
            <div class="inline-flex items-center gap-1 rounded-full bg-[#f0f0f0] px-2 py-0.5 dark:bg-muted">
              <BadgeCheck v-if="channel.agent_id !== 0" class="h-3.5 w-3.5 text-[#595959] dark:text-muted-foreground" />
              <Unlink v-else class="h-3.5 w-3.5 text-[#595959] dark:text-muted-foreground" />
              <span class="text-xs leading-4 text-[#595959] dark:text-muted-foreground">{{ channel.agent_id !== 0 ? '绑定' : '未绑定' }}</span>
            </div>
            <div v-if="channel.agent_id !== 0" class="inline-flex items-center rounded-full bg-[#f0f0f0] px-2 py-0.5 dark:bg-muted">
              <span class="text-xs leading-4 text-[#595959] dark:text-muted-foreground">AI助手</span>
            </div>
          </div>
        </div>
      </div>
      
      <!-- Empty State -->
      <div v-else class="flex flex-col items-center justify-center py-12 text-center">
        <div class="flex h-12 w-12 items-center justify-center rounded-full bg-[#f5f5f5] dark:bg-muted mb-4">
          <SquareDashed class="h-6 w-6 text-[#737373] dark:text-muted-foreground" />
        </div>
        <h3 class="text-base font-medium text-[#262626] dark:text-foreground">{{ t('channels.empty.title', '暂无频道') }}</h3>
        <p class="mt-2 max-w-sm text-sm text-[#737373] dark:text-muted-foreground">
          {{ selectedFilter === 'all' ? '您还没有配置任何频道，点击上方按钮添加一个新频道。' : `您还没有配置 ${getPlatformName(selectedFilter)} 频道。` }}
        </p>
        <Button class="mt-6 bg-[#171717] text-white hover:bg-[#171717]/90 dark:bg-primary dark:text-primary-foreground dark:hover:bg-primary/90" @click="handleAddChannel">
          <Plus class="mr-1.5 h-4 w-4" />
          {{ t('channels.addChannel', '添加频道') }}
        </Button>
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
      @saved="handleConfigSaved"
    />

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
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            @click="handleDelete"
          >
            {{ t('common.delete') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
