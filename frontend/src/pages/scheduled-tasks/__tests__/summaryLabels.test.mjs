import test from 'node:test'
import assert from 'node:assert/strict'

import enUS from '../../../locales/en-US.ts'
import zhCN from '../../../locales/zh-CN.ts'
import { buildScheduledTaskSummaryLabels } from '../summaryLabels.ts'

function createLocaleTranslator(locale) {
  return function translate(key) {
    return key.split('.').reduce((value, segment) => value?.[segment], locale)
  }
}

test('scheduled task summary labels include paused and ended in English', () => {
  const labels = buildScheduledTaskSummaryLabels(createLocaleTranslator(enUS))
  assert.equal(labels.paused, 'Paused/Ended')
})

test('scheduled task summary labels include paused and ended in Simplified Chinese', () => {
  const labels = buildScheduledTaskSummaryLabels(createLocaleTranslator(zhCN))
  assert.equal(labels.paused, '已暂停/已结束')
})
