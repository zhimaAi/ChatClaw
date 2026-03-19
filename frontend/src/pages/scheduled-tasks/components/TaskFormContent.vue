<script setup lang="ts">
import { computed, watch } from 'vue'
import { Check, ChevronDown, Clock3 } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { SCHEDULE_PRESETS, WEEKDAY_OPTIONS } from '../constants'
import type { Agent, Channel, ScheduledTaskFormState } from '../types'

const props = withDefaults(
  defineProps<{
    form: ScheduledTaskFormState
    agents: Agent[]
    channels: Channel[]
    readonly?: boolean
    agentLabelOverride?: string
    notificationPlatformLabelOverride?: string
    notificationChannelLabelOverrides?: string[]
  }>(),
  {
    readonly: false,
    agentLabelOverride: '',
    notificationPlatformLabelOverride: '',
    notificationChannelLabelOverrides: () => [],
  }
)

const scheduleTypeOptions = [
  { value: 'preset', label: '快捷设置' },
  { value: 'custom', label: '自定义时间' },
  { value: 'cron', label: 'Linux Crontab 代码' },
] as const

const customModeOptions = [
  { value: 'interval', label: '每隔' },
  { value: 'daily', label: '每天执行' },
  { value: 'weekly', label: '每周执行' },
  { value: 'monthly', label: '每月执行' },
] as const

const monthlyOptions = Array.from({ length: 31 }, (_, index) => index + 1)

const customTimeValue = computed({
  get() {
    return `${String(props.form.customHour).padStart(2, '0')}:${String(props.form.customMinute).padStart(2, '0')}`
  },
  set(value: string) {
    if (props.readonly) return
    const [hour, minute] = value.split(':')
    props.form.customHour = Number(hour || 0)
    props.form.customMinute = Number(minute || 0)
  },
})

const selectedWeeklyDay = computed({
  get() {
    return props.form.customWeekdays[0] ?? 1
  },
  set(value: number) {
    if (props.readonly) return
    props.form.customWeekdays = [value]
  },
})

const notificationPlatformOptions = computed(() => {
  const labels: Record<string, string> = {
    feishu: '飞书',
    dingtalk: '钉钉',
    wecom: '企微',
    qq: 'QQ',
  }
  const seen = new Set<string>()
  return props.channels
    .filter((channel) => channel.enabled)
    .filter((channel) => {
      const platform = channel.platform || ''
      if (!platform || seen.has(platform)) return false
      seen.add(platform)
      return true
    })
    .map((channel) => ({
      value: channel.platform,
      label: labels[channel.platform] || channel.name || channel.platform,
    }))
})

const filteredNotificationChannels = computed(() => {
  if (!props.form.notificationPlatform) return []
  return props.channels.filter(
    (channel) => channel.enabled && channel.platform === props.form.notificationPlatform
  )
})

const selectedNotificationChannels = computed(() => {
  const matched = filteredNotificationChannels.value
    .filter((channel) => props.form.notificationChannelIds.includes(channel.id))
    .map((channel) => ({ id: channel.id, label: channel.name }))

  if (matched.length) return matched
  if (props.notificationChannelLabelOverrides.length) {
    return props.notificationChannelLabelOverrides.map((label, index) => ({
      id: props.form.notificationChannelIds[index] ?? index,
      label,
    }))
  }

  return props.form.notificationChannelIds.map((channelId) => ({
    id: channelId,
    label: `频道 ${channelId}`,
  }))
})

const selectedNotificationChannelValue = computed(() => props.form.notificationChannelIds.map(String))

const notificationChannelTriggerLabel = computed(() => {
  if (!props.form.notificationPlatform) return '请先选择通知类型'
  if (!selectedNotificationChannels.value.length) return '请选择频道'
  return ''
})

const agentOptions = computed(() => {
  if (!props.readonly || !props.agentLabelOverride || !props.form.agentId) return props.agents
  const exists = props.agents.some((agent) => agent.id === props.form.agentId)
  return exists ? props.agents : [{ id: props.form.agentId, name: props.agentLabelOverride }]
})

