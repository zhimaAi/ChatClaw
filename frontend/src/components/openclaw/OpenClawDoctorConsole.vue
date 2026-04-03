<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useColorMode } from '@vueuse/core'
import { Events } from '@wailsio/runtime'
import { Button } from '@/components/ui/button'
import { CheckCircle2, Terminal, AlertCircle, Clock } from 'lucide-vue-next'
import AnsiToHtml from 'ansi-to-html'
import * as OpenClawRuntimeService from '@bindings/chatclaw/internal/openclaw/runtime/openclawruntimeservice'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'

const { t } = useI18n()

/** Correlates Wails chunks with the current run (backend increments runId per invocation). */
const activeDoctorRunId = ref<number | null>(null)
const stdoutScrollEl = ref<HTMLElement | null>(null)
const stderrScrollEl = ref<HTMLElement | null>(null)

/** Plain-text accumulators (for copy) */
const stdoutRaw = ref('')
const stderrRaw = ref('')

/** ANSI-converted HTML for display */
const stdoutHtml = ref('')
const stderrHtml = ref('')

/** ansi-to-html converter — rebuilt when theme changes */
function buildAnsiConverter(dark: boolean) {
  return new AnsiToHtml({
    fg: dark ? '#e2e8f0' : '#1e293b',
    bg: dark ? '#0f172a' : '#f8fafc',
    newline: true,
    escapeXML: true,
    colors: {
      0:  dark ? '#6b7280' : '#6b7280', // gray  — reset
      1:  dark ? '#f87171' : '#dc2626', // red   — bold
      2:  dark ? '#4ade80' : '#16a34a', // green
      3:  dark ? '#facc15' : '#ca8a04', // yellow
      4:  dark ? '#60a5fa' : '#2563eb', // blue
      5:  dark ? '#e879f9' : '#c026d3', // magenta
      6:  dark ? '#22d3ee' : '#0891b2', // cyan
      7:  dark ? '#f1f5f9' : '#1e293b', // white
      30: dark ? '#64748b' : '#374151', // black (dim)
      31: dark ? '#f87171' : '#dc2626', // red
      32: dark ? '#4ade80' : '#16a34a', // green
      33: dark ? '#facc15' : '#ca8a04', // yellow
      34: dark ? '#60a5fa' : '#2563eb', // blue
      35: dark ? '#e879f9' : '#c026d3', // magenta
      36: dark ? '#22d3ee' : '#0891b2', // cyan
      37: dark ? '#f1f5f9' : '#1e293b', // white
      90: dark ? '#6b7280' : '#9ca3af', // bright black (dim gray)
      91: dark ? '#fca5a5' : '#ef4444', // bright red
      92: dark ? '#86efac' : '#22c55e', // bright green
      93: dark ? '#fde047' : '#eab308', // bright yellow
      94: dark ? '#93c5fd' : '#3b82f6', // bright blue
      95: dark ? '#f5d0fe' : '#d946ef', // bright magenta
      96: dark ? '#67e8f9' : '#06b6d4', // bright cyan
      97: dark ? '#ffffff' : '#f8fafc', // bright white
    },
  })
}

const colorMode = useColorMode()

function ansiToHtml(raw: string, dark: boolean): string {
  if (!raw) return ''
  const converter = buildAnsiConverter(dark)
  return converter.toHtml(raw)
}

/** Re-render ANSI output when theme changes */
watch(
  [stdoutRaw, stderrRaw, colorMode],
  () => {
    const dark = colorMode.value === 'dark'
    stdoutHtml.value = ansiToHtml(stdoutRaw.value, dark)
    stderrHtml.value = ansiToHtml(stderrRaw.value, dark)
  },
  { immediate: false }
)

function parseDoctorOutputEvent(event: unknown): Record<string, unknown> | null {
  const e = event as { data?: unknown }
  const raw = Array.isArray(e?.data) ? e.data[0] : e?.data ?? event
  if (raw && typeof raw === 'object') return raw as Record<string, unknown>
  return null
}

