import zhCN from './zh-CN'
import enUS from './en-US'
import arSA from './ar-SA'
import bnBD from './bn-BD'
import deDE from './de-DE'
import esES from './es-ES'
import frFR from './fr-FR'
import hiIN from './hi-IN'
import itIT from './it-IT'
import jaJP from './ja-JP'
import koKR from './ko-KR'
import ptBR from './pt-BR'
import slSI from './sl-SI'
import trTR from './tr-TR'
import viVN from './vi-VN'
import zhTW from './zh-TW'

export const messages = {
  'zh-CN': zhCN,
  'en-US': enUS,
  'ar-SA': arSA,
  'bn-BD': bnBD,
  'de-DE': deDE,
  'es-ES': esES,
  'fr-FR': frFR,
  'hi-IN': hiIN,
  'it-IT': itIT,
  'ja-JP': jaJP,
  'ko-KR': koKR,
  'pt-BR': ptBR,
  'sl-SI': slSI,
  'tr-TR': trTR,
  'vi-VN': viVN,
  'zh-TW': zhTW,
}

export type Locale = keyof typeof messages
