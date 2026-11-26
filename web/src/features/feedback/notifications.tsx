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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  feedbackActorIDs,
  feedbackKeys,
  getFeedbackAvatars,
  markFeedbackRead,
  type MarkRead,
} from './api'
import { FeedbackAuthor } from './author'
import { useFeedbackNotifications } from './use-feedback-notifications'

export function FeedbackNotificationList({
  onNavigate,
}: {
  onNavigate?: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { inbox, session, unreadCount } = useFeedbackNotifications()
  const items = inbox.data?.items ?? []
  const ids = feedbackActorIDs(items.map((item) => item.actor_id))
  const avatars = useQuery({
    queryKey: feedbackKeys.avatars(session, ids),
    queryFn: ({ signal }) => getFeedbackAvatars(ids, signal),
    enabled: session !== null && ids.length > 0,
  })
  const markRead = useMutation({
    mutationFn: (input: MarkRead) => {
      if (!session) throw new Error(t('Session expired!'))
      return markFeedbackRead(session, input)
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: feedbackKeys.inbox(session) }),
  })

  if (!session) return null
  if (inbox.isPending) {
    return (
      <p className='text-muted-foreground px-2 py-4' role='status'>
        {t('Loading...')}
      </p>
    )
  }
  if (inbox.isError) {
    return (
      <Alert variant='destructive'>{getServerErrorMessage(inbox.error)}</Alert>
    )
  }
  if (!items.length) {
    return (
      <p className='text-muted-foreground px-2 py-4'>
        {t('feedback.inboxEmpty', {
          defaultValue: 'No discussion notifications',
        })}
      </p>
    )
  }
  return (
    <div className='flex flex-col gap-2'>
      {!!unreadCount && (
        <Button
          type='button'
          variant='ghost'
          size='sm'
          className='self-end'
          disabled={markRead.isPending}
          onClick={() => markRead.mutate({ all: true })}
        >
          {t('feedback.markAllRead', { defaultValue: 'Mark all as read' })}
        </Button>
      )}
      {markRead.error && (
        <Alert variant='destructive'>
          {getServerErrorMessage(markRead.error)}
        </Alert>
      )}
      <ScrollArea className='pr-2 [&_[data-slot=scroll-area-viewport]]:max-h-[min(52vh,28rem)]'>
        <ul className='flex flex-col gap-1'>
          {items.map((item) => (
            <li key={item.id}>
              <Link
                to='/feedback'
                search={{ topic: item.issue_id, event: item.event_seq }}
                onClick={() => {
                  if (!item.read_at && !markRead.isPending) {
                    markRead.mutate({ ids: [item.id] })
                  }
                  onNavigate?.()
                }}
                data-unread={!item.read_at}
                className='hover:bg-muted/80 focus-visible:ring-ring data-[unread=true]:bg-accent flex min-w-0 flex-col gap-2 rounded-md px-2 py-3 focus-visible:ring-2 focus-visible:outline-none'
              >
                <span className='font-medium break-words'>
                  {item.issue_title}
                </span>
                <span className='text-muted-foreground line-clamp-2 text-xs break-words'>
                  {item.snippet}
                </span>
                <FeedbackAuthor
                  name={item.actor_username}
                  avatar={avatars.data?.[item.actor_id ?? '']}
                  timestamp={item.created_at}
                />
              </Link>
            </li>
          ))}
        </ul>
      </ScrollArea>
    </div>
  )
}
