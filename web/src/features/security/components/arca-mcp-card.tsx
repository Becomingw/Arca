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

export function ArcaMcpCard() {
  const { t } = useTranslation()
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
            'Create an access token above and grant only the Arca feedback, request diagnostics, or entitlement permissions your agent needs.'
          )}
        </p>
      </CardContent>
    </Card>
  )
}
