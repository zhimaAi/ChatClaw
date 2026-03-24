import test from 'node:test'
import assert from 'node:assert/strict'

import { buildScheduledTaskCopyName } from '../scheduledTaskText.ts'

test('buildScheduledTaskCopyName supports injected localized copy suffix', () => {
  const result = buildScheduledTaskCopyName('Daily Brief', ' Copy')
  assert.equal(result, 'Daily Brief Copy')
})
