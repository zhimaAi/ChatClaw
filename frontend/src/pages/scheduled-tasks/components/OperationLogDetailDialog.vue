<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import type { ScheduledTaskOperationLogDetail } from '../types'
import { operationSnapshotToForm } from '../utils'
import { createScheduleTextFormatter } from '../scheduledTaskText'
import TaskFormContent from './TaskFormContent.vue'

const props = defineProps<{
  open: boolean
  detail: ScheduledTaskOperationLogDetail | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const { t } = useI18n()
const titleRef = ref<HTMLElement | null>(null)
const snapshot = computed(() => props.detail?.task_snapshot ?? null)
const scheduleFormatter = computed(() => createScheduleTextFormatter(t))
const readonlyForm = computed(() =>
  snapshot.value ? operationSnapshotToForm(snapshot.value) : null
)
const readonlyNotificationPlatformLabel = computed(() => {
  const platform = snapshot.value?.notification_platform?.trim()
  if (!platform) return ''
  return scheduleFormatter.value.notificationPlatformLabel(platform)
})
const readonlyChannelLabels = computed(() => {
  if (!snapshot.value?.notification_channels || snapshot.value.notification_channels === '-') {
    return []
  }

  const [, rawLabels = ''] = snapshot.value.notification_channels.split(':')
  return rawLabels
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
})

function handleOpenAutoFocus(event: Event) {
  event.preventDefault()
  void nextTick(() => {
    titleRef.value?.focus()
  })
}

</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent
      class="flex max-h-[90vh] flex-col overflow-hidden p-0 sm:!w-auto sm:min-w-[820px] sm:!max-w-[820px]"
      @open-auto-focus="handleOpenAutoFocus"
    >
      <DialogHeader class="shrink-0 border-b border-[#eef2f6] px-7 py-6">
        <DialogTitle ref="titleRef" tabindex="-1">
          {{ t('scheduledTasks.operationLog.detailTitle') }}
        </DialogTitle>
      </DialogHeader>

      <div v-if="detail && snapshot && readonlyForm" class="flex-1 overflow-y-auto px-7">
          <TaskFormContent
            :form="readonlyForm"
            :agents="[]"
            :channels="[]"
            :readonly="true"
            :agent-label-override="snapshot.agent_name || String(snapshot.agent_id || '')"
            :notification-platform-label-override="readonlyNotificationPlatformLabel"
            :notification-channel-label-overrides="readonlyChannelLabels"
          />
      </div>
    </DialogContent>
  </Dialog>
</template>
