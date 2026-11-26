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
  Boxes,
  ClipboardList,
  KeyRound,
  MessageSquare,
  Radio,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Main } from '@/components/layout/components/main'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PersonalEntitlementsCard } from '@/features/entitlements/components/personal-entitlements-card'
import { getUserModels } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

export function Workspace() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const models = useQuery({
    queryKey: ['user-models'],
    queryFn: async () => requireServerSuccess(await getUserModels()),
    select: (response) => response.data ?? [],
    enabled: Boolean(user),
  })

  return (
    <Main fluid={false}>
      <div className='min-h-0 flex-1 overflow-y-auto px-4 py-5 sm:px-6 sm:py-8'>
        <div className='mx-auto flex w-full max-w-6xl flex-col gap-5'>
          <h1 className='text-2xl font-semibold tracking-tight'>
            {t('Workspace')}
          </h1>
          <div className='flex flex-wrap gap-2'>
            <Button variant='outline' render={<Link to='/keys' />}>
              <KeyRound data-icon='inline-start' aria-hidden='true' />
              {t('API Keys')}
            </Button>
            <Button
              variant='outline'
              render={
                <Link
                  to='/usage-logs/$section'
                  params={{ section: 'common' }}
                />
              }
            >
              <ClipboardList data-icon='inline-start' aria-hidden='true' />
              {t('Usage Logs')}
            </Button>
            <Button variant='outline' render={<Link to='/feedback' />}>
              <MessageSquare data-icon='inline-start' aria-hidden='true' />
              {t('feedback.title', { defaultValue: 'Discussions & feedback' })}
            </Button>
            {(user?.role ?? ROLE.GUEST) >= ROLE.ADMIN && (
              <Button variant='outline' render={<Link to='/dashboard' />}>
                <Radio data-icon='inline-start' aria-hidden='true' />
                {t('Administration')}
              </Button>
            )}
          </div>
          <Card>
            <CardHeader>
              <CardTitle className='flex items-center gap-2'>
                <Boxes className='size-4' aria-hidden='true' />
                {t('Available Models')}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {models.isPending && <p role='status'>{t('Loading')}</p>}
              {models.isError && <p role='alert'>{t('Loading failed')}</p>}
              {models.data?.length === 0 && (
                <p className='text-muted-foreground text-sm'>
                  {t('No models available')}
                </p>
              )}
              {Boolean(models.data?.length) && (
                <ul className='flex flex-wrap gap-2'>
                  {models.data?.map((model) => (
                    <li key={model}>
                      <Badge
                        variant='secondary'
                        className='h-auto break-all whitespace-normal'
                      >
                        {model}
                      </Badge>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
          <PersonalEntitlementsCard compact />
        </div>
      </div>
    </Main>
  )
}
