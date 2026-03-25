<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronDown } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type { OpenClawCronAgentOption } from '@bindings/chatclaw/internal/openclaw/cron'
import type { OpenClawCronFormState } from './utils'

const props = defineProps<{
  open: boolean
  saving: boolean
  title: string
  form: OpenClawCronFormState
  agents: OpenClawCronAgentOption[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  submit: []
}>()

const { t } = useI18n()
const advancedOpen = ref(false)

const canSubmit = computed(() => {
  if (!props.form.name.trim()) return false
  if (!props.form.message.trim() && !props.form.systemEvent.trim()) return false
  if (props.form.scheduleKind === 'cron' && !props.form.cronExpr.trim()) return false
  if (props.form.scheduleKind === 'every' && !props.form.every.trim()) return false
  if (props.form.scheduleKind === 'at' && !props.form.at.trim()) return false
  return true
})

function handleSubmit() {
  if (!canSubmit.value || props.saving) return
  emit('submit')
}

function selectScheduleKind(value: OpenClawCronFormState['scheduleKind']) {
  props.form.scheduleKind = value
}
</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent
      size="xl"
      :show-close-button="true"
      class="flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0 !max-w-[780px] shadow-lg dark:shadow-none dark:ring-1 dark:ring-white/10"
    >
      <DialogHeader class="flex shrink-0 flex-row items-baseline gap-2 border-b border-border px-6 pb-3 pt-4">
        <DialogTitle class="text-xl font-semibold text-[#0a0a0a] dark:text-foreground">
          {{ title }}
        </DialogTitle>
        <p class="text-sm text-muted-foreground">
          {{ t('openclawCron.dialog.subtitle', '使用 OpenClaw 原生 cron / every / at 配置') }}
        </p>
      </DialogHeader>

      <div class="flex-1 overflow-y-auto px-6 py-4">
        <div class="space-y-6">
          <section class="space-y-4">
            <div class="space-y-1.5">
              <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.name', '任务名称') }}
              </Label>
              <Input v-model="form.name" :placeholder="t('openclawCron.dialog.namePlaceholder', '例如：日报总结')" class="h-10" />
            </div>

            <div class="space-y-1.5">
              <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.description', '描述') }}
              </Label>
              <Input v-model="form.description" :placeholder="t('openclawCron.dialog.descriptionPlaceholder', '可选，用于补充说明')" class="h-10" />
            </div>

            <div class="space-y-1.5">
              <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.agent', 'Agent') }}
              </Label>
              <div class="relative">
                <select
                  v-model="form.agentId"
                  class="h-10 w-full appearance-none rounded-md border border-input bg-transparent px-3 pr-10 text-sm text-foreground shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                >
                  <option value="">{{ t('openclawCron.dialog.defaultAgent', '默认 Agent') }}</option>
                  <option v-for="agent in agents" :key="agent.openclaw_agent_id" :value="agent.openclaw_agent_id">
                    {{ agent.name }}
                  </option>
                </select>
                <ChevronDown class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              </div>
            </div>
          </section>

          <section class="space-y-4">
            <div class="space-y-1">
              <h3 class="text-sm font-semibold text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.scheduleTitle', '时间配置') }}
              </h3>
              <p class="text-sm text-muted-foreground">
                {{ t('openclawCron.dialog.scheduleHint', '完全按 OpenClaw 原生格式创建任务') }}
              </p>
            </div>

            <div class="flex flex-wrap gap-2">
              <button
                v-for="item in [
                  { value: 'cron', label: 'Cron' },
                  { value: 'every', label: 'Every' },
                  { value: 'at', label: 'At' },
                ]"
                :key="item.value"
                type="button"
                class="inline-flex h-9 items-center rounded-md border px-3.5 text-sm font-medium transition-colors"
                :class="
                  form.scheduleKind === item.value
                    ? 'border-border bg-muted text-foreground dark:border-white/10'
                    : 'border-border bg-background text-muted-foreground hover:bg-muted/50 hover:text-foreground dark:border-white/10'
                "
                @click="selectScheduleKind(item.value as OpenClawCronFormState['scheduleKind'])"
              >
                {{ item.label }}
              </button>
            </div>

            <div class="rounded-lg border border-border bg-muted/20 p-4 dark:border-white/10 dark:bg-white/5">
              <div v-if="form.scheduleKind === 'cron'" class="space-y-2">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  Cron
                </Label>
                <textarea
                  v-model="form.cronExpr"
                  class="min-h-[88px] w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 font-mono text-sm leading-6 text-foreground shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                  placeholder="0 9 * * *"
                />
              </div>

              <div v-else-if="form.scheduleKind === 'every'" class="space-y-2">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  Every
                </Label>
                <Input v-model="form.every" placeholder="10m / 1h / 30s" class="h-10 max-w-[220px]" />
              </div>

              <div v-else class="space-y-2">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  At
                </Label>
                <Input v-model="form.at" placeholder="2026-03-25T21:00:00+08:00 或 +20m" class="h-10" />
              </div>

              <div class="mt-4 grid gap-4 md:grid-cols-2">
                <div class="space-y-1.5">
                  <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                    {{ t('openclawCron.dialog.timezone', '时区') }}
                  </Label>
                  <Input v-model="form.timezone" placeholder="Asia/Shanghai" class="h-10" />
                </div>
                <div class="flex items-end justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                  <div>
                    <div class="text-sm font-medium text-foreground">
                      {{ t('openclawCron.dialog.exact', '精确执行') }}
                    </div>
                    <div class="text-xs text-muted-foreground">
                      {{ t('openclawCron.dialog.exactHint', '对应 OpenClaw --exact') }}
                    </div>
                  </div>
                  <Switch :model-value="form.exact" @update:model-value="(value) => (form.exact = !!value)" />
                </div>
              </div>
            </div>
          </section>

          <section class="space-y-4">
            <div class="space-y-1.5">
              <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.message', 'Message') }}
              </Label>
              <textarea
                v-model="form.message"
                class="min-h-[118px] w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 text-sm leading-6 text-foreground shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                :placeholder="t('openclawCron.dialog.messagePlaceholder', 'Agent 定时执行时发送的消息内容')"
              />
            </div>

            <div class="space-y-1.5">
              <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.systemEvent', 'System Event') }}
              </Label>
              <Input v-model="form.systemEvent" :placeholder="t('openclawCron.dialog.systemEventPlaceholder', '可选，OpenClaw system event payload')" class="h-10" />
            </div>
          </section>

          <section class="rounded-lg border border-border bg-card dark:border-white/10">
            <button
              type="button"
              class="flex w-full items-center justify-between px-4 py-3 text-left"
              @click="advancedOpen = !advancedOpen"
            >
              <div>
                <div class="text-sm font-semibold text-foreground">
                  {{ t('openclawCron.dialog.advanced', '高级设置') }}
                </div>
                <div class="text-xs text-muted-foreground">
                  {{ t('openclawCron.dialog.advancedHint', '模型、投递、session、超时等 OpenClaw 原生字段') }}
                </div>
              </div>
              <ChevronDown class="size-4 text-muted-foreground transition-transform" :class="advancedOpen ? 'rotate-180' : ''" />
            </button>

            <div v-if="advancedOpen" class="grid gap-4 border-t border-border px-4 py-4 md:grid-cols-2 dark:border-white/10">
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">Model</Label>
                <Input v-model="form.model" placeholder="provider/model 或 alias" class="h-10" />
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">Thinking</Label>
                <select
                  v-model="form.thinking"
                  class="h-10 w-full rounded-md border border-input bg-transparent px-3 text-sm text-foreground shadow-xs outline-none dark:bg-input/30"
                >
                  <option value="off">off</option>
                  <option value="minimal">minimal</option>
                  <option value="low">low</option>
                  <option value="medium">medium</option>
                  <option value="high">high</option>
                  <option value="xhigh">xhigh</option>
                </select>
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">Session Target</Label>
                <select
                  v-model="form.sessionTarget"
                  class="h-10 w-full rounded-md border border-input bg-transparent px-3 text-sm text-foreground shadow-xs outline-none dark:bg-input/30"
                >
                  <option value="isolated">isolated</option>
                  <option value="main">main</option>
                </select>
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">Session Key</Label>
                <Input v-model="form.sessionKey" placeholder="agent:main:my-session" class="h-10" />
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">Wake Mode</Label>
                <select
                  v-model="form.wakeMode"
                  class="h-10 w-full rounded-md border border-input bg-transparent px-3 text-sm text-foreground shadow-xs outline-none dark:bg-input/30"
                >
                  <option value="now">now</option>
                  <option value="next-heartbeat">next-heartbeat</option>
                </select>
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  Timeout (ms)
                </Label>
                <Input v-model.number="form.timeoutMs" type="number" min="1000" class="h-10" />
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">Channel</Label>
                <Input v-model="form.deliveryChannel" placeholder="last" class="h-10" />
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">To</Label>
                <Input v-model="form.deliveryTo" placeholder="目标会话/用户" class="h-10" />
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">Account</Label>
                <Input v-model="form.deliveryAccountId" placeholder="多账号场景可选" class="h-10" />
              </div>

              <div class="space-y-3 md:col-span-2">
                <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">Announce</span>
                    <Switch :model-value="form.announce" @update:model-value="(value) => (form.announce = !!value)" />
                  </label>
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">Expect Final</span>
                    <Switch :model-value="form.expectFinal" @update:model-value="(value) => (form.expectFinal = !!value)" />
                  </label>
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">Light Context</span>
                    <Switch :model-value="form.lightContext" @update:model-value="(value) => (form.lightContext = !!value)" />
                  </label>
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">Best Effort Deliver</span>
                    <Switch :model-value="form.bestEffortDeliver" @update:model-value="(value) => (form.bestEffortDeliver = !!value)" />
                  </label>
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">Delete After Run</span>
                    <Switch :model-value="form.deleteAfterRun" @update:model-value="(value) => (form.deleteAfterRun = !!value)" />
                  </label>
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">Keep After Run</span>
                    <Switch :model-value="form.keepAfterRun" @update:model-value="(value) => (form.keepAfterRun = !!value)" />
                  </label>
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">{{ t('openclawCron.dialog.enabled', '启用任务') }}</span>
                    <Switch :model-value="form.enabled" @update:model-value="(value) => (form.enabled = !!value)" />
                  </label>
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>

      <DialogFooter class="shrink-0 gap-2 border-t border-border px-6 py-4 sm:justify-end">
        <Button type="button" variant="outline" @click="emit('update:open', false)">
          {{ t('common.cancel') }}
        </Button>
        <Button type="button" :disabled="!canSubmit || saving" @click="handleSubmit">
          {{ saving ? t('common.loading', '处理中...') : t('common.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
