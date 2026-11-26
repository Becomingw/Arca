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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { Workspace } from '..'

let client: QueryClient

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  window.localStorage.clear()
})

async function renderWorkspace(
  role: number,
  modelAliases: string[],
  catalogEnabled = true
) {
  useAuthStore.getState().auth.setUser({ id: 12, username: 'member', role })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/models') {
      return { data: { success: true, data: modelAliases } }
    }
    if (url === '/api/status') {
      return {
        data: {
          success: true,
          data: { HeaderNavModules: { pricing: catalogEnabled } },
        },
      }
    }
    if (url === '/api/arca/entitlements/me') {
      return { data: { success: true, data: { enabled: false } } }
    }
    throw new Error(`Unexpected workspace request: ${url}`)
  })

  const router = createRouter({
    routeTree: createRootRoute({ component: Workspace }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

it('presents member actions before models and entitlements in reading order', async () => {
  const alias = `provider/${'long-model-alias-'.repeat(12)}`
  await renderWorkspace(ROLE.USER, [alias])

  const actions = await screen.findByRole('navigation', {
    name: 'Quick actions',
  })
  const models = screen.getByRole('heading', {
    level: 2,
    name: 'Available Models',
  })
  const entitlements = screen.getByRole('region', { name: 'My entitlements' })
  expect(
    within(actions).getByRole('link', { name: 'API Keys' })
  ).toHaveAttribute('href', '/keys')
  expect(
    within(actions).queryByRole('link', { name: 'Administration' })
  ).toBeNull()
  expect(
    actions.compareDocumentPosition(models) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(
    models.compareDocumentPosition(entitlements) &
      Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(await screen.findByText(alias)).toBeVisible()
  expect(screen.getByRole('link', { name: 'Model Square' })).toHaveAttribute(
    'href',
    '/pricing'
  )
})

it('keeps the admin shortcut keyboard accessible when the catalog is disabled', async () => {
  await renderWorkspace(ROLE.ADMIN, [], false)
  await screen.findByText('No models available')
  expect(screen.queryByRole('link', { name: 'Model Square' })).toBeNull()

  const user = userEvent.setup()
  const actions = screen.getByRole('navigation', { name: 'Quick actions' })
  const adminLink = within(actions).getByRole('link', {
    name: 'Administration',
  })
  expect(adminLink).toHaveAttribute('href', '/dashboard')
  within(actions)
    .getByRole('link', { name: /Discussions/ })
    .focus()
  await user.tab()
  expect(adminLink).toHaveFocus()
})
