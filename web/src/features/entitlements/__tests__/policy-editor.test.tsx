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
  await user.click(screen.getByRole('tab', { name: 'Budgets' }))
  await user.click(screen.getByRole('button', { name: 'Add default budget' }))
  fireEvent.change(screen.getByLabelText('Weekly (USD)'), {
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
  expect(screen.getByLabelText('Weekly (USD)')).toHaveValue(12.5)
  expect(screen.getByRole('button', { name: 'Save policy' })).toBeDisabled()
  queryClient.clear()
})
