<script setup lang="ts">
/**
 * 一问多答页面
 * 支持同时向多个 AI 模型发送问题，并排显示各个模型的回答
 * 通过后端 WebView 服务嵌入 AI 网站
 */
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import ModelSelector from './components/ModelSelector.vue'
import ColumnToggle from './components/ColumnToggle.vue'
import ChatPanel from './components/ChatPanel.vue'
import MessageInput from './components/MessageInput.vue'
import { MultiaskService } from '../../../bindings/willchat/internal/services/multiask'
import { useNavigationStore } from '@/stores'

const { t } = useI18n()
const navigationStore = useNavigationStore()

/**
 * Props - tab page instance ID
 */
const props = defineProps<{
  tabId: string
}>()

/**
 * Whether this tab is currently active (visible)
 */
const isTabActive = computed(() => navigationStore.activeTabId === props.tabId)

/**
 * localStorage 存储键名
 */
const STORAGE_KEY_MODEL_ORDER = 'willchat:multiask:model-order'
const STORAGE_KEY_SELECTED_MODELS = 'willchat:multiask:selected-models'

/**
 * 服务是否已初始化
 */
const serviceInitialized = ref(false)

/**
 * 初始化服务
 */
const initService = async () => {
  if (serviceInitialized.value) return true

  try {
    console.log('[MultiaskPage] Initializing MultiaskService...')
    await MultiaskService.Initialize('WillChat')
    serviceInitialized.value = true
    console.log('[MultiaskPage] MultiaskService initialized successfully')
    return true
  } catch (err) {
    console.error('[MultiaskPage] Failed to initialize MultiaskService:', err)
    return false
  }
}

/**
 * AI 模型定义
 */
interface AIModel {
  id: string
  name: string
  icon: string
  displayName?: string
  /** AI 网站 URL */
  url?: string
}

/**
 * 最大可选模型数量
 */
const MAX_SELECTED_MODELS = 3

/**
 * 检测系统是否为中文环境
 */
const isChineseLocale = () => {
  const lang = navigator.language || navigator.languages?.[0] || 'en'
  return lang.toLowerCase().startsWith('zh')
}

/**
 * 中文环境优先的模型（按优先级排序）
 */
const chineseFirstModels: AIModel[] = [
  {
    id: 'deepseek',
    name: 'deepseek',
    icon: 'model-deepseek',
    displayName: 'DeepSeek',
    url: 'https://chat.deepseek.com/',
  },
  {
    id: 'doubao',
    name: 'doubao',
    icon: 'model-doubao',
    displayName: '豆包',
    url: 'https://www.doubao.com/chat/',
  },
  {
    id: 'qwen',
    name: 'qwen',
    icon: 'model-qwen',
    displayName: '通义千问',
    url: 'https://www.qianwen.com/',
  },
  {
    id: 'openai',
    name: 'chatgpt',
    icon: 'model-chatgpt',
    displayName: 'ChatGPT',
    url: 'https://chatgpt.com/',
  },
  {
    id: 'google',
    name: 'gemini',
    icon: 'model-gemini',
    displayName: 'Gemini',
    url: 'https://gemini.google.com/',
  },
  {
    id: 'anthropic',
    name: 'claude',
    icon: 'model-claude',
    displayName: 'Claude',
    url: 'https://claude.ai/',
  },
]

/**
 * 非中文环境优先的模型（按优先级排序）
 */
const englishFirstModels: AIModel[] = [
  {
    id: 'openai',
    name: 'chatgpt',
    icon: 'model-chatgpt',
    displayName: 'ChatGPT',
    url: 'https://chatgpt.com/',
  },
  {
    id: 'google',
    name: 'gemini',
    icon: 'model-gemini',
    displayName: 'Gemini',
    url: 'https://gemini.google.com/',
  },
  {
    id: 'anthropic',
    name: 'claude',
    icon: 'model-claude',
    displayName: 'Claude',
    url: 'https://claude.ai/',
  },
  {
    id: 'deepseek',
    name: 'deepseek',
    icon: 'model-deepseek',
    displayName: 'DeepSeek',
    url: 'https://chat.deepseek.com/',
  },
  {
    id: 'doubao',
    name: 'doubao',
    icon: 'model-doubao',
    displayName: '豆包',
    url: 'https://www.doubao.com/chat/',
  },
  {
    id: 'qwen',
    name: 'qwen',
    icon: 'model-qwen',
    displayName: '通义千问',
    url: 'https://www.qianwen.com/',
  },
]

