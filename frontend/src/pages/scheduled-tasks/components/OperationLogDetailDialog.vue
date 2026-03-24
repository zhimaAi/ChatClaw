<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import type { ScheduledTaskOperationLogDetail } from '../types'
import { operationSnapshotToForm } from '../utils'
import TaskFormContent from './TaskFormContent.vue'

const props = defineProps<{
  open: boolean
  detail: ScheduledTaskOperationLogDetail | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const { t } = useI18n()
const snapshot = computed(() => props.detail?.task_snapshot ?? null)
const readonlyForm = computed(() =>
  snapshot.value ? operationSnapshotToForm(snapshot.value) : null
)
const readonlyChannelLabels = computed(() => {
  if (!snapshot.value?.notification_channels || snapshot.value.notification_channels === '-') {
    return []
  }
  const [, rawLabels = snapshot.value.notification_channels] =
    snapshot.value.notification_channels.split(':')
  return rawLabels
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
})

</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent
      class="flex max-h-[90vh] flex-col overflow-hidden p-0 sm:!w-auto sm:min-w-[820px] sm:!max-w-[820px]"
    >
      <DialogHeader class="shrink-0 border-b border-[#eef2f6] px-7 py-6">
        <DialogTitle>{{ t('scheduledTasks.operationLog.detailTitle') }}</DialogTitle>
      </DialogHeader>

      <div v-if="detail && snapshot && readonlyForm" class="flex-1 overflow-y-auto px-7">
          <TaskFormContent
            :form="readonlyForm"
            :agents="[]"
            :channels="[]"
            :readonly="true"
            :agent-label-override="snapshot.agent_name || String(snapshot.agent_id || '')"
            :notification-platform-label-override="snapshot.notification_platform || ''"
            :notification-channel-label-overrides="readonlyChannelLabels"
          />
      </div>
    </DialogContent>
  </Dialog>
</template>
