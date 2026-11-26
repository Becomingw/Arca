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
import { useTranslation } from 'react-i18next'

import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import type { RankingPeriod } from '../types'

const PERIODS: { id: RankingPeriod; labelKey: string }[] = [
  { id: 'today', labelKey: 'Today' },
  { id: 'week', labelKey: 'Week' },
  { id: 'month', labelKey: 'Month' },
  { id: 'year', labelKey: 'Year' },
]

type RankingsHeroProps = {
  period: RankingPeriod
  onPeriodChange: (period: RankingPeriod) => void
}

export function RankingsHero({ period, onPeriodChange }: RankingsHeroProps) {
  const { t } = useTranslation()

  return (
    <div className='flex flex-wrap items-center justify-between gap-4'>
      <p className='text-muted-foreground max-w-2xl text-sm'>
        {t(
          'Discover the most-used models and rising vendors on the platform, updated from live usage data.'
        )}
      </p>
      <ToggleGroup
        value={[period]}
        aria-label={t('Period')}
        onValueChange={(value) => {
          const next = PERIODS.find(({ id }) => id === value[0])
          if (next) onPeriodChange(next.id)
        }}
      >
        {PERIODS.map(({ id, labelKey }) => (
          <ToggleGroupItem key={id} value={id}>
            {t(labelKey)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  )
}