/**
 * 获取默认模型列表（按语言环境排序）
 */
const getDefaultModels = () => {
  return isChineseLocale() ? [...chineseFirstModels] : [...englishFirstModels]
}

/**
 * 从 localStorage 读取自定义排序
 */
const loadCustomOrder = (): string[] | null => {
  try {
    const saved = localStorage.getItem(STORAGE_KEY_MODEL_ORDER)
    if (saved) {
      const order = JSON.parse(saved) as string[]
      if (Array.isArray(order) && order.length > 0) {
        return order
      }
    }
  } catch (err) {
    console.warn('[MultiaskPage] Failed to load custom order:', err)
  }
  return null
}

/**
 * 保存自定义排序到 localStorage
 */
const saveCustomOrder = (models: AIModel[]) => {
  try {
    const order = models.map((m) => m.id)
    localStorage.setItem(STORAGE_KEY_MODEL_ORDER, JSON.stringify(order))
  } catch (err) {
    console.warn('[MultiaskPage] Failed to save custom order:', err)
  }
}

/**
 * 根据自定义排序或系统语言获取初始模型列表
 */
const getInitialModels = () => {
  const customOrder = loadCustomOrder()

  if (customOrder) {
    // 有自定义排序，按保存的顺序排列
    const defaultModels = getDefaultModels()
    const modelMap = new Map(defaultModels.map((m) => [m.id, m]))
    const orderedModels: AIModel[] = []

    // 按自定义顺序添加模型
    for (const id of customOrder) {
      const model = modelMap.get(id)
      if (model) {
        orderedModels.push(model)
        modelMap.delete(id)
      }
    }

    // 添加新增的模型（不在自定义顺序中的）
    for (const model of modelMap.values()) {
      orderedModels.push(model)
    }

    return orderedModels
  }

  // 没有自定义排序，按语言环境排序
  return getDefaultModels()
}

/**
 * 根据系统语言获取默认选中的模型 ID
 */
const getDefaultSelectedIds = () => {
  const models = getInitialModels()
  return models.slice(0, MAX_SELECTED_MODELS).map((m) => m.id)
}

/**
 * 从 localStorage 读取选中的模型 ID
 */
const loadSelectedModels = (): string[] | null => {
  try {
    const saved = localStorage.getItem(STORAGE_KEY_SELECTED_MODELS)
    if (saved) {
      const ids = JSON.parse(saved) as string[]
      if (Array.isArray(ids) && ids.length > 0) {
        // 验证保存的 ID 都是有效的模型 ID
        const validIds = getDefaultModels().map((m) => m.id)
        const filteredIds = ids.filter((id) => validIds.includes(id))
        if (filteredIds.length > 0) {
          return filteredIds.slice(0, MAX_SELECTED_MODELS)
        }
      }
    }
  } catch (err) {
    console.warn('[MultiaskPage] Failed to load selected models:', err)
  }
  return null
}

/**
 * 保存选中的模型 ID 到 localStorage
 */
const saveSelectedModels = (ids: string[]) => {
  try {
    localStorage.setItem(STORAGE_KEY_SELECTED_MODELS, JSON.stringify(ids))
  } catch (err) {
    console.warn('[MultiaskPage] Failed to save selected models:', err)
  }
}

/**
 * 获取初始选中的模型 ID（优先使用保存的，否则使用默认）
 */
const getInitialSelectedIds = () => {
  const saved = loadSelectedModels()
  return saved || getDefaultSelectedIds()
}

/**
 * 可用的 AI 模型列表
 * 配置各 AI 平台的网站 URL
 */
const availableModels = ref<AIModel[]>(getInitialModels())

/**
 * 选中的模型 ID 列表
 */
const selectedModelIds = ref<string[]>(getInitialSelectedIds())

/**
 * 当前分栏数（1/2/3列）
 * 根据初始选中的模型数量设置
 */
const columnCount = ref(getInitialSelectedIds().length)

/**
 * 用户输入的消息
 */
const userMessage = ref('')

/**
 * 是否正在发送消息
 */