const notificationPlatformDisplayOptions = computed(() => {
  if (
    !props.readonly ||
    !props.notificationPlatformLabelOverride ||
    !props.form.notificationPlatform
  ) {
    return notificationPlatformOptions.value
  }
  const exists = notificationPlatformOptions.value.some(
    (item) => item.value === props.form.notificationPlatform
  )
  return exists
    ? notificationPlatformOptions.value
    : [
        ...notificationPlatformOptions.value,
        {
          value: props.form.notificationPlatform,
          label: props.notificationPlatformLabelOverride,
        },
      ]
})

function selectScheduleType(value: ScheduledTaskFormState['scheduleType']) {
  if (props.readonly) return
  props.form.scheduleType = value
  if (
    value === 'custom' &&
    props.form.customMode === 'weekly' &&
    props.form.customWeekdays.length === 0
  ) {
    props.form.customWeekdays = [1]
  }
}

function selectCustomMode(value: ScheduledTaskFormState['customMode']) {
  if (props.readonly) return
  props.form.customMode = value
  if (value === 'weekly') {
    selectedWeeklyDay.value = props.form.customWeekdays[0] ?? 1
  }
}

function selectWeeklyDay(value: number) {
  if (props.readonly) return
  selectedWeeklyDay.value = value
}

function selectMonthlyDay(value: number) {
  if (props.readonly) return
  props.form.customDayOfMonth = value
}

function handleNotificationPlatformChange(value: string) {
  if (props.readonly) return
  props.form.notificationPlatform = value
  props.form.notificationChannelIds = []
}

function toggleNotificationChannel(channelId: number, checked: boolean) {
  if (props.readonly) return
  if (checked) {
    if (!props.form.notificationChannelIds.includes(channelId)) {
      props.form.notificationChannelIds = [...props.form.notificationChannelIds, channelId]
    }
    return
  }

  props.form.notificationChannelIds = props.form.notificationChannelIds.filter(
    (id) => id !== channelId
  )
}

watch(
  () => props.form.notificationPlatform,
  (platform) => {
    if (props.readonly) return
    if (!platform) {
      props.form.notificationChannelIds = []
      return
    }

    const availableChannelIds = new Set(
      filteredNotificationChannels.value.map((channel) => channel.id)
    )
    props.form.notificationChannelIds = props.form.notificationChannelIds.filter((channelId) =>
      availableChannelIds.has(channelId)
    )
  }
)
</script>