function appendChunk(stream: 'stdout' | 'stderr', text: string) {
  if (stream === 'stdout') {
    stdoutRaw.value += text
  } else {
    stderrRaw.value += text
  }
  const dark = colorMode.value === 'dark'
  if (stream === 'stdout') {
    stdoutHtml.value = ansiToHtml(stdoutRaw.value, dark)
  } else {
    stderrHtml.value = ansiToHtml(stderrRaw.value, dark)
  }
  // scrollIntoView on the last <span> for reliable v-html auto-scroll
  void nextTick(() => {
    const el = stream === 'stdout' ? stdoutScrollEl.value : stderrScrollEl.value
    if (el) el.querySelector('span:last-child')?.scrollIntoView({ block: 'end', behavior: 'instant' })
  })
}

let unsubscribeDoctorOutput: (() => void) | null = null

// 执行状态
const isRunning = ref(false)
const command = ref('openclaw doctor')
const workingDir = ref('')
const exitCode = ref<number | null>(null)
const duration = ref<number>(0)
const wasFixed = ref(false)

// Aliases for template compatibility
const stdout = stdoutRaw
const stderr = stderrRaw

// Command / stdout / stderr block stays hidden until Run or Run and fix
const outputPanelVisible = ref(false)

// 判断执行是否成功
const isSuccess = computed(() => exitCode.value === 0)

// 格式化耗时显示
const formatDuration = (ms: number): string => {
  return `${ms}ms`
}

// 运行 openclaw doctor
const runDoctor = async (fix: boolean = false) => {
  if (isRunning.value) return

  outputPanelVisible.value = true
  activeDoctorRunId.value = null
  isRunning.value = true
  stdoutRaw.value = ''
  stderrRaw.value = ''
  stdoutHtml.value = ''
  stderrHtml.value = ''
  exitCode.value = null
  duration.value = 0
  wasFixed.value = false

  const startTime = Date.now()
  const cmd = fix ? 'openclaw doctor --fix --yes --non-interactive' : 'openclaw doctor'

  command.value = cmd

  try {
    // 调用后端接口执行 doctor 命令
    // 注意：这里需要后端提供对应的接口
    const result = await OpenClawRuntimeService.RunDoctorCommand(cmd, fix)

    const endTime = Date.now()
    duration.value = endTime - startTime

    if (!result) {
      exitCode.value = 1
      stderrRaw.value = t('settings.openclawRuntime.doctor.failed')
      stderrHtml.value = ansiToHtml(stderrRaw.value, colorMode.value === 'dark')
      toast.error(t('settings.openclawRuntime.doctor.failed'))
      return
    }

    // Prefer streamed text; fall back if events were not delivered
    if (!stdoutRaw.value && result.stdout) stdoutRaw.value = result.stdout
    if (!stderrRaw.value && result.stderr) stderrRaw.value = result.stderr
    // Re-render full ANSI if fallback was used
    if (stdoutHtml.value !== ansiToHtml(stdoutRaw.value, colorMode.value === 'dark')) {
      stdoutHtml.value = ansiToHtml(stdoutRaw.value, colorMode.value === 'dark')
    }
    if (stderrHtml.value !== ansiToHtml(stderrRaw.value, colorMode.value === 'dark')) {
      stderrHtml.value = ansiToHtml(stderrRaw.value, colorMode.value === 'dark')
    }
    exitCode.value = result.exitCode
    wasFixed.value = result.fixed || false

    if (result.exitCode === 0) {
      toast.success(t('settings.openclawRuntime.doctor.success'))
    } else {
      toast.error(t('settings.openclawRuntime.doctor.failed'))
    }
  } catch (e: any) {
    const endTime = Date.now()
    duration.value = endTime - startTime
    stderrRaw.value = getErrorMessage(e) || e.message || 'Unknown error'
    stderrHtml.value = ansiToHtml(stderrRaw.value, colorMode.value === 'dark')
    exitCode.value = 1
    toast.error(getErrorMessage(e) || t('settings.openclawRuntime.doctor.failed'))
  } finally {
    isRunning.value = false
  }
}

