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
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

type FeedbackEditFormProps = {
  title?: string
  content: string
  busy: boolean
  disabled: boolean
  onSave: (edit: { title?: string; content: string }) => void
  onCancel: () => void
}

export function FeedbackEditForm({
  title,
  content,
  busy,
  disabled,
  onSave,
  onCancel,
}: FeedbackEditFormProps) {
  const { t } = useTranslation()
  const id = useId()
  const [draftTitle, setDraftTitle] = useState(title ?? '')
  const [draftContent, setDraftContent] = useState(content)
  const hasTitle = title !== undefined
  const canSave =
    !busy &&
    !disabled &&
    draftContent.trim() &&
    (!hasTitle || draftTitle.trim()) &&
    (draftContent !== content || (hasTitle && draftTitle.trim() !== title))

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        if (!canSave) return
        onSave({
          title: hasTitle ? draftTitle.trim() : undefined,
          content: draftContent,
        })
      }}
    >
      <FieldGroup>
        {hasTitle && (
          <Field>
            <FieldLabel htmlFor={`${id}-title`}>
              {t('feedback.fieldTitle', { defaultValue: 'Title' })}
            </FieldLabel>
            <Input
              id={`${id}-title`}
              required
              maxLength={200}
              disabled={busy}
              value={draftTitle}
              onChange={(event) => setDraftTitle(event.target.value)}
            />
          </Field>
        )}
        <Field>
          <FieldLabel htmlFor={`${id}-content`}>
            {hasTitle
              ? t('Content')
              : t('feedback.editReply', { defaultValue: 'Edit reply' })}
          </FieldLabel>
          <Textarea
            id={`${id}-content`}
            required
            rows={hasTitle ? 5 : 3}
            maxLength={20000}
            disabled={busy}
            value={draftContent}
            onChange={(event) => setDraftContent(event.target.value)}
          />
        </Field>
        <div className='flex flex-wrap gap-2'>
          <Button type='submit' disabled={!canSave}>
            {t('Save')}
          </Button>
          <Button
            type='button'
            variant='outline'
            disabled={busy}
            onClick={onCancel}
          >
            {t('Cancel')}
          </Button>
        </div>
      </FieldGroup>
    </form>
  )
}
