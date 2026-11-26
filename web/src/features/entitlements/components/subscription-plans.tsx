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
import { Copy, Plus, Trash2 } from 'lucide-react'
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
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'

import type { EntitlementPolicy, SubscriptionPlan } from '../api'
import { blankAllocation } from '../policy'
import {
  AllocationFields,
  ModelAccessFields,
  PolicyTextField,
} from './policy-fields'

const presets = {
  starter: {
    label: 'Starter',
    five_hour_budget_usd: 2,
    daily_budget_usd: 5,
    weekly_budget_usd: 20,
    monthly_budget_usd: 60,
  },
  standard: {
    label: 'Standard',
    five_hour_budget_usd: 5,
    daily_budget_usd: 10,
    weekly_budget_usd: 40,
    monthly_budget_usd: 120,
  },
  unlimited: { label: 'Unlimited', unlimited: true },
  suspended: { label: 'Paused access' },
} as const

type Preset = keyof typeof presets

function newPlanID(plans: Record<string, SubscriptionPlan>) {
  let sequence = 1
  while (Object.hasOwn(plans, `plan-${sequence}`)) sequence++
  return `plan-${sequence}`
}

type SubscriptionPlansProps = {
  draft: EntitlementPolicy
  aliases: string[]
  onChange: (plans: Record<string, SubscriptionPlan>) => void
}

export function SubscriptionPlans({
  draft,
  aliases,
  onChange,
}: SubscriptionPlansProps) {
  const { t } = useTranslation()
  const id = useId()
  const [preset, setPreset] = useState<Preset>('standard')
  const [selected, setSelected] = useState<string | null>(null)
  const plans = draft.plans ?? {}
  const planIds = Object.keys(plans)
  const planId = selected ?? planIds[0]
  const plan = planId ? plans[planId] : undefined
  const presetOptions = Object.entries(presets).map(([value, option]) => ({
    value,
    label: t(option.label),
  }))
  const assignedCount = planId
    ? Object.values(draft.user_subscriptions ?? {}).filter(
        (target) => target === planId
      ).length +
      Object.values(draft.group_subscriptions ?? {}).filter(
        (target) => target === planId
      ).length
    : 0

  const create = (source?: SubscriptionPlan) => {
    const nextId = newPlanID(plans)
    const { label, ...limits } = presets[preset]
    let next: SubscriptionPlan = {
      name: t(label),
      allocation: { ...blankAllocation(), ...limits, internal_funding: true },
      model_access: {
        mode: preset === 'suspended' ? 'deny_all' : 'unrestricted',
      },
    }
    if (source) {
      next = { ...source, name: t('{{name}} (copy)', { name: source.name }) }
    }
    onChange({ ...plans, [nextId]: next })
    setSelected(nextId)
  }
  const updatePlan = (next: SubscriptionPlan) => {
    if (planId) onChange({ ...plans, [planId]: next })
  }

  return (
    <div className='grid items-start gap-4 lg:grid-cols-[minmax(15rem,1fr)_minmax(0,2fr)]'>
      <Card>
        <CardHeader>
          <CardTitle>{t('Subscription plans')}</CardTitle>
          <CardDescription>
            {t('Start with a template and adjust it before assigning it.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-4'>
          <Field>
            <FieldLabel htmlFor={`${id}-preset`}>
              {t('Plan template')}
            </FieldLabel>
            <Combobox
              id={`${id}-preset`}
              options={presetOptions}
              value={preset}
              onValueChange={(value: string | null) => {
                if (value && Object.hasOwn(presets, value)) {
                  setPreset(value as Preset)
                }
              }}
            />
          </Field>
          <Button
            type='button'
            onClick={() => create()}
            disabled={planIds.length >= 128}
          >
            <Plus data-icon='inline-start' />
            {t('Use template')}
          </Button>
          {planIds.length > 0 && (
            <Field>
              <FieldLabel htmlFor={`${id}-plan`}>{t('Edit plan')}</FieldLabel>
              <Combobox
                id={`${id}-plan`}
                options={planIds.map((value) => ({
                  value,
                  label: plans[value].name || value,
                }))}
                value={planId}
                onValueChange={(value) => setSelected(value)}
              />
            </Field>
          )}
        </CardContent>
      </Card>
      {plan && planId && (
        <Card key={planId}>
          <CardHeader>
            <CardTitle className='break-words'>
              {plan.name || t('Edit plan')}
            </CardTitle>
            <CardDescription>
              {t('Assignments: {{count}}', { count: assignedCount })}
            </CardDescription>
          </CardHeader>
          <CardContent className='flex flex-col gap-4'>
            <FieldGroup>
              <PolicyTextField
                label={t('Plan name')}
                value={plan.name}
                onChange={(name) => updatePlan({ ...plan, name })}
              />
              <Field>
                <FieldLabel htmlFor={`${id}-description`}>
                  {t('Description')}
                </FieldLabel>
                <Textarea
                  id={`${id}-description`}
                  rows={2}
                  maxLength={512}
                  value={plan.description ?? ''}
                  onChange={(event) =>
                    updatePlan({
                      ...plan,
                      description: event.currentTarget.value,
                    })
                  }
                />
              </Field>
              <ModelAccessFields
                value={plan.model_access}
                onChange={(model_access) =>
                  updatePlan({ ...plan, model_access })
                }
                managed
                aliases={aliases}
              />
              <FieldDescription>
                {t(
                  'Allowances use the gateway’s configured model pricing. Leaving a period blank adds no cap for that period.'
                )}
              </FieldDescription>
              <AllocationFields
                value={plan.allocation}
                onChange={(allocation) => updatePlan({ ...plan, allocation })}
                families={Object.keys(draft.model_families ?? {})}
                aliases={aliases}
              />
            </FieldGroup>
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='outline'
                onClick={() => create(plan)}
                disabled={planIds.length >= 128}
              >
                <Copy data-icon='inline-start' />
                {t('Duplicate plan')}
              </Button>
              <Button
                type='button'
                variant='outline'
                disabled={assignedCount > 0}
                onClick={() => {
                  onChange(
                    Object.fromEntries(
                      Object.entries(plans).filter(([key]) => key !== planId)
                    )
                  )
                  setSelected(null)
                }}
              >
                <Trash2 data-icon='inline-start' />
                {t('Delete plan')}
              </Button>
            </div>
            {assignedCount > 0 && (
              <p className='text-muted-foreground text-sm'>
                {t('Remove the assignments before deleting this plan.')}
              </p>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}
