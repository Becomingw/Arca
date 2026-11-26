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
import { Plus, Trash2 } from 'lucide-react'
import { useId, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import type { Allocation, ModelAccess, PolicyMode, SubPool } from '../api'
import { blankAllocation, userIdPattern } from '../policy'

const policyIdPattern = /^[A-Za-z0-9_-]{1,64}$/

function omitPolicyKey<T>(
  values: Record<string, T> | null,
  key: string
): Record<string, T> {
  return Object.fromEntries(
    Object.entries(values ?? {}).filter(([id]) => id !== key)
  )
}

export function PolicyTextField({
  label,
  value,
  onChange,
  description,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  description?: string
}) {
  const id = useId()
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        value={value}
        onChange={(event) => onChange(event.currentTarget.value)}
      />
      {description && <FieldDescription>{description}</FieldDescription>}
    </Field>
  )
}

export function PolicyListField({
  label,
  entries,
  onChange,
  numeric = false,
}: {
  label: string
  entries: (string | number)[] | null | undefined
  onChange: (entries: string[]) => void
  numeric?: boolean
}) {
  const { t } = useTranslation()
  const id = useId()
  const [text, setText] = useState(() => (entries ?? []).join('\n'))
  const separator = numeric ? /[\n,]/ : /\n/
  const items = [
    ...new Set(
      text
        .split(separator)
        .map((value) => value.trim())
        .filter(Boolean)
    ),
  ]
  const invalid =
    numeric &&
    items.some(
      (item) => !userIdPattern.test(item) || !Number.isSafeInteger(Number(item))
    )

  let description = t('Enter one model alias per line.')
  if (numeric) {
    description = invalid
      ? t('Enter positive numeric user IDs.')
      : t('Separate entries with commas or new lines.')
  }

  return (
    <Field data-invalid={invalid || undefined}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Textarea
        id={id}
        rows={3}
        value={text}
        aria-invalid={invalid}
        onChange={(event) => {
          const next = event.currentTarget.value
          setText(next)
          onChange([
            ...new Set(
              next
                .split(separator)
                .map((item) => item.trim())
                .filter(Boolean)
            ),
          ])
        }}
      />
      <FieldDescription>{description}</FieldDescription>
    </Field>
  )
}

