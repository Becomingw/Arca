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
import { Link } from '@tanstack/react-router'
import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'

import './auth-layout.css'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()

  return (
    <div className='arca-auth-layout bg-background text-foreground flex min-h-svh flex-col'>
      <header className='border-border bg-background border-b px-4 py-3 sm:px-8'>
        <Link
          to='/'
          className={buttonVariants({
            variant: 'ghost',
            className: 'min-h-10',
          })}
        >
          <ArrowLeft aria-hidden='true' data-icon='inline-start' />
          {t('Back to Home')}
        </Link>
      </header>
      <main className='flex flex-1 items-center justify-center px-4 py-8 sm:px-6 sm:py-12'>
        <Card className='w-full max-w-md gap-6 py-6 shadow-lg sm:py-8'>
          <CardHeader className='justify-items-center gap-3 px-5 sm:px-8'>
            <img
              src='/brand/arca.svg'
              alt=''
              width={56}
              height={60}
              className='h-auto w-14'
            />
            <span className='font-serif text-3xl leading-none tracking-tight'>
              Arca
            </span>
          </CardHeader>
          <CardContent className='min-w-0 px-5 sm:px-8'>{children}</CardContent>
        </Card>
      </main>
    </div>
  )
}
