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
import { Input } from '@/components/ui/input'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Label } from '@/components/ui/label'
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

// 默认点击偏移量 Y（与后端保持一致，不同应用不同）
const DEFAULT_CLICK_OFFSET_Y: Record<string, string> = {
  snap_wechat: '120',
  snap_wecom: '120',
  snap_qq: '120',
  snap_dingtalk: '120',
  snap_feishu: '50',
  snap_douyin: '50',
}

// 吸附应用点击偏移量 X（像素，0 表示居中）
const snapWechatClickOffsetX = ref('')
const snapWecomClickOffsetX = ref('')
const snapQQClickOffsetX = ref('')
const snapDingtalkClickOffsetX = ref('')
const snapFeishuClickOffsetX = ref('')
const snapDouyinClickOffsetX = ref('')

// 吸附应用点击偏移量 Y（像素）
const snapWechatClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_wechat)
const snapWecomClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_wecom)
const snapQQClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_qq)
const snapDingtalkClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_dingtalk)
const snapFeishuClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_feishu)
const snapDouyinClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_douyin)

// 吸附应用不点击模式
const snapWechatNoClick = ref(false)
const snapWecomNoClick = ref(false)
const snapQQNoClick = ref(false)
const snapDingtalkNoClick = ref(false)
const snapFeishuNoClick = ref(false)
const snapDouyinNoClick = ref(true)

// 所有吸附应用的 ref 映射（每个开关独立，不互斥）
const snapAppRefs: Record<string, { value: boolean }> = {
  snap_wechat: snapWechat,
  snap_wecom: snapWecom,
  snap_qq: snapQQ,
  snap_dingtalk: snapDingtalk,
  snap_feishu: snapFeishu,
  snap_douyin: snapDouyin,
}

// 点击偏移量 X ref 映射
const clickOffsetXRefs: Record<string, { value: string }> = {
  snap_wechat_click_offset_x: snapWechatClickOffsetX,
  snap_wecom_click_offset_x: snapWecomClickOffsetX,
  snap_qq_click_offset_x: snapQQClickOffsetX,
  snap_dingtalk_click_offset_x: snapDingtalkClickOffsetX,
  snap_feishu_click_offset_x: snapFeishuClickOffsetX,
  snap_douyin_click_offset_x: snapDouyinClickOffsetX,
}

// 点击偏移量 Y ref 映射
const clickOffsetYRefs: Record<string, { value: string }> = {
  snap_wechat_click_offset_y: snapWechatClickOffsetY,
  snap_wecom_click_offset_y: snapWecomClickOffsetY,
  snap_qq_click_offset_y: snapQQClickOffsetY,
  snap_dingtalk_click_offset_y: snapDingtalkClickOffsetY,
  snap_feishu_click_offset_y: snapFeishuClickOffsetY,
  snap_douyin_click_offset_y: snapDouyinClickOffsetY,
}