// 复制输出内容
const copyOutput = async () => {
  const content = `Command: ${command.value}\nExit Code: ${exitCode.value}\nDuration: ${formatDuration(duration.value)}\nWorking Directory: ${workingDir.value}\n\n--- STDOUT ---\n${stdoutRaw.value}\n\n--- STDERR ---\n${stderrRaw.value}`

  try {
    await navigator.clipboard.writeText(content)
    toast.success(t('winsnap.toast.copied'))
  } catch (e) {
    toast.error(t('assistant.chat.copyFailed'))
  }
}

// 获取工作目录
const loadWorkingDir = async () => {
  try {
    const status = await OpenClawRuntimeService.GetStatus()
    workingDir.value = status.runtimePath || '-'
  } catch (e) {
    workingDir.value = '-'
  }
}

onMounted(() => {
  void loadWorkingDir()
  unsubscribeDoctorOutput = Events.On('openclaw:doctor-output', (event: unknown) => {
    const d = parseDoctorOutputEvent(event)
    if (!d) return
    const runId = Number(d.runId)
    const stream = String(d.stream ?? '') as 'stdout' | 'stderr'
    const text = String(d.text ?? '')
    if (!Number.isFinite(runId) || runId <= 0) return
    if (activeDoctorRunId.value === null) activeDoctorRunId.value = runId
    if (runId !== activeDoctorRunId.value) return
    if (stream === 'stdout' || stream === 'stderr') appendChunk(stream, text)
  })
})

onUnmounted(() => {
  unsubscribeDoctorOutput?.()
  unsubscribeDoctorOutput = null
})
</script>

