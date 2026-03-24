<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { onClickOutside, useEventListener } from '@vueuse/core'
import { CalendarDays, Check, ChevronDown, ChevronLeft, ChevronRight, Clock3 } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { SCHEDULE_PRESETS, WEEKDAY_OPTIONS } from '../constants'
import {
  addExpirationMonths,
  buildExpirationMonthOptions,
  buildExpirationYearOptions,
  setVisibleMonthYear,
  startOfExpirationMonth,
} from './taskFormExpirationCalendar'
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

const { t } = useI18n()

const scheduleTypeOptions = [
  { value: 'preset', labelKey: 'scheduledTasks.dialog.scheduleType.preset' },
  { value: 'custom', labelKey: 'scheduledTasks.dialog.scheduleType.custom' },
  { value: 'cron', labelKey: 'scheduledTasks.dialog.scheduleType.cron' },
] as const

const customModeOptions = [
  { value: 'interval', labelKey: 'scheduledTasks.dialog.customMode.interval' },
  { value: 'daily', labelKey: 'scheduledTasks.dialog.customMode.daily' },
  { value: 'weekly', labelKey: 'scheduledTasks.dialog.customMode.weekly' },
  { value: 'monthly', labelKey: 'scheduledTasks.dialog.customMode.monthly' },
] as const

const monthlyOptions = Array.from({ length: 31 }, (_, index) => index + 1)
const calendarWeekdayLabels = computed(() =>
  WEEKDAY_OPTIONS.map((item) => t(item.shortLabelKey))
)

type CalendarDay = {
  key: string
  isoDate: string
  label: number
  inCurrentMonth: boolean
  isSelected: boolean
  isToday: boolean
}

function createSafeDate(year: number, month: number, day: number) {
  return new Date(year, month, day, 12, 0, 0, 0)
}

function padDatePart(value: number) {
  return String(value).padStart(2, '0')
}

function formatDateKey(date: Date) {
  return `${date.getFullYear()}-${padDatePart(date.getMonth() + 1)}-${padDatePart(date.getDate())}`
}

function parseDateKey(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  if (!year || month < 1 || month > 12 || day < 1 || day > 31) return null
  const date = createSafeDate(year, month - 1, day)
  if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day) return null
  return date
}

function formatCalendarTitle(date: Date) {
  return t('scheduledTasks.form.calendarTitle', {
    year: date.getFullYear(),
    month: padDatePart(date.getMonth() + 1),
  })
}

function buildCalendarDays(monthAnchor: Date, selectedDateKey: string): CalendarDay[] {
  const monthStart = startOfExpirationMonth(monthAnchor)
  const gridStart = createSafeDate(monthStart.getFullYear(), monthStart.getMonth(), 1 - monthStart.getDay())
  const todayKey = formatDateKey(new Date())

  return Array.from({ length: 42 }, (_, index) => {
    const current = createSafeDate(gridStart.getFullYear(), gridStart.getMonth(), gridStart.getDate() + index)
    const isoDate = formatDateKey(current)
    return {
      key: isoDate,
      isoDate,
      label: current.getDate(),
      inCurrentMonth: current.getMonth() === monthStart.getMonth(),
      isSelected: isoDate === selectedDateKey,
      isToday: isoDate === todayKey,
    }
  })
}

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

const expirationDateValue = computed({
  get() {
    return props.form.expiresAtDate
  },
  set(value: string) {
    if (props.readonly) return
    props.form.expiresAtDate = value
  },
})

const expirationPickerOpen = ref(false)
const expirationPickerRef = ref<HTMLElement | null>(null)
const visibleExpirationMonth = ref(
  startOfExpirationMonth(parseDateKey(props.form.expiresAtDate) ?? new Date())
)
const expirationMonthOptions = buildExpirationMonthOptions()

const expirationDisplayValue = computed(() => {
  if (!props.form.expiresAtDate) return ''
  const [year = '', month = '', day = ''] = props.form.expiresAtDate.split('-')
  if (!year || !month || !day) return ''
  return `${year} / ${month} / ${day}`
})

