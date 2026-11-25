import assert from 'node:assert/strict'
import { test } from 'node:test'

test('a write triggered by the old actor is rejected after refresh changes the session', async () => {
  await import('vue')
  globalThis.document = { cookie: 'new_api_has_session=1' }
  globalThis.window = { location: { origin: 'https://console.example' } }
  const calls = []
  const bundle = (id, expires) => ({
    access_token: 'placeholder',
    access_expires_at: expires,
    user: { id, username: 'person' + id },
    session: { sid: 'fixture-' + id },
  })
  const now = Math.floor(Date.now() / 1000)
  const responses = [
    Response.json({ success: true, data: bundle(1, now - 1) }),
    Response.json({ code: 'AUTH_SESSION_MISMATCH' }, { status: 409 }),
    Response.json({ success: true, data: bundle(2, now + 900) }),
  ]
  const originalFetch = globalThis.fetch
  globalThis.fetch = async (url, options) => {
    calls.push([url, options])
    return responses.shift()
  }
  try {
    const { api, restoreSession, viewer } = await import('./api.js')
    await restoreSession()
    assert.equal(viewer.value.id, 1)
    await assert.rejects(api('/api/arca/feedback', { method: 'POST', body: { title: 'stale draft' } }),
      error => error.status === 409)
    assert.equal(viewer.value.id, 2)
    assert.deepEqual(calls.map(([url]) => url), Array(3).fill('/api/user/auth/refresh'))
    assert.equal(calls[1][1].headers['X-Auth-Session'], 'fixture-1')
    assert.equal(calls[2][1].headers, undefined)
  } finally {
    globalThis.fetch = originalFetch
  }
})
