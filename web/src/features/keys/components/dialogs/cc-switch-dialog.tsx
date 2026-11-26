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
import { useState, useEffect, useId, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { toIntlLocale } from '@/i18n/languages'

import {
  buildCCSwitchURL,
  resolveCCSwitchGateway,
  supportsCCSwitchUsage,
  type CCSwitchUsageLabels,
} from '../../lib/cc-switch-import'
import { fetchOpenCodeConfig } from '../../opencode'

const APP_CONFIGS = {
  claude: {
    label: 'Claude',
    defaultName: 'My Claude',
    modelFields: [
      { key: 'model', labelKey: 'Primary Model', required: true },
      { key: 'haikuModel', labelKey: 'Haiku Model', required: false },
      { key: 'sonnetModel', labelKey: 'Sonnet Model', required: false },
      { key: 'opusModel', labelKey: 'Opus Model', required: false },
    ],
  },
  codex: {
    label: 'Codex',
    defaultName: 'My Codex',
    modelFields: [{ key: 'model', labelKey: 'Primary Model', required: true }],
  },
  gemini: {
    label: 'Gemini',
    defaultName: 'My Gemini',
    modelFields: [{ key: 'model', labelKey: 'Primary Model', required: true }],
  },
  opencode: {
    label: 'OpenCode',
    defaultName: 'Arca',
    modelFields: [{ key: 'model', labelKey: 'Primary Model', required: true }],
  },
} as const

type AppType = keyof typeof APP_CONFIGS

interface KeyCatalog {
  endpoint?: string
  models: string[]
}

async function loadOpenCodeCatalog(
  tokenKey: string,
  signal: AbortSignal
): Promise<KeyCatalog> {
  const data = await fetchOpenCodeConfig(tokenKey, signal)
  const provider = data?.provider?.aether
  const endpoint = provider?.options?.baseURL
  const models = provider?.models
  if (
    typeof endpoint !== 'string' ||
    !models ||
    typeof models !== 'object' ||
    Array.isArray(models)
  ) {
    throw new Error('Failed to load models')
  }
  const url = new URL(endpoint)
  if (
    !['https:', 'http:'].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash
  ) {
    throw new Error('Failed to load models')
  }
  return { endpoint: url.href, models: Object.keys(models) }
}

async function loadRelayCatalog(
  tokenKey: string,
  signal: AbortSignal
): Promise<KeyCatalog> {
  // The relay model list applies the selected key's group and model ACL to
  // all gateway aliases, including those outside OpenCode's chat catalog.
  const response = await fetch('/v1/models', {
    headers: { Authorization: `Bearer ${tokenKey}` },
    credentials: 'omit',
    cache: 'no-store',
    redirect: 'error',
    signal,
  })
  if (!response.ok) throw new Error('Failed to load models')
  const payload = (await response.json()) as {
    success?: boolean
    data?: { id?: string }[]
  } | null
  if (payload?.success === false || !Array.isArray(payload?.data)) {
    throw new Error('Failed to load models')
  }
  const modelIds = payload.data.flatMap((item) =>
    typeof item?.id === 'string' && item.id ? [item.id] : []
  )
  return { models: [...new Set(modelIds)] }
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
  tokenId?: number
  allowUsageQuery?: boolean
}

export function CCSwitchDialog(props: Props) {
  const { t, i18n } = useTranslation()
  const dialogId = useId()
  const apiKey = props.tokenKey.startsWith('sk-')
    ? props.tokenKey
    : `sk-${props.tokenKey}`
  const [app, setApp] = useState<AppType>('claude')
  const [name, setName] = useState<string>(APP_CONFIGS.claude.defaultName)
  const [models, setModels] = useState<Record<string, string>>({})

  const isOpenCode = app === 'opencode'
  const keyCatalog = useQuery({
    // Tie the query to this dialog and key record, without retaining the
    // key itself as a TanStack cache key or devtools identifier.
    queryKey: ['cc-switch-models', dialogId, props.tokenId, isOpenCode],
    queryFn: ({ signal }) => {
      const load = isOpenCode ? loadOpenCodeCatalog : loadRelayCatalog
      return load(apiKey, signal)
    },
    enabled: props.open && Boolean(props.tokenKey),
    gcTime: 0,
    staleTime: 30_000,
    retry: false,
    meta: { errorToast: false },
  })
  const modelOptions = useMemo(
    () => (keyCatalog.data?.models ?? []).map((m) => ({ value: m, label: m })),
    [keyCatalog.data?.models]
  )
  const gateway = resolveCCSwitchGateway(keyCatalog.data?.endpoint)
  const supportsUsage = supportsCCSwitchUsage(gateway)
  const includeUsage = props.allowUsageQuery === true && supportsUsage

  useEffect(() => {
    if (props.open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setModels({})

      setApp('claude')

      setName(APP_CONFIGS.claude.defaultName)
    }
  }, [props.open])

  const currentConfig = APP_CONFIGS[app]

  const handleAppChange = (val: string) => {
    const appVal = val as AppType
    setApp(appVal)
    setName(APP_CONFIGS[appVal].defaultName)
    setModels({})
  }

  const handleSubmit = () => {
    const catalog = keyCatalog.data
    if (
      !models.model ||
      keyCatalog.isFetching ||
      keyCatalog.isError ||
      !catalog?.models.includes(models.model) ||
      Object.values(models).some(
        (model) => model && !catalog.models.includes(model)
      )
    ) {
      toast.warning(t('Please select a primary model'))
      return
    }
    if (isOpenCode) {
      if (
        [name, apiKey, gateway.href, models.model].some((value) =>
          /\$\{|\{env:|\{file:/.test(value)
        )
      ) {
        toast.warning(t('Invalid parameters'))
        return
      }
    }
    let usage: CCSwitchUsageLabels | undefined
    if (includeUsage) {
      usage = {
        locale: toIntlLocale(i18n.resolvedLanguage || i18n.language),
        unit: t('Usage units'),
        subscription: t('Subscription'),
        periods: {
          rolling_5h: t('Rolling 5 hours'),
          day: t('Daily'),
          week: t('Weekly'),
          month: t('Monthly'),
        },
        scopes: { model: t('Model'), family: t('Model family') },
        states: {
          unlimited: t('Unlimited'),
          unassigned: t('No subscription assigned.'),
          paused: t('Paused access'),
          noWindows: t('No subscription quota windows apply to this key.'),
        },
        resetAt: t('Reset at:'),
        reserved: t('Reserved usage'),
        temporary: t('Temporary usage grants'),
        allModels: t('All models'),
        startsAt: t('Available from {{date}}', { date: '%s' }),
        expiresAt: t('Expires at'),
        neverExpires: t('Never expires'),
        queryFailed: t('Failed to fetch usage'),
      }
    }
    const url = buildCCSwitchURL({ app, name, models, apiKey, gateway, usage })
    window.open(url, '_blank')
    props.onOpenChange(false)
  }

  let usageHint = t('Subscription usage will be enabled in CC Switch.')
  if (!props.allowUsageQuery) {
    usageHint = t(
      'Enable “Allow querying user quota” in this API key’s settings to include subscription usage.'
    )
  } else if (!supportsUsage) {
    usageHint = t(
      'Subscription usage requires HTTPS or a local loopback address.'
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Import to CC Switch')}
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            disabled={keyCatalog.isFetching || keyCatalog.isError}
            onClick={handleSubmit}
          >
            {t('Open CC Switch')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='space-y-2'>
          <Label>{t('Application')}</Label>
          <RadioGroup
            value={app}
            onValueChange={handleAppChange}
            className='flex flex-wrap gap-4'
          >
            {(
              Object.entries(APP_CONFIGS) as [
                AppType,
                (typeof APP_CONFIGS)[AppType],
              ][]
            ).map(([key, cfg]) => (
              <div key={key} className='flex items-center gap-2'>
                <RadioGroupItem value={key} id={`app-${key}`} />
                <Label htmlFor={`app-${key}`} className='cursor-pointer'>
                  {cfg.label}
                </Label>
              </div>
            ))}
          </RadioGroup>
        </div>

        <div className='space-y-2'>
          <Label htmlFor='cc-switch-name'>{t('Name')}</Label>
          <Input
            id='cc-switch-name'
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder={currentConfig.defaultName}
          />
        </div>

        {keyCatalog.isError && (
          <p role='alert' className='text-destructive text-sm'>
            {t('Failed to load models')}
          </p>
        )}
        {currentConfig.modelFields.map((field) => (
          <div key={field.key} className='space-y-2'>
            <Label htmlFor={`cc-switch-${field.key}`} required={field.required}>
              {t(field.labelKey)}
            </Label>
            <Combobox
              id={`cc-switch-${field.key}`}
              aria-label={t(field.labelKey)}
              options={modelOptions}
              value={models[field.key] || ''}
              onValueChange={(v) =>
                setModels((prev) => ({ ...prev, [field.key]: v ?? '' }))
              }
              placeholder={t('Select or enter model name')}
              emptyText={t('No models found')}
            />
          </div>
        ))}
        <p className='text-muted-foreground text-sm'>{usageHint}</p>
      </div>
    </Dialog>
  )
}
