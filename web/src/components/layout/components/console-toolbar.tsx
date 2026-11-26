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
import { useTranslation } from 'react-i18next'

import { ConfigDrawer } from '@/components/config-drawer'
import { Search } from '@/components/search'
import { SidebarTrigger, useSidebar } from '@/components/ui/sidebar'

export function ConsoleToolbar() {
  const { t } = useTranslation()
  const { isMobile, open, openMobile } = useSidebar()

  return (
    <div
      role='group'
      aria-label={t('Console')}
      data-slot='console-toolbar'
      className='border-border/60 flex h-[var(--console-toolbar-height)] w-full shrink-0 items-center justify-between gap-2 border-b px-2.5 sm:px-4 lg:px-5'
    >
      <SidebarTrigger
        variant='ghost'
        className='size-8 rounded-lg'
        aria-expanded={isMobile ? openMobile : open}
      />
      <div className='flex min-w-0 items-center gap-1.5'>
        <Search className='w-8 flex-none sm:w-44 max-sm:[&>span]:hidden' />
        <ConfigDrawer />
      </div>
    </div>
  )
}
