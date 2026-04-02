<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
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
import IconAgentAdd from '@/assets/icons/agent-add.svg'
import IconNewConversation from '@/assets/icons/new-conversation.svg'
import IconSettings from '@/assets/icons/settings.svg'
import IconHistory from '@/assets/icons/history-icon.svg'
import IconChannels from '@/assets/icons/channels.svg'
import IconRename from '@/assets/icons/library-rename.svg'
import IconDelete from '@/assets/icons/library-delete.svg'
import IconSidebarCollapse from '@/assets/icons/sidebar-collapse.svg'
import IconChevronDown from '@/assets/icons/down-icon.svg'
import IconChevronRight from '@/assets/icons/right-icon.svg'
import IconSession from '@/assets/icons/session-icon.svg'
import openclawDefaultAvatar from '@/assets/icons/openclaw.svg?url'
import { Pin, PinOff, MoreHorizontal } from 'lucide-vue-next'
import type { OpenClawAgent } from '@bindings/chatclaw/internal/openclaw/agents'
import type { Conversation } from '@bindings/chatclaw/internal/services/conversations'

type ListMode = 'personal' | 'team'

const props = withDefaults(
  defineProps<{
    agents: OpenClawAgent[]
    activeAgentId: number | null
    activeConversationId: number | null
    loading: boolean
    listMode: ListMode
    /** When true, team segment is visible but not selectable (OpenClaw assistant). */
    teamListDisabled?: boolean
    isSnapMode: boolean
    getAgentConversations: (agentId: number) => Conversation[]
    getAllAgentConversations: (agentId: number) => Conversation[]
    ensureConversationsLoaded: (agentId: number) => Promise<void>
    onWakeAttached?: (e: globalThis.PointerEvent) => void
  }>(),
  {
    teamListDisabled: false,
  }
)

const emit = defineEmits<{
  'update:activeAgentId': [value: number]
  'update:listMode': [value: ListMode]
  create: []
  openSettings: [agent: OpenClawAgent]
  openChannels: [agent: OpenClawAgent]
  newConversation: []
  newConversationForAgent: [agentId: number]
  selectConversation: [conversation: Conversation]
  selectConversationForAgent: [agentId: number, conversation: Conversation]
  togglePin: [conversation: Conversation]
  openRename: [conversation: Conversation]
  openDelete: [conversation: Conversation]
  closeSidebar: []
  goBind: []
}>()

const { t } = useI18n()

const handleAgentClick = (agentId: number) => {
  emit('update:activeAgentId', agentId)
}

const handleListModeChange = (mode: ListMode) => {
  if (props.teamListDisabled && mode === 'team') return
  emit('update:listMode', mode)
}

const handleWakeAttached = (e: globalThis.PointerEvent) => {
  // Only trigger wake attached when there are multiple agents
  if (props.agents.length > 1 && props.onWakeAttached) {
    props.onWakeAttached(e)
  }
}
</script>

