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
import { Trash2 } from 'lucide-react'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { getGroups, searchUsers } from '@/features/users/api'
import { useDebounce } from '@/hooks/use-debounce'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { EntitlementPolicy, SubscriptionPlan } from '../api'

const unassigned = 'not assigned'
type Option = { value: string; label: string }
type AssignmentKey = 'user_subscriptions' | 'group_subscriptions'

function AssignmentCard({
  title,
  description,
  subjectLabel,
  subjects,
  plans,
  assigned,
  onChange,
  resetKey,
  children,
}: {
  title: string
  description: string
  subjectLabel: string
  subjects: Option[]
  plans: Record<string, SubscriptionPlan>
  assigned: Record<string, string>
  onChange: (values: Record<string, string>) => void
  resetKey?: string
  children?: React.ReactNode
}) {
  const { t } = useTranslation()
  const id = useId()
  const [selection, setSelection] = useState<{
    subject: string
    key?: string
  } | null>(null)
  const subject = selection?.key === resetKey ? (selection?.subject ?? '') : ''
  const planOptions = [
    { value: unassigned, label: t('No direct assignment') },
    ...Object.entries(plans).map(([value, plan]) => ({
      value,
      label: plan.name,
    })),
  ]
  const subjectsById = new Map(
    subjects.map(({ value, label }) => [value, label])
  )
  const remove = (target: string) =>
    onChange(
      Object.fromEntries(
        Object.entries(assigned).filter(([key]) => key !== target)
      )
    )
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <FieldGroup>
          {children}
          <Field>
            <FieldLabel htmlFor={`${id}-subject`}>{subjectLabel}</FieldLabel>
            <Combobox
              id={`${id}-subject`}
              value={subject}
              options={subjects}
              onValueChange={(value) =>
                setSelection(value ? { subject: value, key: resetKey } : null)
              }
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-plan`}>
              {t('Assigned subscription')}
            </FieldLabel>
            <Combobox
              id={`${id}-plan`}
              value={(subject && assigned[subject]) || unassigned}
              options={planOptions}
              disabled={!subject || Object.keys(plans).length === 0}
              onValueChange={(value) => {
                if (!subject || !value) return
                if (value === unassigned) remove(subject)
                else onChange({ ...assigned, [subject]: value })
              }}
            />
          </Field>
        </FieldGroup>
        {Object.entries(assigned).map(([target, plan]) => {
          const label = subjectsById.get(target) ?? target
          return (
            <div
              key={target}
              className='flex items-center justify-between gap-2 border-t pt-2 text-sm'
            >
              <span className='min-w-0 break-words'>
                {label} · {plans[plan]?.name ?? plan}
              </span>
              <Button
                type='button'
                variant='ghost'
                size='icon'
                aria-label={t('Remove assignment for {{name}}', {
                  name: label,
                })}
                onClick={() => remove(target)}
              >
                <Trash2 />
              </Button>
            </div>
          )
        })}
      </CardContent>
    </Card>
  )
}

export function SubscriptionAssignments({
  draft,
  adminId,
  onChange,
}: {
  draft: EntitlementPolicy
  adminId: number | undefined
  onChange: (key: AssignmentKey, values: Record<string, string>) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const [keyword, setKeyword] = useState('')
  const term = useDebounce(keyword.trim(), 300)
  const users = useQuery({
    queryKey: ['entitlements', 'assignment-users', adminId, term],
    queryFn: async () =>
      requireServerSuccess(await searchUsers({ keyword: term, page_size: 30 }))
        .data?.items ?? [],
    staleTime: 30_000,
  })
  const groups = useQuery({
    queryKey: ['entitlements', 'assignment-groups', adminId],
    queryFn: async () => requireServerSuccess(await getGroups()).data ?? [],
    staleTime: 30_000,
  })
  const plans = draft.plans ?? {}
  return (
    <div className='grid items-start gap-4 lg:grid-cols-2'>
      <AssignmentCard
        title={t('User subscriptions')}
        description={t(
          'A user assignment replaces the group subscription. Each user has one effective subscription.'
        )}
        subjectLabel={t('User')}
        subjects={(users.data ?? []).map((user) => ({
          value: String(user.id),
          label: `${user.username} · #${user.id}`,
        }))}
        plans={plans}
        assigned={draft.user_subscriptions ?? {}}
        onChange={(values) => onChange('user_subscriptions', values)}
        resetKey={term}
      >
        <Field>
          <FieldLabel htmlFor={`${id}-search`}>
            {t('Search by username or user ID')}
          </FieldLabel>
          <Input
            id={`${id}-search`}
            value={keyword}
            onChange={(event) => setKeyword(event.currentTarget.value)}
          />
        </Field>
        {users.isError && (
          <Button
            type='button'
            variant='outline'
            onClick={() => void users.refetch()}
          >
            {t('Retry')}
          </Button>
        )}
      </AssignmentCard>
      <AssignmentCard
        title={t('Group subscriptions')}
        description={t(
          'Users without a direct assignment inherit their user group’s subscription.'
        )}
        subjectLabel={t('User group')}
        subjects={(groups.data ?? []).map((group) => ({
          value: group,
          label: group,
        }))}
        plans={plans}
        assigned={draft.group_subscriptions ?? {}}
        onChange={(values) => onChange('group_subscriptions', values)}
      >
        {groups.isError && (
          <Button
            type='button'
            variant='outline'
            onClick={() => void groups.refetch()}
          >
            {t('Retry')}
          </Button>
        )}
      </AssignmentCard>
    </div>
  )
}
