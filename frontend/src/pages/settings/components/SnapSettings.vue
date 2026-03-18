<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AcceptableValue } from 'reka-ui'
import { AppWindow, Plus, Search, Trash2 } from 'lucide-vue-next'
import { Events } from '@wailsio/runtime'
import { toast } from '@/components/ui/toast'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'

import WechatIcon from '@/assets/icons/snap/wechat.svg'
import WecomIcon from '@/assets/icons/snap/wecom.svg'
import QQIcon from '@/assets/icons/snap/qq.svg'
import DingtalkIcon from '@/assets/icons/snap/dingtalk.svg'
import FeishuIcon from '@/assets/icons/snap/feishu.svg'
import DouyinIcon from '@/assets/icons/snap/douyin.svg'

import { SettingsService, Category } from '@bindings/chatclaw/internal/services/settings'
import { SnapService } from '@bindings/chatclaw/internal/services/windows'

interface CustomSnapApp {
  id: string
  name: string
  processName: string
  icon?: string
  enabled: boolean
  noClick: boolean
  clickOffsetX: string
  clickOffsetY: string
  defaultY: string
}

interface SnapAppCandidate {
  name: string
  processName: string
  icon?: string
}

interface SnapServiceWithCustomApi {
  SyncFromSettings: () => Promise<unknown>
  NotifySettingsChanged: () => Promise<void>
  ListAvailableApps: () => Promise<SnapAppCandidate[]>
}

const CUSTOM_APPS_KEY = 'snap_custom_apps'
const CUSTOM_APP_KEY_PREFIX = 'snap_custom_'
const DEFAULT_CUSTOM_CLICK_OFFSET_Y = '120'

const { t } = useI18n()
const snapService = SnapService as unknown as SnapServiceWithCustomApi

const showAiSendButton = ref(true)
const sendKeyStrategy = ref('enter')
const showAiEditButton = ref(true)

const snapWechat = ref(false)
const snapWecom = ref(false)
const snapQQ = ref(false)
const snapDingtalk = ref(false)
const snapFeishu = ref(false)
const snapDouyin = ref(false)

const DEFAULT_CLICK_OFFSET_Y: Record<string, string> = {
  snap_wechat: '120',
  snap_wecom: '120',
  snap_qq: '120',
  snap_dingtalk: '120',
  snap_feishu: '50',
  snap_douyin: '50',
}

const snapWechatClickOffsetX = ref('')
const snapWecomClickOffsetX = ref('')
const snapQQClickOffsetX = ref('')
const snapDingtalkClickOffsetX = ref('')
const snapFeishuClickOffsetX = ref('')
const snapDouyinClickOffsetX = ref('')

const snapWechatClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_wechat)
const snapWecomClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_wecom)
const snapQQClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_qq)
const snapDingtalkClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_dingtalk)
const snapFeishuClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_feishu)
const snapDouyinClickOffsetY = ref(DEFAULT_CLICK_OFFSET_Y.snap_douyin)

const snapWechatNoClick = ref(false)
const snapWecomNoClick = ref(false)
const snapQQNoClick = ref(false)
const snapDingtalkNoClick = ref(false)
const snapFeishuNoClick = ref(false)
const snapDouyinNoClick = ref(true)

const customSnapApps = ref<CustomSnapApp[]>([])
const availableApps = ref<SnapAppCandidate[]>([])
const loadingAvailableApps = ref(false)
const customPickerOpen = ref(false)
const customAppSearch = ref('')
const selectedCandidateProcess = ref('')
const deleteDialogOpen = ref(false)
const deletingCustomApp = ref<CustomSnapApp | null>(null)

const snapAppRefs: Record<string, { value: boolean }> = {
  snap_wechat: snapWechat,
  snap_wecom: snapWecom,
  snap_qq: snapQQ,
  snap_dingtalk: snapDingtalk,
  snap_feishu: snapFeishu,
  snap_douyin: snapDouyin,
}

const clickOffsetXRefs: Record<string, { value: string }> = {
  snap_wechat_click_offset_x: snapWechatClickOffsetX,
  snap_wecom_click_offset_x: snapWecomClickOffsetX,
  snap_qq_click_offset_x: snapQQClickOffsetX,
  snap_dingtalk_click_offset_x: snapDingtalkClickOffsetX,
  snap_feishu_click_offset_x: snapFeishuClickOffsetX,
  snap_douyin_click_offset_x: snapDouyinClickOffsetX,
}

