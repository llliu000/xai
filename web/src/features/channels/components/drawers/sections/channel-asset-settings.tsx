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
import { Library } from 'lucide-react'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  SideDrawerSection,
  SideDrawerSectionHeader,
} from '@/components/drawer-layout'
import { JsonEditor } from '@/components/json-editor'
import { FieldGroup, FieldLegend, FieldSet } from '@/components/ui/field'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { AssetCategoryOption } from '../../../api'
import { CHANNEL_TYPE_TASK_PLUGIN } from '../../../constants'
import type { ChannelFormValues } from '../../../lib/channel-form'

export function ChannelAssetSettings(props: {
  disabled: boolean
  categories: AssetCategoryOption[]
}) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const type = useWatch({ control: form.control, name: 'type' })
  const plugin = useWatch({ control: form.control, name: 'task_plugin_key' })
  if (type !== CHANNEL_TYPE_TASK_PLUGIN || plugin !== 'doubao') return null
  const categories = props.categories.map((item) => ({
    value: item.value,
    label: t(item.label),
  }))
  const regions = [
    { value: 'cn-beijing', label: t('Domestic channel') },
    { value: 'ap-southeast-1', label: t('Overseas channel') },
  ]

  return (
    <SideDrawerSection>
      <SideDrawerSectionHeader
        title={t('Asset library')}
        icon={<Library className='h-4 w-4' aria-hidden='true' />}
        iconTone='info'
      />
      <FieldSet
        disabled={props.disabled}
        className='border-border/60 bg-muted/10 rounded-lg border p-4 disabled:opacity-60'
      >
        <FieldGroup>
          <FormField
            control={form.control}
            name='region'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Video region')}</FormLabel>
                <Select
                  disabled={props.disabled}
                  value={field.value || 'cn-beijing'}
                  items={regions}
                  onValueChange={field.onChange}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {regions.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='category'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Asset library type')}</FormLabel>
                <Select
                  disabled={props.disabled || categories.length === 0}
                  value={field.value || null}
                  items={categories}
                  onValueChange={(value) => {
                    if (value === field.value) return
                    field.onChange(value)
                    form.setValue('category_setting', '{}', {
                      shouldDirty: true,
                      shouldValidate: true,
                    })
                  }}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue
                        placeholder={t('Select asset library type')}
                      />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {categories.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='category_setting'
            render={({ field }) => (
              <FormItem>
                <FieldSet>
                  <FieldLegend variant='label'>
                    {t('Asset library settings')}
                  </FieldLegend>
                  <JsonEditor
                    value={field.value || '{}'}
                    onChange={(value) => field.onChange(value.trim() || '{}')}
                    disabled={props.disabled}
                    valueType='any'
                    showHeader={false}
                    keyPlaceholder={t('Field name')}
                  />
                </FieldSet>
                <FormMessage />
              </FormItem>
            )}
          />
        </FieldGroup>
      </FieldSet>
    </SideDrawerSection>
  )
}
