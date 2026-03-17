import test from 'node:test'
import assert from 'node:assert/strict'

type Model = {
  model_id: string
  enabled?: boolean
}

type ProviderWithModels = {
  provider: { provider_id: string }
  model_groups: Array<{
    type: string
    models: Model[]
  }>
}

const providersWithModels: ProviderWithModels[] = [
  {
    provider: { provider_id: 'chatwiki' },
    model_groups: [
      { type: 'llm', models: [{ model_id: 'cw-llm-1', enabled: true }] },
      { type: 'embedding', models: [{ model_id: 'cw-emb-1', enabled: true }] },
    ],
  },
  {
    provider: { provider_id: 'openai' },
    model_groups: [
      { type: 'llm', models: [{ model_id: 'gpt-5', enabled: true }] },
      { type: 'embedding', models: [{ model_id: 'text-embedding-3-large', enabled: true }] },
    ],
  },
]

async function loadModule() {
  // Node 24 can execute the TypeScript source directly for this lightweight test.
  // @ts-ignore TS5097: test runtime intentionally imports the source file.
  return import('./chatwikiModelAvailability.ts')
}

test('chatwiki models are disabled when account is not bound', async () => {
  const { isModelSelectionDisabled } = await loadModule()
  assert.equal(isModelSelectionDisabled('chatwiki', false), true)
  assert.equal(isModelSelectionDisabled('chatwiki', true), false)
  assert.equal(isModelSelectionDisabled('openai', false), false)
})

test('unbound chatwiki selections are treated as unavailable', async () => {
  const { isSelectionAvailable } = await loadModule()
  assert.equal(
    isSelectionAvailable(providersWithModels, 'chatwiki::cw-llm-1', 'llm', false),
    false
  )
  assert.equal(isSelectionAvailable(providersWithModels, 'openai::gpt-5', 'llm', false), true)
})

test('saved chatwiki selection is cleared when account is not bound', async () => {
  const { clearUnavailableChatwikiSelection } = await loadModule()
  assert.equal(
    clearUnavailableChatwikiSelection('chatwiki::cw-llm-1', false),
    ''
  )
  assert.equal(
    clearUnavailableChatwikiSelection('chatwiki::cw-llm-1', true),
    'chatwiki::cw-llm-1'
  )
  assert.equal(clearUnavailableChatwikiSelection('openai::gpt-5', false), 'openai::gpt-5')
})

test('fallback selection skips unbound chatwiki models', async () => {
  const { getFirstSelectableModelKey } = await loadModule()
  assert.equal(getFirstSelectableModelKey(providersWithModels, 'llm', false), 'openai::gpt-5')
  assert.equal(
    getFirstSelectableModelKey(providersWithModels, 'embedding', false),
    'openai::text-embedding-3-large'
  )
  assert.equal(getFirstSelectableModelKey(providersWithModels, 'llm', true), 'chatwiki::cw-llm-1')
})