const isSending = ref(false)

/**
 * 聊天面板引用
 */
const chatPanelRefs = ref<Record<string, InstanceType<typeof ChatPanel> | null>>({})

/**
 * 输入框引用
 */
const messageInputRef = ref<InstanceType<typeof MessageInput> | null>(null)

/**
 * 已创建的 WebView 面板 ID
 */
const createdPanelIds = ref<Set<string>>(new Set())

/**
 * 获取选中的模型详情
 */
const selectedModels = computed(() => {
  return availableModels.value.filter((model) => selectedModelIds.value.includes(model.id))
})

/**
 * 处理模型排序变化（拖拽排序）
 * 保存自定义顺序到 localStorage
 * 并更新 WebView 面板位置
 */
const handleReorderModels = async (reorderedModels: AIModel[]) => {
  availableModels.value = reorderedModels
  saveCustomOrder(reorderedModels)

  // 等待 DOM 更新后重新计算并更新所有 WebView 位置
  await nextTick()
  await updateAllPanelBounds()
}

/**
 * 更新所有可见面板的 WebView 位置
 * 用于拖拽排序后同步 WebView 位置
 */
const updateAllPanelBounds = async () => {
  // 等待一小段时间确保 DOM 完全更新
  await new Promise((resolve) => setTimeout(resolve, 50))

  for (const model of visibleModels.value) {
    const panelRef = chatPanelRefs.value[model.id]
    if (panelRef?.getBounds) {
      const bounds = panelRef.getBounds()
      if (bounds && bounds.width > 0 && bounds.height > 0) {
        await updatePanelBounds(model.id, bounds)
      }
    }
  }

  console.log('[MultiaskPage] Updated all panel bounds after reorder')
}

/**
 * 当前显示的模型（根据分栏数限制）
 */
const visibleModels = computed(() => {
  return selectedModels.value.slice(0, columnCount.value)
})

/**
 * 切换模型选中状态
 * - 最多选取 MAX_SELECTED_MODELS 个模型
 * - 选取超过限制时，替换掉已选中的最后一个模型
 * - 自动调整分屏模式与选中模型数量对应
 */
const handleToggleModel = (modelId: string) => {
  const index = selectedModelIds.value.indexOf(modelId)
  if (index > -1) {
    // 取消选中：至少保留一个选中的模型
    if (selectedModelIds.value.length > 1) {
      selectedModelIds.value.splice(index, 1)
    }
  } else {
    // 新增选中
    if (selectedModelIds.value.length >= MAX_SELECTED_MODELS) {
      // 已达到最大数量，替换掉最后一个
      selectedModelIds.value.pop()
    }
    selectedModelIds.value.push(modelId)
  }

  // 自动调整分屏模式与选中模型数量对应
  columnCount.value = selectedModelIds.value.length

  // 保存选中状态到 localStorage
  saveSelectedModels(selectedModelIds.value)
}

/**
 * 生成面板 ID
 */
const getPanelId = (modelId: string) => `multiask-panel-${modelId}`

/**
 * 设置面板引用
 */
const setPanelRef = (modelId: string, el: unknown) => {
  if (el) {
    chatPanelRefs.value[modelId] = el as InstanceType<typeof ChatPanel>
  } else {
    delete chatPanelRefs.value[modelId]
  }
}

/**
 * 创建 WebView 面板
 */
const createPanel = async (
  model: AIModel,
  bounds: { x: number; y: number; width: number; height: number }
) => {
  const panelId = getPanelId(model.id)

  console.log(`[MultiaskPage] createPanel called for ${panelId} with bounds:`, bounds)

  // 确保服务已初始化
  if (!serviceInitialized.value) {
    const ok = await initService()
    if (!ok) {
      console.error(`[MultiaskPage] Cannot create panel, service not initialized`)
      return
    }
  }

  // 如果面板已存在，只更新位置
  if (createdPanelIds.value.has(panelId)) {
    console.log(`[MultiaskPage] Panel ${panelId} already exists, updating bounds`)
    await updatePanelBounds(model.id, bounds)
    return
  }

  if (!model.url) {
    console.warn(`[MultiaskPage] No URL configured for model: ${model.id}`)
    return
  }

  // 验证 bounds
  if (bounds.width <= 0 || bounds.height <= 0) {
    console.warn(`[MultiaskPage] Invalid bounds for ${panelId}:`, bounds)
    return
  }

  try {
    console.log(`[MultiaskPage] Calling MultiaskService.CreatePanel for ${panelId}...`)
    await MultiaskService.CreatePanel(
      panelId,
      model.name,
      model.displayName || model.name,
      model.url,
      bounds
    )
    createdPanelIds.value.add(panelId)
    console.log(`[MultiaskPage] Successfully created panel: ${panelId}`)
  } catch (err) {
    console.error(`[MultiaskPage] Failed to create panel ${panelId}:`, err)
  }
}

