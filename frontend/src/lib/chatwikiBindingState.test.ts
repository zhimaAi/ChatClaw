import test from 'node:test'
import assert from 'node:assert/strict'

async function loadModule() {
  // @ts-ignore TS5097: node test runtime imports the source file directly.
  return import('./chatwikiBindingState.ts')
}

test('binding change listeners receive updates and can unsubscribe', async () => {
  const { notifyChatwikiBindingChanged, onChatwikiBindingChanged } = await loadModule()

  const seen: boolean[] = []
  const unsubscribe = onChatwikiBindingChanged((bound) => {
    seen.push(bound)
  })

  notifyChatwikiBindingChanged(false)
  notifyChatwikiBindingChanged(true)
  unsubscribe()
  notifyChatwikiBindingChanged(false)

  assert.deepEqual(seen, [false, true])
})
