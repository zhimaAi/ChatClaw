import test from 'node:test'
import assert from 'node:assert/strict'

type BindingEventPayload = boolean | { chatwiki_version?: string | null } | null

async function loadModule() {
  // @ts-ignore TS5097: node test runtime imports the source file directly.
  return import('./chatwikiBindingState.ts')
}

test('binding change listeners receive updates and can unsubscribe', async () => {
  const { notifyChatwikiBindingChanged, onChatwikiBindingChanged } = await loadModule()

  const seen: BindingEventPayload[] = []
  const unsubscribe = onChatwikiBindingChanged((payload) => {
    seen.push(payload)
  })

  const devBinding = { chatwiki_version: 'dev' }

  notifyChatwikiBindingChanged(null)
  notifyChatwikiBindingChanged(devBinding)
  unsubscribe()
  notifyChatwikiBindingChanged(false)

  assert.deepEqual(seen, [null, devBinding])
})
