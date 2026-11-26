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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout/components/section-page-layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { FieldGroup } from '@/components/ui/field'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { getEnabledModels } from '@/features/channels/api'
import { handleServerError } from '@/lib/handle-server-error'
import {
  getServerErrorStatus,
  requireServerSuccess,
} from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  previewEntitlementPolicy,
  readEntitlementPolicy,
  writeEntitlementPolicy,
  type EntitlementPolicy,
  type ModelAccess,
  type Organization,
  type PolicyRecord,
} from './api'
import { EntitlementSnapshot } from './components/entitlement-snapshot'
import {
  AllocationFields,
  ModelAccessFields,
  PolicyListField,
  PolicyMapEditor,
  PolicyTextField,
} from './components/policy-fields'
import { SubscriptionAssignments } from './components/subscription-assignments'
import { SubscriptionPlans } from './components/subscription-plans'
import { blankAllocation, userIdPattern } from './policy'

const policyQueryKey = (userId: number | undefined) =>
  ['entitlements', 'policy', userId] as const
const isPositiveId = (value: string) =>
  userIdPattern.test(value) && Number.isSafeInteger(Number(value))

function EntitlementEditor({
  current,
  reload,
}: {
  current: PolicyRecord
  reload: () => Promise<PolicyRecord | undefined>
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const modelCatalog = useQuery({
    queryKey: ['entitlements', 'enabled-models', userId],
    queryFn: async () =>
      requireServerSuccess(await getEnabledModels()).data ?? [],
    staleTime: 60_000,
  })
  const aliases = Array.isArray(modelCatalog.data) ? modelCatalog.data : []
  const [draft, setDraft] = useState(current.policy)
  const [baseline, setBaseline] = useState(current.policy)
  const [version, setVersion] = useState(current.version)
  const [editorRevision, setEditorRevision] = useState(0)
  const [conflict, setConflict] = useState(false)
  const [previewUser, setPreviewUser] = useState('')
  const [previewKey, setPreviewKey] = useState('')
  const [reloading, setReloading] = useState(false)
  const outdated = conflict || current.version !== version
  const dirty = draft !== baseline
  const familyIds = Object.keys(draft.model_families ?? {})
  const organizationIds = Object.keys(draft.organizations ?? {})
  const validPreview =
    isPositiveId(previewUser) && (previewKey === '' || isPositiveId(previewKey))

  const setPolicy = <K extends keyof EntitlementPolicy>(
    key: K,
    value: EntitlementPolicy[K]
  ) => {
    setDraft((previous) => ({ ...previous, [key]: value }))
  }
  const preview = useMutation({
    mutationFn: (config: EntitlementPolicy) =>
      previewEntitlementPolicy(
        Number(previewUser),
        previewKey ? Number(previewKey) : undefined,
        config
      ),
    onError: (error) =>
      handleServerError(error, t('Failed to preview entitlements')),
  })
  const save = useMutation({
    mutationFn: (policy: EntitlementPolicy) =>
      writeEntitlementPolicy(version, policy),
    onSuccess: (result, submitted) => {
      setVersion(result.version)
      setBaseline(result.policy)
      setDraft((previous) =>
        previous === submitted ? result.policy : previous
      )
      setConflict(false)
      preview.reset()
      queryClient.setQueryData(policyQueryKey(userId), result)
      void queryClient.invalidateQueries({
        queryKey: ['entitlements', 'personal'],
      })
      toast.success(t('Saved successfully'))
    },
    onError: (error) => {
      if (getServerErrorStatus(error) === 409) {
        setConflict(true)
        return
      }
      handleServerError(error, t('Failed to save changes'))
    },
  })

  const familyUsed = (id: string) =>
    [
      draft.default,
      ...Object.values(draft.groups ?? {}),
      ...Object.values(draft.users ?? {}),
      ...Object.values(draft.plans ?? {}).map((plan) => plan.allocation),
    ].some(
      (allocation) =>
        allocation?.families && Object.hasOwn(allocation.families, id)
    )

  return (
    <div className='mx-auto flex w-full max-w-7xl flex-col gap-5'>
      {outdated && (
        <Alert>
          <AlertDescription>
            {t(
              'This policy changed on the server. Your draft is still here. Reload the saved policy before editing a new version.'
            )}
          </AlertDescription>
        </Alert>
      )}
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <span className='text-muted-foreground text-sm'>
          {t('Policy version {{version}}', { version })}
        </span>
        <div className='flex gap-2'>
          <Button
            type='button'
            variant='outline'
            disabled={save.isPending || preview.isPending || reloading}
            onClick={async () => {
              if (
                dirty &&
                !window.confirm(t('Discard unsaved policy changes?'))
              ) {
                return
              }
              setReloading(true)
              try {
                const latest = await reload()
                if (latest) {
                  setDraft(latest.policy)
                  setBaseline(latest.policy)
                  setVersion(latest.version)
                  setEditorRevision((previous) => previous + 1)
                  setConflict(false)
                  preview.reset()
                }
              } finally {
                setReloading(false)
              }
            }}
          >
            {t('Reload saved policy')}
          </Button>
          <Button
            type='button'
            disabled={
              !dirty ||
              outdated ||
              save.isPending ||
              preview.isPending ||
              reloading
            }
            onClick={() => save.mutate(draft)}
          >
            {t('Save policy')}
          </Button>
        </div>
      </div>
      <PolicyTextField
        label={t('Budget time zone')}
        value={draft.timezone}
        onChange={(value) => setPolicy('timezone', value)}
        description={t('Use an IANA time zone, for example Asia/Taipei.')}
      />
      <Tabs defaultValue='plans'>
        <div className='overflow-x-auto pb-1'>
          <TabsList className='min-w-max'>
            <TabsTrigger value='plans'>{t('Subscription plans')}</TabsTrigger>
            <TabsTrigger value='assignments'>{t('Assignments')}</TabsTrigger>
            <TabsTrigger value='families'>{t('Model families')}</TabsTrigger>
            <TabsTrigger value='preview'>{t('Preview')}</TabsTrigger>
            <TabsTrigger value='organizations'>
              {t('Organizations')}
            </TabsTrigger>
            <TabsTrigger value='budgets'>{t('Legacy allowances')}</TabsTrigger>
            <TabsTrigger value='models'>
              {t('Legacy user model access')}
            </TabsTrigger>
          </TabsList>
        </div>
        <TabsContent value='plans' key={`plans-${editorRevision}`}>
          <SubscriptionPlans
            draft={draft}
            aliases={aliases}
            onChange={(plans) => setPolicy('plans', plans)}
          />
        </TabsContent>
        <TabsContent value='assignments' key={`assignments-${editorRevision}`}>
          <SubscriptionAssignments
            draft={draft}
            adminId={userId}
            onChange={setPolicy}
          />
        </TabsContent>
        <TabsContent
          value='organizations'
          key={`organization-${editorRevision}`}
        >
          <PolicyMapEditor<Organization>
            label={t('Add organization')}
            idLabel={t('Organization ID')}
            values={draft.organizations}
            create={(id) => ({
              name: id,
              members: [],
              model_access: { mode: 'inherit' },
            })}
            onChange={(value) => setPolicy('organizations', value)}
            mayRemove={(id) =>
              !draft.groups || !Object.hasOwn(draft.groups, id)
            }
          >
            {(organization, update) => (
              <FieldGroup>
                <PolicyTextField
                  label={t('Organization name')}
                  value={organization.name}
                  onChange={(name) => update({ ...organization, name })}
                />
                <PolicyListField
                  label={t('Member user IDs')}
                  entries={organization.members}
                  onChange={(ids) =>
                    update({
                      ...organization,
                      members: ids.map((id) =>
                        isPositiveId(id) ? Number(id) : 0
                      ),
                    })
                  }
                  numeric
                />
                <ModelAccessFields
                  value={organization.model_access}
                  onChange={(model_access) =>
                    update({ ...organization, model_access })
                  }
                  aliases={aliases}
                />
              </FieldGroup>
            )}
          </PolicyMapEditor>
        </TabsContent>
        <TabsContent value='families' key={`family-${editorRevision}`}>
          <PolicyMapEditor
            label={t('Add model family')}
            idLabel={t('Model family ID')}
            values={draft.model_families}
            create={(id) => ({ name: id, models: [] })}
            onChange={(value) => setPolicy('model_families', value)}
            mayRemove={(id) => !familyUsed(id)}
          >
            {(family, update) => (
              <FieldGroup>
                <PolicyTextField
                  label={t('Model family name')}
                  value={family.name}
                  onChange={(name) => update({ ...family, name })}
                />
                <PolicyListField
                  label={t('Models')}
                  entries={family.models}
                  onChange={(models) => update({ ...family, models })}
                  choices={aliases}
                />
              </FieldGroup>
            )}
          </PolicyMapEditor>
        </TabsContent>
        <TabsContent value='budgets' key={`budget-${editorRevision}`}>
          <div className='flex flex-col gap-5'>
            <Card>
              <CardHeader>
                <CardTitle>{t('Default budget')}</CardTitle>
              </CardHeader>
              <CardContent className='flex flex-col gap-4'>
                {draft.default ? (
                  <>
                    <AllocationFields
                      value={draft.default}
                      onChange={(value) => setPolicy('default', value)}
                      families={familyIds}
                      aliases={aliases}
                    />
                    <Button
                      type='button'
                      variant='outline'
                      className='self-start'
                      onClick={() => setPolicy('default', undefined)}
                    >
                      {t('Remove default budget')}
                    </Button>
                  </>
                ) : (
                  <Button
                    type='button'
                    variant='outline'
                    className='self-start'
                    onClick={() => setPolicy('default', blankAllocation())}
                  >
                    {t('Add default budget')}
                  </Button>
                )}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>{t('Organization budgets')}</CardTitle>
              </CardHeader>
              <CardContent className='flex flex-col gap-3'>
                {organizationIds.length > 0 && (
                  <p className='text-muted-foreground text-sm'>
                    {t('Organization IDs')}: {organizationIds.join(', ')}
                  </p>
                )}
                <PolicyMapEditor
                  label={t('Add organization budget')}
                  idLabel={t('Organization ID')}
                  values={draft.groups}
                  create={blankAllocation}
                  onChange={(value) => setPolicy('groups', value)}
                  validKey={(id) => organizationIds.includes(id)}
                >
                  {(allocation, update) => (
                    <AllocationFields
                      value={allocation}
                      onChange={update}
                      families={familyIds}
                      aliases={aliases}
                    />
                  )}
                </PolicyMapEditor>
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>{t('User budgets')}</CardTitle>
              </CardHeader>
              <CardContent>
                <PolicyMapEditor
                  label={t('Add user budget')}
                  idLabel={t('User ID')}
                  values={draft.users}
                  create={blankAllocation}
                  onChange={(value) => setPolicy('users', value)}
                  validKey={isPositiveId}
                >
                  {(allocation, update) => (
                    <AllocationFields
                      value={allocation}
                      onChange={update}
                      families={familyIds}
                      aliases={aliases}
                    />
                  )}
                </PolicyMapEditor>
              </CardContent>
            </Card>
          </div>
        </TabsContent>
        <TabsContent value='models' key={`models-${editorRevision}`}>
          <PolicyMapEditor<ModelAccess>
            label={t('Add user model access')}
            idLabel={t('User ID')}
            values={draft.user_model_access}
            create={() => ({ mode: 'inherit' })}
            onChange={(value) => setPolicy('user_model_access', value)}
            validKey={isPositiveId}
          >
            {(modelAccess, update) => (
              <ModelAccessFields
                value={modelAccess}
                onChange={update}
                aliases={aliases}
              />
            )}
          </PolicyMapEditor>
        </TabsContent>
        <TabsContent value='preview'>
          <Card>
            <CardHeader>
              <CardTitle>{t('Preview effective entitlements')}</CardTitle>
            </CardHeader>
            <CardContent className='flex flex-col gap-5'>
              <FieldGroup className='max-w-xl'>
                <PolicyTextField
                  label={t('User ID')}
                  value={previewUser}
                  onChange={(next) => {
                    setPreviewUser(next)
                    preview.reset()
                  }}
                />
                <PolicyTextField
                  label={t('API key ID (optional)')}
                  value={previewKey}
                  onChange={(next) => {
                    setPreviewKey(next)
                    preview.reset()
                  }}
                />
                <Button
                  type='button'
                  className='self-start'
                  disabled={
                    !validPreview || preview.isPending || save.isPending
                  }
                  onClick={() => preview.mutate(draft)}
                >
                  {t('Preview')}
                </Button>
              </FieldGroup>
              {preview.data && (
                <div className='grid gap-4 lg:grid-cols-2'>
                  <Card>
                    <CardHeader>
                      <CardTitle>{t('Saved policy')}</CardTitle>
                    </CardHeader>
                    <CardContent>
                      <EntitlementSnapshot value={preview.data.saved} />
                    </CardContent>
                  </Card>
                  {preview.data.proposed && (
                    <Card>
                      <CardHeader>
                        <CardTitle>{t('Proposed policy')}</CardTitle>
                      </CardHeader>
                      <CardContent>
                        <EntitlementSnapshot value={preview.data.proposed} />
                      </CardContent>
                    </Card>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}

export function Entitlements() {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const policy = useQuery({
    queryKey: policyQueryKey(userId),
    queryFn: ({ signal }) => readEntitlementPolicy(signal),
    enabled: Boolean(userId),
    staleTime: 60_000,
    meta: { errorToast: false },
  })
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Subscriptions and entitlements')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {policy.isPending && <Skeleton className='h-40 w-full' />}
        {policy.isError && !policy.data && (
          <Button
            type='button'
            variant='outline'
            onClick={() => void policy.refetch()}
          >
            {t('Retry')}
          </Button>
        )}
        {policy.data && (
          <EntitlementEditor
            key={userId}
            current={policy.data}
            reload={async () => (await policy.refetch()).data}
          />
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
