import test from 'node:test'
import assert from 'node:assert/strict'
import { createPinia, setActivePinia } from 'pinia'

async function loadModule() {
  // Node 24 can execute the TypeScript source directly for this lightweight test.
  // @ts-ignore TS5097: test runtime intentionally imports the source file.
  return import('./settings.ts')
}

test('chatwiki cloud login intent is consumed once', async () => {
  const { useSettingsStore } = await loadModule()
  setActivePinia(createPinia())
  const store = useSettingsStore()

  assert.equal(store.consumePendingChatwikiAction(), null)

  store.requestChatwikiCloudLogin()
  assert.equal(store.consumePendingChatwikiAction(), 'cloudLogin')
  assert.equal(store.consumePendingChatwikiAction(), null)
})

test('chatwiki provider selection intent is consumed once', async () => {
  const { useSettingsStore } = await loadModule()
  setActivePinia(createPinia())
  const store = useSettingsStore()

  assert.equal(store.consumePendingModelServiceProviderId(), null)

  store.requestModelServiceProviderSelection('chatwiki')
  assert.equal(store.consumePendingModelServiceProviderId(), 'chatwiki')
  assert.equal(store.consumePendingModelServiceProviderId(), null)
})
