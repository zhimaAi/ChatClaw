import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
// @ts-expect-error Node test runs this file with native TypeScript stripping.
import { createSubmitLock } from './submitLock.ts'

describe('createSubmitLock', () => {
  it('blocks repeated submits before external saving state updates', () => {
    const lock = createSubmitLock()

    assert.equal(lock.acquire(false), true)
    assert.equal(lock.acquire(false), false)
  })

  it('exposes locked state so the UI can disable submit immediately after acquire', () => {
    const lock = createSubmitLock()

    assert.equal(lock.isLocked(), false)
    assert.equal(lock.acquire(false), true)
    assert.equal(lock.isLocked(), true)

    lock.reset()
    assert.equal(lock.isLocked(), false)
  })

  it('blocks submits while external saving is true and allows immediate retry after reset', () => {
    const lock = createSubmitLock()

    assert.equal(lock.acquire(true), false)
    assert.equal(lock.acquire(false), true)

    lock.syncSaving(true)
    assert.equal(lock.acquire(true), false)

    lock.syncSaving(false)
    assert.equal(lock.acquire(false), false)

    lock.reset()
    assert.equal(lock.acquire(false), true)
  })

  it('keeps submit locked for at least one second after saving finishes', async () => {
    const lock = createSubmitLock()

    assert.equal(lock.acquire(false), true)
    lock.syncSaving(false)

    assert.equal(lock.acquire(false), false)

    await new Promise((resolve) => setTimeout(resolve, 1050))
    assert.equal(lock.acquire(false), true)
  })
})
