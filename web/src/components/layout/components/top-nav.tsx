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
import { Menu } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import type { TopNavLink } from '../types'

type TopNavProps = React.HTMLAttributes<HTMLElement> & {
  links: TopNavLink[]
}

const navLinkClass =
  'text-muted-foreground hover:text-foreground hover:bg-accent/60 max-w-[clamp(5.5rem,11vw,10rem)] truncate rounded-lg px-2.5 py-1.5 text-sm font-medium transition-colors data-[status=active]:bg-overview-accent-1/12 data-[status=active]:text-foreground aria-disabled:pointer-events-none aria-disabled:opacity-50 xl:px-3'

/**
 * 顶部导航栏组件
 * 在大屏幕显示水平导航，在小屏幕显示下拉菜单
 */
export function TopNav({ className, links, ...props }: TopNavProps) {
  const { t } = useTranslation()

  return (
    <>
      <div className='lg:hidden'>
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger
            render={
              <Button
                size='icon'
                variant='ghost'
                className='size-8 rounded-lg'
                aria-label={t('Toggle navigation menu')}
              />
            }
          >
            <Menu aria-hidden='true' />
          </DropdownMenuTrigger>
          <DropdownMenuContent side='bottom' align='end'>
            {links.map(({ title, href, disabled, external }) => (
              <DropdownMenuItem
                key={`${title}-${href}`}
                disabled={disabled}
                render={
                  external ? (
                    <a
                      href={disabled ? undefined : href}
                      target='_blank'
                      rel='noopener noreferrer'
                    />
                  ) : (
                    <Link to={href} disabled={disabled} />
                  )
                }
              >
                {title}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <nav
        aria-label={t('Header navigation')}
        className={cn('hidden items-center gap-0.5 lg:flex', className)}
        {...props}
      >
        {links.map(({ title, href, isActive, disabled, external }) =>
          external ? (
            <a
              key={`${title}-${href}`}
              href={disabled ? undefined : href}
              aria-disabled={disabled}
              title={title}
              target='_blank'
              rel='noopener noreferrer'
              className={cn(navLinkClass, isActive && 'text-foreground')}
            >
              {title}
            </a>
          ) : (
            <Link
              key={`${title}-${href}`}
              to={href}
              disabled={disabled}
              title={title}
              activeOptions={{ exact: href === '/' }}
              className={cn(navLinkClass, isActive && 'text-foreground')}
            >
              {title}
            </Link>
          )
        )}
      </nav>
    </>
  )
}
