import test from 'node:test'
import assert from 'node:assert/strict'

type BindingLike = {
  chatwiki_version?: string
} | null

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

const unboundBinding: BindingLike = null
const cloudBinding: BindingLike = { chatwiki_version: 'release' }
const devBinding: BindingLike = { chatwiki_version: 'dev' }

test('chatwiki models are disabled when account is not bound', async () => {
  const { isModelSelectionDisabled } = await loadModule()
  assert.equal(isModelSelectionDisabled('chatwiki', unboundBinding), true)
  assert.equal(isModelSelectionDisabled('chatwiki', cloudBinding), false)
  assert.equal(isModelSelectionDisabled('openai', unboundBinding), false)
})

test('chatwiki dev binding also disables model selection', async () => {
  const { isModelSelectionDisabled } = await loadModule()
  assert.equal(isModelSelectionDisabled('chatwiki', devBinding), true)
  assert.equal(isModelSelectionDisabled('openai', devBinding), false)
})

test('unavailable chatwiki selections are treated as unavailable', async () => {
  const { isSelectionAvailable } = await loadModule()
  assert.equal(
    isSelectionAvailable(providersWithModels, 'chatwiki::cw-llm-1', 'llm', unboundBinding),
    false
  )
  assert.equal(
    isSelectionAvailable(providersWithModels, 'chatwiki::cw-llm-1', 'llm', devBinding),
    false
  )
  assert.equal(
    isSelectionAvailable(providersWithModels, 'openai::gpt-5', 'llm', unboundBinding),
    true
  )
})

test('saved chatwiki selection is cleared when binding is unavailable', async () => {
  const { clearUnavailableChatwikiSelection } = await loadModule()
  assert.equal(clearUnavailableChatwikiSelection('chatwiki::cw-llm-1', unboundBinding), '')
  assert.equal(clearUnavailableChatwikiSelection('chatwiki::cw-llm-1', devBinding), '')
  assert.equal(
    clearUnavailableChatwikiSelection('chatwiki::cw-llm-1', cloudBinding),
    'chatwiki::cw-llm-1'
  )
  assert.equal(
    clearUnavailableChatwikiSelection('openai::gpt-5', unboundBinding),
    'openai::gpt-5'
  )
})

test('fallback selection skips unavailable chatwiki models', async () => {
  const { getFirstSelectableModelKey } = await loadModule()
  assert.equal(
    getFirstSelectableModelKey(providersWithModels, 'llm', unboundBinding),
    'openai::gpt-5'
  )
  assert.equal(
    getFirstSelectableModelKey(providersWithModels, 'embedding', unboundBinding),
    'openai::text-embedding-3-large'
  )
  assert.equal(
    getFirstSelectableModelKey(providersWithModels, 'llm', devBinding),
    'openai::gpt-5'
  )
  assert.equal(
    getFirstSelectableModelKey(providersWithModels, 'llm', cloudBinding),
    'chatwiki::cw-llm-1'
  )
})

test('only provider labels show chatwiki binding suffixes', async () => {
  const { formatModelDisplayLabel, formatProviderDisplayLabel } = await loadModule()
  assert.equal(
    formatModelDisplayLabel('chatwiki', 'ChatWiki Model', unboundBinding),
    'ChatWiki Model'
  )
  assert.equal(formatModelDisplayLabel('chatwiki', 'ChatWiki Model', devBinding), 'ChatWiki Model')
  assert.equal(
    formatModelDisplayLabel('chatwiki', 'ChatWiki Model', cloudBinding),
    'ChatWiki Model'
  )
  assert.equal(
    formatProviderDisplayLabel('chatwiki', 'ChatWiki', unboundBinding),
    'ChatWiki（未登录）'
  )
  assert.equal(
    formatProviderDisplayLabel('chatwiki', 'ChatWiki', devBinding),
    'ChatWiki（非ChatWiki Cloud）'
  )
  assert.equal(formatProviderDisplayLabel('chatwiki', 'ChatWiki', cloudBinding), 'ChatWiki')
  assert.equal(formatProviderDisplayLabel('openai', 'OpenAI', unboundBinding), 'OpenAI')
})
