<script setup lang="ts">
import { CircleAlert, CircleCheck, Clock3, MoreHorizontal } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Switch } from '@/components/ui/switch'
import type { Agent, ScheduledTask } from '../types'
import { buildTaskTableDisplay } from './taskTableDisplay'
import { describeSchedule, formatTaskTime } from '../utils'

const props = defineProps<{
  tasks: ScheduledTask[]
  agents: Agent[]
}>()

const { t } = useI18n()

const emit = defineEmits<{
  edit: [task: ScheduledTask]
  delete: [task: ScheduledTask]
  run: [task: ScheduledTask]
  history: [task: ScheduledTask]
  toggle: [task: ScheduledTask, enabled: boolean]
}>()

function displayTaskStatus(task: ScheduledTask) {
  return task.enabled ? 'running' : 'paused'
}

function displayTaskStatusLabel(task: ScheduledTask) {
  const status = displayTaskStatus(task)
  if (status === 'paused') return t('scheduledTasks.disabled')
  if (status === 'running') return t('scheduledTasks.statusRunning')
  return t('scheduledTasks.statusPending')
}

function lastRunIcon(task: ScheduledTask) {
  if (task.last_status === 'failed') return CircleAlert
  if (task.last_status === 'success') return CircleCheck
  return Clock3
}

function lastRunIconClass(task: ScheduledTask) {
  if (task.last_status === 'failed') return 'text-[#ef4444]'
  if (task.last_status === 'success') return 'text-[#22c55e]'
  return 'text-[#a3a3a3]'
}

function statusTextClass(task: ScheduledTask) {
  return task.enabled ? 'text-[#404040]' : 'text-[#737373]'
}
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-[#e5e5e5] bg-white">
    <div class="overflow-x-auto">
      <table class="min-w-[920px] w-full table-fixed text-sm">
        <thead class="text-left text-sm text-[#0a0a0a]">
          <tr>
            <th class="w-[34%] px-5 py-3 font-medium">{{ t('scheduledTasks.columns.title') }}</th>
            <th class="w-[24%] px-5 py-3 font-medium">{{ t('scheduledTasks.columns.schedule') }}</th>
            <th class="w-[24%] px-5 py-3 font-medium">{{ t('scheduledTasks.columns.agent') }}</th>
            <th class="w-[12%] px-5 py-3 font-medium">{{ t('scheduledTasks.columns.status') }}</th>
            <th class="w-[88px] min-w-[88px] whitespace-nowrap px-5 py-3 text-right font-medium">
              {{ t('scheduledTasks.columns.actions') }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="task in tasks"
            :key="task.id"
            class="border-t border-[#e5e5e5] align-top transition-colors hover:bg-[#fafafa]"
          >
            <td class="px-5 py-3.5">
              <div class="min-w-0 max-w-md space-y-1">
                <div class="truncate text-[15px] font-medium leading-6 text-[#171717]">
                  {{ task.name }}
                </div>
                <div class="truncate text-sm leading-5 text-[#8c8c8c]">{{ task.prompt }}</div>
              </div>
            </td>
            <td class="px-5 py-3.5">
              <template v-if="buildTaskTableDisplay(task, agents).schedule.showLastRun">
              <div class="space-y-1 text-sm">
                <div class="font-medium leading-6 text-[#171717]">{{ describeSchedule(task) }}</div>
                <div class="flex items-center gap-1.5 text-[#8c8c8c]">
                  <TooltipProvider v-if="task.last_status === 'failed' && task.last_error">
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <button
                          type="button"
                          class="inline-flex shrink-0 items-center justify-center rounded-full"
                          :aria-label="t('scheduledTasks.errorReason')"
                        >
                          <component
                            :is="lastRunIcon(task)"
                            class="size-4"
                            :class="lastRunIconClass(task)"
                          />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>
                        <p class="max-w-sm whitespace-pre-wrap text-xs">{{ task.last_error }}</p>
                      </TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                  <component
                    :is="lastRunIcon(task)"
                    v-else
                    class="size-4 shrink-0"
                    :class="lastRunIconClass(task)"
                  />
                  <span class="truncate">
                    {{ t('scheduledTasks.lastRunPrefix') }}{{ formatTaskTime(task.last_run_at) }}
                  </span>
                </div>
              </div>
              </template>
              <div v-else class="space-y-1 text-sm">
                <div class="font-medium leading-6 text-[#171717]">{{ describeSchedule(task) }}</div>
              </div>
            </td>
            <td class="px-5 py-3.5">
              <div class="space-y-1">
                <div class="text-[15px] leading-6 text-[#171717]">
                  {{ buildTaskTableDisplay(task, agents).agent.name }}
                </div>
              </div>
            </td>
            <td class="px-5 py-3.5">
              <div class="flex items-center gap-2 whitespace-nowrap">
                <Switch
                  :model-value="task.enabled"
                  class="h-[18px] w-[33px] border-transparent data-[state=checked]:bg-[#171717] data-[state=unchecked]:bg-[#e5e5e5]"
                  @update:model-value="(value) => emit('toggle', task, !!value)"
                />
                <span class="text-sm" :class="statusTextClass(task)">
                  {{ displayTaskStatusLabel(task) }}
                </span>
              </div>
            </td>
            <td class="w-[88px] min-w-[88px] px-5 py-3.5 text-right">
              <DropdownMenu>
                <DropdownMenuTrigger as-child>
                  <button
                    type="button"
                    class="inline-flex size-9 items-center justify-center rounded-lg text-[#737373] transition-colors hover:bg-[#f5f5f5] hover:text-[#171717]"
                    :aria-label="t('scheduledTasks.actionsMenu')"
                  >
                    <MoreHorizontal class="size-4" />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  align="end"
                  class="w-[92px] rounded-md border-[#ececec] p-1 shadow-[0px_6px_30px_rgba(0,0,0,0.05),0px_16px_24px_rgba(0,0,0,0.04),0px_8px_10px_rgba(0,0,0,0.08)]"
                >
                  <DropdownMenuItem @select="emit('run', task)">{{
                    t('scheduledTasks.runNow')
                  }}</DropdownMenuItem>
                  <DropdownMenuItem @select="emit('history', task)">{{
                    t('scheduledTasks.history')
                  }}</DropdownMenuItem>
                  <DropdownMenuItem @select="emit('edit', task)">{{
                    t('scheduledTasks.edit')
                  }}</DropdownMenuItem>
                  <DropdownMenuItem
                    class="text-red-600 focus:text-red-600"
                    @select="emit('delete', task)"
                  >
                    {{ t('scheduledTasks.delete') }}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
