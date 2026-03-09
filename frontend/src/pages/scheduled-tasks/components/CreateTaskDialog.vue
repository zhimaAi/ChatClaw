<script setup lang="ts">
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { WEEKDAY_OPTIONS } from '../constants'
import type { Agent, ScheduledTaskFormState } from '../types'

defineProps<{
  open: boolean
  saving: boolean
  title: string
  form: ScheduledTaskFormState
  agents: Agent[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  submit: []
}>()
</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent class="max-h-[90vh] overflow-auto sm:max-w-3xl">
      <DialogHeader>
        <DialogTitle>{{ title }}</DialogTitle>
      </DialogHeader>

      <div class="grid gap-4 py-2 md:grid-cols-2">
        <div class="md:col-span-2">
          <label class="mb-1 block text-sm font-medium">任务名称</label>
          <Input v-model="form.name" placeholder="请输入任务名称" />
        </div>

        <div class="md:col-span-2">
          <label class="mb-1 block text-sm font-medium">提示词</label>
          <textarea
            v-model="form.prompt"
            class="min-h-28 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            placeholder="请输入每次自动发送的提示词"
          />
        </div>

        <div>
          <label class="mb-1 block text-sm font-medium">AI 助手</label>
          <select v-model.number="form.agentId" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm">
            <option :value="null">请选择 AI 助手</option>
            <option v-for="agent in agents" :key="agent.id" :value="agent.id">{{ agent.name }}</option>
          </select>
        </div>

        <div class="flex items-center justify-between rounded-md border border-input px-3 py-2">
          <span class="text-sm">启用任务</span>
          <Switch :model-value="form.enabled" @update:model-value="(value) => (form.enabled = !!value)" />
        </div>

        <div class="md:col-span-2">
          <label class="mb-1 block text-sm font-medium">调度方式</label>
          <div class="grid grid-cols-3 gap-2">
            <button
              type="button"
              class="rounded-md border px-3 py-2 text-sm"
              :class="form.scheduleType === 'preset' ? 'border-foreground bg-accent text-foreground' : 'border-input'"
              @click="form.scheduleType = 'preset'"
            >
              快捷设置
            </button>
            <button
              type="button"
              class="rounded-md border px-3 py-2 text-sm"
              :class="form.scheduleType === 'custom' ? 'border-foreground bg-accent text-foreground' : 'border-input'"
              @click="form.scheduleType = 'custom'"
            >
              自定义时间
            </button>
            <button
              type="button"
              class="rounded-md border px-3 py-2 text-sm"
              :class="form.scheduleType === 'cron' ? 'border-foreground bg-accent text-foreground' : 'border-input'"
              @click="form.scheduleType = 'cron'"
            >
              Cron
            </button>
          </div>
        </div>

        <div v-if="form.scheduleType === 'preset'" class="md:col-span-2">
          <label class="mb-1 block text-sm font-medium">快捷设置</label>
          <select v-model="form.schedulePreset" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm">
            <option value="every_hour">每小时整点</option>
            <option value="every_day_0900">每天 09:00</option>
            <option value="weekdays_0900">工作日 09:00</option>
            <option value="every_monday_0900">每周一 09:00</option>
          </select>
        </div>

        <template v-if="form.scheduleType === 'custom'">
          <div>
            <label class="mb-1 block text-sm font-medium">频率</label>
            <select v-model="form.customMode" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm">
              <option value="daily">每天</option>
              <option value="weekly">每周</option>
              <option value="monthly">每月</option>
            </select>
          </div>

          <div>
            <label class="mb-1 block text-sm font-medium">执行时间</label>
            <div class="grid grid-cols-2 gap-2">
              <Input v-model.number="form.customHour" type="number" min="0" max="23" />
              <Input v-model.number="form.customMinute" type="number" min="0" max="59" />
            </div>
          </div>

          <div v-if="form.customMode === 'weekly'" class="md:col-span-2">
            <label class="mb-1 block text-sm font-medium">星期</label>
            <div class="flex flex-wrap gap-2">
              <label v-for="item in WEEKDAY_OPTIONS" :key="item.value" class="flex items-center gap-2 rounded-md border border-input px-3 py-2 text-sm">
                <input v-model="form.customWeekdays" type="checkbox" :value="item.value" />
                <span>{{ item.label }}</span>
              </label>
            </div>
          </div>

          <div v-if="form.customMode === 'monthly'" class="md:col-span-2">
            <label class="mb-1 block text-sm font-medium">每月日期</label>
            <Input v-model.number="form.customDayOfMonth" type="number" min="1" max="31" />
          </div>
        </template>

        <div v-if="form.scheduleType === 'cron'" class="md:col-span-2">
          <label class="mb-1 block text-sm font-medium">Cron</label>
          <Input v-model="form.cronExpr" placeholder="0 9 * * *" />
        </div>
      </div>

      <DialogFooter>
        <button class="rounded-md border border-input px-3 py-2 text-sm" @click="emit('update:open', false)">取消</button>
        <button :disabled="saving" class="rounded-md bg-foreground px-3 py-2 text-sm text-background" @click="emit('submit')">
          {{ saving ? '保存中...' : '保存' }}
        </button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
