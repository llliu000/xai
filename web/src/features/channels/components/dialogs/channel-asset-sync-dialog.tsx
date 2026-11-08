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
import { useMutation, useQuery } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { getChannelAssetSyncProgress, syncChannelAssets } from '../../api'

type ChannelAssetSyncDialogProps = {
  channelId: number
  channelName: string
  start: boolean
  onClose: () => void
}

// Mounted only while open, so closing the dialog stops polling and aborts its query.
export function ChannelAssetSyncDialog(props: ChannelAssetSyncDialogProps) {
  const { t, i18n } = useTranslation()
  const [taskId, setTaskId] = useState<string>()
  const showProgress = !props.start || taskId !== undefined
  const mutation = useMutation({
    mutationFn: () => syncChannelAssets(props.channelId),
    onSuccess: (task) => {
      setTaskId(task.task_id)
      toast.success(t('Asset synchronization queued'))
    },
    onError: (error) => {
      handleServerError(error, t('Failed to start asset synchronization'))
    },
  })
  const query = useQuery({
    queryKey: ['channels', props.channelId, 'asset-sync', taskId],
    queryFn: ({ signal }) =>
      getChannelAssetSyncProgress(props.channelId, taskId, signal),
    enabled: showProgress,
    refetchInterval: (query) => {
      const progress = query.state.data
      if (query.state.status === 'error' || !progress) return false
      return progress.preparing ||
        progress.pending > 0 ||
        progress.processing > 0
        ? 5000
        : false
    },
    refetchOnWindowFocus: false,
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })

  if (!showProgress) {
    return (
      <ConfirmDialog
        open
        onOpenChange={(open) => !open && props.onClose()}
        title={t('Sync Assets')}
        desc={t(
          'Synchronize existing eligible assets to channel "{{name}}" in the background?',
          { name: props.channelName }
        )}
        confirmText={t('Sync Assets')}
        isLoading={mutation.isPending}
        handleConfirm={() => mutation.mutate()}
      />
    )
  }

  const progress = query.data
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  let status = t('No manually synchronized assets')
  if (progress?.preparing) {
    status = t('Preparing asset synchronization')
  } else if (progress?.prepare_task?.status === 'failed') {
    status = t('Asset synchronization preparation failed')
  } else if (progress && (progress.pending > 0 || progress.processing > 0)) {
    status = t('Asset synchronization in progress')
  } else if (progress && progress.total > 0) {
    status =
      progress.failed > 0
        ? t('Asset synchronization finished with failures')
        : t('Asset synchronization completed')
  }
  const counts = [
    { label: t('Total'), value: progress?.total },
    { label: t('Pending upload'), value: progress?.pending },
    { label: t('Processing'), value: progress?.processing },
    { label: t('Available'), value: progress?.active },
    { label: t('Failed'), value: progress?.failed },
  ]

  return (
    <Dialog
      open
      onOpenChange={(open) => !open && props.onClose()}
      title={t('Asset Sync Details')}
      description={t(
        'Manual asset synchronization progress for channel "{{name}}"',
        {
          name: props.channelName,
        }
      )}
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw
              className={query.isFetching ? 'animate-spin' : undefined}
            />
            {t('Refresh')}
          </Button>
          <Button variant='outline' onClick={props.onClose}>
            {t('Close')}
          </Button>
        </>
      }
    >
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          description={getServerErrorMessage(query.error)}
          onRetry={() => void query.refetch()}
        />
      )}
      {!query.isError && progress && (
        <>
          <p role='status' className='text-sm font-medium'>
            {status}
          </p>
          {progress.prepare_task?.status === 'failed' && (
            <Alert variant='destructive'>
              <AlertDescription className='break-words'>
                {progress.prepare_task.error}
              </AlertDescription>
            </Alert>
          )}
          {progress.total === 0 && !progress.preparing ? (
            <EmptyState title={t('No manually synchronized assets')} />
          ) : (
            <dl className='grid grid-cols-2 gap-3 sm:grid-cols-3'>
              {counts.map((count) => (
                <div key={count.label} className='rounded-lg border p-3'>
                  <dt className='text-muted-foreground text-sm'>
                    {count.label}
                  </dt>
                  <dd className='text-xl font-semibold tabular-nums'>
                    {formatNumber(count.value ?? 0, locale)}
                  </dd>
                </div>
              ))}
            </dl>
          )}
        </>
      )}
    </Dialog>
  )
}