<template>
  <aside
    :class="
      cn(
        'flex shrink-0 flex-col border-r border-[#F5F5F5] bg-background transition-all duration-200 w-sidebar',
        // In snap mode, sidebar is an overlay
        isSnapMode && 'absolute inset-y-0 left-0 z-20 shadow-lg'
      )
    "
  >
    <!-- Snap mode: close button at top -->
    <div
      v-if="isSnapMode"
      class="flex items-center justify-end border-b border-[#F5F5F5] px-2 py-1.5"
    >
      <Button
        size="icon"
        variant="ghost"
        class="size-6"
        :title="t('assistant.sidebar.collapse')"
        @click="emit('closeSidebar')"
      >
        <IconSidebarCollapse class="size-4 text-muted-foreground" />
      </Button>
    </div>

    <div class="flex items-center justify-between gap-2 border-b border-[#F5F5F5] px-2 py-2">
      <div class="inline-flex rounded-lg bg-muted p-[3px]">
        <button
          type="button"
          :class="
            cn(
              'min-h-[29px] min-w-[29px] rounded-[10px] px-2 py-1 text-sm transition-all',
              listMode === 'personal'
                ? 'bg-background text-foreground shadow-sm font-medium'
                : 'text-foreground'
            )
          "
          @click="handleListModeChange('personal')"
        >
          {{ t('assistant.modes.personal') }}
        </button>
        <button
          type="button"
          :disabled="teamListDisabled"
          :class="
            cn(
              'min-h-[29px] min-w-[29px] rounded-[10px] px-2 py-1 text-sm transition-all',
              teamListDisabled && 'cursor-not-allowed',
              listMode === 'team'
                ? 'bg-background text-foreground shadow-sm font-medium'
                : 'text-foreground',
              teamListDisabled && listMode !== 'team' && 'text-muted-foreground opacity-60'
            )
          "
          @click="handleListModeChange('team')"
        >
          {{ t('assistant.modes.team') }}
        </button>
      </div>

      <Button
        v-if="listMode === 'personal'"
        size="icon"
        variant="ghost"
        class="size-6 rounded-md text-muted-foreground hover:text-foreground"
        :disabled="loading"
        @click="emit('create')"
      >
        <IconAgentAdd class="size-4" />
      </Button>
    </div>

    <div class="flex-1 overflow-auto">
      <div
        v-if="listMode === 'personal' && agents.length === 0"
        class="mx-2 mt-2 flex items-center justify-center rounded-lg border border-[#F5F5F5] bg-card p-4 text-sm text-muted-foreground"
      >
        <div class="text-center text-sm text-muted-foreground">
          {{ t('assistant.empty') }}
        </div>
      </div>

      <div v-if="listMode === 'personal'" class="flex flex-col">
        <div v-for="a in agents" :key="a.id" class="flex flex-col">
          <!-- Agent item -->
          <div
            :class="
              cn(
                'group flex h-12 w-full items-center gap-2 px-2 text-left outline-none transition-colors',
                a.id === activeAgentId && getAgentConversations(a.id).length > 0
                  ? 'border-b-0'
                  : 'border-b border-[#F5F5F5]/70',
                a.id === activeAgentId
                  ? 'bg-card text-foreground'
                  : 'bg-background text-foreground hover:bg-accent/40 active:bg-accent/60'
              )
            "
            role="button"
            tabindex="0"
            @click="handleAgentClick(a.id)"
            @pointerdown.capture="handleWakeAttached"
            @keydown.enter.prevent="handleAgentClick(a.id)"
            @keydown.space.prevent="handleAgentClick(a.id)"
          >
            <span
              class="flex size-4 shrink-0 items-center justify-center text-muted-foreground"
              aria-hidden="true"
            >
              <IconChevronDown v-if="a.id === activeAgentId" class="size-4" />
              <IconChevronRight v-else class="size-4" />
            </span>
            <div
              class="flex size-6 shrink-0 items-center justify-center overflow-hidden rounded-[5px] bg-muted/70 text-foreground"
            >
              <img v-if="a.icon" :src="a.icon" class="size-full object-cover" />
              <img v-else :src="openclawDefaultAvatar" class="size-5 opacity-90" alt="" />
            </div>

            <div class="min-w-0 flex-1">
              <div class="truncate text-sm font-medium text-foreground">
                {{ a.name }}
              </div>
            </div>

            <!-- Action buttons -->
            <div class="flex items-center gap-0">
              <!-- New conversation button -->
              <Button
                size="icon"
                variant="ghost"
                :class="
                  cn(
                    'size-6 rounded-md text-muted-foreground transition-opacity hover:bg-muted/70 hover:text-foreground',
                    a.id === activeAgentId
                      ? 'opacity-100'
                      : 'opacity-0 group-hover:opacity-100 group-focus-within:opacity-100'
                  )
                "
                :title="t('assistant.sidebar.newConversation')"
                @click.stop="emit('newConversationForAgent', a.id)"
              >
                <IconNewConversation class="size-4" />
              </Button>

              <!-- Settings dropdown -->
              <DropdownMenu>
                <DropdownMenuTrigger as-child>
                  <Button
                    size="icon"
                    variant="ghost"
                    :class="
                      cn(
                        'size-6 rounded-md text-muted-foreground transition-opacity hover:bg-muted/70 hover:text-foreground',
                        a.id === activeAgentId
                          ? 'opacity-100'
                          : 'opacity-0 group-hover:opacity-100 group-focus-within:opacity-100'
                      )
                    "
                    :title="t('assistant.actions.settings')"
                    @click.stop
                  >
                    <IconSettings class="size-4 opacity-80 group-hover:opacity-100" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" class="w-48">
                  <DropdownMenuItem class="gap-2" @click="emit('openSettings', a)">
                    <IconSettings class="size-4 text-muted-foreground" />
                    {{ t('assistant.menu.settings') }}
                  </DropdownMenuItem>
                  <DropdownMenuItem class="gap-2" @click="emit('openChannels', a)">
                    <IconChannels class="size-4 text-muted-foreground" />
                    {{ t('assistant.menu.channels') }}
                  </DropdownMenuItem>
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger
                      class="gap-2"
                      @mouseenter="ensureConversationsLoaded(a.id)"
                      @focus="ensureConversationsLoaded(a.id)"
                    >
                      <IconHistory class="size-4 text-muted-foreground" />
                      {{ t('assistant.menu.history') }}
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent class="max-h-[300px] w-56 overflow-y-auto">
                      <template v-if="getAllAgentConversations(a.id).length > 0">
                        <DropdownMenuItem
                          v-for="conv in getAllAgentConversations(a.id)"
                          :key="conv.id"
                          @click="emit('selectConversationForAgent', a.id, conv)"
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
            class="flex flex-col border-b border-[#F5F5F5]/70 pb-1"
          >
            <div
              v-for="conv in getAgentConversations(a.id)"
              :key="conv.id"
              :class="
                cn(
                  'group mx-2 mt-0.5 flex h-10 items-center gap-1.5 rounded-md px-2 text-left text-sm transition-colors',
                  activeConversationId === conv.id
                    ? 'bg-accent text-foreground'
                    : 'bg-background text-muted-foreground hover:bg-accent/40 hover:text-foreground active:bg-accent/60'
                )
              "
              role="button"
              tabindex="0"
              @click="emit('selectConversation', conv)"
              @keydown.enter.prevent="emit('selectConversation', conv)"
            >
              <span
                class="inline-flex size-[18px] shrink-0 items-center justify-center overflow-visible [&_svg]:block [&_svg]:size-4"
                aria-hidden="true"
              >
                <IconSession
                  :class="
                    cn(
                      activeConversationId === conv.id
                        ? 'text-foreground/70'
                        : 'text-muted-foreground [.group:hover_&]:text-foreground/70'
                    )
                  "
                />
              </span>
              <Pin
                v-if="conv.is_pinned"
                class="size-3 shrink-0 text-muted-foreground [.group:hover_&]:text-foreground/80"
              />
              <span
                :class="
                  cn(
                    'min-w-0 flex-1 truncate',
                    activeConversationId === conv.id
                      ? 'text-foreground'
                      : 'text-muted-foreground group-hover:text-foreground/90'
                  )
                "
                >{{ conv.name }}</span
              >
              <!-- Conversation menu -->
              <DropdownMenu>
                <DropdownMenuTrigger
                  class="flex h-5 w-5 shrink-0 cursor-pointer items-center justify-center rounded text-muted-foreground opacity-0 transition-opacity hover:bg-background/60 hover:text-foreground active:bg-background/80 group-hover:opacity-100"
                  @click.stop
                >
                  <MoreHorizontal class="size-3.5" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" class="w-36">
                  <DropdownMenuItem class="gap-2" @select="emit('togglePin', conv)">
                    <PinOff v-if="conv.is_pinned" class="size-4 text-muted-foreground" />
                    <Pin v-else class="size-4 text-muted-foreground" />
                    {{ conv.is_pinned ? t('assistant.menu.unpin') : t('assistant.menu.pin') }}
                  </DropdownMenuItem>
                  <DropdownMenuItem class="gap-2" @select="emit('openRename', conv)">
                    <IconRename class="size-4 text-muted-foreground" />
                    {{ t('assistant.menu.rename') }}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    class="gap-2 text-muted-foreground focus:text-foreground"
                    @select="emit('openDelete', conv)"
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
</template>
