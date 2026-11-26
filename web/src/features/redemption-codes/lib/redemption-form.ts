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
import type { TFunction } from 'i18next'
import { z } from 'zod'

import { getCurrencyDisplay } from '@/lib/currency'

import {
  REDEMPTION_VALIDATION,
  getRedemptionFormErrorMessages,
} from '../constants'
import type { RedemptionFormData, Redemption, UsageGrantScope } from '../types'

const SECONDS_PER_HOUR = 3600
const MAX_VALIDITY_HOURS = 365 * 24
export const MAX_GRANT_MODELS = 100

export function isGrantModelAlias(alias: string) {
  return (
    alias.length > 0 &&
    alias === alias.trim() &&
    alias !== '*' &&
    new TextEncoder().encode(alias).length <= 255 &&
    !['\r', '\n', '\0'].some((character) => alias.includes(character))
  )
}

export function quotaToUsageUnits(quota: number) {
  return quota / getCurrencyDisplay().config.quotaPerUnit
}

function usageUnitsToQuota(units: number) {
  return Math.round(units * getCurrencyDisplay().config.quotaPerUnit)
}

export function formatUsageUnits(
  amount: number,
  locale?: Intl.LocalesArgument
) {
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 6 }).format(
    amount
  )
}

export function getRedemptionFormSchema(t: TFunction) {
  const msg = getRedemptionFormErrorMessages(t)
  return z
    .object({
      name: z
        .string()
        .trim()
        .min(REDEMPTION_VALIDATION.NAME_MIN_LENGTH, msg.NAME_LENGTH_INVALID)
        .max(REDEMPTION_VALIDATION.NAME_MAX_LENGTH, msg.NAME_LENGTH_INVALID),
      units: z.number().refine((units) => {
        const quota = usageUnitsToQuota(units)
        return Number.isSafeInteger(quota) && quota > 0
      }, t('Enter a positive number of usage units within the supported range')),
      expired_time: z.date().nullable(),
      count: z
        .number()
        .int()
        .min(REDEMPTION_VALIDATION.COUNT_MIN, msg.COUNT_INVALID)
        .max(REDEMPTION_VALIDATION.COUNT_MAX, msg.COUNT_INVALID)
        .optional(),
      scope: z.enum(['all', 'models', 'family']),
      models: z.array(z.string()),
      family: z.string(),
      startsAt: z.date().nullable(),
      endsAt: z.date().nullable(),
      validHours: z
        .number()
        .refine(
          (hours) =>
            hours === 0 ||
            (hours * SECONDS_PER_HOUR >= 1 && hours <= MAX_VALIDITY_HOURS),
          t('Set a duration between one second and 365 days')
        ),
    })
    .superRefine((values, context) => {
      if (
        values.scope === 'models' &&
        (values.models.length === 0 ||
          values.models.length > MAX_GRANT_MODELS ||
          !values.models.every(isGrantModelAlias))
      ) {
        context.addIssue({
          code: 'custom',
          path: ['models'],
          message: t('Choose between 1 and 100 valid model aliases'),
        })
      }
      if (values.scope === 'family' && !values.family.trim()) {
        context.addIssue({
          code: 'custom',
          path: ['family'],
          message: t('Choose a model family'),
        })
      }
      if (
        values.endsAt &&
        dateToSeconds(values.endsAt) <=
          Math.max(
            Math.floor(Date.now() / 1000),
            values.startsAt ? dateToSeconds(values.startsAt) : 0
          )
      ) {
        context.addIssue({
          code: 'custom',
          path: ['endsAt'],
          message: t(
            'The usage end must be after activation and the current time'
          ),
        })
      }
    })
}

export type RedemptionFormValues = z.infer<
  ReturnType<typeof getRedemptionFormSchema>
>

export const REDEMPTION_FORM_DEFAULT_VALUES: RedemptionFormValues = {
  name: '',
  units: 10,
  expired_time: null,
  count: 1,
  scope: 'all',
  models: [],
  family: '',
  startsAt: null,
  endsAt: null,
  validHours: 0,
}

function dateToSeconds(value: Date) {
  return Math.floor(value.getTime() / 1000)
}

function dateFromSeconds(value?: number) {
  return value && value > 0 ? new Date(value * 1000) : null
}

export function transformFormDataToPayload(
  data: RedemptionFormValues
): RedemptionFormData {
  const grant: UsageGrantScope = {}
  if (data.scope === 'models') grant.models = data.models
  if (data.scope === 'family') grant.family = data.family.trim()
  if (data.startsAt) grant.starts_at = dateToSeconds(data.startsAt)
  if (data.endsAt) grant.ends_at = dateToSeconds(data.endsAt)
  if (data.validHours > 0) {
    grant.valid_for_seconds = Math.round(data.validHours * SECONDS_PER_HOUR)
  }
  return {
    name: data.name,
    quota: usageUnitsToQuota(data.units),
    expired_time: data.expired_time ? dateToSeconds(data.expired_time) : 0,
    count: data.count || 1,
    ...(Object.keys(grant).length > 0 ? { grant } : {}),
  }
}

export function transformRedemptionToFormDefaults(
  redemption: Redemption
): RedemptionFormValues {
  const grant = redemption.grant
  let scope: RedemptionFormValues['scope'] = 'all'
  if (grant?.family) {
    scope = 'family'
  } else if (grant?.models?.length) {
    scope = 'models'
  }
  return {
    ...REDEMPTION_FORM_DEFAULT_VALUES,
    name: redemption.name,
    units: quotaToUsageUnits(redemption.quota),
    expired_time: dateFromSeconds(redemption.expired_time),
    scope,
    models: grant?.models ?? [],
    family: grant?.family ?? '',
    startsAt: dateFromSeconds(grant?.starts_at),
    endsAt: dateFromSeconds(grant?.ends_at),
    validHours: (grant?.valid_for_seconds ?? 0) / SECONDS_PER_HOUR,
  }
}
