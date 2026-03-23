import test from 'node:test'
import assert from 'node:assert/strict'

import { prepareCreateTaskDialogState } from '../createTaskDialogState.ts'

test('prepareCreateTaskDialogState reloads base options before opening create dialog', async () => {
  const callOrder = []
  const expectedForm = { name: 'new-task' }

  const state = await prepareCreateTaskDialogState(
    async () => {
      callOrder.push('load-base-options')
    },
    () => {
      callOrder.push('build-empty-form')
      return /** @type {any} */ (expectedForm)
    }
  )

  assert.deepEqual(callOrder, ['load-base-options', 'build-empty-form'])
  assert.equal(state.editingTask, null)
  assert.equal(state.form, expectedForm)
  assert.equal(state.createDialogOpen, true)
})
