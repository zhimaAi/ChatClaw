<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, Copy, FileText, Image as ImageIcon, Loader2, Paperclip, PenLine, PinOff, Plus, Send, SendHorizontal, Type } from 'lucide-vue-next'
import Logo from '@/assets/images/logo.svg'
import { cn } from '@/lib/utils'
import { Events, System, Clipboard } from '@wailsio/runtime'
import { SettingsService, Category } from '@bindings/willchat/internal/services/settings'
import { SnapService } from '@bindings/willchat/internal/services/windows'
import { WinsnapChatService } from '@bindings/willchat/internal/services/winsnapchat'
import { TextSelectionService } from '@bindings/willchat/internal/services/textselection'
import { useToast } from '@/components/ui/toast'
import { Toaster } from '@/components/ui/toast'

const { t } = useI18n()
const { toast } = useToast()

const question = ref('')
const modelLabel = ref('DeepSeek V3.2 Think')

type ChatRole = 'user' | 'assistant'
type ChatMessage = {
  id: string
  role: ChatRole
  content: string
  display: string
  status: 'done' | 'streaming' | 'error' | 'loading'
}

const messages = ref<ChatMessage[]>([])
const scrollEl = ref<HTMLElement | null>(null)

const isStreaming = ref(false)
const activeRequestId = ref<string | null>(null)
const activeAssistantMsgId = ref<string | null>(null)
const hasSending = ref(false)

const typingTimer = ref<number | null>(null)
const typingQueue = ref('')
const typingMsgId = ref<string | null>(null)
const typedTick = ref(0)

// Store ai_message content as fallback when sending has no data
const aiMessageContent = ref('')

// Buffer events that arrive before activeRequestId is set (race condition fix)
const pendingEvents = ref<Array<{ requestId: string; eventName: string; data: any }>>([])
const pendingRequestId = ref<string | null>(null)

const canSend = computed(() => question.value.trim().length > 0 && !isStreaming.value)

// Settings for button visibility
const showAiSendButton = ref(true)
const showAiEditButton = ref(true)

// Track if there's an attached target (for showing send/edit buttons)
const hasAttachedTarget = ref(false)

// Check snap status and update hasAttachedTarget
const checkSnapStatus = async () => {
  try {
    const status = await SnapService.GetStatus()
    hasAttachedTarget.value = status.state === 'attached' && !!status.targetProcess
  } catch (error) {
    console.error('Failed to check snap status:', error)
    hasAttachedTarget.value = false
  }
}

// Load settings
const loadSettings = async () => {
  try {
    const settings = await SettingsService.List(Category.CategorySnap)
    settings.forEach((setting) => {
      if (setting.key === 'show_ai_send_button') {
        showAiSendButton.value = setting.value === 'true'
      }
      if (setting.key === 'show_ai_edit_button') {
        showAiEditButton.value = setting.value === 'true'
      }
    })
  } catch (error) {
    console.error('Failed to load settings:', error)
  }
}

// Action handlers for AI response buttons
const handleSendAndTrigger = async (content: string) => {
  if (!content) return
  try {
    await SnapService.SendTextToTarget(content, true)
  } catch (error) {
    console.error('Failed to send and trigger:', error)
    toast({
      variant: 'error',
      description: t('winsnap.toast.sendFailed'),
    })
  }
}

const handleSendToEdit = async (content: string) => {
  if (!content) return
  try {
    await SnapService.PasteTextToTarget(content)
  } catch (error) {
    console.error('Failed to paste to edit:', error)
  }
}

const handleCopyToClipboard = async (content: string) => {
  if (!content) return
  try {
    await Clipboard.SetText(content)
    toast({
      description: t('winsnap.toast.copied'),
    })
  } catch (error) {
    console.error('Failed to copy to clipboard:', error)
  }
}