// 不点击模式 ref 映射
const noClickRefs: Record<string, { value: boolean }> = {
  snap_wechat_no_click: snapWechatNoClick,
  snap_wecom_no_click: snapWecomNoClick,
  snap_qq_no_click: snapQQNoClick,
  snap_dingtalk_no_click: snapDingtalkNoClick,
  snap_feishu_no_click: snapFeishuNoClick,
  snap_douyin_no_click: snapDouyinNoClick,
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
    noClick: snapWechatNoClick,
    hasNoClickOption: true,
    clickOffsetX: snapWechatClickOffsetX,
    clickOffsetY: snapWechatClickOffsetY,
    defaultY: DEFAULT_CLICK_OFFSET_Y.snap_wechat,
  },
  {
    key: 'snap_wecom',
    label: t('settings.snap.apps.wecom'),
    icon: WecomIcon,
    value: snapWecom,
    noClick: snapWecomNoClick,
    hasNoClickOption: true,
    clickOffsetX: snapWecomClickOffsetX,
    clickOffsetY: snapWecomClickOffsetY,
    defaultY: DEFAULT_CLICK_OFFSET_Y.snap_wecom,
  },
  {
    key: 'snap_qq',
    label: t('settings.snap.apps.qq'),
    icon: QQIcon,
    value: snapQQ,
    noClick: snapQQNoClick,
    hasNoClickOption: true,
    clickOffsetX: snapQQClickOffsetX,
    clickOffsetY: snapQQClickOffsetY,
    defaultY: DEFAULT_CLICK_OFFSET_Y.snap_qq,
  },
  {
    key: 'snap_dingtalk',
    label: t('settings.snap.apps.dingtalk'),
    icon: DingtalkIcon,
    value: snapDingtalk,
    noClick: snapDingtalkNoClick,
    hasNoClickOption: false,
    clickOffsetX: snapDingtalkClickOffsetX,
    clickOffsetY: snapDingtalkClickOffsetY,
    defaultY: DEFAULT_CLICK_OFFSET_Y.snap_dingtalk,
  },
  {
    key: 'snap_feishu',
    label: t('settings.snap.apps.feishu'),
    icon: FeishuIcon,
    value: snapFeishu,
    noClick: snapFeishuNoClick,
    hasNoClickOption: true,
    clickOffsetX: snapFeishuClickOffsetX,
    clickOffsetY: snapFeishuClickOffsetY,
    defaultY: DEFAULT_CLICK_OFFSET_Y.snap_feishu,
  },
  {
    key: 'snap_douyin',
    label: t('settings.snap.apps.douyin'),
    icon: DouyinIcon,
    value: snapDouyin,
    noClick: snapDouyinNoClick,
    hasNoClickOption: true,
    clickOffsetX: snapDouyinClickOffsetX,
    clickOffsetY: snapDouyinClickOffsetY,
    defaultY: DEFAULT_CLICK_OFFSET_Y.snap_douyin,
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
      // 处理不点击模式设置
      const noClickRef = noClickRefs[setting.key]
      if (noClickRef) {
        noClickRef.value = setting.value === 'true'
        return
      }
      // 处理点击偏移量 X 设置
      const offsetXRef = clickOffsetXRefs[setting.key]
      if (offsetXRef) {
        // X offset: empty means center
        offsetXRef.value = setting.value || ''
        return
      }
      // 处理点击偏移量 Y 设置
      const offsetYRef = clickOffsetYRefs[setting.key]
      if (offsetYRef) {
        // Y offset: use saved value or keep the default already set
        if (setting.value) {
          offsetYRef.value = setting.value
        }
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
    isLocalUpdate = true
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
    isLocalUpdate = true
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
      isLocalUpdate = true
      await SnapService.NotifySettingsChanged()
    } catch {
      sendKeyStrategy.value = prev
    }
  }
}

// 处理输入模式变化（点击模式/不点击模式）
const handleInputModeChange = async (key: string, refValue: { value: boolean }, mode: string) => {
  const prev = refValue.value
  const newValue = mode === 'no_click'
  refValue.value = newValue
  try {
    await updateSetting(key + '_no_click', String(newValue))
    // Notify other windows about the settings change (set flag to prevent self-refresh)
    isLocalUpdate = true
    await SnapService.NotifySettingsChanged()
  } catch {
    refValue.value = prev
  }
}

// 处理点击偏移量 X 变化（失去焦点时调用）
const handleClickOffsetXBlur = async (key: string, refValue: { value: string }) => {
  // Only allow numbers, empty means center
  const sanitized = refValue.value.replace(/[^0-9]/g, '')
  refValue.value = sanitized
  try {
    await updateSetting(key + '_click_offset_x', sanitized)
    // Notify other windows about the settings change
    isLocalUpdate = true
    await SnapService.NotifySettingsChanged()
  } catch (error) {
    console.error('Failed to save click offset X:', error)
  }
}

