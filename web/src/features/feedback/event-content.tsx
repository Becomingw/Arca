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

import { Markdown } from '@/components/ui/markdown'

import type { FeedbackEvent } from './api'
import { FeedbackAuthor } from './author'
import { feedbackStatusLabel } from './labels'

export function FeedbackEventContent({
  item,
  avatar,
}: {
  item: FeedbackEvent
  avatar?: string
}) {
  const { t } = useTranslation()
  return (
    <>
      <FeedbackAuthor
        name={item.actor_username}
        avatar={avatar}
        timestamp={item.created_at}
      />
      {item.deleted_at && (
        <p className='text-muted-foreground'>
          {t('feedback.deletedReply', {
            defaultValue: 'Reply deleted',
          })}
        </p>
      )}
      {!item.deleted_at &&
        item.content &&
        item.kind !== 'pin' &&
        item.kind !== 'reply_edit' && (
          <Markdown untrusted breaks className='min-w-0 break-words'>
            {item.content}
          </Markdown>
        )}
      {item.to_status && (
        <p className='text-muted-foreground'>
          {t('feedback.statusChanged', {
            defaultValue: 'Status changed to {{status}}',
            status: feedbackStatusLabel(t, item.to_status),
          })}
        </p>
      )}
      {item.kind === 'pin' && (
        <p className='text-muted-foreground'>
          {item.content === 'pinned'
            ? t('feedback.topicPinned', {
                defaultValue: 'Topic pinned',
              })
            : t('feedback.topicUnpinned', {
                defaultValue: 'Topic unpinned',
              })}
        </p>
      )}
      {item.kind === 'issue_edit' && (
        <p className='text-muted-foreground'>
          {t('feedback.topicEdited', {
            defaultValue: 'Topic edited',
          })}
        </p>
      )}
      {item.kind === 'reply_edit' && (
        <p className='text-muted-foreground'>
          {t('feedback.replyEdited', {
            defaultValue: 'Reply edited',
          })}
        </p>
      )}
      {item.kind === 'reply_delete' && (
        <p className='text-muted-foreground'>
          {t('feedback.deletedReply', {
            defaultValue: 'Reply deleted',
          })}
        </p>
      )}
    </>
  )
}
