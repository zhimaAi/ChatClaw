<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import type { Agent, Channel, ScheduledTaskFormState } from '../types'
import { createSubmitLock } from './submitLock'
import TaskFormContent from './TaskFormContent.vue'

const props = defineProps<{
  open: boolean
  saving: boolean
  title: string
  form: ScheduledTaskFormState
  agents: Agent[]
  channels: Channel[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  submit: []
}>()

const { t } = useI18n()

const dialogSubtitle = computed(() =>
  props.form.id
    ? t('scheduledTasks.dialog.subtitleEdit')
    : t('scheduledTasks.dialog.subtitleCreate')
)
const submitLock = createSubmitLock()
const submitLocked = ref(false)

const canSubmit = computed(() => {
  if (!props.form.name.trim()) return false
  if (!props.form.prompt.trim()) return false
  if (props.form.agentId == null || props.form.agentId <= 0) return false

  if (props.form.scheduleType === 'cron' && !props.form.cronExpr.trim()) return false

  if (props.form.scheduleType === 'custom' && props.form.customMode === 'interval') {
    const n = Number(props.form.customIntervalMinutes)
    if (!Number.isFinite(n) || n < 1 || n > 59) return false
  }

  if (
    props.form.scheduleType === 'custom' &&
    props.form.customMode === 'weekly' &&
    !props.form.customWeekdays?.length
  ) {
    return false
  }

  return true
})

const submitActionDisabled = computed(
  () => !canSubmit.value || props.saving || submitLocked.value
)

function closeDialog() {
  submitLock.reset()
  submitLocked.value = submitLock.isLocked()
  emit('update:open', false)
}

function syncSubmitLockedState() {
  submitLocked.value = submitLock.isLocked()
}

function handleSubmit() {
  if (!canSubmit.value || props.saving) return

  if (!submitLock.acquire(props.saving)) {
    syncSubmitLockedState()
    return
  }

  syncSubmitLockedState()
  emit('submit')
}

watch(
  () => props.saving,
  (saving) => {
    submitLock.syncSaving(saving)
    syncSubmitLockedState()
  }
)

watch(
  () => props.open,
  (open) => {
    if (!open) {
      submitLock.reset()
      syncSubmitLockedState()
    }
  }
)
</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent
      size="xl"
      :show-close-button="true"
      class="flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0 !max-w-[780px] shadow-lg dark:shadow-none dark:ring-1 dark:ring-white/10"
    >
      <DialogHeader
        class="flex shrink-0 flex-row flex-wrap items-baseline gap-2 border-b border-border px-6 pb-3 pt-4"
      >
        <DialogTitle class="text-xl font-semibold text-[#0a0a0a] dark:text-foreground">
          {{ title }}
        </DialogTitle>
        <p class="text-sm text-muted-foreground">
          {{ dialogSubtitle }}
        </p>
      </DialogHeader>

      <div class="flex-1 overflow-y-auto px-7">
        <TaskFormContent :form="form" :agents="agents" :channels="channels" />
      </div>

      <DialogFooter
        class="shrink-0 gap-2 border-t border-border px-6 py-4 sm:justify-end"
      >
        <Button type="button" variant="outline" @click="closeDialog">
          {{ t('common.cancel') }}
        </Button>
        <Button type="button" :disabled="submitActionDisabled" @click="handleSubmit">
          {{ saving || submitLocked ? t('scheduledTasks.dialog.submitting') : t('common.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
