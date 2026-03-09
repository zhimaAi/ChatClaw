<script setup lang="ts">
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Switch } from '@/components/ui/switch'
import TaskRunStatusBadge from './TaskRunStatusBadge.vue'
import type { ScheduledTask } from '../types'
import { describeSchedule, formatTaskTime } from '../utils'

defineProps<{
  tasks: ScheduledTask[]
}>()

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
</script>

<template>
  <div class="overflow-hidden rounded-xl border border-border bg-card">
    <div class="flex items-center justify-between border-b border-border px-4 py-3">
      <div class="text-sm font-medium text-foreground">任务列表</div>
      <button class="rounded-md bg-foreground px-3 py-1.5 text-sm text-background" @click="emit('create')">
        新建任务
      </button>
    </div>

    <div v-if="tasks.length === 0" class="px-4 py-12 text-center text-sm text-muted-foreground">
      暂无定时任务
    </div>

    <div v-else class="overflow-x-auto">
      <table class="min-w-full text-sm">
        <thead class="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            <th class="px-4 py-3 font-medium">任务</th>
            <th class="px-4 py-3 font-medium">执行时间</th>
            <th class="px-4 py-3 font-medium">上次运行</th>
            <th class="px-4 py-3 font-medium">下次运行</th>
            <th class="px-4 py-3 font-medium">状态</th>
            <th class="px-4 py-3 font-medium">启用</th>
            <th class="px-4 py-3 font-medium">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="task in tasks" :key="task.id" class="border-t border-border align-top">
            <td class="px-4 py-3">
              <div class="font-medium text-foreground">{{ task.name }}</div>
              <div class="mt-1 line-clamp-2 max-w-md text-xs text-muted-foreground">{{ task.prompt }}</div>
            </td>
            <td class="px-4 py-3 text-muted-foreground">{{ describeSchedule(task) }}</td>
            <td class="px-4 py-3 text-muted-foreground">{{ formatTaskTime(task.last_run_at) }}</td>
            <td class="px-4 py-3 text-muted-foreground">{{ formatTaskTime(task.next_run_at) }}</td>
            <td class="px-4 py-3">
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger as-child>
                    <div class="inline-flex">
                      <TaskRunStatusBadge :status="displayTaskStatus(task)" />
                    </div>
                  </TooltipTrigger>
                  <TooltipContent v-if="task.last_error">
                    <p class="max-w-sm whitespace-pre-wrap text-xs">{{ task.last_error }}</p>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </td>
            <td class="px-4 py-3">
              <Switch :model-value="task.enabled" @update:model-value="(value) => emit('toggle', task, !!value)" />
            </td>
            <td class="px-4 py-3">
              <div class="flex flex-wrap gap-2">
                <button class="rounded-md border border-border px-2 py-1 text-xs" @click="emit('run', task)">立即运行</button>
                <button class="rounded-md border border-border px-2 py-1 text-xs" @click="emit('history', task)">历史记录</button>
                <button class="rounded-md border border-border px-2 py-1 text-xs" @click="emit('edit', task)">编辑</button>
                <button class="rounded-md border border-border px-2 py-1 text-xs text-red-600" @click="emit('delete', task)">删除</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
