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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { MessageSquare, Pin } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  FEEDBACK_PAGE_SIZE,
  feedbackActorIDs,
  feedbackKeys,
  getFeedbackAvatars,
  listFeedback,
  useFeedbackSession,
} from './api'
import { FeedbackAuthor } from './author'
import { feedbackCategoryLabel, feedbackStatusLabel } from './labels'
import { FeedbackTopicDetail } from './topic-detail'
import { NewFeedbackTopic } from './topic-form'

export function FeedbackPage({
  topic,
  event,
  requestId,
}: {
  topic?: string
  event?: number
  requestId?: string
}) {
  const { t } = useTranslation()
  const session = useFeedbackSession()
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('feedback.title', { defaultValue: 'Discussions & feedback' })}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {session && topic && (
          <FeedbackTopicDetail
            key={`${session}:${topic}:${event ?? 0}`}
            session={session}
            topic={topic}
            event={event}
          />
        )}
        {session && !topic && (
          <FeedbackIndex
            key={`${session}:${requestId ?? ''}`}
            session={session}
            requestId={requestId}
          />
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function FeedbackIndex({
  session,
  requestId,
}: {
  session: string
  requestId?: string
}) {
  const { t } = useTranslation()
  const [offset, setOffset] = useState(0)
  const topics = useQuery({
    queryKey: [...feedbackKeys.list(session), offset],
    queryFn: ({ signal }) => listFeedback(offset, signal),
  })
  const ids = feedbackActorIDs(
    topics.data?.items.map((item) => item.owner_id) ?? []
  )
  const avatars = useQuery({
    queryKey: feedbackKeys.avatars(session, ids),
    queryFn: ({ signal }) => getFeedbackAvatars(ids, signal),
    enabled: ids.length > 0,
  })
  const canMoveForward = offset + FEEDBACK_PAGE_SIZE < (topics.data?.total ?? 0)
  return (
    <div className='grid min-w-0 items-start gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(22rem,0.6fr)]'>
      <Card>
        <CardHeader>
          <CardTitle>
            {t('feedback.topics', { defaultValue: 'Topics' })}
          </CardTitle>
        </CardHeader>
        <CardContent className='flex min-w-0 flex-col gap-3'>
          {topics.isPending && (
            <p role='status' className='text-muted-foreground'>
              {t('Loading...')}
            </p>
          )}
          {topics.isError && (
            <Alert variant='destructive'>
              {getServerErrorMessage(topics.error)}
            </Alert>
          )}
          {topics.data?.items.length === 0 && (
            <Empty className='min-h-36 p-4'>
              <EmptyHeader>
                <EmptyMedia variant='icon'>
                  <MessageSquare />
                </EmptyMedia>
                <EmptyTitle>
                  {t('feedback.noTopics', {
                    defaultValue: 'No discussions yet',
                  })}
                </EmptyTitle>
              </EmptyHeader>
            </Empty>
          )}
          {!!topics.data?.items.length && (
            <ul className='divide-border divide-y'>
              {topics.data.items.map((item) => (
                <li key={item.id}>
                  <Link
                    to='/feedback'
                    search={{ topic: item.id }}
                    className='hover:bg-muted/60 focus-visible:ring-ring flex min-w-0 flex-col gap-2 rounded-md px-2 py-3 focus-visible:ring-2 focus-visible:outline-none'
                  >
                    <span className='flex flex-wrap items-center gap-2'>
                      {item.pinned_at && (
                        <Pin
                          className='text-muted-foreground size-4'
                          aria-label={t('feedback.pinned', {
                            defaultValue: 'Pinned',
                          })}
                        />
                      )}
                      <span className='min-w-0 font-semibold break-words'>
                        {item.title}
                      </span>
                      <Badge variant='secondary'>
                        {feedbackCategoryLabel(t, item.category)}
                      </Badge>
                      <Badge variant='outline'>
                        {feedbackStatusLabel(t, item.status)}
                      </Badge>
                    </span>
                    <FeedbackAuthor
                      name={item.owner_username}
                      avatar={avatars.data?.[item.owner_id ?? '']}
                      timestamp={item.updated_at}
                    />
                  </Link>
                </li>
              ))}
            </ul>
          )}
          {(offset > 0 || canMoveForward) && (
            <div className='flex justify-between gap-2'>
              <Button
                type='button'
                variant='outline'
                disabled={offset === 0 || topics.isFetching}
                onClick={() =>
                  setOffset((value) => Math.max(0, value - FEEDBACK_PAGE_SIZE))
                }
              >
                {t('Previous page')}
              </Button>
              <Button
                type='button'
                variant='outline'
                disabled={!canMoveForward || topics.isFetching}
                onClick={() => setOffset((value) => value + FEEDBACK_PAGE_SIZE)}
              >
                {t('Next page')}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
      <NewFeedbackTopic session={session} requestId={requestId} />
    </div>
  )
}
