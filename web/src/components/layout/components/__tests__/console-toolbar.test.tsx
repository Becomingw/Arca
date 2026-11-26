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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test } from 'vitest'

import { Sidebar, SidebarProvider } from '@/components/ui/sidebar'
import { DirectionProvider } from '@/context/direction-provider'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider, useSearch } from '@/context/search-provider'
import { removeCookie } from '@/lib/cookies'

import { ConsoleToolbar } from '../console-toolbar'

function SearchStatus() {
  const { open } = useSearch()
  return open && <div role='status'>Search is open</div>
}

function renderToolbar() {
  render(
    <DirectionProvider>
      <LayoutProvider>
        <SearchProvider>
          <SidebarProvider defaultOpen>
            <ConsoleToolbar />
            <Sidebar collapsible='icon'>Console navigation</Sidebar>
            <SearchStatus />
          </SidebarProvider>
        </SearchProvider>
      </LayoutProvider>
    </DirectionProvider>
  )
  return within(screen.getByRole('group', { name: 'Console' }))
}

afterEach(() => {
  removeCookie('sidebar_state')
})

test('opens search from the toolbar and toggles the same search by shortcut', async () => {
  const user = userEvent.setup()
  const toolbar = renderToolbar()

  toolbar.getByRole('button', { name: 'Search' }).focus()
  await user.keyboard('{Enter}')

  expect(screen.getByRole('status')).toHaveTextContent('Search is open')

  await user.keyboard('{Control>}k{/Control}')
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
})

test('keeps sidebar state and appearance controls available below the site header', async () => {
  const user = userEvent.setup()
  const toolbar = renderToolbar()
  const sidebar = screen
    .getByText('Console navigation')
    .closest('[data-slot="sidebar"]')

  const sidebarToggle = toolbar.getByRole('button', { name: 'Toggle Sidebar' })
  expect(sidebar).toHaveAttribute('data-state', 'expanded')
  expect(sidebarToggle).toHaveAttribute('aria-expanded', 'true')
  sidebarToggle.focus()
  await user.keyboard('{Enter}')
  expect(sidebar).toHaveAttribute('data-state', 'collapsed')
  expect(sidebarToggle).toHaveAttribute('aria-expanded', 'false')

  await user.click(toolbar.getByRole('button', { name: 'Open theme settings' }))
  expect(
    await screen.findByRole('dialog', { name: 'Theme Settings' })
  ).toBeVisible()
})
