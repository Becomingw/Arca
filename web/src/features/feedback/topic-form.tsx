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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  createFeedback,
  feedbackAttempt,
  feedbackCategories,
  feedbackKeys,
  feedbackRequestIdPattern,
  getFeedbackDiagnostic,
  type FeedbackAttempt,
  type FeedbackCategory,
  type FeedbackDraft,
} from './api'
import { feedbackCategoryLabel } from './labels'

export function NewFeedbackTopic({
  session,
  requestId,
}: {
  session: string
  requestId?: string
}) {
  const { t } = useTranslation()
  const id = useId()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const attempt = useRef<FeedbackAttempt | null>(null)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [category, setCategory] = useState<FeedbackCategory>('bug')
  const [requestID, setRequestID] = useState(requestId ?? '')
  const [includeDiagnostic, setIncludeDiagnostic] = useState(false)

  const publish = useMutation({
    mutationFn: (draft: FeedbackDraft) => {
      attempt.current = feedbackAttempt(draft, attempt.current)
      return createFeedback(session, draft, attempt.current)
    },
    onSuccess: ({ issue }) => {
      attempt.current = null
      void queryClient.invalidateQueries({
        queryKey: feedbackKeys.list(session),
      })
      if (useAuthStore.getState().auth.session?.sid !== session) return
      void navigate({ to: '/feedback', search: { topic: issue.id } })
    },
  })
  const preview = useMutation({
    mutationFn: getFeedbackDiagnostic,
  })
  const request = requestID.trim()
  const summary =
    preview.variables === request ? preview.data?.summary : undefined

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {t('feedback.newTopic', { defaultValue: 'New topic' })}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(event) => {
            event.preventDefault()
            if (
              publish.isPending ||
              !title.trim() ||
              !description.trim() ||
              (request && !feedbackRequestIdPattern.test(request))
            ) {
              return
            }
            publish.mutate({
              title: title.trim(),
              description,
              category,
              request_id: request || undefined,
              include_diagnostics: Boolean(summary && includeDiagnostic),
            })
          }}
        >
          <FieldGroup>
            {publish.error && (
              <Alert variant='destructive'>
                {getServerErrorMessage(publish.error)}
              </Alert>
            )}
            <Field>
              <FieldLabel htmlFor={`${id}-title`}>
                {t('feedback.fieldTitle', { defaultValue: 'Title' })}
              </FieldLabel>
              <Input
                id={`${id}-title`}
                required
                maxLength={200}
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                disabled={publish.isPending}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-category`}>
                {t('Category')}
              </FieldLabel>
              <NativeSelect
                id={`${id}-category`}
                value={category}
                onChange={(event) =>
                  setCategory(event.target.value as FeedbackCategory)
                }
                disabled={publish.isPending}
              >
                {feedbackCategories.map((value) => (
                  <NativeSelectOption key={value} value={value}>
                    {feedbackCategoryLabel(t, value)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-content`}>{t('Content')}</FieldLabel>
              <Textarea
                id={`${id}-content`}
                required
                rows={5}
                maxLength={20000}
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                aria-describedby={`${id}-mention`}
                disabled={publish.isPending}
              />
              <FieldDescription id={`${id}-mention`}>
                {t('feedback.mentionHint', {
                  defaultValue: 'Use @username to notify a member.',
                })}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-request`}>
                {t('Request ID')}
              </FieldLabel>
              <Input
                id={`${id}-request`}
                maxLength={64}
                value={requestID}
                onChange={(event) => {
                  setRequestID(event.target.value)
                  setIncludeDiagnostic(false)
                  preview.reset()
                }}
                disabled={publish.isPending}
              />
              {request && !feedbackRequestIdPattern.test(request) && (
                <FieldDescription>
                  {t('feedback.badRequestId', {
                    defaultValue: 'Enter a valid request ID.',
                  })}
                </FieldDescription>
              )}
            </Field>
            {request && (
              <Button
                type='button'
                variant='outline'
                className='self-start'
                disabled={
                  publish.isPending ||
                  preview.isPending ||
                  !feedbackRequestIdPattern.test(request)
                }
                onClick={() => preview.mutate(request)}
              >
                {t('feedback.previewDiagnostics', {
                  defaultValue: 'Preview request diagnostics',
                })}
              </Button>
            )}
            {preview.error && preview.variables === request && (
              <Alert variant='destructive'>
                {getServerErrorMessage(preview.error)}
              </Alert>
            )}
            {summary && (
              <>
                <pre
                  className='bg-muted/60 max-h-44 overflow-auto rounded-lg p-3 text-xs break-words whitespace-pre-wrap'
                  aria-label={t('feedback.previewDiagnostics', {
                    defaultValue: 'Preview request diagnostics',
                  })}
                >
                  {summary}
                </pre>
                <Field orientation='horizontal'>
                  <Checkbox
                    id={`${id}-attach`}
                    checked={includeDiagnostic}
                    onCheckedChange={setIncludeDiagnostic}
                    disabled={publish.isPending}
                  />
                  <FieldLabel htmlFor={`${id}-attach`}>
                    {t('feedback.shareDiagnostics', {
                      defaultValue:
                        'Attach the redacted summary so other members can see it.',
                    })}
                  </FieldLabel>
                </Field>
              </>
            )}
            <Button
              type='submit'
              className='self-start'
              disabled={
                publish.isPending ||
                !title.trim() ||
                !description.trim() ||
                Boolean(request && !feedbackRequestIdPattern.test(request))
              }
            >
              {t('feedback.publish', { defaultValue: 'Publish topic' })}
            </Button>
          </FieldGroup>
        </form>
      </CardContent>
    </Card>
  )
}