const clickOffsetYRefs: Record<string, { value: string }> = {
  snap_wechat_click_offset_y: snapWechatClickOffsetY,
  snap_wecom_click_offset_y: snapWecomClickOffsetY,
  snap_qq_click_offset_y: snapQQClickOffsetY,
  snap_dingtalk_click_offset_y: snapDingtalkClickOffsetY,
  snap_feishu_click_offset_y: snapFeishuClickOffsetY,
  snap_douyin_click_offset_y: snapDouyinClickOffsetY,
}

const noClickRefs: Record<string, { value: boolean }> = {
  snap_wechat_no_click: snapWechatNoClick,
  snap_wecom_no_click: snapWecomNoClick,
  snap_qq_no_click: snapQQNoClick,
  snap_dingtalk_no_click: snapDingtalkNoClick,
  snap_feishu_no_click: snapFeishuNoClick,
  snap_douyin_no_click: snapDouyinNoClick,
}

const sendKeyOptions = [
  { value: 'enter', label: 'settings.snap.sendKeyOptions.enter' },
  { value: 'ctrl_enter', label: 'settings.snap.sendKeyOptions.ctrlEnter' },
]

const currentSendKeyLabel = computed(() => {
  const option = sendKeyOptions.find((opt) => opt.value === sendKeyStrategy.value)
  return option ? t(option.label) : ''
})

const builtInIconMap: Record<string, unknown> = {
  wechat: WechatIcon,
  wecom: WecomIcon,
  qq: QQIcon,
  dingtalk: DingtalkIcon,
  feishu: FeishuIcon,
  douyin: DouyinIcon,
}

const builtInProcessToAppKeyMap: Record<string, string> = {
  // Windows
  'weixin.exe': 'snap_wechat',
  'wechat.exe': 'snap_wechat',
  'wechatapp.exe': 'snap_wechat',
  'wechatappex.exe': 'snap_wechat',
  'wxwork.exe': 'snap_wecom',
  'qq.exe': 'snap_qq',
  'qqnt.exe': 'snap_qq',
  'dingtalk.exe': 'snap_dingtalk',
  'feishu.exe': 'snap_feishu',
  'lark.exe': 'snap_feishu',
  'douyin.exe': 'snap_douyin',
  // macOS
  微信: 'snap_wechat',
  weixin: 'snap_wechat',
  wechat: 'snap_wechat',
  'com.tencent.xinwechat': 'snap_wechat',
  企业微信: 'snap_wecom',
  wecom: 'snap_wecom',
  wework: 'snap_wecom',
  wxwork: 'snap_wecom',
  qiyeweixin: 'snap_wecom',
  'com.tencent.weworkmac': 'snap_wecom',
  qq: 'snap_qq',
  'com.tencent.qq': 'snap_qq',
  钉钉: 'snap_dingtalk',
  dingtalk: 'snap_dingtalk',
  'com.alibaba.dingtalkmac': 'snap_dingtalk',
  飞书: 'snap_feishu',
  feishu: 'snap_feishu',
  lark: 'snap_feishu',
  'com.bytedance.feishu': 'snap_feishu',
  'com.bytedance.lark': 'snap_feishu',
  'com.electron.lark': 'snap_feishu',
  抖音: 'snap_douyin',
  douyin: 'snap_douyin',
}

const builtInAppLabelByKey: Record<string, string> = {
  snap_wechat: 'settings.snap.apps.wechat',
  snap_wecom: 'settings.snap.apps.wecom',
  snap_qq: 'settings.snap.apps.qq',
  snap_dingtalk: 'settings.snap.apps.dingtalk',
  snap_feishu: 'settings.snap.apps.feishu',
  snap_douyin: 'settings.snap.apps.douyin',
}

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

const customSnapAppsForDisplay = computed(() => [...customSnapApps.value].reverse())
const mergedSnapAppCount = computed(
  () => customSnapAppsForDisplay.value.length + snapApps.value.length
)

const boolSettingsMap: Record<string, { value: boolean }> = {
  show_ai_send_button: showAiSendButton,
  show_ai_edit_button: showAiEditButton,
  ...snapAppRefs,
}

