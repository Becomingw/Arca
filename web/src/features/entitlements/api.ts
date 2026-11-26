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

import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

export type PolicyMode =
  | ''
  | 'inherit'
  | 'unrestricted'
  | 'deny_all'
  | 'specific'
export type ModelAccess = { mode: PolicyMode; models?: string[] }
export type Organization = {
  name: string
  members: number[]
  model_access: ModelAccess
}
export type ModelFamily = { name: string; models: string[] }
export type SubPool = {
  five_hour_budget_usd?: number
  daily_budget_usd?: number
  weekly_budget_usd?: number
  monthly_budget_usd?: number
}
export type Allocation = Omit<SubPool, 'daily_budget_usd'> & {
  internal_funding: boolean
  unlimited: boolean
  models: Record<string, SubPool> | null
  families: Record<string, SubPool> | null
}
export type EntitlementPolicy = {
  timezone: string
  default?: Allocation
  organizations: Record<string, Organization> | null
  groups: Record<string, Allocation> | null
  users: Record<string, Allocation> | null
  user_model_access: Record<string, ModelAccess> | null
  model_families: Record<string, ModelFamily> | null
}
export type BudgetWindow = {
  scope: 'total' | 'family' | 'model'
  name: string
  period: string
  start: number
  end?: number
  limit_micros: number
  used_micros: number
  reserved_micros: number
  remaining_micros: number
}
export type EntitlementSnapshot = {
  user_id: number
  enabled: boolean
  cost_basis: string
  timezone: string
  group_ids: string[]
  source: string
  allocation: Allocation
  model_access: ModelAccess
  model_families: Record<string, ModelFamily> | null
  windows: BudgetWindow[] | null
  key_limits_enabled: boolean
  key_models?: string[]
}

export type PolicyRecord = { version: number; policy: EntitlementPolicy }
export type PolicyPreview = {
  version: number
  saved: EntitlementSnapshot
  proposed: EntitlementSnapshot | null
}

type Envelope<T> = { success: boolean; data: T }
const policyPath = '/api/arca/admin/entitlements'

export function usePersonalEntitlements() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  return useQuery({
    queryKey: ['entitlements', 'personal', userId],
    enabled: Boolean(userId),
    staleTime: 60_000,
    meta: { errorToast: false },
    queryFn: async ({ signal }) => {
      const { data } = await api.get<Envelope<EntitlementSnapshot>>(
        '/api/arca/entitlements/me',
        { signal, skipErrorHandler: true }
      )
      return requireServerSuccess(data).data
    },
  })
}

export async function readEntitlementPolicy(
  signal: AbortSignal
): Promise<PolicyRecord> {
  const { data } = await api.get<Envelope<PolicyRecord>>(policyPath, {
    signal,
    skipErrorHandler: true,
  })
  return requireServerSuccess(data).data
}

export async function writeEntitlementPolicy(
  version: number,
  policy: EntitlementPolicy
): Promise<PolicyRecord> {
  const { data } = await api.put<Envelope<PolicyRecord>>(
    policyPath,
    { expected_version: version, policy },
    { skipErrorHandler: true, singleUseAuthorization: true }
  )
  return requireServerSuccess(data).data
}

export async function previewEntitlementPolicy(
  userId: number,
  keyId: number | undefined,
  policy: EntitlementPolicy
): Promise<PolicyPreview> {
  const { data } = await api.post<Envelope<PolicyPreview>>(
    `${policyPath}/preview`,
    { user_id: userId, api_key_id: keyId, config: policy },
    { skipErrorHandler: true, singleUseAuthorization: true }
  )
  return requireServerSuccess(data).data
}
