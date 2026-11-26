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
import { useState } from 'react'
import { type Control, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  SideDrawerSection,
  SideDrawerSectionHeader,
} from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import dayjs from '@/lib/dayjs'

import { getRedemptionScopes } from '../api'
import {
  isGrantModelAlias,
  MAX_GRANT_MODELS,
  type RedemptionFormValues,
} from '../lib/redemption-form'

type GrantFieldsProps = { control: Control<RedemptionFormValues> }

type ModelPickerProps = {
  value: string[]
  suggestions: string[]
  onChange: (next: string[]) => void
}

function GrantModelPicker({ value, suggestions, onChange }: ModelPickerProps) {
  const { t } = useTranslation()
  const [alias, setAlias] = useState('')
  const trimmedAlias = alias.trim()
  const search = trimmedAlias.toLowerCase()
  const matches = suggestions
    .filter(
      (model) => !value.includes(model) && model.toLowerCase().includes(search)
    )
    .slice(0, 8)
  const atLimit = value.length >= MAX_GRANT_MODELS
  const canAdd =
    !atLimit && isGrantModelAlias(trimmedAlias) && !value.includes(trimmedAlias)

  function addModel(model: string) {
    onChange([...value, model])
    setAlias('')
  }

  return (
    <>
      <div className='flex items-center gap-2'>
        <FormControl>
          <Input
            value={alias}
            placeholder={t('Search or enter a model alias')}
            maxLength={255}
            onChange={(event) => setAlias(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                if (canAdd) addModel(trimmedAlias)
              }
            }}
          />
        </FormControl>
        <Button
          type='button'
          variant='outline'
          disabled={!canAdd}
          onClick={() => addModel(trimmedAlias)}
        >
          {t('Add')}
        </Button>
      </div>
      {matches.length > 0 && !atLimit && (
        <div
          className='flex flex-wrap gap-1.5'
          aria-label={t('Suggested models')}
          role='group'
        >
          {matches.map((model) => (
            <Button
              key={model}
              type='button'
              variant='outline'
              size='xs'
              className='max-w-full'
              onClick={() => addModel(model)}
            >
              <span className='truncate'>{model}</span>
            </Button>
          ))}
        </div>
      )}
      {value.length > 0 && (
        <div className='flex flex-wrap gap-1.5'>
          {value.map((model) => (
            <Button
              key={model}
              type='button'
              variant='secondary'
              size='xs'
              className='max-w-full'
              aria-label={t('Remove {{model}}', { model })}
              onClick={() => onChange(value.filter((item) => item !== model))}
            >
              <span className='truncate'>{model}</span>
              <span aria-hidden='true'>×</span>
            </Button>
          ))}
        </div>
      )}
    </>
  )
}

function GrantScopeFields({
  control,
  scope,
}: GrantFieldsProps & { scope: 'models' | 'family' }) {
  const { t } = useTranslation()
  const scopes = useQuery({
    queryKey: ['redemption-scopes'],
    queryFn: getRedemptionScopes,
    staleTime: 60_000,
  })
  const families = Object.entries(scopes.data?.families ?? {}).sort(
    ([, a], [, b]) => a.localeCompare(b)
  )

  return (
    <>
      {scope === 'models' ? (
        <FormField
          control={control}
          name='models'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Model aliases')}</FormLabel>
              <GrantModelPicker
                value={field.value}
                onChange={field.onChange}
                suggestions={scopes.data?.models ?? []}
              />
              <FormMessage />
            </FormItem>
          )}
        />
      ) : (
        <FormField
          control={control}
          name='family'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Model family')}</FormLabel>
              <FormControl>
                <NativeSelect {...field} className='w-full'>
                  <NativeSelectOption value='' disabled>
                    {t('Choose a model family')}
                  </NativeSelectOption>
                  {field.value &&
                    !families.some(([key]) => key === field.value) && (
                      <NativeSelectOption value={field.value}>
                        {field.value}
                      </NativeSelectOption>
                    )}
                  {families.map(([key, label]) => (
                    <NativeSelectOption key={key} value={key}>
                      {label || key}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </FormControl>
              {scopes.isSuccess && families.length === 0 && (
                <FormDescription>
                  {t('No model families have been configured.')}
                </FormDescription>
              )}
              <FormMessage />
            </FormItem>
          )}
        />
      )}
      {scopes.isError && (
        <p role='status' className='text-muted-foreground text-xs'>
          {t('Could not load model scope choices.')}
        </p>
      )}
    </>
  )
}

const VALIDITY_PRESETS = [
  { label: 'No limit', hours: 0 },
  { label: '1 hour', hours: 1 },
  { label: '1 Day', hours: 24 },
  { label: '1W', hours: 168 },
] as const

export function RedemptionGrantFields({ control }: GrantFieldsProps) {
  const { t } = useTranslation()
  const scope = useWatch({ control, name: 'scope' })

  return (
    <>
      <SideDrawerSection>
        <SideDrawerSectionHeader title={t('Model scope')} />
        <FormField
          control={control}
          name='scope'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Eligible models')}</FormLabel>
              <FormControl>
                <NativeSelect {...field} className='w-full'>
                  <NativeSelectOption value='all'>
                    {t('All permitted models')}
                  </NativeSelectOption>
                  <NativeSelectOption value='models'>
                    {t('Specific models')}
                  </NativeSelectOption>
                  <NativeSelectOption value='family'>
                    {t('Model family')}
                  </NativeSelectOption>
                </NativeSelect>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        {scope !== 'all' && (
          <GrantScopeFields control={control} scope={scope} />
        )}
      </SideDrawerSection>

      <SideDrawerSection>
        <SideDrawerSectionHeader title={t('Grant validity')} />
        <div className='grid gap-3 sm:grid-cols-2'>
          {(['startsAt', 'endsAt'] as const).map((name) => (
            <FormField
              key={name}
              control={control}
              name={name}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {name === 'startsAt' ? t('Usage starts') : t('Usage ends')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      name={field.name}
                      ref={field.ref}
                      onBlur={field.onBlur}
                      type='datetime-local'
                      step='1'
                      value={
                        field.value
                          ? dayjs(field.value).format('YYYY-MM-DDTHH:mm:ss')
                          : ''
                      }
                      onChange={(event) =>
                        field.onChange(
                          event.target.value
                            ? new Date(event.target.value)
                            : null
                        )
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          ))}
        </div>
        <FormField
          control={control}
          name='validHours'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Duration after activation (hours)')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  type='number'
                  step='any'
                  min='0'
                  value={field.value || ''}
                  placeholder={t('No limit')}
                  onChange={(event) =>
                    field.onChange(Number(event.target.value))
                  }
                />
              </FormControl>
              <div className='flex flex-wrap gap-1.5'>
                {VALIDITY_PRESETS.map(({ label, hours }) => (
                  <Button
                    key={hours}
                    type='button'
                    size='sm'
                    variant={field.value === hours ? 'secondary' : 'outline'}
                    aria-pressed={field.value === hours}
                    onClick={() => field.onChange(hours)}
                  >
                    {t(label)}
                  </Button>
                ))}
              </div>
              <FormDescription>
                {t(
                  'Times use your local time zone. Usage starts at redemption unless a later start is set. It ends when the first configured limit is reached.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </SideDrawerSection>
    </>
  )
}
