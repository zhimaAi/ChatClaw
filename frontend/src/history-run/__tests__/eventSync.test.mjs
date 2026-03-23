import test from 'node:test'
import assert from 'node:assert/strict'

import {
  extractHistoryRunStreamingSnapshot,
  HISTORY_RUN_CHAT_EVENT_TYPE,
  HISTORY_RUN_IMMEDIATE_RELOAD_EVENT_NAMES,
  extractConversationId,
  shouldReloadHistoryConversation,
} from '../eventSync.ts'

test('chunk event for current conversation triggers history reload even without start state', () => {
  const shouldReload = shouldReloadHistoryConversation(
    HISTORY_RUN_CHAT_EVENT_TYPE.CHUNK,
    { conversation_id: 42, request_id: 'req-1', delta: 'hello' },
    42
  )

  assert.equal(shouldReload, true)
})

test('event for another conversation does not trigger history reload', () => {
  const shouldReload = shouldReloadHistoryConversation(
    HISTORY_RUN_CHAT_EVENT_TYPE.CHUNK,
    { conversation_id: 9, request_id: 'req-1', delta: 'hello' },
    42
  )

  assert.equal(shouldReload, false)
})

test('complete event is marked for immediate reload', () => {
  assert.equal(
    HISTORY_RUN_IMMEDIATE_RELOAD_EVENT_NAMES.has(HISTORY_RUN_CHAT_EVENT_TYPE.COMPLETE),
    true
  )
})

test('invalid conversation payload is ignored', () => {
  assert.equal(extractConversationId({ conversation_id: 'bad-value' }), null)
  assert.equal(
    shouldReloadHistoryConversation(
      HISTORY_RUN_CHAT_EVENT_TYPE.CHUNK,
      { conversation_id: 'bad-value' },
      42
    ),
    false
  )
})

test('extractHistoryRunStreamingSnapshot returns request identifiers for matching event', () => {
  const snapshot = extractHistoryRunStreamingSnapshot(
    HISTORY_RUN_CHAT_EVENT_TYPE.CHUNK,
    { conversation_id: 42, message_id: 1001, request_id: 'req-1001' },
    42
  )

  assert.deepEqual(snapshot, {
    conversationId: 42,
    requestId: 'req-1001',
    messageId: 1001,
  })
})

test('extractHistoryRunStreamingSnapshot ignores incomplete identifiers', () => {
  assert.equal(
    extractHistoryRunStreamingSnapshot(
      HISTORY_RUN_CHAT_EVENT_TYPE.CHUNK,
      { conversation_id: 42, message_id: 0, request_id: 'req-1001' },
      42
    ),
    null
  )
})
