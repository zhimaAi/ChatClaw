import test from 'node:test'
import assert from 'node:assert/strict'

import { buildRecoveredStreamingState } from '../chatStreamRecovery.ts'

test('buildRecoveredStreamingState bootstraps a streaming state from chunk payload fields', () => {
  const state = buildRecoveredStreamingState({
    message_id: 88,
    request_id: 'req-88',
  })

  assert.deepEqual(state, {
    messageId: 88,
    requestId: 'req-88',
    content: '',
    thinkingContent: '',
    toolCalls: [],
    segments: [],
    status: 'streaming',
  })
})

test('buildRecoveredStreamingState ignores invalid payloads', () => {
  assert.equal(buildRecoveredStreamingState({ message_id: 0, request_id: 'req-1' }), null)
  assert.equal(buildRecoveredStreamingState({ message_id: 2, request_id: '' }), null)
  assert.equal(buildRecoveredStreamingState({ message_id: 'bad', request_id: 'req-1' }), null)
})
