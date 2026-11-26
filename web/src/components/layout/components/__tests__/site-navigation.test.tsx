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
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { act, cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { DirectionProvider } from '@/context/direction-provider'
import { arcaProfileKey } from '@/features/profile/arca-profile-api'
import { ROLE } from '@/lib/roles'
import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { AuthenticatedLayout } from '../authenticated-layout'
import { PublicLayout } from '../public-layout'
import { SiteNavigation } from '../site-navigation'

afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  localStorage.clear()
})

async function renderSite(
  initialPath: string,
  visitor: 'member' | 'guest' = 'member'
) {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  if (visitor === 'member') {
    useAuthStore.getState().auth.setUser({
      id: 17,
      username: 'reader',
      role: ROLE.USER,
    })
  }
  useSystemConfigStore.getState().setConfig({ systemName: 'Arca' })
  useSystemConfigStore.getState().setLoading(false)

  const queryClient = new QueryClient()
  queryClient.setQueryData(STATUS_QUERY_KEY, {
    HeaderNavModules: {
      pricing: { enabled: true, requireAuth: visitor === 'guest' },
    },
  })
  queryClient.setQueryData(['notice'], { success: true, data: '' })
  queryClient.setQueryData(arcaProfileKey(17), {
    avatar: null,
    leaderboard_anonymous: false,
  })

  const root = createRootRoute({
    component: () => (
      <>
        <SiteNavigation />
        <Outlet />
      </>
    ),
  })
  const home = createRoute({
    getParentRoute: () => root,
    path: '/',
    staticData: { siteNavigation: 'public' },
    component: () => <h1>Home</h1>,
  })
  const rankings = createRoute({
    getParentRoute: () => root,
    path: '/rankings',
    staticData: { siteNavigation: 'public' },
    component: () => <h1>Rankings</h1>,
  })
  const catalog = createRoute({
    getParentRoute: () => root,
    path: '/pricing',
    staticData: { siteNavigation: 'public' },
    component: () => (
      <PublicLayout>
        <h1>Model Square</h1>
      </PublicLayout>
    ),
  })
  const console = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
    staticData: { siteNavigation: 'console' },
    component: AuthenticatedLayout,
  })
  const workspace = createRoute({
    getParentRoute: () => console,
    path: '/workspace',
    component: () => <h1>Workspace</h1>,
  })
  const keys = createRoute({
    getParentRoute: () => console,
    path: '/keys',
    component: () => <h1>API keys</h1>,
  })
  const signIn = createRoute({
    getParentRoute: () => root,
    path: '/sign-in',
    component: () => <h1>Sign in</h1>,
  })
  const routeTree = root.addChildren([
    home,
    rankings,
    catalog,
    console.addChildren([workspace, keys]),
    signIn,
  ])
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={queryClient}>
      <DirectionProvider>
        <RouterProvider router={router} />
      </DirectionProvider>
    </QueryClientProvider>
  )
  return router
}

test('keeps the same header when navigating between the catalog and the workspace', async () => {
  await renderSite('/pricing')
  const user = userEvent.setup()
  const banner = await screen.findByRole('banner')
  const header = within(banner)
  const brand = header.getByRole('link', { name: /Arca$/ })
  const catalogLink = header.getByRole('link', { name: 'Model Square' })
  const consoleLink = header.getByRole('link', { name: 'Console' })
  expect(catalogLink).toHaveAttribute('aria-current', 'page')

  await user.click(consoleLink)
  expect(
    await screen.findByRole('heading', { name: 'Workspace' })
  ).toBeVisible()
  expect(
    screen.getByRole('banner'),
    'Opening the workspace should keep the existing top bar in place'
  ).toBe(banner)
  expect(header.getByRole('link', { name: /Arca$/ })).toBe(brand)
  expect(consoleLink).toHaveAttribute('aria-current', 'page')
  expect(catalogLink).not.toHaveAttribute('aria-current')

  await user.click(catalogLink)
  expect(
    await screen.findByRole('heading', { name: 'Model Square' })
  ).toBeVisible()
  expect(screen.getByRole('banner')).toBe(banner)
  expect(catalogLink).toHaveAttribute('aria-current', 'page')
})

test('marks Console active on a direct visit to a console subpage', async () => {
  await renderSite('/keys')
  const header = within(screen.getByRole('banner'))

  expect(screen.getByRole('heading', { name: 'API keys' })).toBeVisible()
  expect(header.getByRole('link', { name: 'Console' })).toHaveAttribute(
    'aria-current',
    'page'
  )
  expect(
    header.getByRole('link', { name: 'Model Square' })
  ).not.toHaveAttribute('aria-current')
  const toolbar = within(screen.getByRole('group', { name: 'Console' }))
  expect(toolbar.getByRole('button', { name: 'Toggle Sidebar' })).toBeVisible()
  expect(header.queryByRole('group', { name: 'Console' })).toBeNull()
})

test('closes the compact menu when a navigation commits outside the menu', async () => {
  const router = await renderSite('/pricing')
  const user = userEvent.setup()
  const toggle = within(screen.getByRole('banner')).getByRole('button', {
    name: 'Toggle navigation menu',
  })
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  const controlledMenu = document.querySelector(
    `[id="${toggle.getAttribute('aria-controls')}"]`
  )
  expect(controlledMenu).toHaveAttribute('inert')

  toggle.focus()
  await user.keyboard('{Enter}')
  expect(toggle).toHaveAttribute('aria-expanded', 'true')
  expect(controlledMenu).not.toHaveAttribute('inert')
  expect(document.body.style.overflow).toBe('hidden')

  await act(() => router.navigate({ to: '/workspace' }))
  expect(
    await screen.findByRole('heading', { name: 'Workspace' })
  ).toBeVisible()
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  expect(controlledMenu).toHaveAttribute('inert')
  expect(document.body.style.overflow).toBe('')
})

test('keeps the standalone sign-in screen outside the site navigation', async () => {
  await renderSite('/sign-in')

  expect(screen.getByRole('heading', { name: 'Sign in' })).toBeVisible()
  expect(screen.queryByRole('banner')).toBeNull()
})

test('dismisses a guest sign-in prompt when a different public route opens', async () => {
  const router = await renderSite('/', 'guest')
  const user = userEvent.setup()
  const banner = screen.getByRole('banner')
  await user.click(within(banner).getByRole('link', { name: 'Model Square' }))
  expect(
    await screen.findByRole('dialog', { name: 'Sign in required' })
  ).toBeVisible()

  await act(() => router.navigate({ to: '/rankings' }))

  expect(await screen.findByRole('heading', { name: 'Rankings' })).toBeVisible()
  expect(screen.queryByRole('dialog', { name: 'Sign in required' })).toBeNull()
  expect(screen.getByRole('banner')).toBe(banner)
})
