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
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { ProductBackdrop } from '@/components/layout/components/product-backdrop'
import { PageTransition } from '@/components/page-transition'
import { buttonVariants } from '@/components/ui/button'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'
import { cn } from '@/lib/utils'

import { ClientCarousel } from './client-carousel'
import {
  AccessSection,
  EntitlementsSection,
  LinksSection,
  OperationsSection,
} from './feature-sections'

type ArcaHomeProps = { isAuthenticated: boolean }

export function ArcaHome(props: ArcaHomeProps) {
  const { t } = useTranslation()
  const modelSquare = useTopNavLinks().find((link) => link.href === '/pricing')
  return (
    <PublicLayout showMainContainer={false}>
      <main className='relative isolate' id='arca-home'>
        <ProductBackdrop />
        <PageTransition className='relative mx-auto flex w-full max-w-[1280px] flex-col gap-12 px-3 pt-16 pb-12 sm:gap-16 sm:px-6 sm:pt-20 sm:pb-16 xl:px-8'>
          <section
            className='mx-auto flex w-full max-w-3xl flex-col items-center gap-8 pt-8 text-center sm:gap-10 sm:pt-12'
            aria-labelledby='arca-title'
          >
            <div>
              <h1
                id='arca-title'
                className='text-[clamp(3.5rem,10vw,5.25rem)] leading-none font-bold tracking-tight'
              >
                Arca
              </h1>
              <p className='text-muted-foreground mt-3 text-base sm:mt-4 sm:text-lg'>
                {t('Connect your AI workflows')}
              </p>
              <div className='mt-6 flex flex-wrap justify-center gap-2.5'>
                <Link
                  to={props.isAuthenticated ? '/workspace' : '/sign-in'}
                  search={
                    props.isAuthenticated
                      ? undefined
                      : { redirect: '/workspace' }
                  }
                  className={cn(
                    buttonVariants({ size: 'lg' }),
                    'h-10 px-4 has-data-[icon=inline-end]:pr-3'
                  )}
                >
                  {props.isAuthenticated ? t('Open workspace') : t('Sign in')}
                  <ArrowRight data-icon='inline-end' aria-hidden='true' />
                </Link>
                {modelSquare && (
                  <Link
                    to={modelSquare.requiresAuth ? '/sign-in' : '/pricing'}
                    search={
                      modelSquare.requiresAuth
                        ? { redirect: '/pricing' }
                        : undefined
                    }
                    className={cn(
                      buttonVariants({ size: 'lg', variant: 'outline' }),
                      'bg-background/70 h-10 px-4'
                    )}
                  >
                    {modelSquare.title}
                  </Link>
                )}
              </div>
            </div>
            <ClientCarousel />
          </section>
          <nav
            className='border-border/70 text-muted-foreground flex flex-wrap items-center gap-x-6 gap-y-2 border-b pb-4 text-sm'
            aria-label={t('On this page')}
          >
            <a className='hover:text-foreground' href='#access'>
              {t('Client access')}
            </a>
            <a className='hover:text-foreground' href='#entitlements'>
              {t('Entitlements')}
            </a>
            <a className='hover:text-foreground' href='#operations'>
              {t('Operations')}
            </a>
            <a className='hover:text-foreground' href='#project-links'>
              {t('Project links')}
            </a>
          </nav>
          <AccessSection />
          <div className='grid gap-12 lg:grid-cols-2 lg:gap-6'>
            <EntitlementsSection />
            <OperationsSection isAuthenticated={props.isAuthenticated} />
          </div>
          <LinksSection />
        </PageTransition>
      </main>
      <Footer name='Arca' logo='/brand/arca.svg' />
    </PublicLayout>
  )
}
