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
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { AuthLayout } from '../auth-layout'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('keeps the return link and authentication content in the keyboard flow', async () => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const root = createRootRoute({ component: Outlet })
  const home = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => <h1>Home</h1>,
  })
  const signIn = createRoute({
    getParentRoute: () => root,
    path: '/sign-in',
    component: () => (
      <AuthLayout>
        <h1>Sign in</h1>
        <label htmlFor='email'>Email</label>
        <input id='email' type='email' />
      </AuthLayout>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([home, signIn]),
    history: createMemoryHistory({ initialEntries: ['/sign-in'] }),
  })
  render(<RouterProvider router={router} />)

  const back = await screen.findByRole('link', { name: 'Back to Home' })
  const main = screen.getByRole('main')
  expect(within(main).getByRole('heading', { name: 'Sign in' })).toBeVisible()
  const email = within(main).getByRole('textbox', { name: 'Email' })

  const user = userEvent.setup()
  await user.tab()
  expect(back).toHaveFocus()
  await user.tab()
  expect(email).toHaveFocus()
  await user.tab({ shift: true })
  expect(back).toHaveFocus()
  await user.keyboard('{Enter}')
  expect(await screen.findByRole('heading', { name: 'Home' })).toBeVisible()
})
