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
import { Buffer } from 'node:buffer'
import { runInNewContext } from 'node:vm'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { CCSwitchDialog } from '../cc-switch-dialog'

let queryClient: QueryClient

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  })
})

afterEach(() => {
  queryClient.clear()
  window.localStorage.removeItem('status')
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

function renderDialog({
  models = ['gpt-5.4', 'claude-sonnet-4-6'],
  openCodeConfig = {},
  openCodeStatus = 200,
  allowUsageQuery,
}: {
  models?: string[]
  openCodeConfig?: object
  openCodeStatus?: number
  allowUsageQuery?: boolean
} = {}) {
  let currentModels = models
  const fetchCatalog = vi.fn(async (url: string) => {
    if (url === '/api/opencode/config') {
      return new Response(JSON.stringify(openCodeConfig), {
        status: openCodeStatus,
      })
    }
    return new Response(
      JSON.stringify({
        data: currentModels.map((id) => ({ id })),
        object: 'list',
      })
    )
  })
  vi.stubGlobal('fetch', fetchCatalog)
  render(
    <QueryClientProvider client={queryClient}>
      <CCSwitchDialog
        open
        onOpenChange={vi.fn()}
        tokenKey='test-only'
        allowUsageQuery={allowUsageQuery}
      />
    </QueryClientProvider>
  )
  return {
    fetchCatalog,
    setModels(next: string[]) {
      currentModels = next
    },
  }
}

async function openCCSwitchWithModel(app: string, model: string) {
  const openApp = vi.spyOn(window, 'open').mockImplementation(() => null)
  const user = userEvent.setup()
  await user.click(screen.getByRole('radio', { name: app }))
  await user.click(screen.getByRole('combobox', { name: 'Primary Model' }))
  await user.click(await screen.findByRole('option', { name: model }))
  await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))
  expect(openApp).toHaveBeenCalledTimes(1)
  return new URL(String(openApp.mock.calls[0]?.[0])).searchParams
}

