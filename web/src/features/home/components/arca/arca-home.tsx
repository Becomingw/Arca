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
import { ArrowDown, ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { Button } from '@/components/ui/button'

import { ClientCarousel } from './client-carousel'
import {
  AccessSection,
  EntitlementsSection,
  LinksSection,
  OperationsSection,
} from './feature-sections'

import './arca-home.css'

type ArcaHomeProps = { isAuthenticated: boolean }

export function ArcaHome(props: ArcaHomeProps) {
  const { t } = useTranslation()
  return (
    <div className='arca-public-shell'>
      <PublicLayout
        logo={<img src='/brand/arca.svg' alt='' className='size-full' />}
        siteName='Arca'
        showMainContainer={false}
      >
        <main className='arca-home' id='arca-home'>
          <section className='arca-home-hero' aria-labelledby='arca-title'>
            <nav className='arca-home-index' aria-label={t('On this page')}>
              <a href='#access'>{t('Client access')}</a>
              <a href='#entitlements'>{t('Entitlements')}</a>
              <a href='#operations'>{t('Operations')}</a>
              <a href='#project-links'>{t('Project links')}</a>
            </nav>
            <img
              className='arca-home-mark'
              src='/brand/arca.svg'
              alt=''
              width='154'
              height='167'
              fetchPriority='high'
            />
            <h1 id='arca-title'>Arca</h1>
            <p className='arca-home-intro'>{t('Connect your AI workflows')}</p>
            <ClientCarousel />
            {props.isAuthenticated ? (
              <Button
                size='lg'
                className='min-h-10 px-5'
                render={<Link to='/workspace' />}
              >
                {t('Open workspace')}{' '}
                <ArrowRight data-icon='inline-end' aria-hidden='true' />
              </Button>
            ) : (
              <Button
                size='lg'
                className='min-h-10 px-5'
                render={
                  <Link to='/sign-in' search={{ redirect: '/workspace' }} />
                }
              >
                {t('Sign in')}{' '}
                <ArrowRight data-icon='inline-end' aria-hidden='true' />
              </Button>
            )}
            <a
              className='arca-home-scroll'
              href='#access'
              aria-label={t('Client access')}
            >
              <ArrowDown aria-hidden='true' />
            </a>
          </section>
          <AccessSection />
          <EntitlementsSection />
          <OperationsSection isAuthenticated={props.isAuthenticated} />
          <LinksSection />
        </main>
        <Footer name='Arca' logo='/brand/arca.svg' />
      </PublicLayout>
    </div>
  )
}
