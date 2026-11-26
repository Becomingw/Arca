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

export interface ArcaProfile {
  avatar: string | null
  leaderboard_anonymous: boolean
}

type Envelope<T> = { success: boolean; data: T }

export function arcaProfileKey(userId: number | undefined) {
  return ['profile', 'arca', userId] as const
}

export function useArcaProfile() {
  const userId = useAuthStore((s) => s.auth.user?.id)
  return useQuery({
    queryKey: arcaProfileKey(userId),
    queryFn: async ({ signal }) => {
      const { data } = await api.get<Envelope<ArcaProfile>>(
        '/api/arca/profile',
        { signal, skipErrorHandler: true }
      )
      return requireServerSuccess(data).data
    },
    enabled: Boolean(userId),
    staleTime: 60_000,
    meta: { errorToast: false },
  })
}

export function arcaAvatarUrl(avatar: string): string {
  return `/api/arca/avatars/${encodeURIComponent(avatar)}`
}

export async function updateArcaAvatar(
  avatar: Blob | null
): Promise<Pick<ArcaProfile, 'avatar'>> {
  const path = '/api/arca/profile/avatar'
  const options = { skipErrorHandler: true, singleUseAuthorization: true }
  const { data } = avatar
    ? await api.put<Envelope<Pick<ArcaProfile, 'avatar'>>>(
        path,
        avatar,
        options
      )
    : await api.delete<Envelope<Pick<ArcaProfile, 'avatar'>>>(path, options)
  return requireServerSuccess(data).data
}

export async function updateArcaActivityPrivacy(
  anonymous: boolean
): Promise<Pick<ArcaProfile, 'leaderboard_anonymous'>> {
  const { data } = await api.put<
    Envelope<Pick<ArcaProfile, 'leaderboard_anonymous'>>
  >(
    '/api/arca/profile/privacy',
    { leaderboard_anonymous: anonymous },
    { skipErrorHandler: true, singleUseAuthorization: true }
  )
  return requireServerSuccess(data).data
}

export interface TokenHeatmap {
  available: boolean
  timezone: string
  days: { date: string; tokens: number }[]
}

export async function getArcaTokenHeatmap(
  signal: AbortSignal
): Promise<TokenHeatmap> {
  const { data } = await api.get<Envelope<TokenHeatmap>>(
    '/api/arca/profile/heatmap',
    { signal, skipErrorHandler: true }
  )
  return requireServerSuccess(data).data
}

export interface ArcaTeamActivity {
  members: {
    id: string
    label: string
    anonymous: boolean
    rank: number
    estimated_minutes: number
    active_days: number
    longest_streak: number
    active_today: boolean
    requests: number | null
    tokens: number | null
  }[]
  popular_models: { model: string; requests: number; tokens: number }[]
  activity_bands: { start_second: number; windows: number }[]
  suppressed: boolean
  usage_available: boolean
  timezone: string
}

export async function getArcaTeamActivity(
  days: 7 | 30,
  signal: AbortSignal
): Promise<ArcaTeamActivity> {
  const { data } = await api.get<Envelope<ArcaTeamActivity>>(
    '/api/arca/team-activity',
    { params: { days }, signal, skipErrorHandler: true }
  )
  return requireServerSuccess(data).data
}
