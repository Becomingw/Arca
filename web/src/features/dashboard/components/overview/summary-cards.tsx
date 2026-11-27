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
import { useId, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StaggerContainer, StaggerItem } from '@/components/page-transition'
import { getUserQuotaDates } from '@/features/dashboard/api'
import { useSummaryCardsConfig } from '@/features/dashboard/hooks/use-dashboard-config'
import type {
  DashboardUsageScope,
  DashboardUsageSummary,
  QuotaDataItem,
} from '@/features/dashboard/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

import { StatCard } from '../ui/stat-card'

const SUMMARY_SPARKLINE_BUCKETS = 12

interface SummaryCardsProps {
  scope: DashboardUsageScope
  userId?: number
  summary?: DashboardUsageSummary
  summaryLoading: boolean
  summaryError: boolean
}

function getBucketIndex(
  timestamp: number,
  start: number,
  end: number,
  bucketCount: number
): number {
  if (end <= start) return 0
  const ratio = (timestamp - start) / (end - start)
  return Math.min(bucketCount - 1, Math.max(0, Math.floor(ratio * bucketCount)))
}

function buildSummarySparklines(
  data: QuotaDataItem[],
  start: number,
  end: number
) {
  const tokens = Array.from({ length: SUMMARY_SPARKLINE_BUCKETS }, () => 0)
  const requests = Array.from({ length: SUMMARY_SPARKLINE_BUCKETS }, () => 0)
  let totalTokens = 0

  for (const item of data) {
    const timestamp = Number(item.created_at) || start
    const index = getBucketIndex(
      timestamp,
      start,
      end,
      SUMMARY_SPARKLINE_BUCKETS
    )
    const tokenCount = item.token_used ?? 0
    tokens[index] += tokenCount
    totalTokens += tokenCount
    requests[index] += item.count ?? 0
  }

  return { tokens, requests, totalTokens }
}

export function SummaryCards(props: SummaryCardsProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const headingId = useId()
  const usageTrendQuery = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'summary-sparklines',
      props.scope,
      props.userId,
    ],
    queryFn: async ({ signal }) => {
      const end = Math.floor(Date.now() / 1000)
      // Quota data is stored by the start of each UTC hour.
      const start = end - (end % 3600) - 23 * 3600
      const result = requireServerSuccess(
        await getUserQuotaDates(
          { start_timestamp: start, end_timestamp: end, default_time: 'hour' },
          props.scope === 'site',
          signal
        )
      )
      return { rows: result.data, start, end }
    },
    enabled: Boolean(props.userId),
    staleTime: 60_000,
    refetchOnWindowFocus: true,
  })
  const trend = useMemo(() => {
    const data = usageTrendQuery.data
    return data && buildSummarySparklines(data.rows, data.start, data.end)
  }, [usageTrendQuery.data])

  const items = useSummaryCardsConfig({
    recentTokensDisplay: formatNumber(trend?.totalTokens, locale),
    totalTokensDisplay: formatNumber(props.summary?.total_tokens, locale),
    requestCountDisplay: formatNumber(props.summary?.request_count, locale),
  }).map((config, index) => {
    const tones = ['accent-1', 'accent-2', 'accent-3'] as const

    const recent = config.key === 'recentTokens'

    return {
      key: config.key,
      title: config.title,
      value: config.value,
      desc: config.description,
      icon: config.icon,
      tone: tones[index] ?? 'accent-3',
      sparkline: recent ? trend?.tokens : trend?.requests,
      loading: recent ? usageTrendQuery.isPending : props.summaryLoading,
      error: recent ? usageTrendQuery.isError : props.summaryError,
    }
  })

  return (
    <section
      aria-labelledby={headingId}
      className='flex flex-col gap-3 sm:gap-4'
    >
      <h3
        id={headingId}
        className='text-base font-semibold tracking-tight sm:text-lg'
      >
        {props.scope === 'site' ? t('Sitewide usage') : t('Your usage')}
      </h3>
      <StaggerContainer className='grid grid-cols-1 gap-2 min-[360px]:grid-cols-3 sm:gap-4'>
        {items.map((it) => (
          <StaggerItem
            key={it.key}
            className='border-border/75 bg-card/85 min-w-0 rounded-xl border p-3 shadow-xs sm:p-4'
          >
            <StatCard
              title={it.title}
              value={it.value}
              description={it.desc}
              icon={it.icon}
              tone={it.tone}
              sparkline={it.key === 'totalTokens' ? undefined : it.sparkline}
              sparklineVariant='line'
              loading={it.loading}
              error={it.error}
              compactMobile
            />
          </StaggerItem>
        ))}
      </StaggerContainer>
    </section>
  )
}