export function ModelAccessFields({
  value,
  onChange,
}: {
  value: ModelAccess
  onChange: (value: ModelAccess) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const labels: Record<Exclude<PolicyMode, ''>, string> = {
    inherit: t('Inherit'),
    unrestricted: t('Unrestricted'),
    specific: t('Selected models'),
    deny_all: t('No models'),
  }
  const mode = value.mode || 'inherit'
  const entries = Object.entries(labels).map(([key, label]) => ({
    value: key,
    label,
  }))

  return (
    <FieldGroup>
      <Field>
        <FieldLabel htmlFor={id}>{t('Model access')}</FieldLabel>
        <Select
          items={entries}
          value={mode}
          onValueChange={(next) => {
            if (next) {
              onChange({
                mode: next as PolicyMode,
                models: next === 'specific' ? (value.models ?? []) : undefined,
              })
            }
          }}
        >
          <SelectTrigger id={id}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {entries.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      {mode === 'specific' && (
        <PolicyListField
          label={t('Allowed models')}
          entries={value.models}
          onChange={(models) => onChange({ ...value, models })}
        />
      )}
    </FieldGroup>
  )
}

export function PolicyMapEditor<T>({
  label,
  idLabel,
  values,
  create,
  onChange,
  children,
  validKey = (key) => policyIdPattern.test(key),
  mayRemove = () => true,
}: {
  label: string
  idLabel: string
  values: Record<string, T> | null
  create: (key: string) => T
  onChange: (next: Record<string, T>) => void
  children: (value: T, update: (value: T) => void, key: string) => ReactNode
  validKey?: (key: string) => boolean
  mayRemove?: (key: string) => boolean
}) {
  const { t } = useTranslation()
  const [idInput, setIdInput] = useState('')
  const id = useId()
  const key = idInput.trim()
  const isExisting = values ? Object.hasOwn(values, key) : false
  const canAdd = key !== '' && validKey(key) && !isExisting
  const items = Object.entries(values ?? {})

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-wrap items-end gap-2'>
        <Field
          className='w-auto min-w-32 grow sm:grow-0'
          data-invalid={Boolean(key && !canAdd) || undefined}
        >
          <FieldLabel htmlFor={id}>{idLabel}</FieldLabel>
          <Input
            id={id}
            value={idInput}
            onChange={(event) => setIdInput(event.currentTarget.value)}
            aria-invalid={Boolean(key && !canAdd)}
          />
        </Field>
        <Button
          type='button'
          variant='outline'
          disabled={!canAdd}
          onClick={() => {
            onChange({ ...values, [key]: create(key) })
            setIdInput('')
          }}
        >
          <Plus data-icon='inline-start' />
          {label}
        </Button>
      </div>
      {items.map(([itemKey, value]) => (
        <Card key={itemKey}>
          <CardHeader className='flex flex-row items-center justify-between gap-2'>
            <CardTitle className='break-all'>{itemKey}</CardTitle>
            <Button
              type='button'
              size='icon'
              variant='ghost'
              disabled={!mayRemove(itemKey)}
              aria-label={t('Remove {{name}}', { name: itemKey })}
              onClick={() => onChange(omitPolicyKey(values, itemKey))}
            >
              <Trash2 />
            </Button>
          </CardHeader>
          <CardContent>
            {children(
              value,
              (next) => onChange({ ...values, [itemKey]: next }),
              itemKey
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

type BudgetKey = keyof SubPool
function BudgetPeriods({
  value,
  onChange,
  daily = false,
}: {
  value: SubPool
  onChange: (next: SubPool) => void
  daily?: boolean
}) {
  const { t } = useTranslation()
  const id = useId()
  const labels: [BudgetKey, string][] = [
    ['five_hour_budget_usd', t('5 hours (USD)')],
    ['weekly_budget_usd', t('Weekly (USD)')],
    ['monthly_budget_usd', t('Monthly (USD)')],
  ]
  if (daily) labels.splice(1, 0, ['daily_budget_usd', t('Daily (USD)')])

  return (
    <div className='grid grid-cols-2 gap-3 md:grid-cols-4'>
      {labels.map(([key, label]) => (
        <Field key={key}>
          <FieldLabel htmlFor={id + key}>{label}</FieldLabel>
          <Input
            id={id + key}
            type='number'
            min='0'
            step='any'
            value={value[key] ?? ''}
            onChange={(event) => {
              const amount = event.currentTarget.valueAsNumber
              onChange({
                ...value,
                [key]: Number.isFinite(amount) ? amount : undefined,
              })
            }}
          />
        </Field>
      ))}
    </div>
  )
}

export function AllocationFields({
  value,
  onChange,
  families,
}: {
  value: Allocation
  onChange: (value: Allocation) => void
  families: string[]
}) {
  const { t } = useTranslation()
  const id = useId()
  const hasLimits =
    value.five_hour_budget_usd !== undefined ||
    value.weekly_budget_usd !== undefined ||
    value.monthly_budget_usd !== undefined ||
    Object.keys(value.models ?? {}).length > 0 ||
    Object.keys(value.families ?? {}).length > 0
  return (
    <FieldGroup>
      <Field orientation='horizontal'>
        <FieldLabel htmlFor={`${id}-funding`}>
          {t('Organization funded')}
        </FieldLabel>
        <Switch
          id={`${id}-funding`}
          checked={value.internal_funding}
          onCheckedChange={(checked) =>
            onChange({ ...value, internal_funding: checked })
          }
        />
      </Field>
      <Field orientation='horizontal'>
        <FieldLabel htmlFor={`${id}-unlimited`}>{t('Unlimited')}</FieldLabel>
        <Switch
          id={`${id}-unlimited`}
          checked={value.unlimited}
          onCheckedChange={(checked) => {
            if (
              checked &&
              hasLimits &&
              !window.confirm(
                t('Remove budget limits and enable unlimited use?')
              )
            ) {
              return
            }
            if (checked) {
              onChange({
                ...blankAllocation(),
                internal_funding: value.internal_funding,
                unlimited: true,
              })
            } else onChange({ ...value, unlimited: false })
          }}
        />
      </Field>
      {!value.unlimited && (
        <>
          <BudgetPeriods
            value={value}
            onChange={(next) => onChange({ ...value, ...next })}
          />
          <PolicyMapEditor
            label={t('Add model budget')}
            idLabel={t('Model alias')}
            values={value.models}
            create={() => ({})}
            onChange={(models) => onChange({ ...value, models })}
            validKey={(key) => key.length <= 255}
          >
            {(pool, update) => (
              <BudgetPeriods value={pool} onChange={update} daily />
            )}
          </PolicyMapEditor>
          <PolicyMapEditor
            label={t('Add model family budget')}
            idLabel={t('Model family ID')}
            values={value.families}
            create={() => ({})}
            onChange={(next) => onChange({ ...value, families: next })}
            validKey={(key) => families.includes(key)}
          >
            {(pool, update) => (
              <BudgetPeriods value={pool} onChange={update} daily />
            )}
          </PolicyMapEditor>
          {families.length > 0 && (
            <p className='text-muted-foreground text-sm'>
              {t('Model family IDs')}: {families.join(', ')}
            </p>
          )}
        </>
      )}
    </FieldGroup>
  )
}
