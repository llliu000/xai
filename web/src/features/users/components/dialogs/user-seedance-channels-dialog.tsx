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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { handleServerError } from '@/lib/handle-server-error'
import {
  createServerError,
  requireServerSuccess,
} from '@/lib/server-error-message'

import {
  getSeedanceChannelOptions,
  getUser,
  updateUserSeedanceChannels,
} from '../../api'
import { userSeedanceChannelsSchema } from '../../lib/seedance-channels'
import type { User, UserSeedanceChannels } from '../../types'

interface Props {
  user: Pick<User, 'id' | 'username'>
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function UserSeedanceChannelsDialog(props: Props) {
  const { t } = useTranslation()
  const formId = useId()
  const client = useQueryClient()
  const queryKey = ['user-seedance-channels', props.user.id]
  const query = useQuery({
    queryKey,
    queryFn: async () => {
      const [userResponse, channelsResponse] = await Promise.all([
        getUser(props.user.id),
        getSeedanceChannelOptions(),
      ])
      const user = requireServerSuccess(userResponse).data
      const channels = requireServerSuccess(channelsResponse).data ?? []
      if (!user) throw createServerError(userResponse, t('Failed to load'))
      const saved = userSeedanceChannelsSchema
        .partial()
        .parse(JSON.parse(user.setting || '{}'))
      return {
        channels,
        bindings: {
          seedance_inner_channel_id: saved.seedance_inner_channel_id ?? 0,
          seedance_outer_channel_id: saved.seedance_outer_channel_id ?? 0,
        },
      }
    },
    staleTime: 0,
    refetchOnWindowFocus: false,
    meta: { errorToast: false },
  })
  const form = useForm<UserSeedanceChannels>({
    resolver: zodResolver(userSeedanceChannelsSchema),
    defaultValues: {
      seedance_inner_channel_id: 0,
      seedance_outer_channel_id: 0,
    },
    values: query.data?.bindings,
  })
  const save = useMutation({
    mutationFn: async (values: UserSeedanceChannels) =>
      requireServerSuccess(
        await updateUserSeedanceChannels(props.user.id, values)
      ),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey })
      toast.success(t('Settings updated'))
      props.onOpenChange(false)
      props.onSuccess?.()
    },
    onError: (error) => handleServerError(error, t('Update failed')),
  })
  const loading = query.isPending || query.isFetching
  const options = (query.data?.channels ?? []).map((channel) => ({
    value: String(channel.id),
    label: `${channel.name} (#${channel.id})${channel.status === 1 ? '' : ` · ${t('Disabled')}`}`,
    disabled: channel.status !== 1,
  }))
  for (const id of Object.values(query.data?.bindings ?? {})) {
    if (id > 0 && !options.some((option) => option.value === String(id))) {
      options.push({
        value: String(id),
        label: t('Deleted channel (ID: {{id}})', { id }),
        disabled: true,
      })
    }
  }
  options.unshift({ value: '0', label: t('No binding'), disabled: false })

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!save.isPending) props.onOpenChange(open)
      }}
      title={t('Doubao channel binding')}
      description={t('Bind Doubao video channels for {{username}}', {
        username: props.user.username,
      })}
      contentClassName='sm:max-w-lg'
      footer={
        <>
          <Button
            variant='outline'
            disabled={save.isPending}
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form={formId}
            disabled={loading || query.isError || save.isPending}
          >
            {save.isPending ? t('Saving...') : t('Save')}
          </Button>
        </>
      }
    >
      {loading && <LoadingState size='sm' />}
      {!loading && query.isError && (
        <ErrorState
          title={t('Failed to load')}
          onRetry={() => void query.refetch()}
        />
      )}
      {!loading && query.isSuccess && (
        <form
          id={formId}
          onSubmit={form.handleSubmit((values) => save.mutate(values))}
        >
          <FieldGroup>
            {(
              [
                {
                  name: 'seedance_inner_channel_id',
                  label: t('Domestic channel'),
                },
                {
                  name: 'seedance_outer_channel_id',
                  label: t('Overseas channel'),
                },
              ] as const
            ).map((item) => (
              <Controller
                key={item.name}
                control={form.control}
                name={item.name}
                render={({ field, fieldState }) => (
                  <Field
                    data-disabled={save.isPending}
                    data-invalid={fieldState.invalid}
                  >
                    <FieldLabel htmlFor={`${formId}-${item.name}`}>
                      {item.label}
                    </FieldLabel>
                    <Combobox
                      id={`${formId}-${item.name}`}
                      ref={field.ref}
                      name={field.name}
                      aria-label={item.label}
                      aria-invalid={fieldState.invalid}
                      value={String(field.value)}
                      options={options}
                      onValueChange={(value) =>
                        field.onChange(Number(value ?? 0))
                      }
                      onBlur={field.onBlur}
                      disabled={save.isPending}
                      emptyText={t('No channels found')}
                    />
                    {fieldState.error && (
                      <FieldError errors={[fieldState.error]} />
                    )}
                  </Field>
                )}
              />
            ))}
            <p className='text-muted-foreground text-sm'>
              {t('Select No binding to clear a channel binding.')}
            </p>
          </FieldGroup>
        </form>
      )}
    </Dialog>
  )
}
