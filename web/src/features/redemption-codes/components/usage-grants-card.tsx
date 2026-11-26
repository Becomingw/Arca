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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toIntlLocale } from '@/i18n/languages'
import { api } from '@/lib/api'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { formatUsageUnits } from '../lib/redemption-form'
import type { ApiResponse, UsageGrant } from '../types'

export function UsageGrantsCard() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const formatter = new Intl.DateTimeFormat(locale, {
    dateStyle: 'medium',
    timeStyle: 'short',
  })
  const session = useAuthStore((state) => state.auth.session?.sid)
  const queryClient = useQueryClient()
  const queryKey = ['usage-grants', session] as const
  const [code, setCode] = useState('')
  const grants = useQuery({
    queryKey,
    enabled: Boolean(session),
    queryFn: async ({ signal }) => {
      const response = await api.get<ApiResponse<UsageGrant[]>>(
        '/api/arca/usage-grants',
        { signal, skipErrorHandler: true }
      )
      return requireServerSuccess(response.data).data ?? []
    },
    meta: { errorToast: false },
  })
  const redeem = useMutation({
    mutationFn: async (value: string) => {
      if (!session || useAuthStore.getState().auth.session?.sid !== session) {
        throw new Error(t('Session expired!'))
      }
      const response = await api.post<ApiResponse<UsageGrant>>(
        '/api/arca/usage-grants/redeem',
        { code: value },
        {
          skipErrorHandler: true,
          singleUseAuthorization: true,
          requestSessionID: session,
        }
      )
      return requireServerSuccess(response.data).data
    },
    onSuccess: async () => {
      setCode('')
      await queryClient.invalidateQueries({ queryKey })
    },
  })
  const error = redeem.error ?? grants.error

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Temporary usage grants')}</CardTitle>
        <CardDescription>
          {t('Use a code issued by an administrator to add temporary usage.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <form
          className='flex flex-wrap items-end gap-2'
          onSubmit={(event) => {
            event.preventDefault()
            if (code.trim() && !redeem.isPending) redeem.mutate(code.trim())
          }}
        >
          <div className='flex min-w-40 flex-1 flex-col gap-2'>
            <Label htmlFor='usage-grant-code'>{t('Redemption Code')}</Label>
            <Input
              id='usage-grant-code'
              value={code}
              onChange={(event) => setCode(event.target.value)}
              maxLength={40}
              autoComplete='off'
              spellCheck={false}
            />
          </div>
          <Button type='submit' disabled={!code.trim() || redeem.isPending}>
            {t('Redeem usage')}
          </Button>
        </form>
        {error && (
          <Alert variant='destructive'>{getServerErrorMessage(error)}</Alert>
        )}
        {redeem.isSuccess && <p role='status'>{t('Usage grant redeemed')}</p>}
        {!!grants.data?.length && (
          <ul className='divide-border divide-y'>
            {grants.data.map((grant) => (
              <li
                key={grant.id}
                className='flex flex-wrap justify-between gap-2 py-3 text-sm'
              >
                <div className='flex min-w-0 flex-col gap-1'>
                  <strong className='break-all'>{grant.name}</strong>
                  <p className='text-muted-foreground break-all'>
                    {grant.family
                      ? t('Model family {{family}}', { family: grant.family })
                      : grant.models?.join(' · ') || t('All permitted models')}
                  </p>
                  {grant.starts_at > Math.floor(Date.now() / 1000) && (
                    <p className='text-muted-foreground'>
                      {t('Available from {{date}}', {
                        date: formatter.format(grant.starts_at * 1000),
                      })}
                    </p>
                  )}
                  {grant.ends_at > 0 && (
                    <p className='text-muted-foreground'>
                      {t('Use before {{date}}', {
                        date: formatter.format(grant.ends_at * 1000),
                      })}
                    </p>
                  )}
                </div>
                <span className='tabular-nums'>
                  {t('{{amount}} units remaining', {
                    amount: formatUsageUnits(
                      grant.remaining_micros / 1_000_000,
                      locale
                    ),
                  })}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
