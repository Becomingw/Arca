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
import { Link } from '@tanstack/react-router'
import {
  ArrowUpRight,
  Boxes,
  ClipboardList,
  KeyRound,
  MessageSquare,
  Radio,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { Main } from '@/components/layout/components/main'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { PersonalEntitlementsCard } from '@/features/entitlements/components/personal-entitlements-card'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'
import { getUserModels } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

export function Workspace() {
  const { t } = useTranslation()
  const isAdmin = useAuthStore(
    (state) => (state.auth.user?.role ?? ROLE.GUEST) >= ROLE.ADMIN
  )

  return (
    <Main>
      <div className='min-h-0 flex-1 overflow-y-auto px-3 py-7 sm:px-6 sm:py-10 xl:px-8'>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-7 sm:gap-9'>
          <header className='space-y-5'>
            <div className='space-y-2'>
              <h1 className='text-[clamp(1.75rem,4vw,2.75rem)] leading-[1.15] font-bold tracking-tight'>
                {t('Workspace')}
              </h1>
              <p className='text-muted-foreground max-w-prose text-sm sm:text-base'>
                {t(
                  'Manage API keys, review usage, and check your model access.'
                )}
              </p>
            </div>
            <nav
              aria-label={t('Quick actions')}
              className='flex flex-wrap gap-2'
            >
              <Link to='/keys' className={buttonVariants({ size: 'lg' })}>
                <KeyRound data-icon='inline-start' aria-hidden />
                {t('API Keys')}
              </Link>
              <Link
                to='/usage-logs/$section'
                params={{ section: 'common' }}
                className={buttonVariants({ size: 'lg', variant: 'outline' })}
              >
                <ClipboardList data-icon='inline-start' aria-hidden />
                {t('Usage Logs')}
              </Link>
              <Link
                to='/feedback'
                className={buttonVariants({ size: 'lg', variant: 'outline' })}
              >
                <MessageSquare data-icon='inline-start' aria-hidden />
                {t('feedback.title', {
                  defaultValue: 'Discussions & feedback',
                })}
              </Link>
              {isAdmin && (
                <Link
                  to='/dashboard'
                  className={buttonVariants({ size: 'lg', variant: 'outline' })}
                >
                  <Radio data-icon='inline-start' aria-hidden />
                  {t('Administration')}
                </Link>
              )}
            </nav>
          </header>
          <div className='grid items-start gap-4 sm:gap-5 @4xl/content:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]'>
            <WorkspaceModels />
            <section aria-label={t('My entitlements')} className='min-w-0'>
              <PersonalEntitlementsCard compact />
            </section>
          </div>
        </div>
      </div>
    </Main>
  )
}

function WorkspaceModels() {
  const { t } = useTranslation()
  const userID = useAuthStore((state) => state.auth.user?.id)
  const canOpenCatalog = useTopNavLinks().some(
    (link) => link.href === '/pricing'
  )
  const models = useQuery({
    queryKey: ['user-models', userID],
    queryFn: async () => requireServerSuccess(await getUserModels()),
    select: (response) => response.data ?? [],
    enabled: userID != null,
    meta: { errorToast: false },
  })

  return (
    <Card className='min-w-0'>
      <CardHeader className='flex flex-row flex-wrap items-center justify-between gap-x-3 gap-y-2'>
        <h2 className='inline-flex min-w-0 items-center gap-2 text-base font-semibold'>
          <Boxes className='text-primary size-4 shrink-0' aria-hidden />
          {t('Available Models')}
        </h2>
        {canOpenCatalog && (
          <CardAction>
            <Link
              to='/pricing'
              className={buttonVariants({ size: 'sm', variant: 'ghost' })}
            >
              {t('Model Square')}
              <ArrowUpRight data-icon='inline-end' aria-hidden />
            </Link>
          </CardAction>
        )}
      </CardHeader>
      <CardContent>
        {models.isPending && (
          <div role='status' className='space-y-2'>
            <span className='sr-only'>{t('Loading')}</span>
            <Skeleton className='h-7 w-3/4' />
            <Skeleton className='h-7 w-1/2' />
          </div>
        )}
        {models.isError && (
          <div role='alert' className='flex flex-wrap items-center gap-3'>
            <p className='text-muted-foreground'>{t('Loading failed')}</p>
            <Button
              size='sm'
              variant='outline'
              onClick={() => void models.refetch()}
            >
              {t('Retry')}
            </Button>
          </div>
        )}
        {models.data?.length === 0 && (
          <EmptyState
            icon={Boxes}
            title={t('No models available')}
            className='bg-muted/25 min-h-36 rounded-lg'
          />
        )}
        {!!models.data?.length && (
          <ul className='grid grid-cols-[repeat(auto-fit,minmax(min(100%,12rem),1fr))] gap-2'>
            {models.data.map((model) => (
              <li
                key={model}
                className='bg-muted/45 min-w-0 rounded-lg px-3 py-2 font-mono text-xs leading-relaxed [overflow-wrap:anywhere]'
              >
                {model}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