const expirationYearOptions = computed(() => buildExpirationYearOptions(visibleExpirationMonth.value))
const visibleExpirationYear = computed(() => visibleExpirationMonth.value.getFullYear())
const visibleExpirationMonthValue = computed(() => visibleExpirationMonth.value.getMonth() + 1)
const expirationCalendarWeeks = computed(() => {
  const days = buildCalendarDays(visibleExpirationMonth.value, props.form.expiresAtDate)
  return Array.from({ length: 6 }, (_, index) => days.slice(index * 7, index * 7 + 7))
})

function syncVisibleExpirationMonth(value: string) {
  visibleExpirationMonth.value = startOfExpirationMonth(parseDateKey(value) ?? new Date())
}

function openExpirationPicker() {
  if (props.readonly) return
  syncVisibleExpirationMonth(props.form.expiresAtDate)
  expirationPickerOpen.value = true
}

function closeExpirationPicker() {
  expirationPickerOpen.value = false
}

function toggleExpirationPicker() {
  if (props.readonly) return
  if (expirationPickerOpen.value) {
    closeExpirationPicker()
    return
  }
  openExpirationPicker()
}

function goToPreviousExpirationMonth() {
  visibleExpirationMonth.value = addExpirationMonths(visibleExpirationMonth.value, -1)
}

function goToNextExpirationMonth() {
  visibleExpirationMonth.value = addExpirationMonths(visibleExpirationMonth.value, 1)
}

function handleExpirationYearChange(value: string) {
  const year = Number(value)
  if (!Number.isInteger(year)) return
  visibleExpirationMonth.value = setVisibleMonthYear(visibleExpirationMonth.value, year)
}

function handleExpirationMonthChange(value: string) {
  const month = Number(value)
  if (!Number.isInteger(month)) return
  visibleExpirationMonth.value = setVisibleMonthYear(visibleExpirationMonth.value, undefined, month)
}

function selectExpirationDate(value: string) {
  expirationDateValue.value = value
  syncVisibleExpirationMonth(value)
  closeExpirationPicker()
}

function selectTodayExpirationDate() {
  selectExpirationDate(formatDateKey(new Date()))
}

function clearExpirationDate() {
  expirationDateValue.value = ''
  syncVisibleExpirationMonth('')
  closeExpirationPicker()
}

const formExpired = computed(() => {
  if (!props.form.expiresAtDate) return false
  const expiresAt = new Date(`${props.form.expiresAtDate}T23:59:59`)
  if (Number.isNaN(expiresAt.getTime())) return props.form.isExpired
  return expiresAt.getTime() <= Date.now()
})

const notificationPlatformOptions = computed(() => {
  const labels: Record<string, string> = {
    feishu: t('channels.platforms.feishu'),
    dingtalk: t('channels.platforms.dingtalk'),
    wecom: t('channels.platforms.wecom'),
    qq: t('channels.platforms.qq'),
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
    label: t('scheduledTasks.notification.channelFallback', { id: channelId }),
  }))
})

const selectedNotificationChannelValue = computed(() =>
  props.form.notificationChannelIds.map(String)
)

