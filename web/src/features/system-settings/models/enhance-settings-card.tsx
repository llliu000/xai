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
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { PasswordInput } from '@/components/password-input'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { handleServerError } from '@/lib/handle-server-error'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const enhanceConfigSchema = z.object({
  tool_version: z.string().catch(''),
  base_url: z.string().catch(''),
  api_key: z.string().catch(''),
})

type EnhanceFormValues = z.infer<typeof enhanceConfigSchema>

type Props = {
  config: string
}

export function EnhanceSettingsCard(props: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = z.object({
    tool_version: z
      .string()
      .refine(
        (value) =>
          ['fast', 'standard', 'professional', 'generative'].includes(value),
        t('Select version')
      ),
    base_url: z
      .string()
      .trim()
      .refine((value) => {
        if (!value) return true
        try {
          const url = new URL(value)
          return url.protocol === 'http:' || url.protocol === 'https:'
        } catch {
          return false
        }
      }, t('Enter a valid HTTP or HTTPS URL')),
    api_key: z.string().trim(),
  })
  const defaultValues = useMemo<EnhanceFormValues>(() => {
    try {
      return enhanceConfigSchema.parse(JSON.parse(props.config))
    } catch {
      return { tool_version: '', base_url: '', api_key: '' }
    }
  }, [props.config])
  const form = useForm<EnhanceFormValues>({
    resolver: zodResolver(schema),
    defaultValues,
  })
  const isSaving = form.formState.isSubmitting || updateOption.isPending
  useResetForm(
    form,
    isSaving || form.formState.isDirty ? undefined : defaultValues
  )

  const onSubmit = async (values: EnhanceFormValues) => {
    if (!form.formState.isDirty) {
      toast.info(t('No changes to save'))
      return
    }

    try {
      await updateOption.mutateAsync({
        key: 'enhance.config',
        value: JSON.stringify(values),
      })
      form.reset(values)
    } catch (error) {
      handleServerError(error)
    }
  }

  return (
    <SettingsSection title={t('Super Resolution Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={isSaving}
            isSaveDisabled={!form.formState.isDirty}
          />
          <FormField
            control={form.control}
            name='tool_version'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Tool version')}</FormLabel>
                <FormControl>
                  <NativeSelect
                    {...field}
                    disabled={isSaving}
                    className='w-full'
                  >
                    <NativeSelectOption value='' disabled hidden>
                      {t('Select version')}
                    </NativeSelectOption>
                    <NativeSelectOption value='fast'>
                      {t('Ultra-fast version')}
                    </NativeSelectOption>
                    <NativeSelectOption value='standard'>
                      {t('Standard version')}
                    </NativeSelectOption>
                    <NativeSelectOption value='professional'>
                      {t('Professional version')}
                    </NativeSelectOption>
                    <NativeSelectOption value='generative'>
                      {t('Large model version')}
                    </NativeSelectOption>
                  </NativeSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='base_url'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Base URL')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    disabled={isSaving}
                    placeholder='https://example.com'
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='api_key'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('API Key')}</FormLabel>
                <FormControl>
                  <PasswordInput
                    {...field}
                    disabled={isSaving}
                    autoComplete='new-password'
                    placeholder={t('Enter API Key')}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