// Map from target process name to settings key
const processToSettingsKey: Record<string, string> = {
  // Windows
  'Weixin.exe': 'snap_wechat',
  'WeChat.exe': 'snap_wechat',
  'WeChatApp.exe': 'snap_wechat',
  'WeChatAppEx.exe': 'snap_wechat',
  'WXWork.exe': 'snap_wecom',
  'QQ.exe': 'snap_qq',
  'QQNT.exe': 'snap_qq',
  'DingTalk.exe': 'snap_dingtalk',
  'Feishu.exe': 'snap_feishu',
  'Lark.exe': 'snap_feishu',
  'Douyin.exe': 'snap_douyin',
  // macOS
  '微信': 'snap_wechat',
  'Weixin': 'snap_wechat',
  'weixin': 'snap_wechat',
  'WeChat': 'snap_wechat',
  'wechat': 'snap_wechat',
  'com.tencent.xinWeChat': 'snap_wechat',
  '企业微信': 'snap_wecom',
  'WeCom': 'snap_wecom',
  'wecom': 'snap_wecom',
  'WeWork': 'snap_wecom',
  'wework': 'snap_wecom',
  'WXWork': 'snap_wecom',
  'wxwork': 'snap_wecom',
  'qiyeweixin': 'snap_wecom',
  'com.tencent.WeWorkMac': 'snap_wecom',
  'QQ': 'snap_qq',
  'qq': 'snap_qq',
  'com.tencent.qq': 'snap_qq',
  '钉钉': 'snap_dingtalk',
  'DingTalk': 'snap_dingtalk',
  'dingtalk': 'snap_dingtalk',
  'com.alibaba.DingTalkMac': 'snap_dingtalk',
  '飞书': 'snap_feishu',
  'Feishu': 'snap_feishu',
  'feishu': 'snap_feishu',
  'Lark': 'snap_feishu',
  'lark': 'snap_feishu',
  'com.bytedance.feishu': 'snap_feishu',
  'com.bytedance.Lark': 'snap_feishu',
  '抖音': 'snap_douyin',
  'Douyin': 'snap_douyin',
  'douyin': 'snap_douyin',
}

const cancelSnap = async () => {
  try {
    // Get current snap status to find the attached target
    const status = await SnapService.GetStatus()

    // If attached to a target, only disable that specific app's toggle
    if (status.state === 'attached' && status.targetProcess) {
      const settingsKey = processToSettingsKey[status.targetProcess]
      if (settingsKey) {
        await SettingsService.SetValue(settingsKey, 'false')
        await SnapService.SyncFromSettings()
        // Update attached target status
        hasAttachedTarget.value = false
        return
      }
    }

    // Fallback: if no attached target or unknown process, do nothing
    // This maintains program robustness when there's no attached window
  } catch (error) {
    console.error('Failed to cancel snap:', error)
  }
}

const scrollToBottom = () => {
  void nextTick(() => {
    const el = scrollEl.value
    if (!el) return
    el.scrollTop = el.scrollHeight
  })
}

const stopTyping = () => {
  if (typingTimer.value != null) {
    window.clearInterval(typingTimer.value)
    typingTimer.value = null
  }
  typingQueue.value = ''
  typingMsgId.value = null
  typedTick.value = 0
}

const startTyping = () => {
  if (typingTimer.value != null) return
  typingTimer.value = window.setInterval(() => {
    if (!typingMsgId.value) {
      stopTyping()
      return
    }
    if (!typingQueue.value) {
      stopTyping()
      return
    }

    const msg = messages.value.find((m) => m.id === typingMsgId.value)
    if (!msg) {
      stopTyping()
      return
    }

    msg.display += typingQueue.value[0]
    typingQueue.value = typingQueue.value.slice(1)
    typedTick.value += 1

    if (typedTick.value % 4 === 0) {
      scrollToBottom()
    }
  }, 18)
}

const enqueueTyping = (msgId: string, text: string) => {
  if (!text) return

  if (typingMsgId.value !== msgId) {
    stopTyping()
    typingMsgId.value = msgId
  }
  typingQueue.value += text
  startTyping()
}

const ensureAssistantMessage = () => {
  if (activeAssistantMsgId.value) {
    const existing = messages.value.find((m) => m.id === activeAssistantMsgId.value)
    if (existing) return existing
  }
  const id = `m_${Date.now()}_${Math.random().toString(16).slice(2)}`
  const msg: ChatMessage = { id, role: 'assistant', content: '', display: '', status: 'loading' }
  messages.value.push(msg)
  activeAssistantMsgId.value = id
  scrollToBottom()
  return msg
}

const startRequest = async (q: string) => {
  isStreaming.value = true
  hasSending.value = false
  activeRequestId.value = null
  activeAssistantMsgId.value = null
  aiMessageContent.value = ''
  pendingEvents.value = []
  pendingRequestId.value = null
  stopTyping()

  messages.value.push({
    id: `m_${Date.now()}_${Math.random().toString(16).slice(2)}`,
    role: 'user',
    content: q,
    display: q,
    status: 'done',
  })
  ensureAssistantMessage()
  scrollToBottom()

  try {
    const requestId = await WinsnapChatService.Ask(q)
    activeRequestId.value = requestId

    // Process any buffered events that arrived before activeRequestId was set
    if (pendingEvents.value.length > 0) {
      const buffered = pendingEvents.value.filter((e) => e.requestId === requestId)
      pendingEvents.value = []
      pendingRequestId.value = null
      for (const ev of buffered) {
        processStreamEvent(ev.requestId, ev.eventName, ev.data)
      }
    }
  } catch (error) {
    console.error('startRequest error:', error)
    const msg = ensureAssistantMessage()
    msg.status = 'error'
    msg.content = String(error)
    msg.display = msg.content
    isStreaming.value = false
    activeRequestId.value = null
    activeAssistantMsgId.value = null
    pendingEvents.value = []
    pendingRequestId.value = null
  }
}

