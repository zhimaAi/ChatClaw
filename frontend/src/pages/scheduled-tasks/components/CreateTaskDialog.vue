<script setup lang="ts">
import { computed, watch } from 'vue'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { WEEKDAY_OPTIONS } from '../constants'
import type { Agent, Library, Model, Provider, ScheduledTaskFormState } from '../types'

const props = defineProps<{
  open: boolean
  saving: boolean
  title: string
  form: ScheduledTaskFormState
  agents: Agent[]
  libraries: Library[]
  providers: Provider[]
  models: Model[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  submit: []
  providerChange: [providerID: string]
}>()

const selectedLibrarySummary = computed(() => {
  if (!props.form.libraryIds.length) return '未选择知识库'
  return `已选 ${props.form.libraryIds.length} 个知识库`
})

watch(
  () => props.form.llmProviderId,
  (value) => {
    emit('providerChange', value)
  }
)
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
          <label class="mb-1 block text-sm font-medium">助手</label>
          <select v-model.number="form.agentId" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm">
            <option :value="null">请选择助手</option>
            <option v-for="agent in agents" :key="agent.id" :value="agent.id">{{ agent.name }}</option>
          </select>
        </div>

        <div>
          <label class="mb-1 block text-sm font-medium">对话模式</label>
          <select v-model="form.chatMode" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm">
            <option value="task">task</option>
            <option value="chat">chat</option>
          </select>
        </div>

        <div>
          <label class="mb-1 block text-sm font-medium">模型供应商</label>
          <select v-model="form.llmProviderId" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm">
            <option value="">默认跟随助手</option>
            <option v-for="provider in providers" :key="provider.provider_id" :value="provider.provider_id">
              {{ provider.name }}
            </option>
          </select>
        </div>

        <div>
          <label class="mb-1 block text-sm font-medium">模型</label>
          <select v-model="form.llmModelId" class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm">
            <option value="">默认跟随助手/供应商</option>
            <option v-for="model in models" :key="model.model_id" :value="model.model_id">
              {{ model.name || model.model_id }}
            </option>
          </select>
        </div>

        <div class="md:col-span-2">
          <label class="mb-1 block text-sm font-medium">知识库</label>
          <div class="rounded-md border border-input p-3">
            <div class="mb-2 text-xs text-muted-foreground">{{ selectedLibrarySummary }}</div>
            <div class="grid gap-2 md:grid-cols-3">
              <label v-for="library in libraries" :key="library.id" class="flex items-center gap-2 text-sm">
                <input v-model="form.libraryIds" type="checkbox" :value="library.id" />
                <span>{{ library.name }}</span>
              </label>
            </div>
          </div>
        </div>

        <div class="flex items-center justify-between rounded-md border border-input px-3 py-2">
          <span class="text-sm">思考模式</span>
          <Switch :model-value="form.enableThinking" @update:model-value="(value) => (form.enableThinking = !!value)" />
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
