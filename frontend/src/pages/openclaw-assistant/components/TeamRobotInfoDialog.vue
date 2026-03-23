<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useThemeLogo } from '@/composables/useLogo'
import type { Robot } from '@bindings/chatclaw/internal/services/chatwiki'

const props = defineProps<{
  open: boolean
  robot: Robot | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const { t } = useI18n()
const { logoSrc } = useThemeLogo()

const setOpen = (value: boolean) => emit('update:open', value)
</script>

<template>
  <Dialog :open="open" @update:open="setOpen">
    <DialogContent size="md">
      <DialogHeader>
        <DialogTitle>{{ t('assistant.teamRobot.infoTitle') }}</DialogTitle>
      </DialogHeader>

      <div v-if="robot" class="flex flex-col gap-4 py-4">
        <div class="flex items-center gap-4">
          <div
            class="flex size-14 shrink-0 items-center justify-center overflow-hidden rounded-xl border border-border bg-muted text-foreground"
          >
            <img v-if="robot.icon" :src="robot.icon" class="size-10 object-contain" alt="" />
            <img v-else :src="logoSrc" class="size-10 opacity-90" alt="ChatClaw logo" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="text-base font-medium text-foreground">{{ robot.name }}</div>
            <div v-if="robot.intro" class="mt-1 text-sm text-muted-foreground line-clamp-3">
              {{ robot.intro }}
            </div>
          </div>
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" @click="setOpen(false)">
          {{ t('assistant.actions.cancel') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