const handleSend = async () => {
  if (!canSend.value) return
  const q = question.value.trim()
  question.value = ''
  await startRequest(q)
}

const handleTextareaKeydown = (e: KeyboardEvent) => {
  if (e.key !== 'Enter') return
  if (e.shiftKey) return
  e.preventDefault()
  void handleSend()
}

// Process a single stream event (extracted for reuse with buffered events)
const processStreamEvent = (requestId: string, eventName: string, data: any) => {
  if (eventName === 'sending') {
    hasSending.value = true
    const msg = ensureAssistantMessage()
    msg.status = 'streaming'
    msg.content += String(data ?? '')
    enqueueTyping(msg.id, String(data ?? ''))
    return
  }

  if (eventName === 'ai_message') {
    let content = ''
    try {
      const obj = JSON.parse(String(data ?? '{}'))
      content = String(obj?.content ?? '')
    } catch {
      content = String(data ?? '')
    }

    if (!content) return

    // Always save ai_message content as fallback
    aiMessageContent.value = content

    const msg = ensureAssistantMessage()
    if (!hasSending.value) {
      // No sending events received, display ai_message content directly
      msg.content = content
      msg.display = content
      msg.status = 'done'
      scrollToBottom()
    } else if (msg.display.length === 0 && msg.content.length > 0) {
      // Typing not started yet but content accumulated
      msg.display = msg.content
      scrollToBottom()
    }
    return
  }

  if (eventName === 'finish') {
    const msg = ensureAssistantMessage()
    msg.status = 'done'

    // Flush remaining typing queue
    if (typingMsgId.value === msg.id && typingQueue.value) {
      msg.display += typingQueue.value
    }
    stopTyping()

    // If sending had no data, use ai_message content as fallback
    if (!msg.content && aiMessageContent.value) {
      msg.content = aiMessageContent.value
      msg.display = aiMessageContent.value
    } else if (!msg.display && msg.content) {
      // If display is empty but content exists, show content
      msg.display = msg.content
    }

    isStreaming.value = false
    activeRequestId.value = null
    activeAssistantMsgId.value = null
    aiMessageContent.value = ''
    pendingEvents.value = []
    pendingRequestId.value = null
    scrollToBottom()
    return
  }
}

let unsubscribeWinsnapChat: (() => void) | null = null
let unsubscribeTextSelection: (() => void) | null = null
let unsubscribeSnapSettings: (() => void) | null = null
let unsubscribeSnapStateChanged: (() => void) | null = null
let onMouseUp: ((e: MouseEvent) => void) | null = null
let onPointerDown: ((e: PointerEvent) => void) | null = null
onMounted(() => {
  // Load settings on mount
  void loadSettings()

  // Check snap status on mount
  void checkSnapStatus()

  // Listen for settings changes
  unsubscribeSnapSettings = Events.On('snap:settings-changed', () => {
    void loadSettings()
    // Also check snap status when settings change (in case snap app was toggled)
    void checkSnapStatus()
  })

  // Listen for snap state changes (attached/hidden/standalone)
  // This updates hasAttachedTarget when state changes
  unsubscribeSnapStateChanged = Events.On('snap:state-changed', (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : event?.data ?? event
    const state = payload?.state
    const targetProcess = payload?.targetProcess
    // Update hasAttachedTarget based on the new state
    hasAttachedTarget.value = state === 'attached' && !!targetProcess
  })

  // Listen for text selection action to set input text
  unsubscribeTextSelection = Events.On('text-selection:send-to-snap', (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : event?.data ?? event
    const text = payload?.text ?? ''
    if (text) {
      question.value = text
      // Auto-send after a short delay
      setTimeout(() => {
        void handleSend()
      }, 100)
    }
  })

  // Wails v3 CustomEvent: payload is inside event.data[0] (first argument passed to Emit)
  unsubscribeWinsnapChat = Events.On('winsnap:chat', (event: any) => {
    // Extract the StreamPayload from event
    // Wails v3 passes arguments as an array in event.data
    const payload = Array.isArray(event?.data) ? event.data[0] : event?.data ?? event

    const requestId = payload?.requestId ?? payload?.RequestID
    const eventName = payload?.event ?? payload?.Event
    const data = payload?.data ?? payload?.Data

    if (!requestId || !eventName) {
      return
    }

    // If activeRequestId is set, process immediately if it matches
    if (activeRequestId.value) {
      if (requestId === activeRequestId.value) {
        processStreamEvent(requestId, eventName, data)
      }
      return
    }

    // If activeRequestId is not set yet (race condition), buffer the event
    // Only buffer if we're in streaming mode (waiting for requestId from Ask call)
    if (isStreaming.value) {
      pendingEvents.value.push({ requestId, eventName, data })
    }
  })

  // In-app text selection within winsnap window.
  onMouseUp = (e: MouseEvent) => {
    if (e.button !== 0) return
    const sel = window.getSelection?.()
    const text = sel?.toString?.().trim?.() ?? ''
    if (!text) return
    // macOS: backend mouse hook uses physical pixels; browser events are in CSS pixels (points).
    const scale = System.IsMac() ? window.devicePixelRatio || 1 : 1
    void TextSelectionService.ShowAtScreenPos(text, Math.round(e.screenX * scale), Math.round(e.screenY * scale))
  }
  window.addEventListener('mouseup', onMouseUp, true)

  // When interacting with winsnap while it's attached, bring both the target window and winsnap to front.
  onPointerDown = (e: PointerEvent) => {
    if (e.button !== 0) return
    void SnapService.WakeAttached()
  }
  window.addEventListener('pointerdown', onPointerDown, true)
})

