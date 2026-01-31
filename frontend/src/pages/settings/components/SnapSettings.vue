<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AcceptableValue } from 'reka-ui'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'

// 导入吸附应用图标
import WechatIcon from '@/assets/icons/snap/wechat.svg'
import WecomIcon from '@/assets/icons/snap/wecom.svg'
import QQIcon from '@/assets/icons/snap/qq.svg'
import DingtalkIcon from '@/assets/icons/snap/dingtalk.svg'
import FeishuIcon from '@/assets/icons/snap/feishu.svg'
import DouyinIcon from '@/assets/icons/snap/douyin.svg'

// 后端绑定
import { SettingsService, Category } from '@bindings/willchat/internal/services/settings'
import { WindowService } from '@bindings/willchat/internal/services/windows'

const { t } = useI18n()

// 子窗口名称常量
const WINDOW_WINSNAP = 'winsnap'

// 设置状态
const showAiSendButton = ref(true)
const sendKeyStrategy = ref('enter')
const showAiEditButton = ref(true)

// 吸附应用状态（互斥，同一时间只能开启一个）
const snapWechat = ref(false)
const snapWecom = ref(false)
const snapQQ = ref(false)
const snapDingtalk = ref(false)
const snapFeishu = ref(false)
const snapDouyin = ref(false)

// 所有吸附应用的 ref 映射，用于互斥控制
const snapAppRefs: Record<string, { value: boolean }> = {
  snap_wechat: snapWechat,
  snap_wecom: snapWecom,
  snap_qq: snapQQ,
  snap_dingtalk: snapDingtalk,
  snap_feishu: snapFeishu,
  snap_douyin: snapDouyin,
}

// 发送按键模式选项
const sendKeyOptions = [
  { value: 'enter', label: 'settings.snap.sendKeyOptions.enter' },
  { value: 'ctrl_enter', label: 'settings.snap.sendKeyOptions.ctrlEnter' },
]

// 当前发送按键模式显示文本
const currentSendKeyLabel = computed(() => {
  const option = sendKeyOptions.find((opt) => opt.value === sendKeyStrategy.value)
  return option ? t(option.label) : ''
})

// 吸附应用列表
const snapApps = computed(() => [
  {
    key: 'snap_wechat',
    label: t('settings.snap.apps.wechat'),
    icon: WechatIcon,
    value: snapWechat,
  },
  {
    key: 'snap_wecom',
    label: t('settings.snap.apps.wecom'),
    icon: WecomIcon,
    value: snapWecom,
  },
  { key: 'snap_qq', label: t('settings.snap.apps.qq'), icon: QQIcon, value: snapQQ },
  {
    key: 'snap_dingtalk',
    label: t('settings.snap.apps.dingtalk'),
    icon: DingtalkIcon,
    value: snapDingtalk,
  },
  {
    key: 'snap_feishu',
    label: t('settings.snap.apps.feishu'),
    icon: FeishuIcon,
    value: snapFeishu,
  },
  {
    key: 'snap_douyin',
    label: t('settings.snap.apps.douyin'),
    icon: DouyinIcon,
    value: snapDouyin,
  },
])

// 布尔设置映射表
const boolSettingsMap: Record<string, { value: boolean }> = {
  show_ai_send_button: showAiSendButton,
  show_ai_edit_button: showAiEditButton,
  ...snapAppRefs,
}

// 吸附应用 key 的固定顺序（用于从“多个为 true”的异常状态中恢复）
const snapKeyOrder = [
  'snap_wechat',
  'snap_wecom',
  'snap_qq',
  'snap_dingtalk',
  'snap_feishu',
  'snap_douyin',
] as const

// 加载设置
const loadSettings = async () => {
  try {
    const settings = await SettingsService.List(Category.CategorySnap)
    settings.forEach((setting) => {
      // 处理布尔类型设置
      const boolRef = boolSettingsMap[setting.key]
      if (boolRef) {
        boolRef.value = setting.value === 'true'
        return
      }
      // 处理其他类型设置
      if (setting.key === 'send_key_strategy') {
        sendKeyStrategy.value = setting.value
      }
    })

    // 互斥修复：如果异常地出现多个吸附应用同时为 true，保留一个，其余写回 false
    const activeSnapKeys = snapKeyOrder.filter((k) => snapAppRefs[k].value)
    if (activeSnapKeys.length > 1) {
      const keepKey = activeSnapKeys[0]
      const keysToDisable = activeSnapKeys.slice(1)
      for (const k of keysToDisable) {
        snapAppRefs[k].value = false
        try {
          await updateSetting(k, 'false')
        } catch {
          // best-effort：失败时保持 UI 已纠正，避免继续处于“多开关同时开”的状态
        }
      }
      // 确保保留的 key 仍然为 true（UI 可能被外部状态影响）
      snapAppRefs[keepKey].value = true
    }

    const hasActiveSnapApp = snapKeyOrder.some((k) => snapAppRefs[k].value)
    // 如果有活跃的吸附应用，显示 winsnap 子窗口
    if (hasActiveSnapApp) {
      try {
        await WindowService.Show(WINDOW_WINSNAP)
      } catch (error) {
        console.error('Failed to show winsnap window on load:', error)
      }
    }
  } catch (error) {
    console.error('Failed to load snap settings:', error)
  }
}

