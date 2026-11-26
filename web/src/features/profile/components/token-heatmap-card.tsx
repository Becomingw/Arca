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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { getArcaTokenHeatmap } from '../arca-profile-api'

const TOKEN_LEVELS = [
  '',
  'bg-primary/25',
  'bg-primary/45',
  'bg-primary/65',
  'bg-primary/85',
]

export function TokenHeatmapCard() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const userId = useAuthStore((s) => s.auth.user?.id)
  const chart = useQuery({
    queryKey: ['profile', 'token-heatmap', userId],
    queryFn: ({ signal }) => getArcaTokenHeatmap(signal),
    enabled: Boolean(userId),
    staleTime: 300_000,
    meta: { errorToast: false },
  })
  const days = chart.data?.days ?? []
  const maxTokens = Math.max(1, ...days.map((day) => day.tokens))
  const total = days.reduce((sum, day) => sum + day.tokens, 0)
  // Fill the partial starting week so subsequent columns align on Sundays.
  const offset = days.length
    ? new Date(`${days[0].date}T00:00:00Z`).getUTCDay()
    : 0

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Token usage heatmap')}</CardTitle>
        <CardDescription>
          {t('Token totals are calculated by UTC date.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {chart.isPending && <Skeleton className='h-24 w-full' />}
        {chart.isError && (
          <Button
            type='button'
            variant='outline'
            onClick={() => void chart.refetch()}
          >
            {t('Retry')}
          </Button>
        )}
        {chart.data?.available === false && (
          <p className='text-muted-foreground text-sm'>
            {t('Usage statistics are unavailable.')}
          </p>
        )}
        {chart.data?.available && (
          <div className='flex flex-col gap-3'>
            <div className='text-muted-foreground text-sm'>
              {t('{{formattedCount}} tokens in this period', {
                formattedCount: formatNumber(total, locale),
              })}
            </div>
            <div
              className='overflow-x-auto pb-1'
              role='region'
              tabIndex={0}
              aria-label={t('Token usage heatmap')}
            >
              <div
                className='grid w-max auto-cols-[10px] grid-flow-col grid-rows-7 gap-1'
                role='list'
              >
                {Array.from({ length: offset }, (_, index) => (
                  <span key={index} aria-hidden='true' />
                ))}
                {days.map((day) => {
                  const label = `${day.date}: ${formatNumber(day.tokens, locale)}`
                  const level =
                    day.tokens > 0
                      ? Math.ceil(4 * Math.sqrt(day.tokens / maxTokens))
                      : 0
                  return (
                    <span
                      key={day.date}
                      role='listitem'
                      aria-label={label}
                      title={label}
                      className={cn(
                        'bg-muted size-2.5 rounded-sm',
                        TOKEN_LEVELS[level]
                      )}
                    />
                  )
                })}
              </div>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
