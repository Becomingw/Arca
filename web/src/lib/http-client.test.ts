/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import axios, { AxiosError, type AxiosAdapter } from 'axios'
import { afterEach, expect, test, vi } from 'vitest'

import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { getFreshAuthHeaders, refreshAuthentication } from './auth-session'
import { api } from './http-client'

vi.mock('./auth-session', () => ({
  applyAuthRotation: vi.fn(),
  clearAuthentication: vi.fn(),
  getFreshAuthHeaders: vi.fn(),
  refreshAuthentication: vi.fn(),
}))

function session(sid: string, id: number, accessToken: string): AuthBundle {
  return {
    user: { id, username: `member-${id}`, role: 1 },
    session: {
      sid,
      current: true,
      login_method: 'password',
      ip: '',
      user_agent: '',
      created_at: 0,
      last_active_at: 0,
      expires_at: 0,
    },
    access_token: accessToken,
    token_type: 'Bearer',
    access_expires_at: 0,
  }
}

function unauthorized(config: Parameters<AxiosAdapter>[0]) {
  const response = {
    config,
    status: 401,
    statusText: 'UNAUTHORIZED',
    data: {},
    headers: {},
  }
  return new AxiosError(
    'Session expired',
    'ERR_BAD_REQUEST',
    config,
    undefined,
    response
  )
}

afterEach(() => useAuthStore.getState().auth.reset())

test('a mutation is not replayed under a different browser session', async () => {
  useAuthStore.getState().auth.setBundle(session('a', 1, 'initial'))
  const replacement = session('b', 2, 'replacement')
  vi.mocked(refreshAuthentication).mockImplementation(async () => {
    useAuthStore.getState().auth.setBundle(replacement)
    return { kind: 'authenticated', bundle: replacement }
  })
  const adapter = vi.fn<AxiosAdapter>(async (config) => {
    throw unauthorized(config)
  })

  await expect(
    api.post('/api/arca/feedback', {}, { adapter })
  ).rejects.toSatisfy(axios.isCancel)
  expect(adapter).toHaveBeenCalledTimes(1)
})

test('a mutation may retry when only the access token rotates', async () => {
  useAuthStore.getState().auth.setBundle(session('a', 1, 'initial'))
  const renewed = session('a', 1, 'renewed')
  vi.mocked(refreshAuthentication).mockImplementation(async () => {
    useAuthStore.getState().auth.setBundle(renewed)
    return { kind: 'authenticated', bundle: renewed }
  })
  const adapter = vi.fn<AxiosAdapter>(async (config) => {
    if (config.headers.get('Authorization') === 'Bearer initial') {
      throw unauthorized(config)
    }
    return { config, status: 200, statusText: 'OK', data: 'saved', headers: {} }
  })

  await expect(api.post('/api/token/', {}, { adapter })).resolves.toMatchObject(
    { data: 'saved' }
  )
  expect(adapter).toHaveBeenCalledTimes(2)
  expect(adapter.mock.calls[1][0].headers.get('Authorization')).toBe(
    'Bearer renewed'
  )
})

test('a single-use mutation is not sent if its preflight refresh changes the session', async () => {
  useAuthStore.getState().auth.setBundle(session('a', 1, 'initial'))
  vi.mocked(getFreshAuthHeaders).mockImplementation(async () => {
    useAuthStore.getState().auth.setBundle(session('b', 2, 'replacement'))
    return { Authorization: 'Bearer replacement' }
  })
  const adapter = vi.fn<AxiosAdapter>()

  await expect(
    api.post(
      '/api/arca/feedback',
      {},
      { adapter, singleUseAuthorization: true }
    )
  ).rejects.toSatisfy(axios.isCancel)
  expect(adapter).not.toHaveBeenCalled()
})

test('a write started anonymously is not replayed as a newly signed-in user', async () => {
  const next = session('b', 2, 'replacement')
  vi.mocked(refreshAuthentication).mockImplementation(async () => {
    useAuthStore.getState().auth.setBundle(next)
    return { kind: 'authenticated', bundle: next }
  })
  const adapter = vi.fn<AxiosAdapter>(async (config) => {
    throw unauthorized(config)
  })

  await expect(
    api.post('/api/arca/feedback', {}, { adapter })
  ).rejects.toSatisfy(axios.isCancel)
  expect(adapter).toHaveBeenCalledTimes(1)
})
