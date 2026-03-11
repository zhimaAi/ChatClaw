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
import { Pin, PinOff, MoreHorizontal } from 'lucide-vue-next'
import type { Agent } from '@bindings/chatclaw/internal/services/agents'
import type { Conversation } from '@bindings/chatclaw/internal/services/conversations'
import type { Robot } from '@bindings/chatclaw/internal/services/chatwiki'
import { useThemeLogo } from '@/composables/useLogo'

type ListMode = 'personal' | 'team'

const props = withDefaults(
  defineProps<{
    agents: Agent[]
    activeAgentId: number | null
    activeConversationId: number | null
    loading: boolean
    listMode: ListMode
    isSnapMode: boolean
    getAgentConversations: (agentId: number) => Conversation[]
    getAllAgentConversations: (agentId: number) => Conversation[]
    ensureConversationsLoaded: (agentId: number) => Promise<void>
    getTeamConversationAgentId?: (robotId: string) => number
    teamRobots?: Robot[]
    activeTeamRobotId?: string | null
    teamLoading?: boolean
    teamBindingChecked?: boolean
    teamBound?: boolean
    onWakeAttached?: (e: globalThis.PointerEvent) => void
  }>(),
  {
    getTeamConversationAgentId: () => 0,
    teamRobots: () => [],
    activeTeamRobotId: null,
    teamLoading: false,
    teamBindingChecked: false,
    teamBound: false,
  }
)

const emit = defineEmits<{
  'update:activeAgentId': [value: number]
  'update:activeTeamRobotId': [value: string | null]
  'update:listMode': [value: ListMode]
  'create': []
  'openSettings': [agent: Agent]
  'openChannels': [agent: Agent]
  'newConversation': []
  'newConversationForAgent': [agentId: number]
  'newConversationForTeamRobot': [robotId: string]
  'selectConversationForTeamRobot': [robotId: string, conversation: Conversation]
  'selectConversation': [conversation: Conversation]
  'selectConversationForAgent': [agentId: number, conversation: Conversation]
  'togglePin': [conversation: Conversation]
  'openRename': [conversation: Conversation]
  'openDelete': [conversation: Conversation]
  'closeSidebar': []
  'goBind': []
}>()

const { t } = useI18n()
const { logoSrc } = useThemeLogo()

const handleAgentClick = (agentId: number) => {
  emit('update:activeAgentId', agentId)
}

const handleListModeChange = (mode: ListMode) => {
  emit('update:listMode', mode)
}

