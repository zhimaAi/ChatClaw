import test from 'node:test'
import assert from 'node:assert/strict'

async function loadModule() {
  // Node 24 can execute the TypeScript source directly for this lightweight test.
  // @ts-ignore TS5097: test runtime intentionally imports the source file.
  return import('./chatwikiAuth.ts')
}

test('buildChatWikiLoginUrl normalizes base url and appends login params', async () => {
  const { buildChatWikiLoginUrl } = await loadModule()

  const url = buildChatWikiLoginUrl('https://cloud.chatwiki.test///', {
    os_type: 'windows',
    os_version: '11',
  })

  assert.equal(
    url,
    'https://cloud.chatwiki.test/#/chatclaw/login?os_type=windows&os_version=11'
  )
})

test('buildChatWikiLoginUrl omits empty login params', async () => {
  const { buildChatWikiLoginUrl } = await loadModule()

  const url = buildChatWikiLoginUrl('https://cloud.chatwiki.test', {
    os_type: '',
    os_version: undefined,
  })

  assert.equal(url, 'https://cloud.chatwiki.test/#/chatclaw/login')
})

test('openChatWikiCloudLogin uses BrowserService methods through one helper', async () => {
  const { openChatWikiCloudLogin } = await loadModule()
  const calls: string[] = []

  const browserService = {
    async GetLoginParams() {
      calls.push('get-login-params')
      return {
        os_type: 'darwin',
        os_version: '15.3',
      }
    },
    async OpenURL(url: string) {
      calls.push(`open:${url}`)
    },
  }

  const url = await openChatWikiCloudLogin('https://cloud.chatwiki.test/', browserService)

  assert.deepEqual(calls, [
    'get-login-params',
    'open:https://cloud.chatwiki.test/#/chatclaw/login?os_type=darwin&os_version=15.3',
  ])
  assert.equal(
    url,
    'https://cloud.chatwiki.test/#/chatclaw/login?os_type=darwin&os_version=15.3'
  )
})