// 处理点击偏移量 Y 变化（失去焦点时调用）
const handleClickOffsetYBlur = async (key: string, refValue: { value: string }, defaultValue: string) => {
  // Only allow numbers
  const sanitized = refValue.value.replace(/[^0-9]/g, '')
  // If empty or zero, use default
  const finalValue = sanitized && sanitized !== '0' ? sanitized : defaultValue
  refValue.value = finalValue
  try {
    // Save empty string to database if using default (so backend uses its default too)
    const saveValue = finalValue === defaultValue ? '' : finalValue
    await updateSetting(key + '_click_offset_y', saveValue)
    // Notify other windows about the settings change
    isLocalUpdate = true
    await SnapService.NotifySettingsChanged()
  } catch (error) {
    console.error('Failed to save click offset Y:', error)
  }
}

// Event subscription for snap settings change (broadcast from backend)
let unsubscribeSnapSettingsChanged: (() => void) | null = null

// Flag to prevent self-triggered refresh
let isLocalUpdate = false

// 页面加载时获取设置
onMounted(() => {
  void loadSettings()

  // Listen for snap settings change event broadcast from backend (e.g., when winsnap window cancels snap)
  // Only refresh UI without calling syncSnapFromSettings to avoid event loop
  unsubscribeSnapSettingsChanged = Events.On('snap:settings-changed', () => {
    // Skip refresh if this was triggered by our own update
    if (isLocalUpdate) {
      isLocalUpdate = false
      return
    }
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
      <div v-for="(app, index) in snapApps" :key="app.key">
        <SettingsItem :bordered="index !== snapApps.length - 1 || app.value.value">
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
        <!-- 输入模式设置（仅当开关开启时显示） -->
        <div
          v-if="app.value.value"
          class="flex flex-col gap-3 px-4 py-3 bg-muted/30"
          :class="{ 'border-b border-border': index !== snapApps.length - 1 }"
        >
          <!-- 输入模式单选（部分应用不支持不点击模式） -->
          <RadioGroup
            v-if="app.hasNoClickOption"
            :model-value="app.noClick.value ? 'no_click' : 'click'"
            class="flex flex-col gap-2"
            @update:model-value="(mode: string) => handleInputModeChange(app.key, app.noClick, mode)"
          >
            <!-- 不点击模式 -->
            <div class="flex items-center gap-2">
              <RadioGroupItem :id="`${app.key}_no_click`" value="no_click" />
              <Label :for="`${app.key}_no_click`" class="text-xs text-muted-foreground cursor-pointer">
                {{ t('settings.snap.noClickMode') }}
              </Label>
            </div>
            <!-- 点击模式 -->
            <div class="flex items-center gap-2">
              <RadioGroupItem :id="`${app.key}_click`" value="click" />
              <Label :for="`${app.key}_click`" class="text-xs text-muted-foreground cursor-pointer">
                {{ t('settings.snap.clickMode') }}
              </Label>
            </div>
          </RadioGroup>
          <!-- 点击偏移量输入（不点击模式时隐藏；无不点击选项的应用始终显示） -->
          <div v-if="!app.hasNoClickOption || !app.noClick.value" class="flex items-center justify-between gap-4 pl-6">
            <div class="flex items-center gap-2">
              <span class="text-xs text-muted-foreground">{{ t('settings.snap.clickOffset.labelX') }}</span>
              <Input
                v-model="app.clickOffsetX.value"
                :placeholder="t('settings.snap.clickOffset.placeholderX')"
                class="w-16 h-7 text-xs text-center"
                @blur="handleClickOffsetXBlur(app.key, app.clickOffsetX)"
              />
            </div>
            <div class="flex items-center gap-2">
              <span class="text-xs text-muted-foreground">{{ t('settings.snap.clickOffset.labelY') }}</span>
              <Input
                v-model="app.clickOffsetY.value"
                class="w-16 h-7 text-xs text-center"
                @blur="handleClickOffsetYBlur(app.key, app.clickOffsetY, app.defaultY)"
              />
            </div>
          </div>
        </div>
      </div>
    </SettingsCard>
  </div>
</template>
