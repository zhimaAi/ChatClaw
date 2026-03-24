<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ScheduledTasksService } from '@bindings/chatclaw/internal/services/scheduledtasks'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import type { ScheduledTaskOperationLog, ScheduledTaskOperationLogDetail } from '../types'
import { formatTaskTime } from '../utils'
import {
  buildOperationLogDisplayRows,
  OPERATION_LOG_EMPTY_FIELD_VALUE,
} from '../operationLogTable'
import {
  createScheduleTextFormatter,
  getOperationLogOperationSourceLabel,
  getOperationLogOperationTypeLabel,
} from '../scheduledTaskText'
import OperationLogDetailDialog from './OperationLogDetailDialog.vue'

const { t } = useI18n()
const loading = ref(false)
const logs = ref<ScheduledTaskOperationLog[]>([])
const detailOpen = ref(false)
const selectedDetail = ref<ScheduledTaskOperationLogDetail | null>(null)
const scheduleFormatter = computed(() => createScheduleTextFormatter(t))
const displayRows = computed(() => buildOperationLogDisplayRows(logs.value as any, scheduleFormatter.value))

function displayOperationType(value: string) {
  return getOperationLogOperationTypeLabel(value, t)
}

function displayOperationSource(value: string) {
  return getOperationLogOperationSourceLabel(value, t)
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

onMounted(() => {
  void loadLogs()
})

function findLogById(logId: number) {
  return logs.value.find((item) => item.id === logId) || null
}

function handleViewDetail(logId: number) {
  const targetLog = findLogById(logId)
  if (!targetLog) return
  void viewDetail(targetLog)
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-[#e5e7eb] bg-white">
    <div class="overflow-auto">
      <div
        v-if="loading"
        class="rounded-xl bg-[#fafafa] px-4 py-10 text-center text-sm text-[#737373]"
      >
        {{ t('common.loading') }}
      </div>

      <div
        v-else-if="logs.length === 0"
        class="rounded-xl bg-[#fafafa] px-4 py-10 text-center text-sm text-[#737373]"
      >
        {{ t('scheduledTasks.operationLog.empty') }}
      </div>

      <table v-else class="min-w-full table-fixed border-collapse text-sm">
        <colgroup>
          <col class="w-[220px]" />
          <col class="w-[132px]" />
          <col class="w-[132px]" />
          <col class="w-[220px]" />
          <col class="w-[240px]" />
          <col class="w-[240px]" />
          <col class="w-[180px]" />
          <col class="w-[108px]" />
        </colgroup>
        <thead>
          <tr class="border-b border-[#e5e7eb] text-left text-[#171717]">
            <th class="px-4 py-3 font-semibold">{{ t('scheduledTasks.operationLog.columns.task') }}</th>
            <th class="px-4 py-3 font-semibold">{{ t('scheduledTasks.operationLog.columns.operationType') }}</th>
            <th class="px-4 py-3 font-semibold">{{ t('scheduledTasks.operationLog.columns.operationSource') }}</th>
            <th class="px-4 py-3 font-semibold">{{ t('scheduledTasks.operationLog.columns.changedField') }}</th>
            <th class="px-4 py-3 font-semibold">{{ t('scheduledTasks.operationLog.columns.before') }}</th>
            <th class="px-4 py-3 font-semibold">{{ t('scheduledTasks.operationLog.columns.after') }}</th>
            <th class="px-4 py-3 font-semibold">{{ t('scheduledTasks.operationLog.columns.time') }}</th>
            <th class="px-4 py-3 font-semibold">{{ t('scheduledTasks.operationLog.columns.action') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in displayRows"
            :key="row.rowKey"
            class="border-b border-[#f1f5f9] align-top text-[#171717]"
          >
            <td class="px-4 py-4">
              <div
                v-if="row.showSharedColumns"
                class="truncate whitespace-nowrap"
                :title="row.taskName || OPERATION_LOG_EMPTY_FIELD_VALUE"
              >
                {{ row.taskName || OPERATION_LOG_EMPTY_FIELD_VALUE }}
              </div>
            </td>
            <td class="px-4 py-4">
              <div v-if="row.showSharedColumns" class="truncate whitespace-nowrap">
                {{ displayOperationType(row.operationType) }}
              </div>
            </td>
            <td class="px-4 py-4">
              <div v-if="row.showSharedColumns" class="truncate whitespace-nowrap">
                {{ displayOperationSource(row.operationSource) }}
              </div>
            </td>
            <td class="px-4 py-4">
              <div class="truncate whitespace-nowrap" :title="row.fieldLabel">
                {{ row.fieldLabel }}
              </div>
            </td>
            <td class="px-4 py-4">
              <div class="truncate whitespace-nowrap" :title="row.beforeValue">
                {{ row.beforeValue }}
              </div>
            </td>
            <td class="px-4 py-4">
              <div class="truncate whitespace-nowrap" :title="row.afterValue">
                {{ row.afterValue }}
              </div>
            </td>
            <td class="px-4 py-4">
              <div v-if="row.showSharedColumns" class="truncate whitespace-nowrap">
                {{ formatTaskTime(row.createdAt) }}
              </div>
            </td>
            <td class="px-4 py-4">
              <button
                v-if="row.showSharedColumns"
                type="button"
                class="truncate whitespace-nowrap text-[#2563eb] transition-colors hover:text-[#1d4ed8]"
                @click="handleViewDetail(row.logId)"
              >
                {{ t('scheduledTasks.operationLog.viewDetail') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>

  <OperationLogDetailDialog
    :open="detailOpen"
    :detail="selectedDetail"
    @update:open="(value) => (detailOpen = value)"
  />
</template>
