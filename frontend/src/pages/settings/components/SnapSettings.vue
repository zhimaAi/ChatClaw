<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AcceptableValue } from 'reka-ui'
import { Events } from '@wailsio/runtime'
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
import { SnapService } from '@bindings/willchat/internal/services/windows'

const { t } = useI18n()

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

// 所有吸附应用的 ref 映射（每个开关独立，不互斥）
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

const syncSnapFromSettings = async () => {
  try {
    await SnapService.SyncFromSettings()
  } catch (error) {
    console.error('Failed to sync snap service from settings:', error)
  }
}

// Refresh UI from settings (without triggering backend sync to avoid event loop)
const refreshSettingsUI = async () => {
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
  } catch (error) {
    console.error('Failed to refresh snap settings UI:', error)
  }
}

// 加载设置（包括同步后端服务）
const loadSettings = async () => {
  await refreshSettingsUI()
  // 同步后端吸附服务（根据当前 settings 的多个开关状态决定启动/隐藏/吸附目标）
  await syncSnapFromSettings()
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
    // Notify other windows (e.g., winsnap) about the settings change
    await SnapService.NotifySettingsChanged()
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
    // Notify other windows (e.g., winsnap) about the settings change
    await SnapService.NotifySettingsChanged()
  } catch {
    showAiEditButton.value = prev
  }
}

// 处理吸附应用开关变化（每个开关独立，不互斥；所有开关共用一个吸附窗体）
const handleSnapAppChange = async (key: string, refValue: { value: boolean }, val: boolean) => {
  const prev = refValue.value
  refValue.value = val
  try {
    await updateSetting(key, String(val))
  } catch {
    refValue.value = prev
    return
  }
  await syncSnapFromSettings()
}

// 处理发送按键模式变化
const handleSendKeyChange = async (value: AcceptableValue) => {
  if (typeof value === 'string') {
    const prev = sendKeyStrategy.value
    sendKeyStrategy.value = value
    try {
      await updateSetting('send_key_strategy', value)
      // Notify other windows about the settings change
      await SnapService.NotifySettingsChanged()
    } catch {
      sendKeyStrategy.value = prev
    }
  }
}

// Event subscription for snap settings change (broadcast from backend)
let unsubscribeSnapSettingsChanged: (() => void) | null = null

// 页面加载时获取设置
onMounted(() => {
  void loadSettings()

  // Listen for snap settings change event broadcast from backend (e.g., when winsnap window cancels snap)
  // Only refresh UI without calling syncSnapFromSettings to avoid event loop
  unsubscribeSnapSettingsChanged = Events.On('snap:settings-changed', () => {
    void refreshSettingsUI()
  })
})

onUnmounted(() => {
  unsubscribeSnapSettingsChanged?.()
  unsubscribeSnapSettingsChanged = null
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
            <SelectItem v-for="option in sendKeyOptions" :key="option.value" :value="option.value">
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