const filteredAvailableApps = computed(() => {
  const keyword = customAppSearch.value.trim().toLowerCase()
  if (!keyword) {
    return availableApps.value
  }
  return availableApps.value.filter((app) => {
    const name = app.name?.toLowerCase() ?? ''
    const processName = app.processName?.toLowerCase() ?? ''
    return name.includes(keyword) || processName.includes(keyword)
  })
})

const selectedCandidate = computed(() =>
  availableApps.value.find((app) => app.processName === selectedCandidateProcess.value)
)

const getCustomAppSettingKey = (id: string) => `${CUSTOM_APP_KEY_PREFIX}${id}`

const normalizeProcess = (value: string) => value.trim().toLowerCase()

const parseCustomApps = (rawValue: string): CustomSnapApp[] => {
  if (!rawValue) {
    return []
  }
  try {
    const parsed = JSON.parse(rawValue) as Array<{
      id?: string
      name?: string
      processName?: string
      icon?: string
    }>
    const seen = new Set<string>()
    const out: CustomSnapApp[] = []
    parsed.forEach((item) => {
      const id = (item.id ?? '').trim()
      const processName = (item.processName ?? '').trim()
      if (!id || !processName) {
        return
      }
      // Ignore any custom apps that duplicate built-in targets.
      // This can happen with older configs created before the UI started
      // blocking built-in apps from being added as "custom".
      const normalizedProcess = normalizeProcess(processName)
      if (builtInProcessToAppKeyMap[normalizedProcess]) {
        return
      }
      const uniqueKey = `${id}#${normalizeProcess(processName)}`
      if (seen.has(uniqueKey)) {
        return
      }
      seen.add(uniqueKey)
      out.push({
        id,
        name: (item.name ?? processName).trim() || processName,
        processName,
        icon: item.icon?.trim() || 'app',
        enabled: false,
        noClick: false,
        clickOffsetX: '',
        clickOffsetY: DEFAULT_CUSTOM_CLICK_OFFSET_Y,
        defaultY: DEFAULT_CUSTOM_CLICK_OFFSET_Y,
      })
    })
    return out
  } catch {
    return []
  }
}

const saveCustomAppsConfig = async (apps: CustomSnapApp[]) => {
  const payload = apps.map((app) => ({
    id: app.id,
    name: app.name,
    processName: app.processName,
    icon: app.icon ?? 'app',
  }))
  await updateSetting(CUSTOM_APPS_KEY, JSON.stringify(payload))
}

const getCandidateIcon = (icon?: string) => {
  return icon && builtInIconMap[icon] ? builtInIconMap[icon] : AppWindow
}

const isDataUrlIcon = (icon?: string) => {
  const value = icon?.trim().toLowerCase()
  return !!value && value.startsWith('data:image/')
}

const syncSnapFromSettings = async () => {
  try {
    await snapService.SyncFromSettings()
  } catch (error) {
    console.error('Failed to sync snap service from settings:', error)
  }
}

const refreshSettingsUI = async () => {
  try {
    const settings = await SettingsService.List(Category.CategorySnap)
    const settingMap = new Map(settings.map((setting) => [setting.key, setting.value]))

    settings.forEach((setting) => {
      const boolRef = boolSettingsMap[setting.key]
      if (boolRef) {
        boolRef.value = setting.value === 'true'
        return
      }
      const noClickRef = noClickRefs[setting.key]
      if (noClickRef) {
        noClickRef.value = setting.value === 'true'
        return
      }
      const offsetXRef = clickOffsetXRefs[setting.key]
      if (offsetXRef) {
        offsetXRef.value = setting.value || ''
        return
      }
      const offsetYRef = clickOffsetYRefs[setting.key]
      if (offsetYRef) {
        if (setting.value) {
          offsetYRef.value = setting.value
        }
        return
      }
      if (setting.key === 'send_key_strategy') {
        sendKeyStrategy.value = setting.value
      }
    })

    const customAppsConfig = parseCustomApps(settingMap.get(CUSTOM_APPS_KEY) ?? '')
    customSnapApps.value = customAppsConfig.map((app) => {
      const key = getCustomAppSettingKey(app.id)
      const clickOffsetY = settingMap.get(`${key}_click_offset_y`) ?? ''
      return {
        ...app,
        enabled: settingMap.get(key) === 'true',
        noClick: settingMap.get(`${key}_no_click`) === 'true',
        clickOffsetX: settingMap.get(`${key}_click_offset_x`) ?? '',
        clickOffsetY: clickOffsetY || DEFAULT_CUSTOM_CLICK_OFFSET_Y,
      }
    })
  } catch (error) {
    console.error('Failed to refresh snap settings UI:', error)
  }
}