const notificationChannelTriggerLabel = computed(() => {
  if (!props.form.notificationPlatform) return t('scheduledTasks.notification.selectTypeFirst')
  if (!selectedNotificationChannels.value.length) return t('scheduledTasks.notification.selectChannel')
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

watch(
  () => props.form.expiresAtDate,
  (value) => {
    if (expirationPickerOpen.value) return
    syncVisibleExpirationMonth(value)
  }
)

onClickOutside(expirationPickerRef, () => {
  if (!expirationPickerOpen.value) return
  closeExpirationPicker()
})

useEventListener(window, 'keydown', (event) => {
  if (event.key === 'Escape' && expirationPickerOpen.value) {
    closeExpirationPicker()
  }
})
</script>

<template>
  <div class="space-y-6 py-6">
    <section class="space-y-5">
      <div class="space-y-2">
        <label class="block text-[15px] font-semibold text-[#1f2937]">
          {{ t('scheduledTasks.dialog.nameLabel') }}
        </label>
        <Input
          v-model="form.name"
          :readonly="readonly"
          :placeholder="t('scheduledTasks.dialog.namePlaceholder')"
          class="h-11 rounded-xl border-[#dbe3ec] bg-white px-4 text-sm text-[#111827] placeholder:text-[#a0aec0] focus-visible:border-[#2563eb] focus-visible:ring-[#bfdbfe] read-only:bg-[#f8fafc] read-only:text-[#475569]"
        />
      </div>

      <div class="space-y-2">
        <label class="block text-[15px] font-semibold text-[#1f2937]">
          {{ t('scheduledTasks.dialog.promptLabel') }}
        </label>
        <textarea
          v-model="form.prompt"
          :readonly="readonly"
          class="min-h-[118px] w-full rounded-xl border border-[#dbe3ec] bg-white px-4 py-3 text-sm leading-6 text-[#111827] outline-none transition-[border-color,box-shadow] placeholder:text-[#a0aec0] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe] read-only:bg-[#f8fafc] read-only:text-[#475569]"
          :placeholder="t('scheduledTasks.dialog.promptPlaceholder')"
        />
      </div>

      <div class="space-y-2">
        <label class="block text-[15px] font-semibold text-[#1f2937]">
          {{ t('scheduledTasks.dialog.agentLabel') }}
        </label>
        <div class="relative">
          <select
            v-model.number="form.agentId"
            :disabled="readonly"
            class="h-11 w-full appearance-none rounded-xl border border-[#dbe3ec] bg-white px-4 pr-11 text-sm text-[#111827] outline-none transition-[border-color,box-shadow] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe] disabled:bg-[#f8fafc] disabled:text-[#475569]"
          >
            <option :value="null">{{ t('scheduledTasks.dialog.selectAgent') }}</option>
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
          {{ t('scheduledTasks.form.expiresAt') }}
        </label>
        <div ref="expirationPickerRef" class="relative">
          <button
            type="button"
            :disabled="readonly"
            :aria-expanded="expirationPickerOpen"
            class="group flex h-11 w-full items-center justify-between rounded-xl border px-4 text-left text-sm outline-none transition-[border-color,box-shadow,background-color] disabled:cursor-default"
            :class="[
              expirationPickerOpen
                ? 'border-[#2563eb] bg-white shadow-[0_0_0_4px_rgba(191,219,254,0.9)]'
                : 'border-[#dbe3ec] bg-white hover:border-[#cbd5e1]',
              readonly ? 'bg-[#f8fafc] text-[#475569]' : 'text-[#111827]',
            ]"
            @click="toggleExpirationPicker"
          >
            <div class="flex min-w-0 flex-1 items-center gap-3">
              <div
                class="flex size-8 shrink-0 items-center justify-center rounded-lg border"
                :class="
                  expirationPickerOpen
                    ? 'border-[#bfdbfe] bg-[#eff6ff] text-[#2563eb]'
                    : 'border-[#e2e8f0] bg-[#f8fafc] text-[#64748b]'
                "
              >
                <CalendarDays class="size-4" />
              </div>
              <div class="min-w-0">
                <p class="truncate font-medium">
                  {{ expirationDisplayValue || t('scheduledTasks.form.selectExpirationDate') }}
                </p>
              </div>
            </div>
            <ChevronDown
              class="size-4 shrink-0 text-[#94a3b8] transition-transform"
              :class="expirationPickerOpen ? 'rotate-180 text-[#2563eb]' : ''"
            />
          </button>

          <div
            v-if="expirationPickerOpen"
            class="absolute left-0 z-50 mt-3 w-full min-w-[320px] overflow-hidden rounded-[22px] border border-[#dbe3ec] bg-[linear-gradient(180deg,#ffffff_0%,#f8fbff_100%)] shadow-[0_22px_60px_rgba(15,23,42,0.18)] backdrop-blur"
          >
            <div class="border-b border-[#e5eef8] px-4 pb-4 pt-4">
              <div class="flex items-center justify-between gap-3">
                <div class="flex items-center gap-2">
                  <button
                    type="button"
                    class="inline-flex size-9 items-center justify-center rounded-full border border-[#dbe3ec] bg-white text-[#475569] transition-colors hover:border-[#bfdbfe] hover:bg-[#eff6ff] hover:text-[#2563eb]"
                    @click="goToPreviousExpirationMonth"
                  >
                    <ChevronLeft class="size-4" />
                  </button>
                  <button
                    type="button"
                    class="inline-flex size-9 items-center justify-center rounded-full border border-[#dbe3ec] bg-white text-[#475569] transition-colors hover:border-[#bfdbfe] hover:bg-[#eff6ff] hover:text-[#2563eb]"
                    @click="goToNextExpirationMonth"
                  >
                    <ChevronRight class="size-4" />
                  </button>
                </div>
                <div class="flex items-center gap-2">
                  <div class="relative">
                    <select
                      :value="visibleExpirationYear"
                      class="h-9 appearance-none rounded-full border border-[#dbe3ec] bg-white pl-4 pr-9 text-sm font-medium text-[#111827] outline-none transition-[border-color,box-shadow] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe]"
                      @change="handleExpirationYearChange(($event.target as HTMLSelectElement).value)"
                    >
                      <option v-for="year in expirationYearOptions" :key="year" :value="year">
                        {{ t('scheduledTasks.form.yearOption', { year }) }}
                      </option>
                    </select>
                    <ChevronDown
                      class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-[#94a3b8]"
                    />
                  </div>
                  <div class="relative">
                    <select
                      :value="visibleExpirationMonthValue"
                      class="h-9 appearance-none rounded-full border border-[#dbe3ec] bg-white pl-4 pr-9 text-sm font-medium text-[#111827] outline-none transition-[border-color,box-shadow] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe]"
                      @change="handleExpirationMonthChange(($event.target as HTMLSelectElement).value)"
                    >
                      <option
                        v-for="month in expirationMonthOptions"
                        :key="month"
                        :value="month"
                      >
                        {{ t('scheduledTasks.form.monthOption', { month }) }}
                      </option>
                    </select>
                    <ChevronDown
                      class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-[#94a3b8]"
                    />
                  </div>
                </div>
              </div>
            </div>

            <div class="px-4 pb-4 pt-3">
              <div class="mb-2 grid grid-cols-7 gap-1">
                <span
                  v-for="weekday in calendarWeekdayLabels"
                  :key="weekday"
                  class="flex h-8 items-center justify-center text-xs font-semibold text-[#94a3b8]"
                >
                  {{ weekday }}
                </span>
              </div>

              <div class="space-y-1">
                <div
                  v-for="(week, weekIndex) in expirationCalendarWeeks"
                  :key="'week-' + weekIndex"
                  class="grid grid-cols-7 gap-1"
                >
                  <button
                    v-for="day in week"
                    :key="day.key"
                    type="button"
                    :class="
                      cn(
                        'flex h-10 items-center justify-center rounded-xl border text-sm font-medium transition-all',
                        day.isSelected
                          ? 'border-[#111827] bg-[#111827] text-white shadow-[0_10px_22px_rgba(15,23,42,0.18)]'
                          : day.isToday
                            ? 'border-[#bfdbfe] bg-[#eff6ff] text-[#1d4ed8]'
                            : day.inCurrentMonth
                              ? 'border-transparent bg-white text-[#334155] hover:border-[#dbe3ec] hover:bg-[#f8fafc]'
                              : 'border-transparent bg-transparent text-[#c0cad6] hover:bg-[#f8fafc]',
                      )
                    "
                    @click="selectExpirationDate(day.isoDate)"
                  >
                    {{ day.label }}
                  </button>
                </div>
              </div>
            </div>

            <div class="flex items-center justify-between border-t border-[#e5eef8] bg-white/80 px-4 py-3">
              <button
                type="button"
                class="inline-flex items-center rounded-full px-3 py-1.5 text-sm font-medium text-[#2563eb] transition-colors hover:bg-[#eff6ff]"
                @click="selectTodayExpirationDate"
              >
                {{ t('scheduledTasks.form.today') }}
              </button>
              <button
                type="button"
                class="inline-flex items-center rounded-full px-3 py-1.5 text-sm font-medium text-[#64748b] transition-colors hover:bg-[#f1f5f9]"
                @click="clearExpirationDate"
              >
                {{ t('scheduledTasks.form.clear') }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <div class="space-y-2">
        <label class="block text-[15px] font-semibold text-[#1f2937]">
          {{ t('scheduledTasks.notification.label') }}
          <span class="ml-1 text-xs font-medium text-[#94a3b8]">
            {{ t('scheduledTasks.notification.optional') }}
          </span>
        </label>
        <div class="relative">
          <select
            :value="form.notificationPlatform"
            :disabled="readonly"
            class="h-11 w-full appearance-none rounded-xl border border-[#dbe3ec] bg-white px-4 pr-11 text-sm text-[#111827] outline-none transition-[border-color,box-shadow] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe] disabled:bg-[#f8fafc] disabled:text-[#475569]"
            @change="handleNotificationPlatformChange(($event.target as HTMLSelectElement).value)"
          >
            <option value="">{{ t('scheduledTasks.notification.none') }}</option>
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
        <label class="block text-[15px] font-semibold text-[#1f2937]">
          {{ t('scheduledTasks.notification.channelsLabel') }}
        </label>
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
            <p v-else class="px-3 py-2 text-sm text-[#94a3b8]">
              {{ t('scheduledTasks.notification.emptyChannels') }}
            </p>
          </DropdownMenuContent>
        </DropdownMenu>
        <p class="text-xs text-[#94a3b8]">
          {{
            form.notificationPlatform
              ? t('scheduledTasks.notification.hintSelected')
              : t('scheduledTasks.notification.hintUnselected')
          }}
        </p>
      </div>
    </section>

    <section class="space-y-4">
      <div class="space-y-1">
        <h3 class="text-[15px] font-semibold text-[#1f2937]">
          {{ t('scheduledTasks.dialog.scheduleTitle') }}
        </h3>
        <p class="text-sm text-[#94a3b8]">{{ t('scheduledTasks.dialog.scheduleHint') }}</p>
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
          {{ t(option.labelKey) }}
        </button>
      </div>

      <div class="rounded-2xl border border-[#e5e7eb] bg-[#f8fafc] p-4">
        <div v-if="form.scheduleType === 'preset'" class="space-y-2">
          <label class="block text-sm font-semibold text-[#334155]">
            {{ t('scheduledTasks.dialog.presetLabel') }}
          </label>
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
              <span class="flex-1 font-medium">{{ t(item.labelKey) }}</span>
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
                  <span>{{ t(item.labelKey) }}</span>
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
                  <span>{{ t(item.labelKey) }}</span>
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
                  {{ t('scheduledTasks.dialog.monthlyDay', { day }) }}
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
              <span class="ml-3 text-sm text-[#64748b]">
                {{ t('scheduledTasks.dialog.minutes') }}
              </span>
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
          <label class="block text-sm font-semibold text-[#334155]">
            {{ t('scheduledTasks.dialog.cronLabel') }}
          </label>
          <textarea
            v-model="form.cronExpr"
            :readonly="readonly"
            class="min-h-[108px] w-full rounded-xl border border-[#dbe3ec] bg-white px-4 py-3 font-mono text-sm leading-6 text-[#111827] outline-none transition-[border-color,box-shadow] placeholder:text-[#a0aec0] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe] read-only:bg-[#f8fafc] read-only:text-[#475569]"
            :placeholder="t('scheduledTasks.dialog.cronPlaceholder')"
          />
        </div>
      </div>
    </section>

    <section
      class="flex items-center justify-between rounded-2xl border border-[#e5e7eb] bg-white px-4 py-4"
    >
      <div class="space-y-1">
        <h3 class="text-[15px] font-semibold text-[#1f2937]">
          {{ t('scheduledTasks.dialog.enableNowTitle') }}
        </h3>
        <p class="text-sm text-[#94a3b8]">{{ t('scheduledTasks.dialog.enableNowHint') }}</p>
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
