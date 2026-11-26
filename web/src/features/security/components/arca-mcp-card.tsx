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

import { CopyButton } from '@/components/copy-button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export function ArcaMcpCard() {
  const { t } = useTranslation()
  const canManageSubscriptions = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const endpoint = new URL('/mcp', window.location.origin).href

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Agent collaboration')}</CardTitle>
        <CardDescription>
          {t('Connect an MCP client with a scoped access token.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        <div className='flex items-center justify-between gap-3'>
          <code className='text-sm break-all'>{endpoint}</code>
          <CopyButton value={endpoint} tooltip={t('Copy MCP endpoint')} />
        </div>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Create a scoped access token above. Agents can inspect users, groups, channels, model routes, usage allowances, and feedback according to their permissions.'
          )}
        </p>
        {canManageSubscriptions && (
          <p className='text-muted-foreground text-sm'>
            {t(
              'Subscription changes require entitlement write access and a confirmed preview.'
            )}
          </p>
        )}
      </CardContent>
    </Card>
  )
}
