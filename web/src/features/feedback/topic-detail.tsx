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
import { Link, useNavigate } from '@tanstack/react-router'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Markdown } from '@/components/ui/markdown'
import { Textarea } from '@/components/ui/textarea'
import { ROLE } from '@/lib/roles'
import {
  getServerErrorMessage,
  getServerErrorStatus,
} from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  applyFeedbackCommand,
  feedbackActorIDs,
  feedbackAttempt,
  feedbackKeys,
  getFeedback,
  getFeedbackAvatars,
  type FeedbackAttempt,
  type FeedbackCommand,
} from './api'
import { FeedbackAuthor } from './author'
import { FeedbackEditForm } from './edit-form'
import { FeedbackEventContent } from './event-content'
import { feedbackCategoryLabel, feedbackStatusLabel } from './labels'

type VersionedCommand = { command: FeedbackCommand; version: number }
type ReplyEdit = { seq: number; version: number }

export function FeedbackTopicDetail({
  session,
  topic,
  event,
}: {
  session: string
  topic: string
  event?: number
}) {
  const { t } = useTranslation()
  const id = useId()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = (user?.role ?? 0) >= ROLE.ADMIN
  const actorID = String(user?.id ?? '')
  const attempt = useRef<FeedbackAttempt | null>(null)
  const linkedEventRef = useRef<HTMLElement | null>(null)
  const [cursorStack, setCursorStack] = useState(() =>
    event ? [0, event - 1] : [0]
  )
  const [topicEdit, setTopicEdit] = useState<number | null>(null)
  const [replyEdit, setReplyEdit] = useState<ReplyEdit | null>(null)
  const [pendingDelete, setPendingDelete] = useState<VersionedCommand | null>(
    null
  )
  const [reply, setReply] = useState('')
  const cursor = cursorStack.at(-1) ?? 0

  const detail = useQuery({
    queryKey: [...feedbackKeys.topic(session, topic), cursor],
    queryFn: ({ signal }) => getFeedback(topic, cursor, signal),
  })
  const issue = detail.data?.issue
  const visibleEvents =
    detail.data?.events.filter((item) => item.kind !== 'created') ?? []
  const ids = feedbackActorIDs([
    issue?.owner_id,
    ...visibleEvents.map((item) => item.actor_id),
  ])
  const avatars = useQuery({
    queryKey: feedbackKeys.avatars(session, ids),
    queryFn: ({ signal }) => getFeedbackAvatars(ids, signal),
    enabled: ids.length > 0,
  })
  useEffect(() => {
    if (event && cursor === event - 1) {
      linkedEventRef.current?.scrollIntoView?.({ block: 'nearest' })
    }
  }, [detail.data, cursor, event])

  const operation = useMutation({
    mutationFn: (input: VersionedCommand) => {
      if (!issue) throw new Error(t('Request failed'))
      attempt.current = feedbackAttempt(input, attempt.current)
      return applyFeedbackCommand(
        session,
        { ...issue, version: input.version },
        input.command,
        attempt.current
      )
    },
    onSuccess: async (result, { command }) => {
      attempt.current = null
      const listRefresh = queryClient.invalidateQueries({
        queryKey: feedbackKeys.list(session),
      })
      if (command.action === 'delete_issue') {
        await listRefresh
        if (useAuthStore.getState().auth.session?.sid === session) {
          void navigate({ to: '/feedback', search: {} })
        }
        return
      }
      await Promise.all([
        listRefresh,
        queryClient.invalidateQueries({
          queryKey: feedbackKeys.topic(session, topic),
        }),
      ])
      if (command.action === 'reply' && (detail.data?.has_more || cursor > 0)) {
        setCursorStack((history) => [
          ...history,
          Math.max(0, result.issue.activity_seq - 1),
        ])
      }
      if (command.action === 'reply' || command.action === 'set_status') {
        setReply('')
      }
      setTopicEdit(null)
      setReplyEdit(null)
      setPendingDelete(null)
    },
    onError: () => setPendingDelete(null),
  })

  const back = (
    <Button variant='outline' render={<Link to='/feedback' search={{}} />}>
      {t('feedback.backToTopics', { defaultValue: 'Back to topics' })}
    </Button>
  )
  if (detail.isPending) {
    return (
      <div className='flex flex-col items-start gap-4'>
        {back}
        <p role='status'>{t('Loading...')}</p>
      </div>
    )
  }
  if (detail.isError || !issue) {
    return (
      <div className='flex flex-col items-start gap-4'>
        {back}
        <Alert variant='destructive'>
          {getServerErrorMessage(detail.error)}
        </Alert>
      </div>
    )
  }

  const isOwner = actorID === issue.owner_id
  const busy = operation.isPending
  const conflict = getServerErrorStatus(operation.error) === 409
  const apply = (command: FeedbackCommand, version = issue.version) => {
    if (!busy && !conflict) operation.mutate({ command, version })
  }
  const describeDelete = (command: FeedbackCommand) =>
    setPendingDelete({ command, version: issue.version })
  const reload = () => {
    operation.reset()
    setTopicEdit(null)
    setReplyEdit(null)
    void detail.refetch()
  }

  return (
    <div className='flex w-full min-w-0 flex-col items-start gap-4'>
      {back}
      {operation.error && (
        <Alert variant='destructive'>
          {conflict
            ? t('feedback.conflict', {
                defaultValue:
                  'This topic has changed. Reload it before trying again.',
              })
            : getServerErrorMessage(operation.error)}
          {conflict && (
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='mt-2 self-start'
              onClick={reload}
            >
              {t('feedback.reloadTopic', { defaultValue: 'Reload topic' })}
            </Button>
          )}
        </Alert>
      )}
      <Card className='w-full min-w-0'>
        <CardHeader className='gap-2'>
          <div className='flex flex-wrap items-center gap-2'>
            <h2 className='text-base font-medium break-words'>{issue.title}</h2>
            <Badge variant='secondary'>
              {feedbackCategoryLabel(t, issue.category)}
            </Badge>
            <Badge variant='outline'>
              {feedbackStatusLabel(t, issue.status)}
            </Badge>
            {issue.pinned_at && (
              <Badge variant='outline'>
                {t('feedback.pinned', { defaultValue: 'Pinned' })}
              </Badge>
            )}
          </div>
          <FeedbackAuthor
            name={issue.owner_username}
            avatar={avatars.data?.[issue.owner_id ?? '']}
            timestamp={issue.created_at}
          />
        </CardHeader>
        <CardContent className='flex min-w-0 flex-col gap-4'>
          <Markdown untrusted breaks className='min-w-0 break-words'>
            {issue.description ?? ''}
          </Markdown>
          {(isAdmin || isOwner) && (
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={busy || conflict}
                onClick={() => setTopicEdit(issue.version)}
              >
                {t('feedback.editTopic', { defaultValue: 'Edit topic' })}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={busy || conflict}
                onClick={() => describeDelete({ action: 'delete_issue' })}
              >
                {t('feedback.deleteTopic', { defaultValue: 'Delete topic' })}
              </Button>
              {isAdmin && (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={busy || conflict}
                  onClick={() =>
                    apply({ action: 'set_pinned', pinned: !issue.pinned_at })
                  }
                >
                  {issue.pinned_at
                    ? t('feedback.unpinTopic', { defaultValue: 'Unpin topic' })
                    : t('feedback.pinTopic', { defaultValue: 'Pin topic' })}
                </Button>
              )}
            </div>
          )}
          {topicEdit !== null && (
            <FeedbackEditForm
              title={issue.title}
              content={issue.description ?? ''}
              busy={busy}
              disabled={conflict}
              onCancel={() => setTopicEdit(null)}
              onSave={({ title, content }) =>
                apply(
                  {
                    action: 'edit_issue',
                    title: title ?? '',
                    description: content,
                  },
                  topicEdit
                )
              }
            />
          )}
        </CardContent>
      </Card>

      <Card className='w-full min-w-0'>
        <CardHeader>
          <CardTitle>
            {t('feedback.replies', { defaultValue: 'Replies' })}
          </CardTitle>
        </CardHeader>
        <CardContent className='flex min-w-0 flex-col gap-4'>
          {visibleEvents.length === 0 && (
            <p className='text-muted-foreground'>
              {t('feedback.noReplies', {
                defaultValue: 'No replies on this page',
              })}
            </p>
          )}
          {visibleEvents.map((item) => {
            const isReply = item.kind === 'reply'
            const isAuthor = item.actor_id === actorID
            const linked = item.seq === event
            return (
              <article
                key={item.seq}
                ref={linked ? linkedEventRef : null}
                id={`feedback-event-${item.seq}`}
                className='border-border data-[highlight=true]:bg-muted/60 flex min-w-0 flex-col gap-2 border-b pb-4 last:border-0 last:pb-0 data-[highlight=true]:rounded-md data-[highlight=true]:p-2'
                data-highlight={linked}
              >
                <FeedbackEventContent
                  item={item}
                  avatar={avatars.data?.[item.actor_id ?? '']}
                />
                {isReply && !item.deleted_at && (isAdmin || isAuthor) && (
                  <div className='flex flex-wrap gap-2'>
                    {isAuthor && (
                      <Button
                        type='button'
                        size='sm'
                        variant='outline'
                        disabled={busy || conflict}
                        onClick={() =>
                          setReplyEdit({
                            seq: item.seq,
                            version: issue.version,
                          })
                        }
                      >
                        {t('feedback.editReply', {
                          defaultValue: 'Edit reply',
                        })}
                      </Button>
                    )}
                    <Button
                      type='button'
                      size='sm'
                      variant='outline'
                      disabled={busy || conflict}
                      onClick={() =>
                        describeDelete({
                          action: 'delete_reply',
                          seq: item.seq,
                        })
                      }
                    >
                      {t('feedback.deleteReply', {
                        defaultValue: 'Delete reply',
                      })}
                    </Button>
                  </div>
                )}
                {replyEdit?.seq === item.seq && (
                  <FeedbackEditForm
                    content={item.content ?? ''}
                    busy={busy}
                    disabled={conflict}
                    onCancel={() => setReplyEdit(null)}
                    onSave={({ content }) =>
                      apply(
                        { action: 'edit_reply', seq: item.seq, content },
                        replyEdit.version
                      )
                    }
                  />
                )}
              </article>
            )
          })}
          {detail.data && (cursorStack.length > 1 || detail.data.has_more) && (
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='outline'
                disabled={busy || cursorStack.length <= 1}
                onClick={() =>
                  setCursorStack((history) => history.slice(0, -1))
                }
              >
                {t('feedback.earlierReplies', {
                  defaultValue: 'Earlier replies',
                })}
              </Button>
              <Button
                type='button'
                variant='outline'
                disabled={busy || !detail.data.has_more}
                onClick={() => {
                  const next = detail.data?.next_event_cursor
                  if (next !== undefined) {
                    setCursorStack((history) => [...history, next])
                  }
                }}
              >
                {t('feedback.laterReplies', { defaultValue: 'Later replies' })}
              </Button>
            </div>
          )}
          <form
            onSubmit={(submission) => {
              submission.preventDefault()
              if (reply.trim()) apply({ action: 'reply', content: reply })
            }}
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor={`${id}-reply`}>
                  {t('feedback.reply', { defaultValue: 'Reply' })}
                </FieldLabel>
                <Textarea
                  id={`${id}-reply`}
                  required
                  rows={4}
                  maxLength={20000}
                  disabled={busy}
                  value={reply}
                  onChange={(input) => setReply(input.target.value)}
                  aria-describedby={`${id}-mention`}
                />
                <FieldDescription id={`${id}-mention`}>
                  {t('feedback.mentionHint', {
                    defaultValue: 'Use @username to notify a member.',
                  })}
                </FieldDescription>
              </Field>
              <div className='flex flex-wrap gap-2'>
                <Button
                  type='submit'
                  disabled={busy || conflict || !reply.trim()}
                >
                  {t('feedback.sendReply', { defaultValue: 'Send reply' })}
                </Button>
                {(isOwner || isAdmin) && issue.status === 'open' && (
                  <Button
                    type='button'
                    variant='outline'
                    disabled={busy || conflict || !reply.trim()}
                    onClick={() =>
                      apply({
                        action: 'set_status',
                        status: 'resolved',
                        comment: reply,
                      })
                    }
                  >
                    {t('feedback.resolve', { defaultValue: 'Mark resolved' })}
                  </Button>
                )}
                {(isOwner || isAdmin) && issue.status !== 'open' && (
                  <Button
                    type='button'
                    variant='outline'
                    disabled={busy || conflict}
                    onClick={() =>
                      apply({ action: 'set_status', status: 'open' })
                    }
                  >
                    {t('feedback.reopen', { defaultValue: 'Reopen topic' })}
                  </Button>
                )}
                {isAdmin && issue.status !== 'closed' && (
                  <Button
                    type='button'
                    variant='outline'
                    disabled={busy || conflict}
                    onClick={() =>
                      apply({ action: 'set_status', status: 'closed' })
                    }
                  >
                    {t('feedback.closeTopic', { defaultValue: 'Close topic' })}
                  </Button>
                )}
              </div>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        onOpenChange={(open) => {
          if (!open && !busy) setPendingDelete(null)
        }}
        title={t('Delete')}
        desc={t('feedback.confirmDelete', {
          defaultValue: 'Delete this content from the discussion?',
        })}
        confirmText={t('Delete')}
        destructive
        isLoading={busy}
        handleConfirm={() => {
          if (pendingDelete && !busy) operation.mutate(pendingDelete)
        }}
      />
    </div>
  )
}
