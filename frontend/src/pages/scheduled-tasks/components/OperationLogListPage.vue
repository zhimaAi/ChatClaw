<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ScheduledTasksService } from '@bindings/chatclaw/internal/services/scheduledtasks'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import type { ScheduledTaskOperationLog, ScheduledTaskOperationLogDetail } from '../types'
import { formatTaskTime } from '../utils'
import {
  buildOperationLogDisplayRows,
  OPERATION_LOG_EMPTY_FIELD_VALUE,
} from '../operationLogTable'
import OperationLogDetailDialog from './OperationLogDetailDialog.vue'

// Keep the operation-log page copy centralized so the table stays consistent.
const OPERATION_LOG_LOADING_TEXT = '加载中...'
const OPERATION_LOG_EMPTY_TEXT = '暂无操作记录'
const OPERATION_LOG_ACTION_VIEW_DETAIL = '查看详情'
const OPERATION_LOG_OPERATION_TYPE_CREATE = '创建任务'
const OPERATION_LOG_OPERATION_TYPE_DELETE = '删除任务'
const OPERATION_LOG_OPERATION_TYPE_UPDATE = '修改任务'
const OPERATION_LOG_OPERATION_SOURCE_AI = 'AI助手'
const OPERATION_LOG_OPERATION_SOURCE_MANUAL = '手动'
const TABLE_COLUMN_TASK = '任务'
const TABLE_COLUMN_OPERATION_TYPE = '操作类型'
const TABLE_COLUMN_OPERATION_SOURCE = '操作方式'
const TABLE_COLUMN_CHANGED_FIELD = '操作项'
const TABLE_COLUMN_BEFORE = '修改前'
const TABLE_COLUMN_AFTER = '修改后'
const TABLE_COLUMN_TIME = '操作时间'
const TABLE_COLUMN_ACTION = '操作'

const loading = ref(false)
const logs = ref<ScheduledTaskOperationLog[]>([])
const detailOpen = ref(false)
const selectedDetail = ref<ScheduledTaskOperationLogDetail | null>(null)
const displayRows = computed(() => buildOperationLogDisplayRows(logs.value as any))

function displayOperationType(value: string) {
  if (value === 'create') return OPERATION_LOG_OPERATION_TYPE_CREATE
  if (value === 'delete') return OPERATION_LOG_OPERATION_TYPE_DELETE
  return OPERATION_LOG_OPERATION_TYPE_UPDATE
}

function displayOperationSource(value: string) {
  if (value === 'ai') return OPERATION_LOG_OPERATION_SOURCE_AI
  return OPERATION_LOG_OPERATION_SOURCE_MANUAL
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
        {{ OPERATION_LOG_LOADING_TEXT }}
      </div>

      <div
        v-else-if="logs.length === 0"
        class="rounded-xl bg-[#fafafa] px-4 py-10 text-center text-sm text-[#737373]"
      >
        {{ OPERATION_LOG_EMPTY_TEXT }}
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
            <th class="px-4 py-3 font-semibold">{{ TABLE_COLUMN_TASK }}</th>
            <th class="px-4 py-3 font-semibold">{{ TABLE_COLUMN_OPERATION_TYPE }}</th>
            <th class="px-4 py-3 font-semibold">{{ TABLE_COLUMN_OPERATION_SOURCE }}</th>
            <th class="px-4 py-3 font-semibold">{{ TABLE_COLUMN_CHANGED_FIELD }}</th>
            <th class="px-4 py-3 font-semibold">{{ TABLE_COLUMN_BEFORE }}</th>
            <th class="px-4 py-3 font-semibold">{{ TABLE_COLUMN_AFTER }}</th>
            <th class="px-4 py-3 font-semibold">{{ TABLE_COLUMN_TIME }}</th>
            <th class="px-4 py-3 font-semibold">{{ TABLE_COLUMN_ACTION }}</th>
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
                {{ OPERATION_LOG_ACTION_VIEW_DETAIL }}
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