describe('CC Switch model selection', () => {
  it.each(['Claude', 'Codex', 'Gemini'])(
    'opens %s models outside the clipping dialog and keeps the dialog open after selection',
    async (app) => {
      const { fetchCatalog } = renderDialog()
      const user = userEvent.setup()
      await user.click(screen.getByRole('radio', { name: app }))
      const input = screen.getByRole('combobox', { name: 'Primary Model' })

      await user.click(input)

      expect(input).toHaveAttribute('aria-expanded', 'true')
      const list = await screen.findByRole('listbox')
      const dialog = screen.getByRole('dialog', { name: 'Import to CC Switch' })
      // The dialog is translated and clips overflow. Its popup must escape
      // that containing block to remain aligned and fully visible.
      expect(dialog).not.toContainElement(list)
      await user.click(screen.getByRole('option', { name: 'gpt-5.4' }))
      await waitFor(() => expect(input).toHaveValue('gpt-5.4'))
      expect(input).toHaveAttribute('aria-expanded', 'false')
      expect(dialog).toBeVisible()
      expect(fetchCatalog).toHaveBeenCalledWith(
        '/v1/models',
        expect.objectContaining({
          headers: { Authorization: 'Bearer sk-test-only' },
          credentials: 'omit',
          cache: 'no-store',
          redirect: 'error',
        })
      )
    }
  )

  it('filters model names and supports keyboard selection and Escape without closing the dialog', async () => {
    renderDialog()
    const user = userEvent.setup()
    await user.click(screen.getByRole('radio', { name: 'Codex' }))
    const input = screen.getByRole('combobox', { name: 'Primary Model' })
    await user.click(input)
    await user.type(input, 'sonnet')

    expect(
      screen.queryByRole('option', { name: 'gpt-5.4' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: 'claude-sonnet-4-6' })
    ).toBeVisible()
    await user.keyboard('{ArrowDown}{Enter}')
    await waitFor(() => expect(input).toHaveValue('claude-sonnet-4-6'))
    await user.click(input)
    await user.keyboard('{Escape}')

    expect(input).toHaveValue('claude-sonnet-4-6')
    expect(input).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByRole('dialog')).toBeVisible()
  })

  it('shows an empty result when no models are available and updates an open dropdown when models arrive', async () => {
    const { setModels } = renderDialog({ models: [] })
    const user = userEvent.setup()
    await user.click(screen.getByRole('radio', { name: 'Codex' }))
    const input = screen.getByRole('combobox', { name: 'Primary Model' })
    await user.click(screen.getByRole('button', { name: 'Primary Model' }))

    expect(await screen.findByText('No models found')).toBeVisible()
    await act(async () => {
      setModels(['gpt-5.4'])
      await queryClient.invalidateQueries({ queryKey: ['cc-switch-models'] })
    })
    await user.click(await screen.findByRole('option', { name: 'gpt-5.4' }))
    await waitFor(() => expect(input).toHaveValue('gpt-5.4'))
  })

  it('imports an OpenCode provider using the endpoint and model authorized for the selected key', async () => {
    const { fetchCatalog } = renderDialog({
      models: ['profile-only-model'],
      openCodeConfig: {
        provider: {
          aether: {
            options: { baseURL: 'https://arca.example/infra/v1' },
            models: { 'key-allowed-model': { name: 'Allowed' } },
          },
        },
      },
      allowUsageQuery: true,
    })

    const params = await openCCSwitchWithModel('OpenCode', 'key-allowed-model')

    expect(fetchCatalog).toHaveBeenCalledWith(
      '/api/opencode/config',
      expect.objectContaining({
        headers: { Authorization: 'Bearer sk-test-only' },
        credentials: 'omit',
        cache: 'no-store',
        redirect: 'error',
      })
    )
    expect(params.get('app')).toBe('opencode')
    expect(params.get('apiFormat')).toBe('openai_chat')
    expect(params.get('endpoint')).toBe('https://arca.example/infra/v1')
    expect(params.get('usageBaseUrl')).toBe('https://arca.example/infra')
    expect(params.get('model')).toBe('key-allowed-model')
  })

  it('shows the OpenCode catalog error and prevents opening CC Switch', async () => {
    renderDialog({ openCodeStatus: 401 })
    const user = userEvent.setup()
    await user.click(screen.getByRole('radio', { name: 'OpenCode' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load models'
    )
    expect(
      screen.getByRole('button', { name: 'Open CC Switch' })
    ).toBeDisabled()
  })

  it('lets users edit the provider name without opening a model dropdown', async () => {
    renderDialog()
    const user = userEvent.setup()
    const input = screen.getByRole('textbox', { name: 'Name' })

    await user.clear(input)
    await user.type(input, 'Development')

    expect(input).toHaveValue('Development')
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })
})

describe('CC Switch subscription queries', () => {
  function configureGateway(url: string) {
    window.localStorage.setItem(
      'status',
      JSON.stringify({ server_address: url })
    )
  }

  it('uses a same-origin Codex query and emits separate allowance and grant rows without Intl', async () => {
    configureGateway('https://arca.example/infra/v1/')
    renderDialog({ allowUsageQuery: true })
    const params = await openCCSwitchWithModel('Codex', 'gpt-5.4')
    expect(params.get('endpoint')).toBe('https://arca.example/infra/v1')
    expect(params.get('usageBaseUrl')).toBe('https://arca.example/infra')
    expect(params.get('usageEnabled')).toBe('true')
    expect(params.get('usageAutoInterval')).toBe('30')
    expect(params.get('usageApiKey')).toBeNull()
    expect(params.get('usageAccessToken')).toBeNull()

    const script = Buffer.from(
      params.get('usageScript') ?? '',
      'base64url'
    ).toString('utf8')
    expect(script).not.toContain('sk-test-only')
    const emitter = runInNewContext(script, { Intl: undefined }) as {
      request: { url: string; method: string; headers: Record<string, string> }
      extractor: (response: object) => unknown
    }
    expect(emitter.request).toMatchObject({
      url: '{{baseUrl}}/api/ccswitch/usage',
      method: 'GET',
      headers: { Authorization: 'Bearer {{apiKey}}' },
    })
    expect(emitter.extractor({ success: false })).toEqual({
      isValid: false,
      invalidMessage: 'Failed to fetch usage',
    })
    expect(
      emitter.extractor({
        success: true,
        data: {
          subscription_name: '课题组',
          state: 'active',
          timezone: 'Asia/Shanghai',
          windows: [
            {
              scope: 'total',
              period: 'rolling_5h',
              limit_micros: 10_000_000,
              used_micros: 4_000_000,
              reserved_micros: 500_000,
              remaining_micros: 5_500_000,
            },
            {
              scope: 'model',
              name: 'gpt-5.4',
              period: 'day',
              end: 1893456000,
              limit_micros: 3_000_000,
              used_micros: 500_000,
              reserved_micros: 0,
              remaining_micros: 2_500_000,
            },
          ],
          grants: [
            {
              name: '专项',
              models: ['gpt-5.4'],
              amount_micros: 2_000_000,
              remaining_micros: 250_000,
              ends_at: 1893456000,
            },
          ],
        },
      })
    ).toEqual([
      {
        planName: '课题组 · Rolling 5 hours',
        isValid: true,
        total: 10,
        used: 4,
        remaining: 5.5,
        unit: 'Usage units',
        extra: 'Reserved usage: 0.5 Usage units',
      },
      {
        planName: '课题组 · Model: gpt-5.4 · Daily',
        isValid: true,
        total: 3,
        used: 0.5,
        remaining: 2.5,
        unit: 'Usage units',
        extra: 'Reset at: 2030-01-01 00:00 UTC',
      },
      {
        planName: 'Temporary usage grants · 专项',
        isValid: true,
        total: 2,
        remaining: 0.25,
        unit: 'Usage units',
        extra: 'gpt-5.4 · Expires at 2030-01-01 00:00 UTC',
      },
    ])
  })

  it('uses the accessed origin when the configured origin is the private container default', async () => {
    configureGateway('http://127.0.0.1:3000/')
    renderDialog({ allowUsageQuery: true })
    const params = await openCCSwitchWithModel('Claude', 'claude-sonnet-4-6')
    expect(params.get('endpoint')).toBe(window.location.origin)
    expect(params.get('usageBaseUrl')).toBe(window.location.origin)
    expect(params.get('usageEnabled')).toBe('true')
  })

  it('keeps the accessed origin when the container configuration contains a double-slash path', async () => {
    configureGateway('http://localhost:3000//wrong.example')
    renderDialog({ allowUsageQuery: true })
    const params = await openCCSwitchWithModel('Codex', 'gpt-5.4')
    expect(params.get('endpoint')).toBe(`${window.location.origin}/v1`)
    expect(params.get('usageBaseUrl')).toBe(window.location.origin)
  })

  it('keeps a normal provider import when key-owner usage sharing is disabled', async () => {
    configureGateway('https://arca.example')
    renderDialog({ allowUsageQuery: false })
    expect(screen.getByText(/Allow querying user quota/)).toBeVisible()
    const params = await openCCSwitchWithModel('Claude', 'claude-sonnet-4-6')
    expect(params.get('usageScript')).toBeNull()
  })

  it('keeps a normal provider import without a usage script for non-loopback HTTP', async () => {
    configureGateway('http://127.0.0.1.example:8080')
    renderDialog({ allowUsageQuery: true })
    expect(screen.getByText(/Subscription usage requires HTTPS/)).toBeVisible()
    const params = await openCCSwitchWithModel('Gemini', 'gpt-5.4')
    expect(params.get('usageScript')).toBeNull()
  })
})
