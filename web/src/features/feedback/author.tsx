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

import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { formatDateTimeObject } from '@/lib/time'

export function FeedbackAuthor({
  name,
  avatar,
  timestamp,
}: {
  name?: string | null
  avatar?: string
  timestamp: number
}) {
  const { t } = useTranslation()
  const label = name || t('feedback.member', { defaultValue: 'Member' })
  return (
    <span className='text-muted-foreground inline-flex flex-wrap items-center gap-2 text-xs'>
      <Avatar className='size-6' aria-hidden='true'>
        {avatar && (
          <AvatarImage
            src={`/api/arca/avatars/${encodeURIComponent(avatar)}`}
            alt=''
          />
        )}
        <AvatarFallback>{[...label][0]?.toUpperCase()}</AvatarFallback>
      </Avatar>
      <span>{label}</span>
      <time dateTime={new Date(timestamp * 1000).toISOString()}>
        {formatDateTimeObject(new Date(timestamp * 1000))}
      </time>
    </span>
  )
}
