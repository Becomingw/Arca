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
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { ProductBackdrop } from '@/components/layout/components/product-backdrop'
import { PageTransition } from '@/components/page-transition'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useAuthStore } from '@/stores/auth-store'

import {
  MarketShareSection,
  ModelsSection,
  PulseSection,
  RankingsHero,
} from './components'
import { TeamRankings } from './components/team-rankings'
import { useRankings } from './hooks/use-rankings'
import type { RankingPeriod } from './types'

export function Rankings() {
  const { t } = useTranslation()
  const search = useSearch({ from: '/rankings/' })
  const navigate = useNavigate()
  const userId = useAuthStore((state) =>
    state.auth.accessToken ? state.auth.user?.id : undefined
  )
  const period = search.period ?? 'week'
  const tab =
    userId && (search.tab === 'team' || (!search.tab && !search.period))
      ? 'team'
      : 'models'

  return (
    <PublicLayout showMainContainer={false}>
      <div className='relative'>
        <ProductBackdrop />
        <PageTransition className='relative mx-auto flex w-full max-w-[1280px] flex-col gap-6 px-3 pt-16 pb-10 sm:px-6 sm:pt-20 sm:pb-12 xl:px-8'>
          <h1 className='text-[clamp(1.75rem,4vw,2.5rem)] leading-[1.15] font-bold tracking-tight'>
            {t('Rankings')}
          </h1>
          {userId ? (
            <Tabs
              value={tab}
              className='gap-4'
              onValueChange={(next) => {
                if (next !== 'models' && next !== 'team') return
                void navigate({
                  to: '/rankings',
                  search: (previous) => ({ ...previous, tab: next }),
                })
              }}
            >
              <TabsList aria-label={t('Rankings')}>
                <TabsTrigger value='team'>{t('Team activity')}</TabsTrigger>
                <TabsTrigger value='models'>{t('Model rankings')}</TabsTrigger>
              </TabsList>
              <TabsContent value='team'>
                <TeamRankings userId={userId} />
              </TabsContent>
              <TabsContent value='models'>
                <PlatformRankings period={period} />
              </TabsContent>
            </Tabs>
          ) : (
            <PlatformRankings period={period} />
          )}
        </PageTransition>
      </div>
    </PublicLayout>
  )
}

function PlatformRankings({ period }: { period: RankingPeriod }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const rankingsQuery = useRankings(period)
  const snapshot = rankingsQuery.data?.data
  const message =
    rankingsQuery.error instanceof Error
      ? rankingsQuery.error.message
      : t('Unable to load rankings data')

  return (
    <div className='flex flex-col gap-6'>
      <RankingsHero
        period={period}
        onPeriodChange={(next) => {
          void navigate({
            to: '/rankings',
            search: (previous) => ({ ...previous, period: next }),
          })
        }}
      />
      {rankingsQuery.isLoading && <RankingsLoading />}
      {!rankingsQuery.isLoading && !snapshot && (
        <RankingsError message={message} />
      )}
      {snapshot && (
        <>
          <ModelsSection
            history={snapshot.models_history}
            rows={snapshot.models}
            period={period}
          />
          <MarketShareSection
            history={snapshot.vendor_share_history}
            rows={snapshot.vendors}
            period={period}
          />
          <PulseSection
            movers={snapshot.top_movers}
            droppers={snapshot.top_droppers}
          />
        </>
      )}
    </div>
  )
}

function RankingsLoading() {
  return (
    <div className='space-y-6'>
      <Skeleton className='h-[420px] w-full rounded-xl' />
      <Skeleton className='h-[360px] w-full rounded-xl' />
      <Skeleton className='h-[180px] w-full rounded-xl' />
    </div>
  )
}

function RankingsError(props: { message: string }) {
  const { t } = useTranslation()
  return (
    <div className='bg-card rounded-xl border border-dashed px-6 py-12 text-center'>
      <h2 className='text-foreground text-base font-semibold'>
        {t('Unable to load rankings')}
      </h2>
      <p className='text-muted-foreground mx-auto mt-2 max-w-md text-sm'>
        {props.message}
      </p>
    </div>
  )
}
