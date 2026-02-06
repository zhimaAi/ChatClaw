<script setup lang="ts">
import { computed } from 'vue'
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
import LogoIcon from '@/assets/images/logo.svg'
import IconAgentAdd from '@/assets/icons/agent-add.svg'
import IconNewConversation from '@/assets/icons/new-conversation.svg'
import IconSettings from '@/assets/icons/settings.svg'
import IconRename from '@/assets/icons/library-rename.svg'
import IconDelete from '@/assets/icons/library-delete.svg'
import IconSidebarCollapse from '@/assets/icons/sidebar-collapse.svg'
import { Pin, PinOff, MoreHorizontal } from 'lucide-vue-next'
import type { Agent } from '@bindings/willchat/internal/services/agents'
import type { Conversation } from '@bindings/willchat/internal/services/conversations'

type ListMode = 'personal' | 'team'

const props = defineProps<{
  agents: Agent[]
  activeAgentId: number | null
  activeConversationId: number | null
  loading: boolean
  listMode: ListMode
  isSnapMode: boolean
  getAgentConversations: (agentId: number) => Conversation[]
  getAllAgentConversations: (agentId: number) => Conversation[]
  ensureConversationsLoaded: (agentId: number) => Promise<void>
}>()

const emit = defineEmits<{
  'update:activeAgentId': [value: number]
  'update:listMode': [value: ListMode]
  'create': []
  'openSettings': [agent: Agent]
  'newConversation': []
  'newConversationForAgent': [agentId: number]
  'selectConversation': [conversation: Conversation]
  'selectConversationForAgent': [agentId: number, conversation: Conversation]
  'togglePin': [conversation: Conversation]
  'openRename': [conversation: Conversation]
  'openDelete': [conversation: Conversation]
  'closeSidebar': []
}>()

const { t } = useI18n()

const handleAgentClick = (agentId: number) => {
  emit('update:activeAgentId', agentId)
}

const handleListModeChange = (mode: ListMode) => {
  emit('update:listMode', mode)
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
          :class="cn('rounded px-3 py-1 text-sm transition-colors', 'cursor-not-allowed opacity-50')"
          disabled
        >
          {{ t('assistant.modes.team') }}
        </button>
      </div>

      <Button size="icon" variant="ghost" :disabled="loading" @click="emit('create')">
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
            @click="handleAgentClick(a.id)"
            @keydown.enter.prevent="handleAgentClick(a.id)"
            @keydown.space.prevent="handleAgentClick(a.id)"
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
                  <DropdownMenuItem @click="emit('openSettings', a)">
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
                    class="gap-2 text-destructive focus:text-destructive"
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