<template>
  <div class="bg-white relative size-full rounded-xl border border-border shadow-sm dark:border-white/10 dark:bg-card dark:shadow-none dark:ring-1 dark:ring-white/5">
    <!-- 标题栏 -->
    <div class="flex items-center justify-between border-b border-border bg-muted/30 p-4 dark:border-white/10 dark:bg-white/5">
      <div class="flex flex-col gap-1">
        <h2 class="text-base font-semibold text-foreground">
          {{ t('settings.openclawRuntime.doctor.title') }}
        </h2>
      </div>
      <div class="flex items-center gap-2">
        <!-- 运行按钮 -->
        <Button
          size="sm"
          variant="outline"
          :disabled="isRunning"
          @click="runDoctor(false)"
        >
          <Terminal class="mr-1.5 size-3.5" />
          {{ t('settings.openclawRuntime.doctor.run') }}
        </Button>
        <!-- 运行并修复按钮 -->
        <Button
          size="sm"
          variant="outline"
          :disabled="isRunning"
          @click="runDoctor(true)"
        >
          <CheckCircle2 class="mr-1.5 size-3.5" />
          {{ t('settings.openclawRuntime.doctor.runAndFix') }}
        </Button>
        <!-- 复制按钮 -->
        <Button
          size="sm"
          variant="outline"
          :disabled="isRunning || (!stdout && !stderr)"
          @click="copyOutput"
        >
          {{ t('common.copy') }}
        </Button>
      </div>
    </div>

    <!-- Output area: hidden until Run / Run and fix -->
    <div v-if="outputPanelVisible" class="bg-white p-4 dark:bg-card">
      <!-- 顶部状态标签 -->
      <div class="mb-4 flex items-center gap-2">
        <div
          v-if="isRunning"
          class="bg-muted flex items-center gap-1 rounded-full px-3 py-1"
        >
          <Terminal class="size-3.5 animate-pulse text-primary" />
          <span class="text-xs text-muted-foreground">
            {{ t('settings.openclawRuntime.doctor.running') }}
          </span>
        </div>

        <div
          v-if="exitCode !== null"
          :class="[
            'flex items-center gap-1 rounded-full px-3 py-1',
            isSuccess
              ? 'bg-green-50 dark:bg-green-950/20'
              : 'bg-red-50 dark:bg-red-950/20'
          ]"
        >
          <CheckCircle2
            v-if="isSuccess"
            class="size-3.5 text-green-600 dark:text-green-400"
          />
          <AlertCircle
            v-else
            class="size-3.5 text-red-600 dark:text-red-400"
          />
          <span
            :class="[
              'text-xs',
              isSuccess
                ? 'text-green-700 dark:text-green-400'
                : 'text-red-700 dark:text-red-400'
            ]"
          >
            {{
              isSuccess
                ? t('settings.openclawRuntime.doctor.fixed')
                : t('settings.openclawRuntime.doctor.failed')
            }}
          </span>
        </div>

        <div
          v-if="exitCode !== null"
          class="bg-muted flex items-center gap-1 rounded-full px-3 py-1"
        >
          <span class="text-xs font-mono text-muted-foreground">
            {{ t('settings.openclawRuntime.doctor.exitCode') }}: {{ exitCode }}
          </span>
        </div>

        <div
          v-if="duration > 0"
          class="bg-muted flex items-center gap-1 rounded-full px-3 py-1"
        >
          <Clock class="size-3.5 text-muted-foreground" />
          <span class="text-xs text-muted-foreground">
            {{ t('settings.openclawRuntime.doctor.duration') }}: {{ formatDuration(duration) }}
          </span>
        </div>
      </div>

      <!-- 命令和工作目录信息 -->
      <div class="mb-4 space-y-2 text-sm text-muted-foreground">
        <div class="flex items-center gap-2">
          <span class="shrink-0 font-medium text-foreground">
            {{ t('settings.openclawRuntime.doctor.command') }}:
          </span>
          <code class="rounded bg-muted px-2 py-0.5 font-mono text-xs">
            {{ command }}
          </code>
        </div>
        <div class="flex items-center gap-2">
          <span class="shrink-0 font-medium text-foreground">
            {{ t('settings.openclawRuntime.doctor.workingDir') }}:
          </span>
          <code class="rounded bg-muted px-2 py-0.5 font-mono text-xs break-all">
            {{ workingDir }}
          </code>
        </div>
      </div>

      <!-- 输出区域 - 两列布局 -->
      <div class="grid grid-cols-2 gap-6">
        <!-- 标准输出 -->
        <div class="flex flex-col gap-2">
          <div class="flex items-center gap-2">
            <Terminal class="size-4 text-muted-foreground" />
            <span class="text-sm font-medium text-foreground">
              {{ t('settings.openclawRuntime.doctor.stdout') }}
            </span>
          </div>
          <div
            ref="stdoutScrollEl"
            class="min-h-48 max-h-[32rem] overflow-auto rounded-lg border border-border bg-muted/20 p-3 dark:bg-muted/10"
          >
            <div
              v-if="stdoutHtml"
              class="whitespace-pre-wrap break-words font-mono text-xs"
              v-html="stdoutHtml"
            />
            <p
              v-else
              class="text-sm text-muted-foreground/60"
            >
              {{ t('settings.openclawRuntime.doctor.noOutput') }}
            </p>
          </div>
        </div>

        <!-- 标准错误 -->
        <div class="flex flex-col gap-2">
          <div class="flex items-center gap-2">
            <AlertCircle class="size-4 text-muted-foreground" />
            <span class="text-sm font-medium text-foreground">
              {{ t('settings.openclawRuntime.doctor.stderr') }}
            </span>
          </div>
          <div
            ref="stderrScrollEl"
            class="min-h-48 max-h-[32rem] overflow-auto rounded-lg border border-border bg-muted/20 p-3 dark:bg-muted/10"
          >
            <div
              v-if="stderrHtml"
              class="whitespace-pre-wrap break-words font-mono text-xs"
              v-html="stderrHtml"
            />
            <p
              v-else
              class="text-sm text-muted-foreground/60"
            >
              {{ t('settings.openclawRuntime.doctor.noErrors') }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
