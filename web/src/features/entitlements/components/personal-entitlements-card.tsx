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

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

import { usePersonalEntitlements } from '../api'
import { EntitlementSnapshot } from './entitlement-snapshot'

export function PersonalEntitlementsCard({
  compact = false,
}: {
  compact?: boolean
}) {
  const { t } = useTranslation()
  const snapshot = usePersonalEntitlements()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('My entitlements')}</CardTitle>
        <CardDescription>
          {t('Usage is based on gateway pricing in USD.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {snapshot.isPending && <Skeleton className='h-20 w-full' />}
        {snapshot.isError && (
          <Button
            type='button'
            variant='outline'
            onClick={() => void snapshot.refetch()}
          >
            {t('Retry')}
          </Button>
        )}
        {snapshot.data && (
          <EntitlementSnapshot value={snapshot.data} compact={compact} />
        )}
      </CardContent>
    </Card>
  )
}
