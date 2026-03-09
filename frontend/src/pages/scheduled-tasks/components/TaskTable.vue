<script setup lang="ts">
import { computed } from 'vue'
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
import TaskRunStatusBadge from './TaskRunStatusBadge.vue'
import type { ScheduledTask } from '../types'
import { describeSchedule, formatTaskTime } from '../utils'

const props = defineProps<{
  tasks: ScheduledTask[]
}>()

const { t } = useI18n()

const emit = defineEmits<{
  create: []
  edit: [task: ScheduledTask]
  delete: [task: ScheduledTask]
  run: [task: ScheduledTask]
  history: [task: ScheduledTask]
  toggle: [task: ScheduledTask, enabled: boolean]
}>()

function displayTaskStatus(task: ScheduledTask) {
  if (!task.enabled) return 'paused'
  if (task.last_status === 'running') return 'running'
  if (task.last_status === 'failed') return 'failed'
  if (task.last_status === 'success') return 'success'
  return 'pending'
}

function displayTaskStatusLabel(task: ScheduledTask) {
  const status = displayTaskStatus(task)
  if (status === 'paused') return t('scheduledTasks.disabled')
  if (status === 'running') return t('scheduledTasks.statusRunning')
  if (status === 'failed') return t('scheduledTasks.statusFailed')
  if (status === 'success') return t('scheduledTasks.statusSuccess')
  return t('scheduledTasks.statusPending')
}

function lastRunIcon(task: ScheduledTask) {
  if (task.last_status === 'failed') return CircleAlert
  if (task.last_status === 'success') return CircleCheck
  return Clock3
}

function lastRunIconClass(task: ScheduledTask) {
  if (task.last_status === 'failed') return 'text-red-500'
  if (task.last_status === 'success') return 'text-emerald-500'
  return 'text-muted-foreground/70'
}

const hasTasks = computed(() => props.tasks.length > 0)
</script>

<template>
  <div class="overflow-hidden rounded-xl border border-border bg-card">
    <div class="flex items-center justify-between border-b border-border px-4 py-3">
      <div class="text-sm font-medium text-foreground">{{ t('scheduledTasks.listTitle') }}</div>
      <button
        class="rounded-md bg-foreground px-3 py-1.5 text-sm text-background"
        @click="emit('create')"
      >
        {{ t('scheduledTasks.create') }}
      </button>
    </div>

    <div v-if="!hasTasks" class="px-4 py-12 text-center text-sm text-muted-foreground">
      {{ t('scheduledTasks.empty') }}
    </div>

    <div v-else class="overflow-x-auto">
      <table class="min-w-full text-sm">
        <thead class="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            <th class="px-4 py-3 font-medium">{{ t('scheduledTasks.columns.title') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('scheduledTasks.columns.schedule') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('scheduledTasks.columns.lastRun') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('scheduledTasks.columns.nextRun') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('scheduledTasks.columns.status') }}</th>
            <th class="px-4 py-3 text-right font-medium">
              {{ t('scheduledTasks.columns.actions') }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="task in tasks"
            :key="task.id"
            class="border-t border-border align-top transition-colors hover:bg-muted/20"
          >
            <td class="px-4 py-3">
              <div class="max-w-md">
                <div class="font-medium text-foreground">{{ task.name }}</div>
                <div class="mt-1 line-clamp-2 text-xs text-muted-foreground">{{ task.prompt }}</div>
              </div>
            </td>
            <td class="px-4 py-3">
              <div class="flex items-center gap-2 text-muted-foreground">
                <Clock3 class="size-4 shrink-0 text-muted-foreground/70" />
                <span>{{ describeSchedule(task) }}</span>
              </div>
            </td>
            <td class="px-4 py-3">
              <div v-if="task.last_run_at" class="flex items-center gap-2 text-muted-foreground">
                <component
                  :is="lastRunIcon(task)"
                  class="size-4 shrink-0"
                  :class="lastRunIconClass(task)"
                />
                <span>{{ formatTaskTime(task.last_run_at) }}</span>
                <TooltipProvider v-if="task.last_error">
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <button
                        type="button"
                        class="inline-flex size-5 items-center justify-center rounded-full text-amber-600 transition-colors hover:bg-amber-500/10"
                        :aria-label="t('scheduledTasks.errorReason')"
                      >
                        <CircleAlert class="size-3.5" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent>
                      <p class="max-w-sm whitespace-pre-wrap text-xs">{{ task.last_error }}</p>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              </div>
              <span v-else class="text-muted-foreground">-</span>
            </td>
            <td class="px-4 py-3 text-muted-foreground">{{ formatTaskTime(task.next_run_at) }}</td>
            <td class="px-4 py-3">
              <div class="flex min-w-[180px] items-center justify-between gap-3">
                <TaskRunStatusBadge
                  :status="displayTaskStatus(task)"
                  :label="displayTaskStatusLabel(task)"
                />
                <Switch
                  :model-value="task.enabled"
                  @update:model-value="(value) => emit('toggle', task, !!value)"
                />
              </div>
            </td>
            <td class="px-4 py-3 text-right">
              <DropdownMenu>
                <DropdownMenuTrigger as-child>
                  <button
                    type="button"
                    class="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                    :aria-label="t('scheduledTasks.actionsMenu')"
                  >
                    <MoreHorizontal class="size-4" />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" class="w-36">
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
