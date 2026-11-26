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

import { toIntlLocale } from '@/i18n/languages'
import { formatTimestampToDate } from '@/lib/format'

import type { UsageGrantScope } from '../types'

export function RedemptionGrantSummary({ grant }: { grant?: UsageGrantScope }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  let scope = t('All permitted models')
  if (grant?.family) {
    scope = t('Family {{family}}', { family: grant.family })
  } else if (grant?.models?.length) {
    scope = grant.models.join(', ')
  }
  const details = []
  if (grant?.starts_at) {
    details.push(
      t('Starts {{date}}', { date: formatTimestampToDate(grant.starts_at) })
    )
  }
  if (grant?.ends_at) {
    details.push(
      t('Ends {{date}}', { date: formatTimestampToDate(grant.ends_at) })
    )
  }
  if (grant?.valid_for_seconds) {
    const hours = new Intl.NumberFormat(locale, {
      maximumFractionDigits: 4,
    }).format(grant.valid_for_seconds / 3600)
    details.push(t('{{hours}} h after activation', { hours }))
  }

  return (
    <div className='min-w-0 space-y-0.5 text-xs'>
      <div className='truncate font-medium' title={scope}>
        {scope}
      </div>
      <div className='text-muted-foreground'>
        {details.join(' · ') || t('No time limit')}
      </div>
    </div>
  )
}