/**
 * 更新面板位置和大小
 */
const updatePanelBounds = async (
  modelId: string,
  bounds: { x: number; y: number; width: number; height: number }
) => {
  const panelId = getPanelId(modelId)
  if (!createdPanelIds.value.has(panelId)) return

  try {
    await MultiaskService.UpdatePanelBounds(panelId, bounds)
  } catch (err) {
    console.error(`[MultiaskPage] Failed to update panel bounds ${panelId}:`, err)
  }
}

/**
 * 显示面板
 */
const showPanel = async (modelId: string) => {
  const panelId = getPanelId(modelId)
  if (!createdPanelIds.value.has(panelId)) return

  try {
    await MultiaskService.ShowPanel(panelId)
  } catch (err) {
    console.error(`[MultiaskPage] Failed to show panel ${panelId}:`, err)
  }
}

/**
 * 隐藏面板
 */
const hidePanel = async (modelId: string) => {
  const panelId = getPanelId(modelId)
  if (!createdPanelIds.value.has(panelId)) return

  try {
    await MultiaskService.HidePanel(panelId)
  } catch (err) {
    console.error(`[MultiaskPage] Failed to hide panel ${panelId}:`, err)
  }
}

/**
 * 销毁面板
 */
const destroyPanel = async (modelId: string) => {
  const panelId = getPanelId(modelId)
  if (!createdPanelIds.value.has(panelId)) return

  try {
    await MultiaskService.DestroyPanel(panelId)
    createdPanelIds.value.delete(panelId)
    console.log(`[MultiaskPage] Destroyed panel: ${panelId}`)
  } catch (err) {
    console.error(`[MultiaskPage] Failed to destroy panel ${panelId}:`, err)
  }
}

/**
 * 销毁所有面板
 */
const destroyAllPanels = async () => {
  try {
    await MultiaskService.DestroyAllPanels()
    createdPanelIds.value.clear()
    console.log('[MultiaskPage] Destroyed all panels')
  } catch (err) {
    console.error('[MultiaskPage] Failed to destroy all panels:', err)
  }
}

/**
 * 处理面板挂载完成
 */
const handlePanelMounted = (
  model: AIModel,
  bounds: { x: number; y: number; width: number; height: number }
) => {
  createPanel(model, bounds)
}

/**
 * 处理面板大小变化
 */
const handlePanelResize = (
  model: AIModel,
  bounds: { x: number; y: number; width: number; height: number }
) => {
  updatePanelBounds(model.id, bounds)
}

/**
 * 发送消息到所有选中的 AI 面板
 */
const handleSend = async () => {
  if (!userMessage.value.trim() || isSending.value) return

  const message = userMessage.value.trim()
  isSending.value = true

  try {
    // 向所有已显示的面板发送消息
    const errors = await MultiaskService.SendMessageToAllPanels(message)
    if (errors && errors.length > 0) {
      console.warn('[MultiaskPage] Some panels failed to receive message:', errors)
    }
  } catch (err) {
    console.error('[MultiaskPage] Failed to send message:', err)
  }

  // 清空输入框
  userMessage.value = ''
  isSending.value = false
}

/**
 * 聚焦内容输入框（仅在多屏模式显示时）
 */
const focusMessageInput = async () => {
  if (columnCount.value <= 1) return
  await nextTick()
  // Delay focus to override WebView auto-focus.
  messageInputRef.value?.focus?.()
}

/**
 * 页面首次进入时，如果输入框存在则自动聚焦
 */
onMounted(() => {
  setTimeout(() => {
    focusMessageInput()
  }, 4000)
})

/**
 * 分栏切换为多屏时，自动聚焦输入框
 */
