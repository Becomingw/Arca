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
import { useQuery } from '@tanstack/react-query'

import { USER_STATUS } from '@/features/users/constants'
import { ROLE } from '@/lib/roles'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { getDashboardUsageSummary } from '../api'
import type { DashboardUsageScope } from '../types'

export function useUsageSummary(scope: DashboardUsageScope) {
  const user = useAuthStore((state) => state.auth.user)
  const canLoad =
    Boolean(user?.id) &&
    (user?.status === undefined || user.status === USER_STATUS.ENABLED) &&
    (scope === 'self' || (user?.role ?? ROLE.GUEST) >= ROLE.ADMIN)

  return useQuery({
    queryKey: [
      'dashboard',
      'usage-summary',
      scope,
      canLoad ? user?.id : undefined,
    ],
    queryFn: async ({ signal }) =>
      requireServerSuccess(await getDashboardUsageSummary(scope, signal)).data,
    enabled: canLoad,
    staleTime: 30_000,
    refetchOnWindowFocus: true,
  })
}
