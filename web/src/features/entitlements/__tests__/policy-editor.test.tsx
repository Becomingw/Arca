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
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { Entitlements } from '..'
import type { PolicyRecord } from '../api'

const initialPolicy: PolicyRecord = {
  version: 7,
  policy: {
    timezone: 'Asia/Taipei',
    organizations: null,
    groups: null,
    users: null,
    user_model_access: null,
    model_families: null,
  },
}

afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

it('submits a structured policy by expected version and keeps its draft on a conflict', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'fixture-root', role: ROLE.SUPER_ADMIN })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: initialPolicy },
  })
  const put = vi
    .spyOn(api, 'put')
    .mockRejectedValueOnce({ response: { status: 409 } })
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={queryClient}>
      <Entitlements />
    </QueryClientProvider>
  )

  await user.click(await screen.findByRole('tab', { name: 'Organizations' }))
  fireEvent.change(await screen.findByLabelText('Organization ID'), {
    target: { value: 'support' },
  })
  await user.click(screen.getByRole('button', { name: 'Add organization' }))
  fireEvent.change(screen.getByLabelText('Organization name'), {
    target: { value: 'Support' },
  })
  fireEvent.change(screen.getByLabelText('Member user IDs'), {
    target: { value: '5, 13' },
  })
  await user.click(screen.getByRole('tab', { name: 'Model families' }))
  fireEvent.change(screen.getByLabelText('Model family ID'), {
    target: { value: 'fast' },
  })
  await user.click(screen.getByRole('button', { name: 'Add model family' }))
  fireEvent.change(screen.getByLabelText('Model family name'), {
    target: { value: 'Fast models' },
  })
  fireEvent.change(screen.getByLabelText('Models'), {
    target: { value: 'model-a' },
  })
  await user.click(screen.getByRole('tab', { name: 'Legacy allowances' }))
  await user.click(screen.getByRole('button', { name: 'Add default budget' }))
  fireEvent.change(screen.getByLabelText('Weekly allowance'), {
    target: { value: '12.50' },
  })
  await user.click(screen.getByRole('button', { name: 'Save policy' }))

  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/arca/admin/entitlements',
      {
        expected_version: 7,
        policy: expect.objectContaining({
          organizations: {
            support: {
              name: 'Support',
              members: [5, 13],
              model_access: { mode: 'inherit' },
            },
          },
          model_families: {
            fast: { name: 'Fast models', models: ['model-a'] },
          },
          default: expect.objectContaining({ weekly_budget_usd: 12.5 }),
        }),
      },
      expect.objectContaining({ singleUseAuthorization: true })
    )
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Your draft is still here'
  )
  expect(screen.getByLabelText('Weekly allowance')).toHaveValue(12.5)
  expect(screen.getByRole('button', { name: 'Save policy' })).toBeDisabled()
  queryClient.clear()
})

it('creates and duplicates a template, then assigns one plan per user or user group', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'fixture-root', role: ROLE.SUPER_ADMIN })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/group/') {
      return { data: { success: true, data: ['staff'] } }
    }
    if (url === '/api/channel/models_enabled') {
      return { data: { success: true, data: ['model-a'] } }
    }
    if (url.startsWith('/api/user/search')) {
      return {
        data: {
          success: true,
          data: { items: [{ id: 13, username: 'alex' }] },
        },
      }
    }
    return { data: { success: true, data: initialPolicy } }
  })
  const put = vi
    .spyOn(api, 'put')
    .mockRejectedValueOnce({ response: { status: 409 } })
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={queryClient}>
      <Entitlements />
    </QueryClientProvider>
  )

  await user.click(await screen.findByRole('button', { name: 'Use template' }))
  expect(screen.getByLabelText('Daily allowance')).toHaveValue(10)
  fireEvent.change(screen.getByLabelText('Plan name'), {
    target: { value: 'Research' },
  })
  await user.click(screen.getByRole('button', { name: 'Duplicate plan' }))
  expect(screen.getByLabelText('Plan name')).toHaveValue('Research (copy)')
  await user.click(screen.getByRole('tab', { name: 'Assignments' }))
  const directCard = (
    await screen.findByText('User subscriptions')
  ).closest<HTMLElement>('[data-slot=card]')
  const groupsCard = screen
    .getByText('Group subscriptions')
    .closest<HTMLElement>('[data-slot=card]')
  if (!directCard || !groupsCard) throw new Error('Assignment cards missing')
  const direct = within(directCard)
  const groups = within(groupsCard)
  const choose = async (field: HTMLElement, name: string) => {
    await user.click(field)
    await user.click(await screen.findByRole('option', { name }))
  }
  await choose(direct.getByRole('combobox', { name: 'User' }), 'alex · #13')
  await choose(
    direct.getByRole('combobox', { name: 'Assigned subscription' }),
    'Research'
  )
  await choose(
    direct.getByRole('combobox', { name: 'Assigned subscription' }),
    'Research (copy)'
  )
  await choose(groups.getByRole('combobox', { name: 'User group' }), 'staff')
  await choose(
    groups.getByRole('combobox', { name: 'Assigned subscription' }),
    'Research'
  )
  await user.click(screen.getByRole('button', { name: 'Save policy' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/arca/admin/entitlements',
      {
        expected_version: 7,
        policy: expect.objectContaining({
          user_subscriptions: { '13': 'plan-2' },
          group_subscriptions: { staff: 'plan-1' },
          plans: expect.objectContaining({
            'plan-1': expect.objectContaining({ name: 'Research' }),
            'plan-2': expect.objectContaining({ name: 'Research (copy)' }),
          }),
        }),
      },
      expect.anything()
    )
  )
  queryClient.clear()
})
