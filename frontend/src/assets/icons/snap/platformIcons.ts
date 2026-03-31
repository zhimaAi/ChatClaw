/**
 * Platform icon URL map for Snap/channel UIs. Use ?url to get asset URL for <img src>.
 */
import dingtalkIcon from './dingtalk.svg?url'
import feishuIcon from './feishu.svg?url'
import wecomIcon from './wecom.svg?url'
import wechatIcon from './wechat.svg?url'
import qqIcon from './qq.svg?url'
import twitterIcon from './twitter.svg?url'

export const platformIconMap: Record<string, string> = {
  dingtalk: dingtalkIcon,
  feishu: feishuIcon,
  wecom: wecomIcon,
  wechat: wechatIcon,
  qq: qqIcon,
  twitter: twitterIcon,
}
