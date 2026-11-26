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

import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'

import type { EntitlementSnapshot as Snapshot } from '../api'

export function EntitlementSnapshot({
  value,
  compact = false,
}: {
  value: Snapshot
  compact?: boolean
}) {
  const { t, i18n } = useTranslation()
  const usage = new Intl.NumberFormat(
    toIntlLocale(i18n.resolvedLanguage || i18n.language),
    { maximumFractionDigits: 6 }
  )
  const scopes = {
    total: t('Total'),
    family: t('Model family'),
    model: t('Model'),
  }
  const periods: Record<string, string> = {
    rolling_5h: t('5 hours'),
    day: t('Daily'),
    week: t('Weekly'),
    month: t('Monthly'),
    daily: t('Daily'),
    weekly: t('Weekly'),
    monthly: t('Monthly'),
  }
  const access = {
    '': t('Inherit'),
    inherit: t('Inherit'),
    unrestricted: t('Unrestricted'),
    specific: t('Selected models'),
    deny_all: t('No models'),
  }
  const allWindows = value.windows ?? []
  const totals = allWindows.filter((entry) => entry.scope === 'total')
  const windows = compact && totals.length > 0 ? totals : allWindows
  if (!value.enabled) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('Budget enforcement is inactive.')}
      </p>
    )
  }
  if (
    value.source === 'none' &&
    windows.length === 0 &&
    !value.allocation.unlimited &&
    value.model_access.mode === 'unrestricted'
  ) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('No subscription assigned.')}
      </p>
    )
  }

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-wrap gap-2'>
        {value.subscription_name && <Badge>{value.subscription_name}</Badge>}
        <Badge variant='outline'>{access[value.model_access.mode]}</Badge>
        {value.allocation.unlimited && (
          <Badge variant='outline'>{t('Unlimited')}</Badge>
        )}
        <Badge variant='outline'>{value.timezone}</Badge>
      </div>
      {value.subscription_name && (
        <p className='text-muted-foreground text-sm'>
          {value.source === 'user_subscription'
            ? t('Assigned directly to this user')
            : t('Inherited from the user group')}
        </p>
      )}
      {value.group_ids.length > 0 && (
        <p className='text-muted-foreground text-sm'>
          {t('Organizations')}: {value.group_ids.join(', ')}
        </p>
      )}
      {!compact && (value.model_access.models?.length ?? 0) > 0 && (
        <p className='text-muted-foreground text-sm break-words'>
          {t('Allowed models')}: {value.model_access.models?.join(', ')}
        </p>
      )}
      {!compact && value.key_limits_enabled && (
        <p className='text-muted-foreground text-sm break-words'>
          {t('API key model limits')}:{' '}
          {value.key_models?.join(', ') || t('No models')}
        </p>
      )}
      {windows.length > 0 ? (
        <div className='overflow-x-auto'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Usage allowance')}</TableHead>
                <TableHead>{t('Period')}</TableHead>
                <TableHead className='text-right'>{t('Used')}</TableHead>
                <TableHead className='text-right'>{t('Remaining')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {windows.map((window) => (
                <TableRow
                  key={`${window.scope}:${window.name}:${window.period}:${window.start}`}
                >
                  <TableCell>
                    {window.scope === 'total'
                      ? scopes.total
                      : `${scopes[window.scope]} · ${value.model_families?.[window.name]?.name || window.name}`}
                  </TableCell>
                  <TableCell>
                    {periods[window.period] || window.period}
                  </TableCell>
                  <TableCell className='text-right tabular-nums'>
                    {usage.format(window.used_micros / 1e6)} /{' '}
                    {usage.format(window.limit_micros / 1e6)}
                  </TableCell>
                  <TableCell className='text-right tabular-nums'>
                    {usage.format(window.remaining_micros / 1e6)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : (
        !value.allocation.unlimited && (
          <p className='text-muted-foreground text-sm'>
            {t('No budget limits defined.')}
          </p>
        )
      )}
    </div>
  )
}
