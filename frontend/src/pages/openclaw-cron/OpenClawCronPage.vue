<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  CircleAlert,
  CircleCheck,
  Clock3,
  LoaderCircle,
  MoreHorizontal,
  Plus,
  RefreshCcw,
} from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import {
  OpenClawCronService,
  type OpenClawCronAgentOption,
  type OpenClawCronDeliveryPlatformOption,
  type OpenClawCronJob,
  type OpenClawCronSummary,
} from '@bindings/chatclaw/internal/openclaw/cron'
import OpenClawCronTaskDialog from './OpenClawCronTaskDialog.vue'
import OpenClawCronHistoryDialog from './OpenClawCronHistoryDialog.vue'
import {
  buildCreateInput,
  buildUpdateInput,
  createEmptyOpenClawCronForm,
  describeOpenClawSchedule,
  formatOpenClawCronTime,
  jobToForm,
  type OpenClawCronFormState,
} from './utils'
import { getLastRunVisualState } from './status'

defineProps<{
  tabId: string
}>()

const { t } = useI18n()
const loading = ref(false)
const saving = ref(false)
const jobs = ref<OpenClawCronJob[]>([])
const summary = ref<OpenClawCronSummary | null>(null)
const agents = ref<OpenClawCronAgentOption[]>([])
const deliveryPlatforms = ref<OpenClawCronDeliveryPlatformOption[]>([])
const createDialogOpen = ref(false)
const editingJob = ref<OpenClawCronJob | null>(null)
const historyJob = ref<OpenClawCronJob | null>(null)
const historyTriggerAtMs = ref<number | null>(null)
const historyRunId = ref<string | null>(null)
const historyConversationId = ref<number | null>(null)
const form = ref<OpenClawCronFormState>(createEmptyOpenClawCronForm())
const deleteDialogOpen = ref(false)
const deleting = ref(false)
const deletingJob = ref<OpenClawCronJob | null>(null)

const JOB_LAST_STATUS_FAILED = 'failed'
const JOB_LAST_STATUS_SUCCESS = 'success'

function displayFailedRunSummaryCount() {
  return jobs.value.filter(
    (job) => showLastRun(job) && lastRunState(job) === JOB_LAST_STATUS_FAILED
  ).length
}

const summaryCards = computed(() => [
  {
    key: 'total',
    label: t('openclawCron.summary.total', '任务总数'),
    value: summary.value?.total ?? 0,
  },
  {
    key: 'enabled',
    label: t('openclawCron.summary.enabled', '运行中'),
    value: summary.value?.enabled ?? 0,
  },
  {
    key: 'disabled',
    label: t('openclawCron.summary.disabled', '已暂停'),
    value: summary.value?.disabled ?? 0,
  },
  {
    key: 'failed',
    label: t('openclawCron.summary.failedRuns', '失败'),
    value: displayFailedRunSummaryCount(),
  },
])

const agentNameMap = computed(() => {
  const entries = new Map<string, string>()
  for (const agent of agents.value) {
    const agentID = String(agent.openclaw_agent_id || '').trim()
    const agentName = String(agent.name || '').trim()
    if (!agentID || !agentName || entries.has(agentID)) continue
    entries.set(agentID, agentName)
  }
  return entries
})

async function reloadAll() {
  loading.value = true
  try {
    const [jobList, summaryValue, agentList, deliveryPlatformList] = await Promise.all([
      OpenClawCronService.ListJobs(),
      OpenClawCronService.GetSummary(),
      OpenClawCronService.ListAgents(),
      OpenClawCronService.ListDeliveryPlatforms(),
    ])
    jobs.value = jobList || []
    summary.value = summaryValue
    agents.value = agentList || []
    deliveryPlatforms.value = deliveryPlatformList || []
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void reloadAll()
})

function openCreateDialog() {
  editingJob.value = null
  form.value = createEmptyOpenClawCronForm()
  if (!form.value.channelPlatform && deliveryPlatforms.value.length === 1) {
    form.value.channelPlatform = deliveryPlatforms.value[0].platform
  }
  createDialogOpen.value = true
}

function openEditDialog(job: OpenClawCronJob) {
  editingJob.value = job
  form.value = jobToForm(job)
  if (!form.value.channelPlatform && deliveryPlatforms.value.length === 1) {
    form.value.channelPlatform = deliveryPlatforms.value[0].platform
  }
  createDialogOpen.value = true
}

