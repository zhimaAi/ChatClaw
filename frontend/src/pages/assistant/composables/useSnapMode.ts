import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@/components/ui/toast'
import { SnapService } from '@bindings/willchat/internal/services/windows'
import { SettingsService, Category } from '@bindings/willchat/internal/services/settings'
import { Clipboard } from '@wailsio/runtime'

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

export function useSnapMode() {
  const { t } = useI18n()

  const hasAttachedTarget = ref(false)
  const showAiSendButton = ref(true)
  const showAiEditButton = ref(true)

  const checkSnapStatus = async () => {
    try {
      const status = await SnapService.GetStatus()
      hasAttachedTarget.value = status.state === 'attached' && !!status.targetProcess
    } catch (error) {
      console.error('Failed to check snap status:', error)
      hasAttachedTarget.value = false
    }
  }

  const loadSnapSettings = async () => {
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
      console.error('Failed to load snap settings:', error)
    }
  }

  const cancelSnap = async () => {
    try {
      const status = await SnapService.GetStatus()
      if (status.state === 'attached' && status.targetProcess) {
        const settingsKey = processToSettingsKey[status.targetProcess]
        if (settingsKey) {
          await SettingsService.SetValue(settingsKey, 'false')
        }
      }
      // Detach but keep window visible at standalone position (right side of screen)
      await SnapService.DetachToStandalone()
      hasAttachedTarget.value = false
    } catch (error) {
      console.error('Failed to cancel snap:', error)
    }
  }

  const findAndAttach = async () => {
    try {
      const key = await SnapService.FindSnapTarget()
      if (!key) {
        toast.error(t('winsnap.toast.noSnapTarget'))
        return
      }
      await SettingsService.SetValue(key, 'true')
      await SnapService.SyncFromSettings()
    } catch (error) {
      console.error('Failed to find and attach:', error)
      toast.error(t('winsnap.toast.attachFailed'))
    }
  }

  // All snap settings keys
  const allSnapKeys = [
    'snap_wechat', 'snap_wecom', 'snap_qq',
    'snap_dingtalk', 'snap_feishu', 'snap_douyin',
  ]

  const closeSnapWindow = async () => {
    try {
      // 1. Disable all snap toggles in settings
      const status = await SnapService.GetStatus()
      const keysToDisable = status.enabledKeys?.length
        ? status.enabledKeys
        : allSnapKeys
      await Promise.all(
        keysToDisable.map((key) => SettingsService.SetValue(key, 'false'))
      )

      // 2. Stop snap service and hide window
      await SnapService.CloseSnapWindow()
      hasAttachedTarget.value = false
    } catch (error) {
      console.error('Failed to close snap window:', error)
    }
  }

  const handleSendAndTrigger = async (content: string) => {
    if (!content) return
    try {
      await SnapService.SendTextToTarget(content, true)
      toast.success(t('winsnap.toast.sent'))
    } catch (error) {
      console.error('Failed to send and trigger:', error)
      await checkSnapStatus()
      toast.error(
        hasAttachedTarget.value ? t('winsnap.toast.sendFailed') : t('winsnap.toast.noTarget')
      )
    }
  }

  const handleSendToEdit = async (content: string) => {
    if (!content) return
    try {
      await SnapService.PasteTextToTarget(content)
      toast.success(t('winsnap.toast.pasted'))
    } catch (error) {
      console.error('Failed to paste to edit:', error)
      await checkSnapStatus()
      toast.error(
        hasAttachedTarget.value ? t('winsnap.toast.pasteFailed') : t('winsnap.toast.noTarget')
      )
    }
  }

  const handleCopyToClipboard = async (content: string) => {
    if (!content) return
    try {
      await Clipboard.SetText(content)
      toast.success(t('winsnap.toast.copied'))
    } catch (error) {
      console.error('Failed to copy to clipboard:', error)
    }
  }

  return {
    hasAttachedTarget,
    showAiSendButton,
    showAiEditButton,
    checkSnapStatus,
    loadSnapSettings,
    cancelSnap,
    findAndAttach,
    closeSnapWindow,
    handleSendAndTrigger,
    handleSendToEdit,
    handleCopyToClipboard,
  }
}