const handleTeamRobotClick = (robotId: string) => {
  emit('update:activeTeamRobotId', robotId)
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
        'flex shrink-0 flex-col border-r border-border bg-background transition-all duration-200 w-sidebar',
        // In snap mode, sidebar is an overlay
        isSnapMode && 'absolute inset-y-0 left-0 z-20 shadow-lg'
      )
    "
  >
    <!-- Snap mode: close button at top -->
    <div
      v-if="isSnapMode"
      class="flex items-center justify-end border-b border-border px-2 py-1.5"
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
          @click="handleListModeChange('personal')"
        >
          {{ t('assistant.modes.personal') }}
        </button>
        <button
          :class="
            cn(
              'rounded px-3 py-1 text-sm transition-colors',
              listMode === 'team'
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
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
        :disabled="loading"
        @click="emit('create')"
      >
        <IconAgentAdd class="size-4 text-muted-foreground" />
      </Button>
    </div>

    <div class="flex-1 overflow-auto px-2 pb-3">
      <!-- Personal mode: empty state -->
      <div
        v-if="listMode === 'personal' && agents.length === 0"
        class="mx-2 mt-2 flex items-center justify-center rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground"
      >
        <div class="text-center text-sm text-muted-foreground">
          {{ t('assistant.empty') }}
        </div>
      </div>

      <!-- Team mode: loading binding -->
      <div
        v-if="listMode === 'team' && !teamBindingChecked"
        class="mx-2 mt-2 flex items-center justify-center rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground"
      >
        <div class="text-center text-sm text-muted-foreground">
          {{ t('knowledge.loading') }}
        </div>
      </div>

      <!-- Team mode: not bound - hint only (no bind button in sidebar; right side shows full empty-style block with button) -->
      <div
        v-else-if="listMode === 'team' && !teamBound"
        class="mx-2 mt-2 flex items-center justify-center rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground"
      >
        <div class="text-center text-sm text-muted-foreground">
          {{ t('knowledge.team.notBoundTitle') }}
        </div>
      </div>

      <!-- Team mode: bound but no robots - empty data hint only (same style as personal empty) -->
      <div
        v-else-if="listMode === 'team' && teamBound && teamRobots.length === 0"
        class="mx-2 mt-2 flex items-center justify-center rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground"
      >
        <div class="text-center text-sm text-muted-foreground">
          {{ t('assistant.teamEmpty') }}
        </div>
      </div>

      <!-- Personal mode: agent list -->
      <div v-if="listMode === 'personal'" class="flex flex-col gap-1.5">
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
            @click="handleAgentClick(a.id)"
            @pointerdown.capture="handleWakeAttached"
            @keydown.enter.prevent="handleAgentClick(a.id)"
            @keydown.space.prevent="handleAgentClick(a.id)"
          >
            <div
              class="flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-[10px] border border-border bg-white text-foreground dark:border-white/15 dark:bg-white/5"
            >
              <img v-if="a.icon" :src="a.icon" class="size-6 object-contain" />
              <img v-else :src="logoSrc" class="size-6 opacity-90" alt="ChatClaw logo" />
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
                @click.stop="emit('newConversationForAgent', a.id)"
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
              @click="emit('selectConversation', conv)"
              @keydown.enter.prevent="emit('selectConversation', conv)"
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

      <!-- Team mode: robot list from ChatWiki getRobotList (only when has robots) -->
      <div v-if="listMode === 'team' && teamBound && teamRobots.length > 0" class="flex flex-col gap-1.5">
        <div v-for="r in teamRobots" :key="r.id" class="flex flex-col">
          <div
            :class="
              cn(
                'group flex h-11 w-full items-center gap-2 rounded px-2 text-left outline-none transition-colors',
                r.id === activeTeamRobotId
                  ? 'bg-zinc-100 text-foreground dark:bg-accent'
                  : 'bg-white text-muted-foreground shadow-[0px_1px_4px_0px_rgba(0,0,0,0.1)] hover:bg-accent/50 hover:text-foreground dark:bg-zinc-800/50 dark:shadow-[0px_1px_4px_0px_rgba(255,255,255,0.05)]'
              )
            "
            role="button"
            tabindex="0"
            @click="handleTeamRobotClick(r.id)"
            @keydown.enter.prevent="handleTeamRobotClick(r.id)"
            @keydown.space.prevent="handleTeamRobotClick(r.id)"
          >
            <div
              class="flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-[10px] border border-border bg-white text-foreground dark:border-white/15 dark:bg-white/5"
            >
              <img v-if="r.icon" :src="r.icon" class="size-6 object-contain" alt="" />
              <img v-else :src="logoSrc" class="size-6 opacity-90" alt="ChatClaw logo" />
            </div>
            <div class="min-w-0 flex-1">
              <div class="truncate text-sm font-normal">{{ r.name }}</div>
            </div>

            <!-- Action buttons (same as personal agents) -->
            <div class="flex items-center gap-0">
              <Button
                size="icon"
                variant="ghost"
                class="size-7 opacity-0 group-hover:opacity-100 hover:bg-muted/60 dark:hover:bg-white/10"
                :title="t('assistant.sidebar.newConversation')"
                @click.stop="emit('newConversationForTeamRobot', r.id)"
              >
                <IconNewConversation class="size-4 text-muted-foreground" />
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger as-child>
                  <Button
                    size="icon"
                    variant="ghost"
                    class="size-7 opacity-0 group-hover:opacity-100 hover:bg-muted/60 dark:hover:bg-white/10"
                    :title="t('assistant.menu.history')"
                    @click.stop
                  >
                    <IconSettings class="size-4 opacity-80 group-hover:opacity-100" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" class="w-48">
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger
                      @mouseenter="ensureConversationsLoaded(getTeamConversationAgentId(r.id))"
                      @focus="ensureConversationsLoaded(getTeamConversationAgentId(r.id))"
                    >
                      {{ t('assistant.menu.history') }}
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent class="max-h-[300px] w-56 overflow-y-auto">
                      <template v-if="getAllAgentConversations(getTeamConversationAgentId(r.id)).length > 0">
                        <DropdownMenuItem
                          v-for="conv in getAllAgentConversations(getTeamConversationAgentId(r.id))"
                          :key="conv.id"
                          @click="emit('selectConversationForTeamRobot', r.id, conv)"
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

          <!-- Team conversation list (max 3 items) - only show for active robot -->
          <div
            v-if="
              r.id === activeTeamRobotId &&
              getAgentConversations(getTeamConversationAgentId(r.id)).length > 0
            "
            class="mt-1 flex flex-col gap-0.5"
          >
            <div
              v-for="conv in getAgentConversations(getTeamConversationAgentId(r.id))"
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
              @click="emit('selectConversationForTeamRobot', r.id, conv)"
              @keydown.enter.prevent="emit('selectConversationForTeamRobot', r.id, conv)"
            >
              <Pin v-if="conv.is_pinned" class="size-3 shrink-0 text-muted-foreground" />
              <span class="min-w-0 flex-1 truncate">{{ conv.name }}</span>
              <DropdownMenu>
                <DropdownMenuTrigger
                  class="flex h-5 w-5 shrink-0 items-center justify-center rounded text-muted-foreground opacity-0 transition-opacity hover:bg-background/60 hover:text-foreground group-hover:opacity-100"
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
