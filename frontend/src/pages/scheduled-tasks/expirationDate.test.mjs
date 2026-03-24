import test from 'node:test'
import assert from 'node:assert/strict'

const { buildExpirationDateTime, toDateInputValue } = await import('./expirationDate.ts')

test('toDateInputValue keeps the calendar date for UTC end-of-day timestamps in UTC tasks', () => {
  assert.equal(toDateInputValue('2026-03-23T23:59:59Z', 'UTC'), '2026-03-23')
})

test('toDateInputValue keeps the calendar date for task-timezone end-of-day timestamps', () => {
  assert.equal(toDateInputValue('2026-03-23T15:59:59Z', 'Asia/Shanghai'), '2026-03-23')
})

test('buildExpirationDateTime serializes end-of-day dates using the task timezone', () => {
  const expiresAt = buildExpirationDateTime('2026-03-23', 'UTC')

  assert.ok(expiresAt instanceof Date)
  assert.equal(expiresAt?.toISOString(), '2026-03-23T23:59:59.000Z')
})

test('buildExpirationDateTime converts Asia/Shanghai end-of-day dates to the stored UTC instant', () => {
  const expiresAt = buildExpirationDateTime('2026-03-23', 'Asia/Shanghai')

  assert.ok(expiresAt instanceof Date)
  assert.equal(expiresAt?.toISOString(), '2026-03-23T15:59:59.000Z')
})