<template>
  <div class="space-y-6 py-6">
    <section class="space-y-5">
      <div class="space-y-2">
        <label class="block text-[15px] font-semibold text-[#1f2937]">任务名称</label>
        <Input
          v-model="form.name"
          :readonly="readonly"
          placeholder="例如：早间简报"
          class="h-11 rounded-xl border-[#dbe3ec] bg-white px-4 text-sm text-[#111827] placeholder:text-[#a0aec0] focus-visible:border-[#2563eb] focus-visible:ring-[#bfdbfe] read-only:bg-[#f8fafc] read-only:text-[#475569]"
        />
      </div>

      <div class="space-y-2">
        <label class="block text-[15px] font-semibold text-[#1f2937]">消息提示词</label>
        <textarea
          v-model="form.prompt"
          :readonly="readonly"
          class="min-h-[118px] w-full rounded-xl border border-[#dbe3ec] bg-white px-4 py-3 text-sm leading-6 text-[#111827] outline-none transition-[border-color,box-shadow] placeholder:text-[#a0aec0] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe] read-only:bg-[#f8fafc] read-only:text-[#475569]"
          placeholder="AI 应该做什么？例如：给我一份今天的新闻和天气摘要"
        />
      </div>

      <div class="space-y-2">
        <label class="block text-[15px] font-semibold text-[#1f2937]">关联 AI 助手</label>
        <div class="relative">
          <select
            v-model.number="form.agentId"
            :disabled="readonly"
            class="h-11 w-full appearance-none rounded-xl border border-[#dbe3ec] bg-white px-4 pr-11 text-sm text-[#111827] outline-none transition-[border-color,box-shadow] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe] disabled:bg-[#f8fafc] disabled:text-[#475569]"
          >
            <option :value="null">请选择助手</option>
            <option v-for="agent in agentOptions" :key="agent.id" :value="agent.id">
              {{ agent.name }}
            </option>
          </select>
          <ChevronDown
            class="pointer-events-none absolute right-4 top-1/2 size-4 -translate-y-1/2 text-[#94a3b8]"
          />
        </div>
      </div>

      <div class="space-y-2">
        <label class="block text-[15px] font-semibold text-[#1f2937]">
          通知
          <span class="ml-1 text-xs font-medium text-[#94a3b8]">可选</span>
        </label>
        <div class="relative">
          <select
            :value="form.notificationPlatform"
            :disabled="readonly"
            class="h-11 w-full appearance-none rounded-xl border border-[#dbe3ec] bg-white px-4 pr-11 text-sm text-[#111827] outline-none transition-[border-color,box-shadow] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe] disabled:bg-[#f8fafc] disabled:text-[#475569]"
            @change="
              handleNotificationPlatformChange(($event.target as HTMLSelectElement).value)
            "
          >
            <option value="">不发送通知</option>
            <option
              v-for="option in notificationPlatformDisplayOptions"
              :key="option.value"
              :value="option.value"
            >
              {{ option.label }}
            </option>
          </select>
          <ChevronDown
            class="pointer-events-none absolute right-4 top-1/2 size-4 -translate-y-1/2 text-[#94a3b8]"
          />
        </div>
      </div>

      <div class="space-y-2">
        <label class="block text-[15px] font-semibold text-[#1f2937]">选择频道</label>
        <template v-if="readonly">
          <div
            class="flex min-h-[44px] w-full flex-wrap items-center gap-2 rounded-xl border border-[#dbe3ec] bg-[#f8fafc] px-4 py-2 text-sm text-[#475569]"
          >
            <template v-if="selectedNotificationChannels.length">
              <Badge
                v-for="channel in selectedNotificationChannels"
                :key="channel.id"
                variant="outline"
                class="max-w-full border-[#dbe3ec] bg-white px-2.5 py-1 text-[#334155]"
              >
                <span class="truncate">{{ channel.label }}</span>
              </Badge>
            </template>
            <span v-else class="truncate text-sm text-[#94a3b8]">
              {{ notificationChannelTriggerLabel }}
            </span>
          </div>
        </template>
        <DropdownMenu v-else>
          <DropdownMenuTrigger as-child :disabled="!form.notificationPlatform">
            <button
              type="button"
              class="flex min-h-[44px] w-full items-center justify-between gap-3 rounded-xl border border-[#dbe3ec] bg-white px-4 py-2 text-left text-sm text-[#111827] outline-none transition-[border-color,box-shadow] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe] disabled:cursor-not-allowed disabled:bg-[#f8fafc] disabled:text-[#94a3b8]"
            >
              <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
                <template v-if="selectedNotificationChannels.length">
                  <Badge
                    v-for="channel in selectedNotificationChannels"
                    :key="channel.id"
                    variant="outline"
                    class="max-w-full border-[#bfdbfe] bg-[#eff6ff] px-2.5 py-1 text-[#1d4ed8]"
                  >
                    <span class="truncate">{{ channel.label }}</span>
                  </Badge>
                </template>
                <span v-else class="truncate text-sm text-[#94a3b8]">
                  {{ notificationChannelTriggerLabel }}
                </span>
              </div>
              <ChevronDown class="size-4 shrink-0 text-[#94a3b8]" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="start"
            class="w-[var(--reka-dropdown-menu-trigger-width)] min-w-[320px] rounded-xl border border-[#dbe3ec] bg-white p-1.5 shadow-[0_16px_40px_rgba(15,23,42,0.14)]"
          >
            <div v-if="filteredNotificationChannels.length" class="max-h-64 overflow-y-auto py-1">
              <DropdownMenuCheckboxItem
                v-for="channel in filteredNotificationChannels"
                :key="channel.id"
                :model-value="selectedNotificationChannelValue.includes(String(channel.id))"
                class="rounded-lg px-8 py-2 text-sm text-[#111827] focus:bg-[#eff6ff] focus:text-[#1d4ed8]"
                @select.prevent
                @update:model-value="toggleNotificationChannel(channel.id, !!$event)"
              >
                {{ channel.name }}
              </DropdownMenuCheckboxItem>
            </div>
            <p v-else class="px-3 py-2 text-sm text-[#94a3b8]">当前通知类型下暂无可用频道</p>
          </DropdownMenuContent>
        </DropdownMenu>
        <p class="text-xs text-[#94a3b8]">
          {{
            form.notificationPlatform
              ? '可多选，任务完成后会通过这些频道发送结果。'
              : '先选择通知类型，再从对应主菜单频道中多选具体频道。'
          }}
        </p>
      </div>
    </section>

    <section class="space-y-4">
      <div class="space-y-1">
        <h3 class="text-[15px] font-semibold text-[#1f2937]">设置定时时间</h3>
        <p class="text-sm text-[#94a3b8]">选择快捷方案，或按你的节奏自定义执行时间。</p>
      </div>

      <div class="flex flex-wrap gap-2">
        <button
          v-for="option in scheduleTypeOptions"
          :key="option.value"
          type="button"
          :disabled="readonly"
          class="inline-flex h-10 items-center rounded-xl border px-4 text-sm font-medium transition-colors disabled:cursor-default"
          :class="
            form.scheduleType === option.value
              ? 'border-[#2563eb] bg-[#eff6ff] text-[#2563eb]'
              : 'border-[#dbe3ec] bg-white text-[#64748b]'
          "
          @click="selectScheduleType(option.value)"
        >
          {{ option.label }}
        </button>
      </div>

      <div class="rounded-2xl border border-[#e5e7eb] bg-[#f8fafc] p-4">
        <div v-if="form.scheduleType === 'preset'" class="space-y-2">
          <label class="block text-sm font-semibold text-[#334155]">快捷设置</label>
          <div class="grid gap-3 md:grid-cols-2">
            <button
              v-for="item in SCHEDULE_PRESETS"
              :key="item.value"
              type="button"
              :disabled="readonly"
              class="flex min-h-11 items-center gap-3 rounded-xl border bg-white px-4 py-3 text-left text-sm transition-all disabled:cursor-default"
              :class="
                form.schedulePreset === item.value
                  ? 'border-[#2563eb] bg-[#eff6ff] text-[#1d4ed8] shadow-[0_0_0_1px_rgba(37,99,235,0.08)]'
                  : 'border-[#dbe3ec] text-[#334155]'
              "
              @click="form.schedulePreset = item.value"
            >
              <Clock3 class="size-4 shrink-0" />
              <span class="flex-1 font-medium">{{ item.label }}</span>
              <Check v-if="form.schedulePreset === item.value" class="size-4 shrink-0" />
            </button>
          </div>
        </div>

        <div v-else-if="form.scheduleType === 'custom'" class="space-y-4">
          <div class="flex items-start gap-5">
            <div class="inline-flex overflow-hidden rounded-2xl border border-[#dbe3ec] bg-white">
              <div class="w-[140px] shrink-0">
                <button
                  v-for="item in customModeOptions"
                  :key="item.value"
                  type="button"
                  :disabled="readonly"
                  class="flex h-12 w-full items-center justify-between px-4 text-sm font-medium transition-colors disabled:cursor-default"
                  :class="
                    form.customMode === item.value
                      ? 'bg-[#f3f4f6] text-[#111827]'
                      : 'text-[#475569]'
                  "
                  @click="selectCustomMode(item.value)"
                >
                  <span>{{ item.label }}</span>
                  <ChevronDown
                    v-if="item.value !== 'daily' && item.value !== 'interval'"
                    class="size-4 shrink-0 rotate-[-90deg] text-[#94a3b8]"
                    :class="form.customMode === item.value ? 'opacity-100' : 'opacity-40'"
                  />
                </button>
              </div>

              <div
                v-if="form.customMode === 'weekly'"
                class="max-h-[336px] w-[140px] shrink-0 overflow-y-auto border-l border-[#dbe3ec]"
              >
                <button
                  v-for="item in WEEKDAY_OPTIONS"
                  :key="item.value"
                  type="button"
                  :disabled="readonly"
                  class="flex h-12 w-full items-center justify-between px-4 text-sm transition-colors disabled:cursor-default"
                  :class="
                    selectedWeeklyDay === item.value
                      ? 'bg-[#f3f4f6] font-medium text-[#111827]'
                      : 'text-[#475569]'
                  "
                  @click="selectWeeklyDay(item.value)"
                >
                  <span>{{ item.label }}</span>
                  <Check
                    v-if="selectedWeeklyDay === item.value"
                    class="size-4 shrink-0 text-[#2563eb]"
                  />
                </button>
              </div>

              <div
                v-else-if="form.customMode === 'monthly'"
                class="max-h-[336px] w-[140px] shrink-0 overflow-y-auto border-l border-[#dbe3ec]"
              >
                <button
                  v-for="day in monthlyOptions"
                  :key="day"
                  type="button"
                  :disabled="readonly"
                  class="flex h-12 w-full items-center px-4 text-sm transition-colors disabled:cursor-default"
                  :class="
                    form.customDayOfMonth === day
                      ? 'bg-[#f3f4f6] font-medium text-[#111827]'
                      : 'text-[#475569]'
                  "
                  @click="selectMonthlyDay(day)"
                >
                  {{ day }}号
                </button>
              </div>
            </div>

            <div v-if="form.customMode === 'interval'" class="flex min-h-12 items-center">
              <Input
                v-model.number="form.customIntervalMinutes"
                :readonly="readonly"
                type="number"
                min="1"
                max="59"
                step="1"
                class="h-9 w-[120px] rounded-md border-[#dbe3ec] bg-white text-sm shadow-none read-only:bg-[#f8fafc] read-only:text-[#475569]"
              />
              <span class="ml-3 text-sm text-[#64748b]">分钟</span>
            </div>

            <Input
              v-else-if="form.customMode === 'daily'"
              v-model="customTimeValue"
              :readonly="readonly"
              type="time"
              class="h-11 w-[156px] rounded-xl border-[#dbe3ec] bg-white px-4 text-sm read-only:bg-[#f8fafc] read-only:text-[#475569]"
            />
          </div>

          <Input
            v-if="form.customMode === 'weekly' || form.customMode === 'monthly'"
            v-model="customTimeValue"
            :readonly="readonly"
            type="time"
            class="h-11 w-[156px] rounded-xl border-[#dbe3ec] bg-white px-4 text-sm read-only:bg-[#f8fafc] read-only:text-[#475569]"
          />
        </div>

        <div v-else class="space-y-2">
          <label class="block text-sm font-semibold text-[#334155]">Linux Crontab 代码</label>
          <textarea
            v-model="form.cronExpr"
            :readonly="readonly"
            class="min-h-[108px] w-full rounded-xl border border-[#dbe3ec] bg-white px-4 py-3 font-mono text-sm leading-6 text-[#111827] outline-none transition-[border-color,box-shadow] placeholder:text-[#a0aec0] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe] read-only:bg-[#f8fafc] read-only:text-[#475569]"
            placeholder="例如 0 9 * * *"
          />
        </div>
      </div>
    </section>

    <section class="flex items-center justify-between rounded-2xl border border-[#e5e7eb] bg-white px-4 py-4">
      <div class="space-y-1">
        <h3 class="text-[15px] font-semibold text-[#1f2937]">立即启用</h3>
        <p class="text-sm text-[#94a3b8]">创建后立即开始运行此任务</p>
      </div>
      <Switch
        :model-value="form.enabled"
        :disabled="readonly"
        class="data-[state=checked]:bg-[#111827] data-[state=unchecked]:bg-[#cbd5e1]"
        @update:model-value="(value) => !readonly && (form.enabled = !!value)"
      />
    </section>
  </div>
</template>
