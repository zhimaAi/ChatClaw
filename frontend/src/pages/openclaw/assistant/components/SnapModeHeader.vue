<script setup lang="ts">
import { onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, X } from 'lucide-vue-next'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
} from '@/components/ui/select'
import IconSnapAttached from '@/assets/icons/snap-attached.svg'
import IconSnapDetached from '@/assets/icons/snap-detached.svg'
import openclawIconUrl from '@/assets/icons/openclaw.svg?url'
import type { OpenClawAgent } from '@bindings/chatclaw/internal/openclaw/agents'
import type { Robot } from '@bindings/chatclaw/internal/services/chatwiki'
import { SettingsService } from '@bindings/chatclaw/internal/services/settings'
import { Window } from '@wailsio/runtime'

export type SnapListMode = 'personal' | 'team'

const props = withDefaults(
  defineProps<{
    listMode: SnapListMode
    agents: OpenClawAgent[]
    activeAgent: OpenClawAgent | null
    activeAgentId: number | null
    teamRobots: Robot[]
    activeTeamRobot: Robot | null
    activeTeamRobotId: string | null
    teamLoading: boolean
    hasAttachedTarget: boolean
    /** When true, hide personal/team switch — OpenClaw snap mode is personal-only. */
    teamListDisabled?: boolean
  }>(),
  {
    teamListDisabled: false,
  }
)

const emit = defineEmits<{
  'update:listMode': [value: SnapListMode]
  'update:activeAgentId': [value: number]
  'update:activeTeamRobotId': [value: string | null]
  newConversation: []
  cancelSnap: []
  findAndAttach: []
  closeWindow: []
}>()

const { t } = useI18n()
const SNAP_DRAG_GUARD_UNTIL_KEY = 'snap_drag_guard_until_unix_ms'

const setDragGuardUntil = (untilMs: number) => {
  void SettingsService.SetValue(SNAP_DRAG_GUARD_UNTIL_KEY, String(untilMs)).catch(() => {})
}

const clearDragGuard = () => {
  setDragGuardUntil(0)
}

const nonDragSelector =
  '[data-no-drag], [data-snap-action], button, input, textarea, [role="combobox"], [contenteditable="true"]'

const dragState = {
  pointerId: -1,
  startScreenX: 0,
  startScreenY: 0,
  startWindowX: 0,
  startWindowY: 0,
}
let dragRafId: number | null = null
let pendingDragPos: { x: number; y: number } | null = null

const flushDragMove = () => {
  dragRafId = null
  if (!pendingDragPos) return
  const nextPos = pendingDragPos
  pendingDragPos = null
  void Window.SetPosition(Math.round(nextPos.x), Math.round(nextPos.y))
}

const scheduleDragMove = (x: number, y: number) => {
  pendingDragPos = { x, y }
  if (dragRafId != null) return
  dragRafId = requestAnimationFrame(flushDragMove)
}

const stopCustomDrag = () => {
  dragState.pointerId = -1
  pendingDragPos = null
  if (dragRafId != null) {
    cancelAnimationFrame(dragRafId)
    dragRafId = null
  }
  clearDragGuard()
}

const onHeaderPointerDown = async (e: PointerEvent) => {
  if (e.button !== 0) return
  const target = e.target instanceof Element ? e.target : null
  if (target?.closest(nonDragSelector)) return

  dragState.pointerId = e.pointerId
  dragState.startScreenX = e.screenX
  dragState.startScreenY = e.screenY
  setDragGuardUntil(Date.now() + 15000)
  ;(e.currentTarget as HTMLElement | null)?.setPointerCapture?.(e.pointerId)
  try {
    const pos = (await Window.Position()) as { x: number; y: number }
    dragState.startWindowX = Number(pos?.x ?? 0)
    dragState.startWindowY = Number(pos?.y ?? 0)
  } catch {
    stopCustomDrag()
  }
}

const onHeaderPointerMove = (e: PointerEvent) => {
  if (dragState.pointerId !== e.pointerId) return
  const dx = e.screenX - dragState.startScreenX
  const dy = e.screenY - dragState.startScreenY
  scheduleDragMove(dragState.startWindowX + dx, dragState.startWindowY + dy)
}

const onHeaderPointerUp = (e: PointerEvent) => {
  if (dragState.pointerId !== e.pointerId) return
  try {
    ;(e.currentTarget as HTMLElement | null)?.releasePointerCapture?.(e.pointerId)
  } catch {
    // Ignore release errors.
  }
  stopCustomDrag()
}

onUnmounted(() => {
  stopCustomDrag()
})

const handleListModeChange = (value: unknown) => {
  const s = value != null ? String(value) : ''
  if (props.teamListDisabled && s === 'team') return
  if (s === 'personal' || s === 'team') {
    emit('update:listMode', s)
  }
}

const handleAssistantChange = (value: unknown) => {
  const s = value != null ? String(value) : ''
  if (s === '') return
  if (props.teamListDisabled || props.listMode === 'personal') {
    emit('update:activeAgentId', Number(s))
  } else {
    emit('update:activeTeamRobotId', s)
  }
  emit('newConversation')
}
</script>

