<script setup lang="ts">
import { computed } from 'vue'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import type { ScheduledTaskOperationLogDetail } from '../types'
import { formatTaskTime, operationSnapshotToForm } from '../utils'
import TaskFormContent from './TaskFormContent.vue'

const props = defineProps<{
  open: boolean
  detail: ScheduledTaskOperationLogDetail | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

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

function displayOperationType(value?: string) {
  if (value === 'create') return '创建任务'
  if (value === 'delete') return '删除任务'
  return '修改任务'
}

function displayOperationSource(value?: string) {
  if (value === 'ai') return 'AI助手'
  return '手动'
}
</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent
      class="flex max-h-[90vh] flex-col overflow-hidden p-0 sm:!w-auto sm:min-w-[820px] sm:!max-w-[820px]"
    >
      <DialogHeader class="shrink-0 border-b border-[#eef2f6] px-7 py-6">
        <DialogTitle>操作记录详情</DialogTitle>
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
