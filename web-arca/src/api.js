import { ref } from 'vue'

export const viewer = ref(null)
let session
let refreshInFlight
let sessionGeneration = 0

function sessionHint() {
  return document.cookie.split(/;\s*/).includes('new_api_has_session=1')
}

export class APIError extends Error {
  constructor(status, message) {
    super(message)
    this.status = status
  }
}

export function parseBundle(data) {
  if (
    !data || typeof data.access_token !== 'string' ||
    !Number.isInteger(data.access_expires_at) ||
    !Number.isInteger(data.user?.id) ||
    typeof data.user.username !== 'string' ||
    typeof data.session?.sid !== 'string'
  ) return null
  return data
}

async function readEnvelope(response) {
  const data = await response.json().catch(() => null)
  if (!response.ok || !data?.success) {
    throw new APIError(response.status, data?.message || `HTTP ${response.status}`)
  }
  return data.data
}

export async function restoreSession({ force = false } = {}) {
  if (refreshInFlight) return refreshInFlight
  if (!force && session && session.access_expires_at > Date.now() / 1000 + 15) return session
  if (!session && !sessionHint()) return null

  const generation = sessionGeneration
  const renew = async () => {
    if (generation !== sessionGeneration) return null
    const fresh = () => fetch('/api/user/auth/refresh', {
      method: 'POST', cache: 'no-store', credentials: 'same-origin',
      headers: session ? { 'X-Auth-Session': session.session.sid } : undefined,
    })
    let response = await fresh()
    if (generation !== sessionGeneration) return null
    if (response.status === 409 && session) {
      const error = await response.clone().json().catch(() => null)
      if (error?.code === 'AUTH_SESSION_MISMATCH') {
        session = undefined
        viewer.value = null
        response = await fresh()
      }
    }
    if (generation !== sessionGeneration) return null
    if (response.status === 401 || response.status === 403) {
      session = undefined
      viewer.value = null
      return null
    }
    const next = parseBundle(await readEnvelope(response))
    if (!next) throw new Error('登录响应格式有误')
    if (generation !== sessionGeneration) return null
    session = next
    viewer.value = next.user
    return session
  }
  refreshInFlight = (navigator.locks ? navigator.locks.request('new-api:auth-refresh', renew) : renew())
    .finally(() => { refreshInFlight = undefined })
  return refreshInFlight
}

export async function signOut() {
  const previous = session
  sessionGeneration++
  if (refreshInFlight) await refreshInFlight.catch(() => {})
  const logout = () => fetch('/api/user/auth/logout', {
    method: 'POST', credentials: 'same-origin', cache: 'no-store',
    headers: previous ? { 'X-Auth-Session': previous.session.sid, Authorization: `Bearer ${previous.access_token}` } : undefined,
  })
  const response = await (navigator.locks ? navigator.locks.request('new-api:auth-refresh', logout) : logout())
  if (response.ok || response.status === 401) {
    session = undefined
    viewer.value = null
    return
  }
  await readEnvelope(response)
}

export async function api(path, { method = 'GET', body, signal } = {}) {
  const url = new URL(path, window.location.origin)
  if (url.origin !== window.location.origin || !url.pathname.startsWith('/api/')) {
    throw new Error('Unsupported API URL')
  }
  const previous = session
  const current = await restoreSession()
  if (!current) throw new APIError(401, '请先登录')
  if (method !== 'GET' && method !== 'HEAD' && (
    !previous || current.session.sid !== previous.session.sid || current.user.id !== previous.user.id
  )) {
    throw new APIError(409, '登录账号已切换，请确认后重试')
  }
  const send = (auth) => {
    const headers = { Authorization: `Bearer ${auth.access_token}` }
    let payload = body
    if (body !== undefined && !(body instanceof Blob)) {
      headers['Content-Type'] = 'application/json'
      payload = JSON.stringify(body)
    }
    return fetch(url, { method, headers, body: payload, cache: 'no-store', credentials: 'same-origin', signal })
  }
  let response = await send(current)
  if (response.status === 401) {
    const renewed = await restoreSession({ force: true })
    if (renewed && renewed.session.sid === current.session.sid && renewed.user.id === current.user.id) {
      response = await send(renewed)
    } else if (renewed) {
      throw new APIError(409, '登录账号已切换，请确认后重试')
    }
  }
  return readEnvelope(response)
}

export async function exportOpenCode(relayKey) {
  const response = await fetch('/api/opencode/config', {
    headers: { Authorization: `Bearer ${relayKey}` },
    credentials: 'omit',
    cache: 'no-store',
    redirect: 'error',
  })
  if (!response.ok) throw new APIError(response.status, '无法获取 OpenCode 配置')
  return response.json()
}
