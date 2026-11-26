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
import { Link, useSearch } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { useStatus } from '@/hooks/use-status'

import { AuthLayout } from '../auth-layout'
import { TermsFooter } from '../components/terms-footer'
import { UserAuthForm } from './components/user-auth-form'

export function SignIn() {
  const { t } = useTranslation()
  const { redirect } = useSearch({ from: '/(auth)/sign-in' })
  const { status } = useStatus()

  return (
    <AuthLayout>
      <div className='flex w-full flex-col gap-6'>
        <div className='flex flex-col gap-2 text-center'>
          <h1 className='text-xl font-semibold tracking-tight'>
            {t('Sign in')}
          </h1>
          {!status?.self_use_mode_enabled &&
            status?.register_enabled !== false && (
              <p className='text-muted-foreground text-sm'>
                {t("Don't have an account?")}{' '}
                <Link
                  to='/sign-up'
                  className='hover:text-primary font-medium underline underline-offset-4'
                >
                  {t('Sign up')}
                </Link>
                .
              </p>
            )}
        </div>

        <UserAuthForm
          className='[&_[data-slot=input]]:h-11 [&_button[type=submit]]:h-11'
          redirectTo={redirect}
        />

        <TermsFooter
          variant='sign-in'
          status={status}
          className='text-center'
        />
      </div>
    </AuthLayout>
  )
}
