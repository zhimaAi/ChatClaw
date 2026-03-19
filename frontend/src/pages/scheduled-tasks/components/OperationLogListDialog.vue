<script setup lang="ts">
import { ref, watch } from 'vue'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ScheduledTasksService } from '@bindings/chatclaw/internal/services/scheduledtasks'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import type { ScheduledTaskOperationLog, ScheduledTaskOperationLogDetail } from '../types'
import { formatTaskTime } from '../utils'
import OperationLogDetailDialog from './OperationLogDetailDialog.vue'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const loading = ref(false)
const logs = ref<ScheduledTaskOperationLog[]>([])
const detailOpen = ref(false)
const selectedDetail = ref<ScheduledTaskOperationLogDetail | null>(null)

function displayOperationType(value: string) {
  if (value === 'create') return '创建任务'
  if (value === 'delete') return '删除任务'
  return '修改任务'
}

function displayOperationSource(value: string) {
  if (value === 'ai') return 'AI助手'
  return '手动'
}

async function loadLogs() {
  loading.value = true
  try {
    const result = await ScheduledTasksService.ListScheduledTaskOperationLogs(0, 1, 100)
    logs.value = Array.isArray(result) ? result : []
  } catch (error) {
    logs.value = []
    toast.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

async function viewDetail(log: ScheduledTaskOperationLog) {
  try {
    selectedDetail.value = await ScheduledTasksService.GetScheduledTaskOperationLogDetail(log.id)
    detailOpen.value = true
  } catch (error) {
    toast.error(getErrorMessage(error))
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) {
      void loadLogs()
    }
  },
  { immediate: true }
)
</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent class="max-h-[90vh] overflow-hidden sm:!w-auto sm:min-w-[1080px] sm:!max-w-[1180px]">
      <DialogHeader>
        <DialogTitle>操作记录</DialogTitle>
      </DialogHeader>

      <div class="overflow-auto">
        <div
          v-if="loading"
          class="rounded-xl border border-[#e5e7eb] bg-[#fafafa] px-4 py-10 text-center text-sm text-[#737373]"
        >
          加载中...
        </div>

        <div
          v-else-if="logs.length === 0"
          class="rounded-xl border border-[#e5e7eb] bg-[#fafafa] px-4 py-10 text-center text-sm text-[#737373]"
        >
          暂无操作记录
        </div>

        <table v-else class="min-w-full border-collapse text-sm">
          <thead>
            <tr class="border-b border-[#e5e7eb] text-left text-[#171717]">
              <th class="px-4 py-3 font-semibold">任务</th>
              <th class="px-4 py-3 font-semibold">操作类型</th>
              <th class="px-4 py-3 font-semibold">操作方式</th>
              <th class="px-4 py-3 font-semibold">操作项</th>
              <th class="px-4 py-3 font-semibold">修改前</th>
              <th class="px-4 py-3 font-semibold">修改后</th>
              <th class="px-4 py-3 font-semibold">操作时间</th>
              <th class="px-4 py-3 font-semibold">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="log in logs"
              :key="log.id"
              class="border-b border-[#f1f5f9] align-top text-[#171717]"
            >
              <td class="px-4 py-4">{{ log.task_name_snapshot || '-' }}</td>
              <td class="px-4 py-4">{{ displayOperationType(log.operation_type) }}</td>
              <td class="px-4 py-4">{{ displayOperationSource(log.operation_source) }}</td>
              <td class="px-4 py-4">
                <div class="space-y-2">
                  <div v-for="item in log.changed_fields" :key="`${log.id}-${item.field_key}`">
                    {{ item.field_label || '-' }}
                  </div>
                </div>
              </td>
              <td class="px-4 py-4">
                <div class="space-y-2">
                  <div v-for="item in log.changed_fields" :key="`${log.id}-${item.field_key}-before`">
                    {{ item.before || '--' }}
                  </div>
                </div>
              </td>
              <td class="px-4 py-4">
                <div class="space-y-2">
                  <div v-for="item in log.changed_fields" :key="`${log.id}-${item.field_key}-after`">
                    {{ item.after || '--' }}
                  </div>
                </div>
              </td>
              <td class="whitespace-nowrap px-4 py-4">{{ formatTaskTime(log.created_at) }}</td>
              <td class="px-4 py-4">
                <button
                  type="button"
                  class="text-[#2563eb] transition-colors hover:text-[#1d4ed8]"
                  @click="viewDetail(log)"
                >
                  查看详情
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </DialogContent>
  </Dialog>

  <OperationLogDetailDialog
    :open="detailOpen"
    :detail="selectedDetail"
    @update:open="(value) => (detailOpen = value)"
  />
</template>