function displayAgentName(job: OpenClawCronJob) {
  const explicitName = String(job.agent_name || '').trim()
  if (explicitName) return explicitName

  const agentID = String(job.agent_id || '').trim()
  if (!agentID) return t('openclawCron.dialog.defaultAgent', '默认助手')

  return agentNameMap.value.get(agentID) || agentID
}

function displayJobStatusLabel(job: OpenClawCronJob) {
  if (job.enabled) return t('openclawCron.statusRunning', '运行中')
  return t('openclawCron.disabled', '已暂停')
}

function statusTextClass(job: OpenClawCronJob) {
  return job.enabled ? 'text-[#404040]' : 'text-[#737373]'
}

function lastRunState(job: OpenClawCronJob) {
  return getLastRunVisualState({
    lastStatus: job.last_status,
    lastError: job.last_error,
  })
}

function lastRunIcon(job: OpenClawCronJob) {
  if (lastRunState(job) === JOB_LAST_STATUS_FAILED) return CircleAlert
  if (lastRunState(job) === JOB_LAST_STATUS_SUCCESS) return CircleCheck
  return Clock3
}

function lastRunIconClass(job: OpenClawCronJob) {
  if (lastRunState(job) === JOB_LAST_STATUS_FAILED) return 'text-[#ef4444]'
  if (lastRunState(job) === JOB_LAST_STATUS_SUCCESS) return 'text-[#22c55e]'
  return 'text-[#a3a3a3]'
}

function showLastRun(job: OpenClawCronJob) {
  return Boolean(job.last_run_at_ms)
}

async function handleSubmit() {
  saving.value = true
  try {
    if (editingJob.value?.id) {
      await OpenClawCronService.UpdateJob(editingJob.value.id, buildUpdateInput(form.value))
    } else {
      await OpenClawCronService.CreateJob(buildCreateInput(form.value))
    }
    createDialogOpen.value = false
    await reloadAll()
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    saving.value = false
  }
}

async function handleToggle(job: OpenClawCronJob, enabled: boolean) {
  try {
    if (enabled) {
      await OpenClawCronService.EnableJob(job.id)
    } else {
      await OpenClawCronService.DisableJob(job.id)
    }
    await reloadAll()
  } catch (error) {
    toast.error(getErrorMessage(error))
  }
}

async function handleRun(job: OpenClawCronJob) {
  try {
    const result = await OpenClawCronService.RunJobNow(job.id)
    historyJob.value = job
    historyConversationId.value = Number(result?.conversation_id || 0) || null
    historyTriggerAtMs.value = Number(result?.trigger_at_ms || Date.now())
    historyRunId.value = result?.run_id || null
  } catch (error) {
    toast.error(getErrorMessage(error))
  }
}

function askDelete(job: OpenClawCronJob) {
  deletingJob.value = job
  deleteDialogOpen.value = true
}

