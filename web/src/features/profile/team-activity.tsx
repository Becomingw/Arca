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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout/components/section-page-layout'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { getArcaTeamActivity } from './arca-profile-api'

export function TeamActivity() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [days, setDays] = useState<7 | 30>(7)
  const userId = useAuthStore((state) => state.auth.user?.id)
  const result = useQuery({
    queryKey: ['team-activity', userId, days],
    queryFn: ({ signal }) => getArcaTeamActivity(days, signal),
    enabled: Boolean(userId),
    staleTime: 60_000,
    meta: { errorToast: false },
  })
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Team activity')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Card className='mx-auto w-full max-w-4xl'>
          <CardHeader>
            <CardTitle>{t('Estimated active time')}</CardTitle>
            <CardDescription>
              {t(
                'Each five-minute UTC window with a successful request counts once.'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className='flex flex-col gap-4'>
            <ToggleGroup
              value={[String(days)]}
              aria-label={t('Period')}
              onValueChange={(value) => setDays(value[0] === '30' ? 30 : 7)}
            >
              <ToggleGroupItem value='7'>{t('Last 7 days')}</ToggleGroupItem>
              <ToggleGroupItem value='30'>{t('Last 30 days')}</ToggleGroupItem>
            </ToggleGroup>
            {result.isPending && <Skeleton className='h-32 w-full' />}
            {result.isError && (
              <Button
                type='button'
                variant='outline'
                className='self-start'
                onClick={() => void result.refetch()}
              >
                {t('Retry')}
              </Button>
            )}
            {result.data?.suppressed && (
              <p className='text-muted-foreground text-sm'>
                {t('Fewer than three members were active during this period.')}
              </p>
            )}
            {result.data && !result.data.suppressed && (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Rank')}</TableHead>
                    <TableHead>{t('Member')}</TableHead>
                    <TableHead className='text-right'>{t('Minutes')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {result.data.members.map((member) => (
                    <TableRow key={member.id}>
                      <TableCell>{member.rank}</TableCell>
                      <TableCell>{member.label}</TableCell>
                      <TableCell className='text-right'>
                        {formatNumber(member.estimated_minutes, locale)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