watch(columnCount, (count) => {
  if (count > 1) {
    focusMessageInput()
  }
})

/**
 * 监听可见模型变化，管理面板的显示/隐藏/创建
 */
watch(
  visibleModels,
  async (newModels, oldModels) => {
    const oldIds = new Set((oldModels || []).map((m) => m.id))
    const newIds = new Set(newModels.map((m) => m.id))

    // 隐藏不再显示的面板
    for (const model of oldModels || []) {
      if (!newIds.has(model.id)) {
        await hidePanel(model.id)
      }
    }

    // 显示新增的面板（如果已创建）或等待组件挂载后创建
    await nextTick()
    for (const model of newModels) {
      if (!oldIds.has(model.id)) {
        const panelId = getPanelId(model.id)
        if (createdPanelIds.value.has(panelId)) {
          await showPanel(model.id)
          // 更新位置
          const panelRef = chatPanelRefs.value[model.id]
          if (panelRef?.getBounds) {
            const bounds = panelRef.getBounds()
            if (bounds) {
              await updatePanelBounds(model.id, bounds)
            }
          }
        }
      }
    }
  },
  { deep: true }
)

/**
 * Monitor tab active state to hide/show native WebView panels.
 * Native WebViews are rendered outside the DOM tree, so v-show cannot hide them.
 * We need to call backend methods to explicitly hide/show them when switching tabs.
 */
watch(isTabActive, async (active, wasActive) => {
  if (active === wasActive) return
  
  if (active) {
    // Tab is now active - show all visible panels
    console.log('[MultiaskPage] Tab activated, showing all panels')
    for (const model of visibleModels.value) {
      await showPanel(model.id)
      // Update bounds in case layout changed while hidden
      const panelRef = chatPanelRefs.value[model.id]
      if (panelRef?.getBounds) {
        const bounds = panelRef.getBounds()
        if (bounds && bounds.width > 0 && bounds.height > 0) {
          await updatePanelBounds(model.id, bounds)
        }
      }
    }
  } else {
    // Tab is now hidden - hide all panels
    console.log('[MultiaskPage] Tab deactivated, hiding all panels')
    for (const panelId of createdPanelIds.value) {
      try {
        await MultiaskService.HidePanel(panelId)
      } catch (err) {
        console.error(`[MultiaskPage] Failed to hide panel ${panelId}:`, err)
      }
    }
  }
}, { immediate: false })

/**
 * 组件卸载时销毁所有面板
 */
onUnmounted(() => {
  destroyAllPanels()
})
</script>

<template>
  <div class="flex h-full w-full flex-col overflow-hidden bg-background">
    <!-- 顶部模型选择区域 -->
    <div
      class="flex shrink-0 items-center justify-between gap-2 border-b border-border bg-background px-2 py-2"
    >
      <!-- 模型选择器 -->
      <ModelSelector
        :models="availableModels"
        :selected-ids="selectedModelIds"
        @toggle="handleToggleModel"
        @reorder="handleReorderModels"
      />

      <!-- 分栏切换 -->
      <ColumnToggle v-model="columnCount" />
    </div>

    <!-- 聊天面板区域 -->
    <div class="flex min-h-0 flex-1 px-4 pb-4 pt-2">
      <!-- 面板容器 -->
      <div
        class="grid h-full w-full gap-2"
        :style="{
          gridTemplateColumns: `repeat(${Math.min(columnCount, selectedModels.length)}, 1fr)`,
        }"
      >
        <ChatPanel
          v-for="model in visibleModels"
          :key="model.id"
          :ref="(el) => setPanelRef(model.id, el)"
          :model="model"
          :panel-id="getPanelId(model.id)"
          @mounted="(bounds) => handlePanelMounted(model, bounds)"
          @resize="(bounds) => handlePanelResize(model, bounds)"
        />
      </div>
    </div>

    <!-- 底部输入区域（仅多屏模式显示） -->
    <div v-if="columnCount > 1" class="shrink-0 bg-background px-4 pb-6">
      <div class="mx-auto max-w-[800px]">
        <MessageInput
          ref="messageInputRef"
          v-model="userMessage"
          :disabled="isSending"
          :placeholder="t('multiask.inputPlaceholder')"
          @send="handleSend"
        />
      </div>
    </div>
  </div>
</template>