const loadSettings = async () => {
  await refreshSettingsUI()
  await syncSnapFromSettings()
}

const updateSetting = async (key: string, value: string) => {
  try {
    await SettingsService.SetValue(key, value)
  } catch (error) {
    console.error(`Failed to update setting ${key}:`, error)
    throw error
  }
}

const handleAiSendButtonChange = async (val: boolean) => {
  const prev = showAiSendButton.value
  showAiSendButton.value = val
  try {
    await updateSetting('show_ai_send_button', String(val))
    isLocalUpdate = true
    await snapService.NotifySettingsChanged()
  } catch {
    showAiSendButton.value = prev
  }
}

const handleAiEditButtonChange = async (val: boolean) => {
  const prev = showAiEditButton.value
  showAiEditButton.value = val
  try {
    await updateSetting('show_ai_edit_button', String(val))
    isLocalUpdate = true
    await snapService.NotifySettingsChanged()
  } catch {
    showAiEditButton.value = prev
  }
}

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

const handleCustomSnapAppChange = async (app: CustomSnapApp, val: boolean) => {
  const prev = app.enabled
  app.enabled = val
  try {
    await updateSetting(getCustomAppSettingKey(app.id), String(val))
  } catch {
    app.enabled = prev
    return
  }
  await syncSnapFromSettings()
}

const handleCustomInputModeChange = async (app: CustomSnapApp, mode: string) => {
  const prev = app.noClick
  const newValue = mode === 'no_click'
  app.noClick = newValue
  const key = getCustomAppSettingKey(app.id)
  try {
    await updateSetting(key + '_no_click', String(newValue))
    isLocalUpdate = true
    await snapService.NotifySettingsChanged()
  } catch {
    app.noClick = prev
  }
}

const handleCustomClickOffsetXBlur = async (app: CustomSnapApp) => {
  const sanitized = app.clickOffsetX.replace(/[^0-9]/g, '')
  app.clickOffsetX = sanitized
  const key = getCustomAppSettingKey(app.id)
  try {
    await updateSetting(key + '_click_offset_x', sanitized)
    isLocalUpdate = true
    await snapService.NotifySettingsChanged()
  } catch (error) {
    console.error('Failed to save custom click offset X:', error)
  }
}

const handleCustomClickOffsetYBlur = async (app: CustomSnapApp) => {
  const sanitized = app.clickOffsetY.replace(/[^0-9]/g, '')
  const finalValue = sanitized && sanitized !== '0' ? sanitized : app.defaultY
  app.clickOffsetY = finalValue
  const key = getCustomAppSettingKey(app.id)
  try {
    const saveValue = finalValue === app.defaultY ? '' : finalValue
    await updateSetting(key + '_click_offset_y', saveValue)
    isLocalUpdate = true
    await snapService.NotifySettingsChanged()
  } catch (error) {
    console.error('Failed to save custom click offset Y:', error)
  }
}

const handleSendKeyChange = async (value: AcceptableValue) => {
  if (typeof value === 'string') {
    const prev = sendKeyStrategy.value
    sendKeyStrategy.value = value
    try {
      await updateSetting('send_key_strategy', value)
      isLocalUpdate = true
      await snapService.NotifySettingsChanged()
    } catch {
      sendKeyStrategy.value = prev
    }
  }
}

const handleInputModeChange = async (key: string, refValue: { value: boolean }, mode: string) => {
  const prev = refValue.value
  const newValue = mode === 'no_click'
  refValue.value = newValue
  try {
    await updateSetting(key + '_no_click', String(newValue))
    isLocalUpdate = true
    await snapService.NotifySettingsChanged()
  } catch {
    refValue.value = prev
  }
}

const handleClickOffsetXBlur = async (key: string, refValue: { value: string }) => {
  const sanitized = refValue.value.replace(/[^0-9]/g, '')
  refValue.value = sanitized
  try {
    await updateSetting(key + '_click_offset_x', sanitized)
    isLocalUpdate = true
    await snapService.NotifySettingsChanged()
  } catch (error) {
    console.error('Failed to save click offset X:', error)
  }
}

