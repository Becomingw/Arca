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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { TopNav } from '../top-nav'

const links = [
  { title: 'Home', href: '/' },
  { title: 'Model Square', href: '/pricing' },
  { title: 'Rankings', href: '/rankings' },
]

async function renderNavigation() {
  const user = userEvent.setup()
  const root = createRootRoute({
    component: () => (
      <>
        <TopNav links={links} />
        <Outlet />
      </>
    ),
  })
  const routeTree = root.addChildren([
    createRoute({ getParentRoute: () => root, path: '/' }),
    createRoute({ getParentRoute: () => root, path: 'pricing' }),
    createRoute({ getParentRoute: () => root, path: 'rankings' }),
  ])
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: ['/rankings'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  return user
}

test('marks the selected public destination without selecting Home for nested locations', async () => {
  await renderNavigation()
  const navigation = screen.getByRole('navigation', {
    name: 'Header navigation',
  })
  expect(
    within(navigation).getByRole('link', { name: 'Rankings' })
  ).toHaveAttribute('aria-current', 'page')
  expect(
    within(navigation).getByRole('link', { name: 'Home' })
  ).not.toHaveAttribute('aria-current')
})

test('the compact menu opens with the keyboard and navigates to a public destination', async () => {
  const user = await renderNavigation()
  const toggle = screen.getByRole('button', { name: 'Toggle navigation menu' })
  toggle.focus()
  await user.keyboard('{Enter}')
  expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await user.click(
    await screen.findByRole('menuitem', { name: 'Model Square' })
  )

  const navigation = screen.getByRole('navigation', {
    name: 'Header navigation',
  })
  expect(
    within(navigation).getByRole('link', { name: 'Model Square' })
  ).toHaveAttribute('aria-current', 'page')
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
})