async function confirmDelete() {
  if (!deletingJob.value?.id) return
  deleting.value = true
  try {
    await OpenClawCronService.DeleteJob(deletingJob.value.id)
    deleteDialogOpen.value = false
    deletingJob.value = null
    await reloadAll()
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col overflow-y-auto bg-white dark:bg-background">
    <div class="flex h-20 shrink-0 items-center justify-between px-6">
      <div class="flex flex-col gap-1">
        <h1 class="text-base font-semibold text-[#262626] dark:text-foreground">
          {{ t('openclawCron.title', '定时任务') }}
        </h1>
        <p class="text-sm text-[#737373] dark:text-muted-foreground">
          {{ t('openclawCron.subtitle', '通过定时任务自动化执行 AI 任务') }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <Button
          class="h-9 gap-1 border-none bg-[#f5f5f5] text-[#171717] shadow-none hover:bg-[#e5e5e5] dark:bg-muted dark:text-foreground dark:hover:bg-muted/80"
          @click="reloadAll"
        >
          <RefreshCcw class="h-4 w-4 shrink-0" />
          {{ t('openclawCron.refresh', '刷新') }}
        </Button>
        <Button class="h-9 gap-1" variant="default" @click="openCreateDialog">
          <Plus class="h-4 w-4 shrink-0" />
          {{ t('openclawCron.addTask', '新增任务') }}
        </Button>
      </div>
    </div>

    <div class="flex flex-1 min-h-0 flex-col overflow-auto px-6 pb-6">
      <div class="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <div
          v-for="card in summaryCards"
          :key="card.key"
          class="flex items-center gap-4 rounded-2xl border border-[#d9d9d9] bg-white px-6 py-5 shadow-[0px_1px_3px_rgba(0,0,0,0.10),0px_1px_2px_rgba(0,0,0,0.06)]"
        >
          <div
            class="flex size-12 shrink-0 items-center justify-center rounded-full bg-[#f5f5f5] text-[#171717]"
          >
            <Clock3 class="size-5" />
          </div>
          <div class="min-w-0">
            <div class="text-[32px] font-semibold leading-none tracking-[-0.04em] text-[#171717]">
              {{ card.value }}
            </div>
            <div class="mt-1 text-sm text-[#737373]">{{ card.label }}</div>
          </div>
        </div>
      </div>

      <div class="mt-4">
        <div
          v-if="loading"
          class="rounded-2xl border border-[#e5e5e5] bg-white px-4 py-16 text-center text-sm text-[#737373]"
        >
          {{ t('common.loading', 'Loading...') }}
        </div>

        <div
          v-else-if="jobs.length === 0"
          class="flex min-h-[420px] items-center justify-center px-4 py-16"
        >
          <div class="flex w-full max-w-[356px] flex-col items-center gap-4 text-center">
            <div
              class="flex size-10 items-center justify-center rounded-lg bg-[#f5f5f5] text-[#171717]"
            >
              <Clock3 class="size-5" />
            </div>
            <div class="space-y-1">
              <div class="text-base font-medium leading-6 text-[#171717]">
                {{ t('openclawCron.empty', '暂无定时任务') }}
              </div>
              <div class="text-sm leading-5 text-[#737373]">
                {{ t('openclawCron.emptyDescription', '创建后，系统会按设定时间自动执行任务。') }}
              </div>
            </div>
            <button
              type="button"
              class="inline-flex h-9 items-center gap-2 rounded-lg bg-[#171717] px-4 text-sm font-medium text-white transition-colors hover:bg-[#0f0f0f]"
              @click="openCreateDialog"
            >
              <Plus class="size-4" />
              {{ t('openclawCron.create', '创建任务') }}
            </button>
          </div>
        </div>

        <div v-else class="overflow-hidden rounded-lg border border-[#e5e5e5] bg-white">
          <div class="overflow-x-auto">
            <table class="min-w-[920px] w-full table-fixed text-sm">
              <thead class="text-left text-sm text-[#0a0a0a]">
                <tr>
                  <th class="w-[34%] px-5 py-3 font-medium">
                    {{ t('openclawCron.columns.title', '任务') }}
                  </th>
                  <th class="w-[24%] px-5 py-3 font-medium">
                    {{ t('openclawCron.columns.schedule', '执行时间') }}
                  </th>
                  <th class="w-[24%] px-5 py-3 font-medium">
                    {{ t('openclawCron.columns.agent', '关联助手') }}
                  </th>
                  <th class="w-[12%] px-5 py-3 font-medium">
                    {{ t('openclawCron.columns.status', '状态') }}
                  </th>
                  <th
                    class="w-[88px] min-w-[88px] whitespace-nowrap px-5 py-3 text-right font-medium"
                  >
                    {{ t('openclawCron.columns.actions', '操作') }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="job in jobs"
                  :key="job.id"
                  class="border-t border-[#e5e5e5] align-top transition-colors hover:bg-[#fafafa]"
                >
                  <td class="px-5 py-3.5">
                    <div class="min-w-0 max-w-md space-y-1">
                      <div class="truncate text-[15px] font-medium leading-6 text-[#171717]">
                        {{ job.name }}
                      </div>
                      <div class="truncate text-sm leading-5 text-[#8c8c8c]">
                        {{ job.message || job.system_event || job.description }}
                      </div>
                    </div>
                  </td>
                  <td class="px-5 py-3.5">
                    <template v-if="showLastRun(job)">
                      <div class="space-y-1 text-sm">
                        <div class="font-medium leading-6 text-[#171717]">
                          {{ describeOpenClawSchedule(job) }}
                        </div>
                        <div class="flex items-center gap-1.5 text-[#8c8c8c]">
                          <TooltipProvider
                            v-if="lastRunState(job) === JOB_LAST_STATUS_FAILED && job.last_error"
                          >
                            <Tooltip>
                              <TooltipTrigger as-child>
                                <button
                                  type="button"
                                  class="inline-flex shrink-0 items-center justify-center rounded-full"
                                  :aria-label="t('openclawCron.errorReason', '查看错误原因')"
                                >
                                  <component
                                    :is="lastRunIcon(job)"
                                    class="size-4"
                                    :class="lastRunIconClass(job)"
                                  />
                                </button>
                              </TooltipTrigger>
                              <TooltipContent>
                                <p class="max-w-sm whitespace-pre-wrap text-xs">
                                  {{ job.last_error }}
                                </p>
                              </TooltipContent>
                            </Tooltip>
                          </TooltipProvider>
                          <component
                            :is="lastRunIcon(job)"
                            v-else
                            class="size-4 shrink-0"
                            :class="lastRunIconClass(job)"
                          />
                          <span class="truncate">
                            {{ t('openclawCron.lastRunPrefix', '上次: ')
                            }}{{ formatOpenClawCronTime(job.last_run_at_ms) }}
                          </span>
                        </div>
                      </div>
                    </template>
                    <div v-else class="space-y-1 text-sm">
                      <div class="font-medium leading-6 text-[#171717]">
                        {{ describeOpenClawSchedule(job) }}
                      </div>
                    </div>
                  </td>
                  <td class="px-5 py-3.5">
                    <div class="space-y-1">
                      <div class="text-[15px] leading-6 text-[#171717]">
                        {{ displayAgentName(job) }}
                      </div>
                    </div>
                  </td>
                  <td class="px-5 py-3.5">
                    <div class="flex items-center gap-2 whitespace-nowrap">
                      <Switch
                        :model-value="job.enabled"
                        class="h-[18px] w-[33px] border-transparent data-[state=checked]:bg-[#171717] data-[state=unchecked]:bg-[#e5e5e5]"
                        @update:model-value="(value) => handleToggle(job, !!value)"
                      />
                      <span class="text-sm" :class="statusTextClass(job)">
                        {{ displayJobStatusLabel(job) }}
                      </span>
                    </div>
                  </td>
                  <td class="w-[88px] min-w-[88px] px-5 py-3.5 text-right">
                    <DropdownMenu>
                      <DropdownMenuTrigger as-child>
                        <button
                          type="button"
                          class="inline-flex size-9 items-center justify-center rounded-lg text-[#737373] transition-colors hover:bg-[#f5f5f5] hover:text-[#171717]"
                          :aria-label="t('openclawCron.actionsMenu', '操作菜单')"
                        >
                          <MoreHorizontal class="size-4" />
                        </button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent
                        align="end"
                        class="w-[104px] rounded-md border-[#ececec] p-1 shadow-[0px_6px_30px_rgba(0,0,0,0.05),0px_16px_24px_rgba(0,0,0,0.04),0px_8px_10px_rgba(0,0,0,0.08)]"
                      >
                        <DropdownMenuItem @select="handleRun(job)">{{
                          t('openclawCron.runNow', '立即运行')
                        }}</DropdownMenuItem>
                        <DropdownMenuItem @select="historyJob = job">{{
                          t('openclawCron.historyAction', '历史')
                        }}</DropdownMenuItem>
                        <DropdownMenuItem @select="openEditDialog(job)">{{
                          t('openclawCron.edit', '编辑')
                        }}</DropdownMenuItem>
                        <DropdownMenuItem
                          class="text-red-600 focus:text-red-600"
                          @select="askDelete(job)"
                        >
                          {{ t('openclawCron.delete', '删除') }}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <OpenClawCronTaskDialog
      :open="createDialogOpen"
      :saving="saving"
      :title="
        editingJob ? t('openclawCron.edit', '编辑任务') : t('openclawCron.create', '创建任务')
      "
      :form="form"
      :agents="agents"
      :delivery-platforms="deliveryPlatforms"
      @update:open="(value) => (createDialogOpen = value)"
      @submit="handleSubmit"
    />

    <OpenClawCronHistoryDialog
      :open="!!historyJob"
      :job="historyJob"
      :conversation-id="historyConversationId"
      :trigger-at-ms="historyTriggerAtMs"
      :run-id="historyRunId"
      @update:open="
        (value) => {
          if (!value) {
            historyJob = null
            historyConversationId = null
            historyTriggerAtMs = null
            historyRunId = null
          }
        }
      "
    />

    <AlertDialog
      :open="deleteDialogOpen"
      @update:open="(value) => !value && (deleteDialogOpen = false)"
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('openclawCron.deleteTitle', '删除任务') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ `确认删除任务 ${deletingJob?.name || ''}？` }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel :disabled="deleting" @click="deleteDialogOpen = false">
            {{ t('common.cancel') }}
          </AlertDialogCancel>
          <Button :disabled="deleting" variant="default" @click.prevent="confirmDelete">
            <LoaderCircle v-if="deleting" class="size-4 shrink-0 animate-spin" />
            {{ t('openclawCron.confirmDelete', '确认删除') }}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