const handleClickOffsetYBlur = async (
  key: string,
  refValue: { value: string },
  defaultValue: string
) => {
  const sanitized = refValue.value.replace(/[^0-9]/g, '')
  const finalValue = sanitized && sanitized !== '0' ? sanitized : defaultValue
  refValue.value = finalValue
  try {
    const saveValue = finalValue === defaultValue ? '' : finalValue
    await updateSetting(key + '_click_offset_y', saveValue)
    isLocalUpdate = true
    await snapService.NotifySettingsChanged()
  } catch (error) {
    console.error('Failed to save click offset Y:', error)
  }
}

const loadAvailableApps = async () => {
  loadingAvailableApps.value = true
  try {
    const apps = await snapService.ListAvailableApps()
    const selectedProcesses = new Set(
      customSnapApps.value.map((item) => normalizeProcess(item.processName))
    )
    availableApps.value = apps.filter((app) => {
      const processName = app.processName?.trim()
      const name = app.name?.trim()
      if (!processName || !name) {
        return false
      }
      return !selectedProcesses.has(normalizeProcess(processName))
    })
  } catch (error) {
    console.error('Failed to list running apps:', error)
    availableApps.value = []
  } finally {
    loadingAvailableApps.value = false
  }
}

const openCustomAppPicker = async () => {
  customPickerOpen.value = true
  customAppSearch.value = ''
  selectedCandidateProcess.value = ''
  await loadAvailableApps()
}

const handleConfirmAddCustomApp = async () => {
  const selected = selectedCandidate.value
  if (!selected) {
    return
  }
  const normalizedProcess = normalizeProcess(selected.processName)
  const matchedBuiltInKey = builtInProcessToAppKeyMap[normalizedProcess]
  if (matchedBuiltInKey) {
    const appLabelKey = builtInAppLabelByKey[matchedBuiltInKey]
    const appLabel = appLabelKey ? t(appLabelKey) : selected.name
    toast.error(t('settings.snap.customAppExistsBuiltIn', { app: appLabel }))
    return
  }
  const existingCustom = customSnapApps.value.find(
    (app) => normalizeProcess(app.processName) === normalizedProcess
  )
  if (existingCustom) {
    toast.error(t('settings.snap.customAppExistsCustom', { name: existingCustom.name }))
    return
  }

  const newApp: CustomSnapApp = {
    id: `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`,
    name: selected.name.trim(),
    processName: selected.processName.trim(),
    icon: selected.icon?.trim() || 'app',
    enabled: true,
    noClick: false,
    clickOffsetX: '',
    clickOffsetY: DEFAULT_CUSTOM_CLICK_OFFSET_Y,
    defaultY: DEFAULT_CUSTOM_CLICK_OFFSET_Y,
  }

  const nextApps = [...customSnapApps.value, newApp]
  try {
    await saveCustomAppsConfig(nextApps)
    await updateSetting(getCustomAppSettingKey(newApp.id), 'true')
    customSnapApps.value = nextApps
    customPickerOpen.value = false
    await syncSnapFromSettings()
  } catch (error) {
    console.error('Failed to add custom snap app:', error)
  }
}

const requestDeleteCustomApp = (app: CustomSnapApp) => {
  deletingCustomApp.value = app
  deleteDialogOpen.value = true
}

const handleConfirmDeleteCustomApp = async () => {
  const target = deletingCustomApp.value
  if (!target) {
    return
  }
  const nextApps = customSnapApps.value.filter((app) => app.id !== target.id)
  try {
    await saveCustomAppsConfig(nextApps)
    await updateSetting(getCustomAppSettingKey(target.id), 'false')
    customSnapApps.value = nextApps
    deleteDialogOpen.value = false
    deletingCustomApp.value = null
    await syncSnapFromSettings()
  } catch (error) {
    console.error('Failed to delete custom snap app:', error)
  }
}

let unsubscribeSnapSettingsChanged: (() => void) | null = null
let isLocalUpdate = false