onUnmounted(() => {
  unsubscribeWinsnapChat?.()
  unsubscribeWinsnapChat = null
  unsubscribeTextSelection?.()
  unsubscribeTextSelection = null
  unsubscribeSnapSettings?.()
  unsubscribeSnapSettings = null
  unsubscribeSnapStateChanged?.()
  unsubscribeSnapStateChanged = null
  if (onMouseUp) {
    window.removeEventListener('mouseup', onMouseUp, true)
    onMouseUp = null
  }
  if (onPointerDown) {
    window.removeEventListener('pointerdown', onPointerDown, true)
    onPointerDown = null
  }
  stopTyping()
})
</script>

<template>
  <div class="flex h-screen w-screen flex-col overflow-hidden bg-background text-foreground">
    <!-- Toolbar (compact, below native titlebar) -->
    <div class="flex h-10 items-center justify-between border-b border-border bg-background px-3">
      <div class="flex items-center gap-1">
        <div class="text-sm font-medium text-foreground">{{ t('winsnap.assistantName') }}</div>
        <ChevronDown class="size-3.5 text-muted-foreground" />
      </div>

      <div class="flex items-center gap-1.5">
        <button class="rounded-md p-1 hover:bg-muted" aria-label="add" type="button">
          <Plus class="size-4 text-muted-foreground" />
        </button>
        <button class="rounded-md p-1 hover:bg-muted" aria-label="edit" type="button">
          <PenLine class="size-4 text-muted-foreground" />
        </button>
        <button
          class="ml-1 inline-flex items-center gap-1.5 rounded-md bg-muted/50 px-2 py-1 text-xs hover:bg-muted"
          @click="cancelSnap"
          type="button"
        >
          <PinOff class="size-3.5 text-muted-foreground" />
          <span class="text-muted-foreground">{{ t('winsnap.cancelSnap') }}</span>
        </button>
      </div>
    </div>

    <!-- Main -->
    <div class="flex min-h-0 flex-1 flex-col bg-background">
      <!-- Empty state (Figma: 132-4484) -->
      <div v-if="messages.length === 0" class="flex flex-1 flex-col items-center justify-center gap-4 px-4">
        <div class="flex items-center gap-3">
          <Logo class="size-12" />
          <div class="text-center text-4xl font-semibold leading-[44px] text-foreground">{{ t('winsnap.title') }}</div>
        </div>
      </div>

      <!-- Chat state (Figma: 139-5673) -->
      <div v-else ref="scrollEl" class="min-h-0 flex-1 overflow-y-auto px-3 py-4">
        <div class="mx-auto flex w-full max-w-[360px] flex-col gap-3">
          <div
            v-for="m in messages"
            :key="m.id"
            class="flex flex-col"
            :class="m.role === 'user' ? 'items-end' : 'items-start'"
          >
            <div
              :class="
                cn(
                  'max-w-[85%] rounded-2xl px-3 py-2 text-sm leading-6 shadow-sm',
                  m.role === 'user' ? 'bg-primary text-primary-foreground' : 'bg-muted text-foreground',
                  m.status === 'error' && 'bg-destructive text-primary-foreground'
                )
              "
            >
              <!-- Loading state -->
              <div v-if="m.status === 'loading'" class="flex items-center gap-2 text-muted-foreground">
                <Loader2 class="size-4 animate-spin" />
                <span>{{ t('winsnap.thinking') }}</span>
              </div>
              <!-- Content -->
              <div v-else class="whitespace-pre-wrap break-words">{{ m.display || m.content }}</div>
            </div>
            <!-- Action buttons for assistant messages (only when done) -->
            <div
              v-if="m.role === 'assistant' && m.status === 'done' && (m.display || m.content)"
              class="mt-2 flex items-center gap-2"
            >
              <!-- Button 1: Send and trigger send key (only when attached to target) -->
              <button
                v-if="showAiSendButton && hasAttachedTarget"
                class="inline-flex items-center justify-center rounded-md border border-border bg-background p-2 hover:bg-muted"
                style="--wails-draggable: no-drag"
                type="button"
                :title="t('winsnap.actions.sendAndTrigger')"
                @click="handleSendAndTrigger(m.display || m.content)"
              >
                <SendHorizontal class="size-4 text-muted-foreground" />
              </button>
              <!-- Button 2: Send to edit box (only when attached to target) -->
              <button
                v-if="showAiEditButton && hasAttachedTarget"
                class="inline-flex items-center justify-center rounded-md border border-border bg-background p-2 hover:bg-muted"
                style="--wails-draggable: no-drag"
                type="button"
                :title="t('winsnap.actions.sendToEdit')"
                @click="handleSendToEdit(m.display || m.content)"
              >
                <Type class="size-4 text-muted-foreground" />
              </button>
              <!-- Button 3: Copy to clipboard -->
              <button
                class="inline-flex items-center justify-center rounded-md border border-border bg-background p-2 hover:bg-muted"
                style="--wails-draggable: no-drag"
                type="button"
                :title="t('winsnap.actions.copyToClipboard')"
                @click="handleCopyToClipboard(m.display || m.content)"
              >
                <Copy class="size-4 text-muted-foreground" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Composer -->
    <div class="p-3">
      <div class="rounded-[16px] border-2 border-border bg-background px-3 py-3 shadow-[0px_4px_8px_rgba(0,0,0,0.06)]">
        <textarea
          v-model="question"
          class="min-h-[64px] w-full resize-none border-0 bg-transparent p-0 text-base leading-6 text-foreground outline-none placeholder:text-muted-foreground"
          :placeholder="t('winsnap.placeholder')"
          style="--wails-draggable: no-drag"
          @keydown="handleTextareaKeydown"
        />

        <div class="mt-3 flex items-end justify-between gap-3">
          <div class="flex flex-1 items-center gap-2">
            <button
              class="inline-flex items-center gap-2 rounded-sm border border-border bg-background px-4 py-1 text-sm leading-[22px] text-muted-foreground"
              style="--wails-draggable: no-drag"
              type="button"
            >
              <span class="text-muted-foreground">{{ modelLabel }}</span>
              <ChevronDown class="size-4 text-muted-foreground" />
            </button>

            <button
              class="inline-flex size-8 items-center justify-center rounded-full border border-border bg-background"
              style="--wails-draggable: no-drag"
              type="button"
            >
              <Paperclip class="size-4 text-muted-foreground" />
            </button>
            <button
              class="inline-flex size-8 items-center justify-center rounded-full border border-border bg-background"
              style="--wails-draggable: no-drag"
              type="button"
            >
              <FileText class="size-4 text-muted-foreground" />
            </button>
            <button
              class="inline-flex size-8 items-center justify-center rounded-full border border-border bg-background"
              style="--wails-draggable: no-drag"
              type="button"
            >
              <ImageIcon class="size-4 text-muted-foreground" />
            </button>
          </div>

          <button
            :class="
              cn(
                'inline-flex items-center justify-center rounded-full p-[6px] transition-colors',
                canSend ? 'bg-primary text-primary-foreground hover:bg-primary/90' : 'bg-muted text-muted-foreground'
              )
            "
            style="--wails-draggable: no-drag"
            type="button"
            aria-label="send"
            :disabled="!canSend"
            @click="handleSend"
          >
            <Send class="size-[22px] rotate-180" />
          </button>
        </div>
      </div>
    </div>
    <Toaster />
  </div>
</template>
