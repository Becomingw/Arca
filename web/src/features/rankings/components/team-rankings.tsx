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

import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
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
import { getArcaTeamActivity } from '@/features/profile/arca-profile-api'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

const hourLabel = (seconds: number) =>
  `${String(seconds / 3600).padStart(2, '0')}:00`

export function TeamRankings({ userId }: { userId: number }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [days, setDays] = useState<7 | 30>(7)
  const result = useQuery({
    queryKey: ['team-activity', userId, days],
    queryFn: ({ signal }) => getArcaTeamActivity(days, signal),
    staleTime: 60_000,
    meta: { errorToast: false },
  })
  const report = result.data

  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'See how the team uses the gateway and which models are most common.'
          )}
        </p>
        <ToggleGroup
          value={[String(days)]}
          aria-label={t('Period')}
          onValueChange={(value) => {
            if (value[0]) setDays(value[0] === '30' ? 30 : 7)
          }}
        >
          <ToggleGroupItem value='7'>{t('Last 7 days')}</ToggleGroupItem>
          <ToggleGroupItem value='30'>{t('Last 30 days')}</ToggleGroupItem>
        </ToggleGroup>
      </div>
      {result.isPending && <Skeleton className='h-48 w-full' />}
      {result.isError && (
        <Alert
          variant='destructive'
          className='flex flex-wrap items-center justify-between gap-3'
        >
          <p>{t('Unable to load rankings data')}</p>
          <Button
            type='button'
            variant='outline'
            onClick={() => void result.refetch()}
          >
            {t('Retry')}
          </Button>
        </Alert>
      )}
      {report?.suppressed && (
        <Card>
          <CardHeader>
            <CardTitle>{t('Team activity')}</CardTitle>
            <CardDescription>
              {t('Fewer than three members were active during this period.')}
            </CardDescription>
          </CardHeader>
        </Card>
      )}
      {report && !report.suppressed && (
        <>
          <Card>
            <CardHeader>
              <CardTitle>{t('Member activity')}</CardTitle>
              <CardDescription className='flex flex-col gap-1'>
                <span>
                  {t(
                    'Each five-minute UTC window with a successful request counts once.'
                  )}
                </span>
                <span>
                  {t(
                    'Estimated activity includes automated use and does not measure working hours.'
                  )}
                </span>
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Rank')}</TableHead>
                    <TableHead>{t('Member')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Estimated active time')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Active days')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Longest active streak')}
                    </TableHead>
                    {report.usage_available && (
                      <>
                        <TableHead className='text-right'>
                          {t('Requests')}
                        </TableHead>
                        <TableHead className='text-right'>
                          {t('Tokens')}
                        </TableHead>
                      </>
                    )}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {report.members.map((member, index) => (
                    <TableRow key={member.id}>
                      <TableCell className='tabular-nums'>
                        {formatNumber(member.rank, locale)}
                      </TableCell>
                      <TableCell className='font-medium'>
                        {member.anonymous
                          ? t('Anonymous member {{number}}', {
                              number: formatNumber(index + 1, locale),
                            })
                          : member.label}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            member.active_today ? 'secondary' : 'outline'
                          }
                        >
                          {member.active_today
                            ? t('Active today (UTC)')
                            : t('Active earlier in this period')}
                        </Badge>
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {t('{{minutes}} min', {
                          minutes: formatNumber(
                            member.estimated_minutes,
                            locale
                          ),
                        })}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatNumber(member.active_days, locale)}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatNumber(member.longest_streak, locale)}
                      </TableCell>
                      {report.usage_available && (
                        <>
                          <TableCell className='text-right tabular-nums'>
                            {member.requests == null
                              ? '—'
                              : formatNumber(member.requests, locale)}
                          </TableCell>
                          <TableCell className='text-right tabular-nums'>
                            {member.tokens == null
                              ? '—'
                              : formatNumber(member.tokens, locale)}
                          </TableCell>
                        </>
                      )}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
          <div className='grid min-w-0 items-start gap-4 lg:grid-cols-2'>
            <Card className='min-w-0'>
              <CardHeader>
                <CardTitle>{t('Popular team models')}</CardTitle>
                <CardDescription>
                  {t(
                    'Ranked by recorded requests from at least three members per model.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent>
                {!report.usage_available && (
                  <p className='text-muted-foreground'>
                    {t('Usage aggregation is disabled.')}
                  </p>
                )}
                {report.usage_available &&
                  report.popular_models.length === 0 && (
                    <p className='text-muted-foreground'>
                      {t(
                        'No models meet the minimum participation for this period.'
                      )}
                    </p>
                  )}
                {report.usage_available && report.popular_models.length > 0 && (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('Model')}</TableHead>
                        <TableHead className='text-right'>
                          {t('Requests')}
                        </TableHead>
                        <TableHead className='text-right'>
                          {t('Tokens')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {report.popular_models.map((entry) => (
                        <TableRow key={entry.model}>
                          <TableCell className='max-w-56 font-medium break-words whitespace-normal'>
                            {entry.model}
                          </TableCell>
                          <TableCell className='text-right tabular-nums'>
                            {formatNumber(entry.requests, locale)}
                          </TableCell>
                          <TableCell className='text-right tabular-nums'>
                            {formatNumber(entry.tokens, locale)}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </CardContent>
            </Card>
            <Card className='min-w-0'>
              <CardHeader>
                <CardTitle>{t('Team activity by time of day')}</CardTitle>
                <CardDescription>
                  {t(
                    'Six-hour UTC intervals appear when at least three members were active.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent>
                {report.activity_bands.length === 0 ? (
                  <p className='text-muted-foreground'>
                    {t(
                      'No time intervals meet the minimum participation for this period.'
                    )}
                  </p>
                ) : (
                  <dl className='divide-border divide-y'>
                    {report.activity_bands.map((band) => (
                      <div
                        key={band.start_second}
                        className='flex justify-between gap-3 py-2'
                      >
                        <dt className='tabular-nums'>
                          {hourLabel(band.start_second)}–
                          {hourLabel(band.start_second + 21600)}
                        </dt>
                        <dd className='text-muted-foreground tabular-nums'>
                          {t('{{windows}} active windows', {
                            windows: formatNumber(band.windows, locale),
                          })}
                        </dd>
                      </div>
                    ))}
                  </dl>
                )}
              </CardContent>
            </Card>
          </div>
        </>
      )}
    </div>
  )
}
