import test from 'node:test'
import assert from 'node:assert/strict'

async function loadModule() {
  // @ts-ignore TS5097: node test runtime imports the source file directly.
  return import('./chatwikiSidebarAccountCard.ts')
}

test('unbound binding maps to login card state', async () => {
  const { buildChatwikiSidebarAccountCardState } = await loadModule()

  const state = buildChatwikiSidebarAccountCardState(null, null)

  assert.equal(state.mode, 'login')
  assert.equal(state.action, 'login')
})

test('dev binding also maps to login card state', async () => {
  const { buildChatwikiSidebarAccountCardState } = await loadModule()

  const state = buildChatwikiSidebarAccountCardState(
    { chatwiki_version: 'dev', user_name: 'dev-user', user_id: 'dev-id' },
    null
  )

  assert.equal(state.mode, 'login')
  assert.equal(state.action, 'login')
})

test('cloud binding maps to bound state with display name and remaining credits', async () => {
  const { buildChatwikiSidebarAccountCardState } = await loadModule()

  const state = buildChatwikiSidebarAccountCardState(
    { chatwiki_version: 'yun', user_name: 'xianyu@test.com', user_id: '1001' },
    { integral_stats: { raw: JSON.stringify({ all_surplus: '1250' }) } }
  )

  assert.equal(state.mode, 'bound')
  assert.equal(state.action, 'openProviderSettings')
  assert.equal(state.accountLabel, 'xianyu@test.com')
  assert.equal(state.creditsLabel, '1,250 积分')
})

test('cloud binding falls back to user id and placeholder credits when stats are missing', async () => {
  const { buildChatwikiSidebarAccountCardState } = await loadModule()

  const state = buildChatwikiSidebarAccountCardState(
    { chatwiki_version: 'yun', user_name: '   ', user_id: 'account-42' },
    null
  )

  assert.equal(state.mode, 'bound')
  assert.equal(state.accountLabel, 'account-42')
  assert.equal(state.creditsLabel, '-- 积分')
})

test('auto refresh is enabled only for cloud bindings', async () => {
  const { shouldAutoRefreshChatwikiCredits, getChatwikiCreditsRefreshMode } = await loadModule()

  assert.equal(shouldAutoRefreshChatwikiCredits(null), false)
  assert.equal(shouldAutoRefreshChatwikiCredits({ chatwiki_version: 'dev' }), false)
  assert.equal(shouldAutoRefreshChatwikiCredits({ chatwiki_version: 'yun' }), true)
  assert.equal(getChatwikiCreditsRefreshMode('initial'), false)
  assert.equal(getChatwikiCreditsRefreshMode('polling'), true)
})
