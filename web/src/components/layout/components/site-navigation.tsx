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
import { useRouterState } from '@tanstack/react-router'

import { SkipToMain } from '@/components/skip-to-main'

import { PublicHeader } from './public-header'

declare module '@tanstack/react-router' {
  interface StaticDataRouteOption {
    siteNavigation?: 'public' | 'console'
  }
}

export function SiteNavigation() {
  const section = useRouterState({
    select: ({ matches }) =>
      matches.find((match) => match.staticData.siteNavigation)?.staticData
        .siteNavigation,
  })

  if (!section) return null
  return (
    <>
      {section === 'console' && <SkipToMain />}
      <PublicHeader consoleActive={section === 'console'} />
    </>
  )
}
