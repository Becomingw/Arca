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
import { render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { STATUS_QUERY_KEY } from '@/lib/status-query'

import { ArcaHome } from '../components/arca/arca-home'

beforeEach(() => {
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      observe() {}
      disconnect() {}
    }
  )
})

afterEach(() => vi.unstubAllGlobals())

function renderHomepage(pricing: { enabled: boolean; requireAuth: boolean }) {
  const queryClient = new QueryClient()
  queryClient.setQueryData(STATUS_QUERY_KEY, {
    HeaderNavModules: { pricing },
  })
  queryClient.setQueryData(['notice'], { success: true, data: '' })

  const routeTree = createRootRoute({
    component: () => <ArcaHome isAuthenticated={false} />,
  })
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

test('when Model Square is disabled the homepage omits its action', async () => {
  renderHomepage({ enabled: false, requireAuth: false })

  const main = within(await screen.findByRole('main'))
  expect(main.queryByRole('link', { name: 'Model Square' })).toBeNull()
})

test('when the catalog requires authentication its homepage action returns to the catalog after sign-in', async () => {
  renderHomepage({ enabled: true, requireAuth: true })

  const main = within(await screen.findByRole('main'))
  const href =
    main.getByRole('link', { name: 'Model Square' }).getAttribute('href') ?? ''
  const url = new URL(href, window.location.origin)
  expect(url.pathname).toBe('/sign-in')
  expect(url.searchParams.get('redirect')).toBe('/pricing')
})

test('the homepage outline keeps the original section targets', async () => {
  renderHomepage({ enabled: true, requireAuth: false })

  const outline = within(
    await screen.findByRole('navigation', { name: 'On this page' })
  )
  expect(
    outline.getAllByRole('link').map((link) => link.getAttribute('href'))
  ).toEqual(['#access', '#entitlements', '#operations', '#project-links'])
  for (const anchor of outline.getAllByRole('link')) {
    const id = anchor.getAttribute('href')?.slice(1) ?? ''
    expect(document.querySelector(`#${id}`)).toHaveAttribute('aria-labelledby')
  }
})
