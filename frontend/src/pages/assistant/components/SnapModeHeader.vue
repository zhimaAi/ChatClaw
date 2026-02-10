<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Plus, X } from 'lucide-vue-next'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
} from '@/components/ui/select'
import LogoIcon from '@/assets/images/logo.svg'
import IconSnapAttached from '@/assets/icons/snap-attached.svg'
import IconSnapDetached from '@/assets/icons/snap-detached.svg'
import type { Agent } from '@bindings/willclaw/internal/services/agents'

defineProps<{
  agents: Agent[]
  activeAgent: Agent | null
  activeAgentId: number | null
  hasAttachedTarget: boolean
}>()

const emit = defineEmits<{
  'update:activeAgentId': [value: number]
  'newConversation': []
  'cancelSnap': []
  'findAndAttach': []
  'closeWindow': []
}>()

const { t } = useI18n()

const handleAgentChange = (value: any) => {
  if (value) {
    emit('update:activeAgentId', Number(value))
    emit('newConversation')
  }
}
</script>

<template>
  <div
    class="flex h-10 shrink-0 items-center justify-between border-b border-border bg-background px-3"
    style="--wails-draggable: drag"
  >
    <!-- Left: Agent selector -->
    <div class="flex items-center gap-1" style="--wails-draggable: no-drag">
      <Select
        :model-value="activeAgentId?.toString() ?? ''"
        @update:model-value="handleAgentChange"
      >
        <SelectTrigger
          class="h-7 w-auto min-w-[120px] max-w-[180px] border-0 bg-transparent px-2 text-sm font-medium shadow-none hover:bg-muted/50"
        >
          <div v-if="activeAgent" class="flex items-center gap-1.5">
            <img v-if="activeAgent.icon" :src="activeAgent.icon" class="size-4 rounded object-contain" />
            <LogoIcon v-else class="size-4" />
            <span class="truncate">{{ activeAgent.name }}</span>
          </div>
          <span v-else class="text-muted-foreground">{{ t('assistant.placeholders.noAgentSelected') }}</span>
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem v-for="a in agents" :key="a.id" :value="a.id.toString()">
              <div class="flex items-center gap-2">
                <img v-if="a.icon" :src="a.icon" class="size-4 rounded object-contain" />
                <LogoIcon v-else class="size-4" />
                <span>{{ a.name }}</span>
              </div>
            </SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>

    <!-- Right: New conversation + Snap toggle icon -->
    <div class="flex items-center gap-2" style="--wails-draggable: no-drag">
      <TooltipProvider :delay-duration="300">
        <Tooltip>
          <TooltipTrigger as-child>
            <button
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
