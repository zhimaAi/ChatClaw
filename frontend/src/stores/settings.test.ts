import test from 'node:test'
import assert from 'node:assert/strict'
import { createPinia, setActivePinia } from 'pinia'
import { useSettingsStore } from './settings.ts'

test('chatwiki cloud login intent is consumed once', () => {
  setActivePinia(createPinia())
  const store = useSettingsStore()

  assert.equal(store.consumePendingChatwikiAction(), null)

  store.requestChatwikiCloudLogin()
  assert.equal(store.consumePendingChatwikiAction(), 'cloudLogin')
  assert.equal(store.consumePendingChatwikiAction(), null)
})