onMounted(() => {
  void loadSettings()
  unsubscribeSnapSettingsChanged = Events.On('snap:settings-changed', () => {
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
    <SettingsCard :title="t('settings.snap.title')">
      <SettingsItem :label="t('settings.snap.showAiSendButton')">
        <Switch :model-value="showAiSendButton" @update:model-value="handleAiSendButtonChange" />
      </SettingsItem>

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

      <SettingsItem :label="t('settings.snap.showAiEditButton')" :bordered="false">
        <Switch :model-value="showAiEditButton" @update:model-value="handleAiEditButtonChange" />
      </SettingsItem>
    </SettingsCard>

    <SettingsCard :title="t('settings.snap.appsTitle')">
      <template #header-right>
        <Button variant="outline" size="sm" class="h-7 gap-1" @click="openCustomAppPicker">
          <Plus class="size-3.5 shrink-0" />
          {{ t('settings.snap.addCustomApp') }}
        </Button>
      </template>

      <div v-for="(customApp, customIndex) in customSnapAppsForDisplay" :key="customApp.id">
        <SettingsItem :bordered="customIndex !== mergedSnapAppCount - 1 || customApp.enabled">
          <template #default>
            <Switch
              :model-value="customApp.enabled"
              @update:model-value="(val: boolean) => handleCustomSnapAppChange(customApp, val)"
            />
          </template>
          <template #label>
            <div class="flex items-center gap-2 min-w-0">
              <img
                v-if="isDataUrlIcon(customApp.icon)"
                :src="customApp.icon"
                alt=""
                class="size-5 shrink-0 rounded-sm object-contain"
              />
              <component
                :is="getCandidateIcon(customApp.icon)"
                v-else
                class="size-5 text-muted-foreground shrink-0"
              />
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-foreground truncate">{{
                    customApp.name
                  }}</span>
                  <button
                    type="button"
                    class="inline-flex items-center justify-center text-muted-foreground hover:text-destructive transition-colors"
                    @click="requestDeleteCustomApp(customApp)"
                  >
                    <Trash2 class="size-3.5" />
                  </button>
                </div>
                <div class="text-xs text-muted-foreground truncate">
                  {{ customApp.processName }}
                </div>
              </div>
            </div>
          </template>
        </SettingsItem>
        <div
          v-if="customApp.enabled"
          class="flex flex-col gap-3 px-4 py-3 bg-muted/30"
          :class="{ 'border-b border-border': customIndex !== mergedSnapAppCount - 1 }"
        >
          <RadioGroup
            :model-value="customApp.noClick ? 'no_click' : 'click'"
            class="flex flex-col gap-2"
            @update:model-value="(mode) => handleCustomInputModeChange(customApp, String(mode))"
          >
            <div class="flex items-center gap-2">
              <RadioGroupItem :id="`${customApp.id}_no_click`" value="no_click" />
              <Label
                :for="`${customApp.id}_no_click`"
                class="text-xs text-muted-foreground cursor-pointer"
              >
                {{ t('settings.snap.noClickMode') }}
              </Label>
            </div>
            <div class="flex items-center gap-2">
              <RadioGroupItem :id="`${customApp.id}_click`" value="click" />
              <Label
                :for="`${customApp.id}_click`"
                class="text-xs text-muted-foreground cursor-pointer"
              >
                {{ t('settings.snap.clickMode') }}
              </Label>
            </div>
          </RadioGroup>
          <div v-if="!customApp.noClick" class="flex items-center justify-between gap-4 pl-6">
            <div class="flex items-center gap-2">
              <span class="text-xs text-muted-foreground">{{
                t('settings.snap.clickOffset.labelX')
              }}</span>
              <Input
                v-model="customApp.clickOffsetX"
                :placeholder="t('settings.snap.clickOffset.placeholderX')"
                class="w-16 h-7 text-xs text-center"
                @blur="handleCustomClickOffsetXBlur(customApp)"
              />
            </div>
            <div class="flex items-center gap-2">
              <span class="text-xs text-muted-foreground">{{
                t('settings.snap.clickOffset.labelY')
              }}</span>
              <Input
                v-model="customApp.clickOffsetY"
                class="w-16 h-7 text-xs text-center"
                @blur="handleCustomClickOffsetYBlur(customApp)"
              />
            </div>
          </div>
        </div>
      </div>

      <div v-for="(app, index) in snapApps" :key="app.key">
        <SettingsItem
          :bordered="
            customSnapAppsForDisplay.length + index !== mergedSnapAppCount - 1 || app.value.value
          "
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
        <div
          v-if="app.value.value"
          class="flex flex-col gap-3 px-4 py-3 bg-muted/30"
          :class="{
            'border-b border-border':
              customSnapAppsForDisplay.length + index !== mergedSnapAppCount - 1,
          }"
        >
          <RadioGroup
            v-if="app.hasNoClickOption"
            :model-value="app.noClick.value ? 'no_click' : 'click'"
            class="flex flex-col gap-2"
            @update:model-value="
              (mode) => handleInputModeChange(app.key, app.noClick, String(mode))
            "
          >
            <div class="flex items-center gap-2">
              <RadioGroupItem :id="`${app.key}_no_click`" value="no_click" />
              <Label
                :for="`${app.key}_no_click`"
                class="text-xs text-muted-foreground cursor-pointer"
              >
                {{ t('settings.snap.noClickMode') }}
              </Label>
            </div>
            <div class="flex items-center gap-2">
              <RadioGroupItem :id="`${app.key}_click`" value="click" />
              <Label :for="`${app.key}_click`" class="text-xs text-muted-foreground cursor-pointer">
                {{ t('settings.snap.clickMode') }}
              </Label>
            </div>
          </RadioGroup>
          <div
            v-if="!app.hasNoClickOption || !app.noClick.value"
            class="flex items-center justify-between gap-4 pl-6"
          >
            <div class="flex items-center gap-2">
              <span class="text-xs text-muted-foreground">{{
                t('settings.snap.clickOffset.labelX')
              }}</span>
              <Input
                v-model="app.clickOffsetX.value"
                :placeholder="t('settings.snap.clickOffset.placeholderX')"
                class="w-16 h-7 text-xs text-center"
                @blur="handleClickOffsetXBlur(app.key, app.clickOffsetX)"
              />
            </div>
            <div class="flex items-center gap-2">
              <span class="text-xs text-muted-foreground">{{
                t('settings.snap.clickOffset.labelY')
              }}</span>
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

  <Dialog v-model:open="customPickerOpen">
    <DialogContent class="sm:max-w-[560px]">
      <DialogHeader>
        <DialogTitle>{{ t('settings.snap.customPickerTitle') }}</DialogTitle>
        <DialogDescription>{{ t('settings.snap.customPickerDesc') }}</DialogDescription>
      </DialogHeader>

      <div class="space-y-3">
        <div class="relative">
          <Search class="size-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
          <Input
            v-model="customAppSearch"
            :placeholder="t('settings.snap.customSearchPlaceholder')"
            class="pl-9"
          />
        </div>

        <div class="max-h-80 overflow-y-auto rounded-md border border-border">
          <div
            v-if="loadingAvailableApps"
            class="px-3 py-8 text-center text-xs text-muted-foreground"
          >
            {{ t('settings.snap.customAppsLoading') }}
          </div>
          <div
            v-else-if="filteredAvailableApps.length === 0"
            class="px-3 py-8 text-center text-xs text-muted-foreground"
          >
            {{ t('settings.snap.customAppsNoResult') }}
          </div>
          <button
            v-for="app in filteredAvailableApps"
            :key="app.processName"
            type="button"
            class="w-full flex items-center gap-3 px-3 py-2.5 text-left border-b last:border-b-0 border-border transition-colors hover:bg-muted/40"
            :class="selectedCandidateProcess === app.processName && 'bg-muted/60'"
            @click="selectedCandidateProcess = app.processName"
          >
            <img
              v-if="isDataUrlIcon(app.icon)"
              :src="app.icon"
              alt=""
              class="size-5 shrink-0 rounded-sm object-contain"
            />
            <component
              :is="getCandidateIcon(app.icon)"
              v-else
              class="size-5 text-muted-foreground shrink-0"
            />
            <div class="min-w-0">
              <div class="text-sm text-foreground truncate">{{ app.name }}</div>
              <div class="text-xs text-muted-foreground truncate">{{ app.processName }}</div>
            </div>
          </button>
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" @click="customPickerOpen = false">
          {{ t('settings.snap.cancel') }}
        </Button>
        <Button :disabled="!selectedCandidate" @click="handleConfirmAddCustomApp">
          {{ t('settings.snap.confirmAddCustomApp') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <AlertDialog v-model:open="deleteDialogOpen">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ t('settings.snap.deleteCustomConfirmTitle') }}</AlertDialogTitle>
        <AlertDialogDescription>
          {{
            t('settings.snap.deleteCustomConfirmDesc', {
              name: deletingCustomApp?.name ?? '',
            })
          }}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>{{ t('settings.snap.cancel') }}</AlertDialogCancel>
        <AlertDialogAction @click.prevent="handleConfirmDeleteCustomApp">
          {{ t('settings.snap.confirmDeleteCustomApp') }}
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>