// 更新设置
const updateSetting = async (key: string, value: string) => {
  try {
    await SettingsService.SetValue(key, value)
  } catch (error) {
    console.error(`Failed to update setting ${key}:`, error)
    throw error
  }
}

// 处理 AI 发送按钮开关变化
const handleAiSendButtonChange = async (val: boolean) => {
  const prev = showAiSendButton.value
  showAiSendButton.value = val
  try {
    await updateSetting('show_ai_send_button', String(val))
  } catch {
    showAiSendButton.value = prev
  }
}

// 处理 AI 编辑按钮开关变化
const handleAiEditButtonChange = async (val: boolean) => {
  const prev = showAiEditButton.value
  showAiEditButton.value = val
  try {
    await updateSetting('show_ai_edit_button', String(val))
  } catch {
    showAiEditButton.value = prev
  }
}

// 处理吸附应用开关变化（互斥逻辑）
const handleSnapAppChange = async (key: string, refValue: { value: boolean }, val: boolean) => {
  if (val) {
    // 开启时：先关闭其他所有开关，再开启当前开关（并支持失败回滚）
    const prevSnapState: Record<string, boolean> = {}
    for (const [k, r] of Object.entries(snapAppRefs)) {
      prevSnapState[k] = r.value
    }

    // UI 先做互斥纠正
    for (const [appKey, appRef] of Object.entries(snapAppRefs)) {
      if (appKey !== key) {
        appRef.value = false
      }
    }
    refValue.value = true

    try {
      // 顺序写入，避免并发写导致状态抖动
      for (const [appKey, appRef] of Object.entries(snapAppRefs)) {
        if (appKey !== key && prevSnapState[appKey] && !appRef.value) {
          await updateSetting(appKey, 'false')
        }
      }
      await updateSetting(key, 'true')
    } catch {
      // 回滚 UI
      for (const [k, r] of Object.entries(snapAppRefs)) {
        r.value = prevSnapState[k] ?? false
      }
      return
    }

    // 显示 winsnap 子窗口
    try {
      await WindowService.Show(WINDOW_WINSNAP)
    } catch (error) {
      console.error('Failed to show winsnap window:', error)
    }
  } else {
    const prev = refValue.value
    refValue.value = false
    try {
      await updateSetting(key, 'false')
    } catch {
      refValue.value = prev
      return
    }

    // 关闭 winsnap 子窗口
    try {
      await WindowService.Close(WINDOW_WINSNAP)
    } catch (error) {
      console.error('Failed to close winsnap window:', error)
    }
  }
}

// 处理发送按键模式变化
const handleSendKeyChange = async (value: AcceptableValue) => {
  if (typeof value === 'string') {
    const prev = sendKeyStrategy.value
    sendKeyStrategy.value = value
    try {
      await updateSetting('send_key_strategy', value)
    } catch {
      sendKeyStrategy.value = prev
    }
  }
}

// 页面加载时获取设置
onMounted(() => {
  void loadSettings()
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <!-- 设置卡片 -->
    <SettingsCard :title="t('settings.snap.title')">
      <!-- AI回复显示发送到聊天按钮 -->
      <SettingsItem :label="t('settings.snap.showAiSendButton')">
        <Switch :model-value="showAiSendButton" @update:model-value="handleAiSendButtonChange" />
      </SettingsItem>

      <!-- 发送消息按键模式 -->
      <SettingsItem :label="t('settings.snap.sendKeyStrategy')">
        <Select :model-value="sendKeyStrategy" @update:model-value="handleSendKeyChange">
          <SelectTrigger class="w-54">
            <SelectValue>{{ currentSendKeyLabel }}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem
              v-for="option in sendKeyOptions"
              :key="option.value"
              :value="option.value"
            >
              {{ t(option.label) }}
            </SelectItem>
          </SelectContent>
        </Select>
      </SettingsItem>

      <!-- AI回复显示编辑内容按钮 -->
      <SettingsItem :label="t('settings.snap.showAiEditButton')" :bordered="false">
        <Switch :model-value="showAiEditButton" @update:model-value="handleAiEditButtonChange" />
      </SettingsItem>
    </SettingsCard>

    <!-- 吸附应用卡片 -->
    <SettingsCard :title="t('settings.snap.appsTitle')">
      <SettingsItem
        v-for="(app, index) in snapApps"
        :key="app.key"
        :bordered="index !== snapApps.length - 1"
      >
        <template #default>
          <Switch
            :model-value="app.value.value"
            @update:model-value="(val: boolean) => handleSnapAppChange(app.key, app.value, val)"
          />
        </template>
        <template #label>
          <div class="flex items-center gap-2">
            <component :is="app.icon" class="size-5" />
            <span class="text-sm font-medium text-foreground">{{ app.label }}</span>
          </div>
        </template>
      </SettingsItem>
    </SettingsCard>
  </div>
</template>
