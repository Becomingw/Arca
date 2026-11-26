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
import { ImagePlus, Trash2 } from 'lucide-react'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldLabel,
} from '@/components/ui/field'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  arcaProfileKey,
  updateArcaActivityPrivacy,
  updateArcaAvatar,
  useArcaProfile,
  type ArcaProfile,
} from '../arca-profile-api'

async function cropAvatar(file: File): Promise<Blob> {
  const bitmap = await createImageBitmap(file)
  try {
    const canvas = document.createElement('canvas')
    canvas.width = canvas.height = 256
    const context = canvas.getContext('2d')
    if (!context) throw new Error('Image rendering unavailable')
    const side = Math.min(bitmap.width, bitmap.height)
    context.drawImage(
      bitmap,
      (bitmap.width - side) / 2,
      (bitmap.height - side) / 2,
      side,
      side,
      0,
      0,
      256,
      256
    )
    return await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(
        (blob) => {
          if (blob) resolve(blob)
          else reject(new Error('Image encoding failed'))
        },
        'image/webp',
        0.85
      )
    })
  } finally {
    bitmap.close()
  }
}

export function ArcaAppearanceCard() {
  const { t } = useTranslation()
  const userId = useAuthStore((s) => s.auth.user?.id)
  const profile = useArcaProfile()
  const queryClient = useQueryClient()
  const fileInput = useRef<HTMLInputElement>(null)
  const avatarMutation = useMutation({
    mutationFn: async (file: File | null) => {
      const sessionId = useAuthStore.getState().auth.session?.sid
      const blob = file ? await cropAvatar(file) : null
      if (useAuthStore.getState().auth.session?.sid !== sessionId) {
        throw new Error(t('Session expired!'))
      }
      return updateArcaAvatar(blob)
    },
    onSuccess: (next) => {
      queryClient.setQueryData<ArcaProfile>(
        arcaProfileKey(userId),
        (previous) => previous && { ...previous, ...next }
      )
      toast.success(t('Saved successfully'))
    },
    onError: (error) => handleServerError(error, t('Failed to update avatar')),
  })
  const privacyMutation = useMutation({
    mutationFn: updateArcaActivityPrivacy,
    onSuccess: (next) => {
      queryClient.setQueryData<ArcaProfile>(
        arcaProfileKey(userId),
        (previous) => previous && { ...previous, ...next }
      )
      void queryClient.invalidateQueries({ queryKey: ['team-activity'] })
      toast.success(t('Saved successfully'))
    },
    onError: (error) => handleServerError(error, t('Failed to save changes')),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Avatar and activity privacy')}</CardTitle>
        <CardDescription>
          {t('Manage the picture and name shown to other members.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-5'>
        {profile.isPending && <Skeleton className='h-20 w-full' />}
        {profile.isError && (
          <Button
            type='button'
            variant='outline'
            className='self-start'
            onClick={() => void profile.refetch()}
          >
            {t('Retry')}
          </Button>
        )}
        {profile.data && (
          <>
            <div className='flex flex-wrap gap-2'>
              <input
                ref={fileInput}
                type='file'
                accept='image/png,image/jpeg,image/webp'
                hidden
                onChange={(event) => {
                  const file = event.currentTarget.files?.[0]
                  event.currentTarget.value = ''
                  if (!file) return
                  if (
                    !['image/png', 'image/jpeg', 'image/webp'].includes(
                      file.type
                    ) ||
                    file.size > 10 * 1024 * 1024
                  ) {
                    toast.error(
                      t('Choose a PNG, JPEG, or WebP image under 10 MB.')
                    )
                    return
                  }
                  avatarMutation.mutate(file)
                }}
              />
              <Button
                type='button'
                variant='outline'
                onClick={() => fileInput.current?.click()}
                disabled={avatarMutation.isPending}
              >
                <ImagePlus data-icon='inline-start' />
                {t('Change avatar')}
              </Button>
              {profile.data.avatar && (
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => avatarMutation.mutate(null)}
                  disabled={avatarMutation.isPending}
                >
                  <Trash2 data-icon='inline-start' />
                  {t('Remove avatar')}
                </Button>
              )}
            </div>
            <Field orientation='horizontal'>
              <FieldContent>
                <FieldLabel htmlFor='activity-anonymous'>
                  {t('Appear anonymously in team activity')}
                </FieldLabel>
                <FieldDescription>
                  {t(
                    'The leaderboard shows an anonymous label instead of your username.'
                  )}
                </FieldDescription>
              </FieldContent>
              <Switch
                id='activity-anonymous'
                checked={profile.data.leaderboard_anonymous}
                onCheckedChange={(checked) => privacyMutation.mutate(checked)}
                disabled={privacyMutation.isPending}
              />
            </Field>
          </>
        )}
      </CardContent>
    </Card>
  )
}