<template>
  <div
    data-snap-no-wake="true"
    class="flex h-10 shrink-0 items-center border-b border-border bg-background px-3"
    style="--wails-draggable: no-drag"
    @pointerdown.capture="onHeaderPointerDown"
    @pointermove.capture="onHeaderPointerMove"
    @pointerup.capture="onHeaderPointerUp"
    @pointercancel.capture="onHeaderPointerUp"
  >
    <!-- Left: List mode (personal/team) + Assistant selector -->
    <div
      data-no-drag="true"
      class="flex min-w-0 items-center gap-1.5"
      style="--wails-draggable: no-drag"
    >
      <!-- First dropdown: Personal / Team (hidden in OpenClaw — team list not supported) -->
      <Select
        v-if="!teamListDisabled"
        :model-value="listMode"
        @update:model-value="handleListModeChange"
      >
        <SelectTrigger
          class="h-7 w-auto min-w-[72px] max-w-[90px] border-0 bg-transparent px-2 text-sm font-medium shadow-none hover:bg-muted/50"
        >
          <span class="truncate">{{
            listMode === 'personal' ? t('assistant.modes.personal') : t('assistant.modes.team')
          }}</span>
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value="personal">{{ t('assistant.modes.personal') }}</SelectItem>
            <SelectItem value="team">{{ t('assistant.modes.team') }}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>

      <!-- Second dropdown: Agent (personal) or Team robot (team) -->
      <Select
        :model-value="
          teamListDisabled || listMode === 'personal'
            ? (activeAgentId?.toString() ?? '')
            : (activeTeamRobotId ?? '')
        "
        :disabled="
          teamListDisabled || listMode === 'personal'
            ? agents.length === 0
            : teamLoading || teamRobots.length === 0
        "
        @update:model-value="handleAssistantChange"
      >
        <SelectTrigger
          class="h-7 w-auto min-w-[120px] max-w-[180px] border-0 bg-transparent px-2 text-sm font-medium shadow-none hover:bg-muted/50"
        >
          <template v-if="teamListDisabled || listMode === 'personal'">
            <div v-if="activeAgent" class="flex items-center gap-1.5">
              <img
                v-if="activeAgent.icon"
                :src="activeAgent.icon"
                class="size-4 rounded object-contain"
              />
              <img
                v-else
                :src="openclawIconUrl"
                class="size-4 rounded object-contain"
                alt=""
              />
              <span class="truncate">{{ activeAgent.name }}</span>
            </div>
            <span v-else class="text-muted-foreground">{{
              t('assistant.placeholders.noAgentSelected')
            }}</span>
          </template>
          <template v-else>
            <div v-if="activeTeamRobot" class="flex items-center gap-1.5">
              <img
                v-if="activeTeamRobot.icon"
                :src="activeTeamRobot.icon"
                class="size-4 rounded object-contain"
                alt=""
              />
              <img
                v-else
                :src="openclawIconUrl"
                class="size-4 rounded object-contain"
                alt=""
              />
              <span class="truncate">{{ activeTeamRobot.name }}</span>
            </div>
            <span v-else class="text-muted-foreground">{{
              t('assistant.placeholders.noAgentSelected')
            }}</span>
          </template>
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectLabel>{{ t('assistant.placeholders.noAgentSelected') }}</SelectLabel>
            <template v-if="teamListDisabled || listMode === 'personal'">
              <SelectItem v-for="a in agents" :key="a.id" :value="a.id.toString()">
                <div class="flex items-center gap-2">
                  <img v-if="a.icon" :src="a.icon" class="size-4 rounded object-contain" />
                  <img
                    v-else
                    :src="openclawIconUrl"
                    class="size-4 rounded object-contain"
                    alt=""
                  />
                  <span>{{ a.name }}</span>
                </div>
              </SelectItem>
            </template>
            <template v-else>
              <SelectItem v-for="r in teamRobots" :key="r.id" :value="r.id">
                <div class="flex items-center gap-2">
                  <img v-if="r.icon" :src="r.icon" class="size-4 rounded object-contain" alt="" />
                  <img
                    v-else
                    :src="openclawIconUrl"
                    class="size-4 rounded object-contain"
                    alt=""
                  />
                  <span>{{ r.name }}</span>
                </div>
              </SelectItem>
            </template>
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>

    <!-- Middle spacer: draggable blank area -->
    <div class="mx-2 min-w-4 flex-1" />

    <!-- Right: New conversation + Snap toggle icon -->
    <div
      data-no-drag="true"
      class="flex items-center justify-end gap-2"
      style="--wails-draggable: no-drag"
    >
      <TooltipProvider :delay-duration="300">
        <Tooltip>
          <TooltipTrigger as-child>
            <button
              data-snap-action="new-conversation"
              class="rounded-md p-1 hover:bg-muted"
              type="button"
              @click="emit('newConversation')"
            >
              <Plus class="size-4 text-muted-foreground" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="bottom">
            {{ t('assistant.sidebar.newConversation') }}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>

      <!-- Snap icon: attached state (with bg + tooltip) -->
      <TooltipProvider v-if="hasAttachedTarget" :delay-duration="300">
        <Tooltip>
          <TooltipTrigger as-child>
            <button
              data-snap-action="cancel-snap"
              class="rounded-md bg-muted p-1"
              type="button"
              @click="emit('cancelSnap')"
            >
              <IconSnapAttached class="size-5 text-muted-foreground" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="bottom">
            {{ t('winsnap.cancelSnap') }}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>

      <!-- Snap icon: detached state (no bg, with tooltip) -->
      <TooltipProvider v-else :delay-duration="300">
        <Tooltip>
          <TooltipTrigger as-child>
            <button
              data-snap-action="find-and-attach"
              class="rounded-md p-1 hover:bg-muted"
              type="button"
              @click="emit('findAndAttach')"
            >
              <IconSnapDetached class="size-5 text-muted-foreground" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="bottom">
            {{ t('winsnap.snapApp') }}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>

      <!-- Close button -->
      <button
        data-snap-action="close-window"
        class="rounded-md p-1 hover:bg-muted"
        :title="t('winsnap.closeWindow')"
        type="button"
        @click="emit('closeWindow')"
      >
        <X class="size-4 text-muted-foreground" />
      </button>
    </div>
  </div>
</template>
