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
import {
  Link,
  useLocation,
  useNavigate,
  useRouter,
} from '@tanstack/react-router'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { LanguageSwitcher } from '@/components/language-switcher'
import { NotificationPopover } from '@/components/notification-popover'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { ThemeSwitch } from '@/components/theme-switch'
import { Button, buttonVariants } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { SystemUpdateAction } from '@/features/system-update/system-update-action'
import { useNotifications } from '@/hooks/use-notifications'
import { useSystemConfig } from '@/hooks/use-system-config'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import type { TopNavLink } from '../types'
import { HeaderLogo } from './header-logo'

const AUTH_PROMPT_SECONDS = 5

type AuthPromptTarget = {
  title: string
  href: string
}

export function PublicHeader({ consoleActive }: { consoleActive: boolean }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const router = useRouter()
  const [scrolled, setScrolled] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)
  const mobileToggle = useRef<HTMLButtonElement>(null)
  const [authPromptTarget, setAuthPromptTarget] =
    useState<AuthPromptTarget | null>(null)
  const [authPromptSecondsLeft, setAuthPromptSecondsLeft] =
    useState(AUTH_PROMPT_SECONDS)
  const isAuthenticated = useAuthStore((state) => !!state.auth.user)
  const {
    systemName,
    logo: systemLogo,
    loading,
    logoLoaded,
  } = useSystemConfig()
  const links = useTopNavLinks()
  const notifications = useNotifications()
  const pathname = useLocation({ select: (location) => location.pathname })

  let authContent = (
    <Link
      to='/sign-in'
      className={buttonVariants({
        size: 'sm',
        className: 'h-8 rounded-lg px-3.5 text-xs font-medium',
      })}
    >
      {t('Sign in')}
    </Link>
  )
  if (isAuthenticated) authContent = <ProfileDropdown />
  if (loading) authContent = <Skeleton className='h-8 w-20 rounded-lg' />

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 20)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  useEffect(
    () =>
      router.subscribe('onResolved', ({ hrefChanged }) => {
        if (!hrefChanged) return
        setMobileOpen(false)
        setAuthPromptTarget(null)
      }),
    [router]
  )

  useEffect(() => {
    if (!mobileOpen) return
    const previousOverflow = document.body.style.overflow
    const desktop = window.matchMedia('(min-width: 64rem)')
    const onDesktop = (event: MediaQueryListEvent) => {
      if (event.matches) setMobileOpen(false)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setMobileOpen(false)
        mobileToggle.current?.focus()
      }
    }
    document.body.style.overflow = 'hidden'
    desktop.addEventListener('change', onDesktop)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      desktop.removeEventListener('change', onDesktop)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [mobileOpen])

  useEffect(() => {
    if (!authPromptTarget) return

    const intervalId = window.setInterval(() => {
      setAuthPromptSecondsLeft((seconds) => Math.max(seconds - 1, 0))
    }, 1000)

    const timeoutId = window.setTimeout(() => {
      const redirect = authPromptTarget.href
      setAuthPromptTarget(null)
      navigate({ to: '/sign-in', search: { redirect } })
    }, AUTH_PROMPT_SECONDS * 1000)

    return () => {
      window.clearInterval(intervalId)
      window.clearTimeout(timeoutId)
    }
  }, [authPromptTarget, navigate])

  const closeAuthPrompt = useCallback(() => {
    setAuthPromptTarget(null)
    setAuthPromptSecondsLeft(AUTH_PROMPT_SECONDS)
  }, [])

  const navigateToSignIn = useCallback(() => {
    const redirect = authPromptTarget?.href || '/'
    setAuthPromptTarget(null)
    navigate({ to: '/sign-in', search: { redirect } })
  }, [authPromptTarget?.href, navigate])

  const handleNavLinkClick = useCallback(
    (
      event: React.MouseEvent<HTMLAnchorElement>,
      link: TopNavLink,
      closeMobile = false
    ) => {
      if (link.disabled) {
        event.preventDefault()
        return
      }

      if (link.requiresAuth) {
        event.preventDefault()
        if (closeMobile) {
          setMobileOpen(false)
        }
        setAuthPromptSecondsLeft(AUTH_PROMPT_SECONDS)
        setAuthPromptTarget({
          title: t(link.title),
          href: link.href,
        })
        return
      }

      if (closeMobile) {
        setMobileOpen(false)
      }
    },
    [t]
  )

  const isCurrent = (href: string) =>
    (href === '/workspace' && consoleActive) ||
    pathname === href ||
    (href !== '/' && pathname.startsWith(`${href}/`))

  return (
    <>
      <header
        id='site-header'
        className={cn(
          'fixed inset-x-0 top-0 z-50 h-[var(--site-header-height)] border-b border-transparent transition-colors motion-reduce:transition-none',
          (scrolled || consoleActive || mobileOpen) &&
            'border-border/70 bg-background/90 backdrop-blur-xl'
        )}
      >
        <div className='mx-auto h-full max-w-7xl px-3 sm:px-6 xl:px-8'>
          <nav
            aria-label={t('Header navigation')}
            className='flex h-full items-center gap-1 sm:gap-2'
          >
            {/* Logo */}
            <div className='@container/system-brand flex min-w-0 flex-1 items-center gap-1 lg:min-w-36'>
              <Link
                to='/'
                activeOptions={{ exact: true }}
                className='group flex min-w-0 items-center gap-2.5'
              >
                <div className='flex size-7 shrink-0 items-center justify-center transition-all duration-300 group-hover:scale-105'>
                  <HeaderLogo
                    src={systemLogo}
                    loading={loading}
                    logoLoaded={logoLoaded}
                    className='size-full rounded-lg object-contain'
                  />
                </div>
                <span
                  className='max-w-48 truncate text-sm font-semibold tracking-tight'
                  title={systemName}
                >
                  {loading ? <Skeleton className='h-4 w-16' /> : systemName}
                </span>
              </Link>
              <SystemUpdateAction presentation='version' />
            </div>

            {/* Desktop nav */}
            <div className='hidden min-w-0 items-center gap-0.5 lg:flex'>
              {links.map((link) => {
                const isActive = isCurrent(link.href)
                if (link.external) {
                  return (
                    <a
                      key={`${link.title}:${link.href}`}
                      href={link.href}
                      title={t(link.title)}
                      target='_blank'
                      rel='noopener noreferrer'
                      aria-disabled={link.disabled}
                      tabIndex={link.disabled ? -1 : undefined}
                      onClick={(event) => handleNavLinkClick(event, link)}
                      className={cn(
                        'text-muted-foreground hover:text-foreground min-w-0 truncate rounded-lg px-3 py-1.5 text-sm font-medium transition-colors duration-200',
                        link.disabled && 'pointer-events-none opacity-50'
                      )}
                    >
                      {t(link.title)}
                    </a>
                  )
                }
                return (
                  <Link
                    key={`${link.title}:${link.href}`}
                    to={link.href}
                    activeOptions={{ exact: true }}
                    aria-current={isActive ? 'page' : undefined}
                    title={t(link.title)}
                    disabled={link.disabled}
                    onClick={(event) => handleNavLinkClick(event, link)}
                    className={cn(
                      'min-w-0 truncate rounded-lg px-3 py-1.5 text-sm font-medium transition-colors duration-200',
                      isActive
                        ? 'text-foreground'
                        : 'text-muted-foreground hover:text-foreground',
                      link.disabled && 'pointer-events-none opacity-50'
                    )}
                  >
                    {t(link.title)}
                  </Link>
                )
              })}
            </div>

            <div className='bg-border/40 mx-1 hidden h-4 w-px shrink-0 lg:block' />
            <div className='flex shrink-0 items-center gap-0.5 sm:gap-1'>
              <div className={cn(!isAuthenticated && 'hidden lg:flex')}>
                <LanguageSwitcher />
              </div>
              <ThemeSwitch />
              <div className={cn(!isAuthenticated && 'hidden lg:flex')}>
                <NotificationPopover
                  open={notifications.popoverOpen}
                  onOpenChange={notifications.setPopoverOpen}
                  unreadCount={notifications.unreadCount}
                  activeTab={notifications.activeTab}
                  onTabChange={notifications.setActiveTab}
                  notice={notifications.notice}
                  announcements={notifications.announcements}
                  loading={notifications.loading}
                />
              </div>
              <div className={cn(!isAuthenticated && 'hidden lg:flex')}>
                {authContent}
              </div>
              <Button
                ref={mobileToggle}
                type='button'
                variant='ghost'
                size='icon'
                className='size-9 lg:hidden'
                onClick={() => setMobileOpen((v) => !v)}
                aria-label={t('Toggle navigation menu')}
                aria-controls='site-navigation-mobile'
                aria-expanded={mobileOpen}
              >
                <div className='relative size-4'>
                  <span
                    className={cn(
                      'absolute inset-x-0 block h-[1.5px] origin-center rounded-full bg-current transition-all duration-300 motion-reduce:transition-none',
                      mobileOpen ? 'top-[7px] rotate-45' : 'top-[3px]'
                    )}
                  />
                  <span
                    className={cn(
                      'absolute inset-x-0 top-[7px] block h-[1.5px] rounded-full bg-current transition-all duration-300 motion-reduce:transition-none',
                      mobileOpen ? 'scale-x-0 opacity-0' : 'opacity-100'
                    )}
                  />
                  <span
                    className={cn(
                      'absolute inset-x-0 block h-[1.5px] origin-center rounded-full bg-current transition-all duration-300 motion-reduce:transition-none',
                      mobileOpen ? 'top-[7px] -rotate-45' : 'top-[11px]'
                    )}
                  />
                </div>
              </Button>
            </div>
          </nav>
        </div>
      </header>

      {/* Mobile full-screen overlay */}
      <div
        id='site-navigation-mobile'
        aria-hidden={!mobileOpen}
        inert={!mobileOpen}
        className={cn(
          'bg-background/98 fixed inset-0 z-40 overflow-y-auto overscroll-contain backdrop-blur-2xl transition-opacity motion-reduce:transition-none lg:pointer-events-none lg:hidden',
          mobileOpen
            ? 'pointer-events-auto opacity-100'
            : 'pointer-events-none opacity-0'
        )}
      >
        <div className='flex min-h-full flex-col justify-between gap-6 px-8 pt-20 pb-10'>
          <nav className='flex flex-col gap-1'>
            {links.map((link, i) => {
              const isActive = isCurrent(link.href)
              const linkClassName = cn(
                'flex items-center gap-3 py-3 text-base font-medium tracking-tight transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] motion-reduce:transition-none',
                mobileOpen
                  ? 'translate-y-0 opacity-100'
                  : 'translate-y-4 opacity-0',
                isActive ? 'text-foreground' : 'text-muted-foreground',
                link.disabled && 'pointer-events-none opacity-50'
              )
              const transitionStyle = {
                transitionDelay: mobileOpen ? `${100 + i * 50}ms` : '0ms',
              }
              if (link.external) {
                return (
                  <a
                    key={`${link.title}:${link.href}`}
                    href={link.href}
                    target='_blank'
                    rel='noopener noreferrer'
                    aria-disabled={link.disabled}
                    tabIndex={link.disabled ? -1 : undefined}
                    onClick={(event) => handleNavLinkClick(event, link, true)}
                    className={linkClassName}
                    style={transitionStyle}
                  >
                    {t(link.title)}
                  </a>
                )
              }
              return (
                <Link
                  key={`${link.title}:${link.href}`}
                  to={link.href}
                  activeOptions={{ exact: true }}
                  aria-current={isActive ? 'page' : undefined}
                  disabled={link.disabled}
                  onClick={(event) => handleNavLinkClick(event, link, true)}
                  className={linkClassName}
                  style={transitionStyle}
                >
                  {t(link.title)}
                </Link>
              )
            })}
          </nav>

          <div
            className={cn(
              'flex flex-col gap-3 transition-all duration-500',
              mobileOpen
                ? 'translate-y-0 opacity-100'
                : 'translate-y-4 opacity-0'
            )}
            style={{ transitionDelay: mobileOpen ? '250ms' : '0ms' }}
          >
            <Link
              to={isAuthenticated ? '/workspace' : '/sign-in'}
              onClick={() => setMobileOpen(false)}
              className='bg-foreground text-background inline-flex h-10 items-center justify-center rounded-lg text-sm font-medium transition-opacity hover:opacity-90 active:opacity-80'
            >
              {isAuthenticated ? t('Go to Dashboard') : t('Sign in')}
            </Link>
          </div>
        </div>
      </div>

      <Dialog
        open={!!authPromptTarget}
        onOpenChange={(open) => {
          if (!open) {
            closeAuthPrompt()
          }
        }}
        title={t('Sign in required')}
        description={t('Please sign in to view {{module}}.', {
          module: authPromptTarget?.title || '',
        })}
        contentClassName='sm:max-w-md'
        contentHeight='auto'
        footer={
          <>
            <Button variant='outline' onClick={closeAuthPrompt}>
              {t('Cancel')}
            </Button>
            <Button onClick={navigateToSignIn}>{t('Sign in now')}</Button>
          </>
        }
      >
        <div className='bg-muted/40 text-muted-foreground rounded-lg px-3 py-2 text-sm'>
          {t('Redirecting to sign in in {{seconds}} seconds.', {
            seconds: authPromptSecondsLeft,
          })}
        </div>
      </Dialog>
    </>
  )
}
