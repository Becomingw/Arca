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
import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import i18n from '@/i18n/config'

import type { UserProfile } from '../../types'
import { ProfileHeader } from '../profile-header'

const profile: UserProfile = {
  id: 1,
  username: 'member',
  display_name: 'Member',
  role: 1,
  group: 'default',
  quota: 0,
  used_quota: 0,
  request_count: 12_500,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 0,
}

afterEach(async () => {
  cleanup()
  await i18n.changeLanguage('en')
})

describe('profile request count', () => {
  it('localizes the decimal separator when the interface language changes', async () => {
    await i18n.changeLanguage('zhTW')
    const client = new QueryClient()
    render(
      <QueryClientProvider client={client}>
        <ProfileHeader profile={profile} loading={false} />
      </QueryClientProvider>
    )

    expect(screen.getByText('1.3w')).toBeVisible()
    await act(() => i18n.changeLanguage('fr'))
    expect(screen.getByText('1,3w')).toBeVisible()
  })
})
