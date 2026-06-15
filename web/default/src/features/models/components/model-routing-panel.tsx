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
import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, RefreshCw, Search, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { CHANNEL_TYPES } from '@/features/channels/constants'
import { getUserGroups } from '@/lib/api'
import {
  disableModelRouteBinding,
  getModelRouteBindings,
  getModelRouteCandidates,
  saveModelRouteBinding,
} from '../api'
import { modelsQueryKeys } from '../lib'
import type { ModelRouteBindingView, ModelRouteChannel } from '../types'

const PAGE_SIZE = 20

function channelLabel(channel?: ModelRouteChannel) {
  if (!channel) return '-'
  return `#${channel.id} ${channel.name}`
}

function channelTypeName(type?: number) {
  if (type == null) return '-'
  return CHANNEL_TYPES[type] ?? `Type ${type}`
}

function currentRouteChannel(row: ModelRouteBindingView) {
  return row.channel ?? row.automatic_channel
}

function RouteStatus({ row }: { row: ModelRouteBindingView }) {
  const { t } = useTranslation()
  if (!row.binding) {
    return <Badge variant='outline'>{t('Automatic')}</Badge>
  }
  if (!row.channel) {
    return <Badge variant='destructive'>{t('Broken binding')}</Badge>
  }
  if (row.channel.status !== 1) {
    return <Badge variant='destructive'>{t('Channel disabled')}</Badge>
  }
  return <Badge variant='secondary'>{t('Manual')}</Badge>
}

function SwitchRouteDialog({
  row,
  open,
  onOpenChange,
}: {
  row: ModelRouteBindingView | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [reason, setReason] = useState('')

  const candidatesQuery = useQuery({
    queryKey: modelsQueryKeys.routeCandidates(
      row?.group ?? '',
      row?.model_name ?? ''
    ),
    queryFn: () =>
      getModelRouteCandidates({
        group: row?.group ?? '',
        model: row?.model_name ?? '',
      }),
    enabled: open && row != null,
  })

  const saveMutation = useMutation({
    mutationFn: (channelId: number) =>
      saveModelRouteBinding({
        group: row?.group ?? '',
        model_name: row?.model_name ?? '',
        channel_id: channelId,
        reason,
      }),
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Route update failed'))
        return
      }
      toast.success(t('Route updated'))
      setReason('')
      onOpenChange(false)
      queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
      queryClient.invalidateQueries({ queryKey: modelsQueryKeys.all })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Route update failed'))
    },
  })

  const candidates = candidatesQuery.data?.data ?? []

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Switch model route')}</DialogTitle>
          <DialogDescription>
            {row
              ? `${row.group} / ${row.model_name}`
              : t('Select a model route first')}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-3'>
          <Textarea
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder={t('Reason for this route change')}
            rows={2}
          />

          <div className='max-h-80 overflow-auto rounded-md border'>
            <Table className='min-w-[760px]'>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Channel')}</TableHead>
                  <TableHead>{t('Provider')}</TableHead>
                  <TableHead>{t('Upstream model')}</TableHead>
                  <TableHead>{t('Priority')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {candidatesQuery.isLoading ? (
                  <TableRow>
                    <TableCell colSpan={5} className='text-muted-foreground'>
                      {t('Loading...')}
                    </TableCell>
                  </TableRow>
                ) : candidates.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className='text-muted-foreground'>
                      {t('No candidate channels')}
                    </TableCell>
                  </TableRow>
                ) : (
                  candidates.map((channel) => {
                    const isCurrent = row?.binding?.channel_id === channel.id
                    return (
                      <TableRow key={channel.id}>
                        <TableCell>
                          <div className='flex min-w-48 items-center gap-2'>
                            <span>{channelLabel(channel)}</span>
                            {isCurrent && (
                              <Badge variant='outline'>{t('Current')}</Badge>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>{channelTypeName(channel.type)}</TableCell>
                        <TableCell>{channel.upstream_model || '-'}</TableCell>
                        <TableCell>{channel.priority}</TableCell>
                        <TableCell className='text-right'>
                          <Button
                            size='sm'
                            disabled={isCurrent || saveMutation.isPending}
                            onClick={() => saveMutation.mutate(channel.id)}
                          >
                            <Check className='h-4 w-4' />
                            {t('Use')}
                          </Button>
                        </TableCell>
                      </TableRow>
                    )
                  })
                )}
              </TableBody>
            </Table>
          </div>
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function ModelRoutingPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [groupInput, setGroupInput] = useState('')
  const [keywordInput, setKeywordInput] = useState('')
  const [group, setGroup] = useState('')
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [selectedRow, setSelectedRow] = useState<ModelRouteBindingView | null>(
    null
  )

  const groupsQuery = useQuery({
    queryKey: ['model-route-groups'],
    queryFn: getUserGroups,
  })

  const groupOptions = useMemo(() => {
    const groups = Object.keys(groupsQuery.data?.data ?? {}).filter(
      (name) => name && name !== 'auto'
    )
    return groups.sort((a, b) => a.localeCompare(b))
  }, [groupsQuery.data?.data])

  useEffect(() => {
    if (group || groupOptions.length === 0) return
    const initialGroup = groupOptions[0]
    setGroup(initialGroup)
    setGroupInput(initialGroup)
  }, [group, groupOptions])

  const filters = useMemo(
    () => ({ group, keyword, p: page, page_size: PAGE_SIZE }),
    [group, keyword, page]
  )

  const routesQuery = useQuery({
    queryKey: modelsQueryKeys.routes(filters),
    queryFn: () => getModelRouteBindings(filters),
    enabled: group !== '',
  })

  const clearMutation = useMutation({
    mutationFn: (row: ModelRouteBindingView) =>
      disableModelRouteBinding({
        group: row.group,
        model: row.model_name,
      }),
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Route clear failed'))
        return
      }
      toast.success(t('Route cleared'))
      queryClient.invalidateQueries({ queryKey: modelsQueryKeys.all })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Route clear failed'))
    },
  })

  const rows = routesQuery.data?.data?.items ?? []
  const total = routesQuery.data?.data?.total ?? 0
  const hasNextPage = page * PAGE_SIZE < total

  const applyFilters = () => {
    setGroup(groupInput.trim())
    setKeyword(keywordInput.trim())
    setPage(1)
  }

  const handleGroupChange = (value: string | null) => {
    if (!value) return
    setGroupInput(value)
    setGroup(value)
    setPage(1)
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
        <Select
          value={groupInput}
          onValueChange={handleGroupChange}
          disabled={groupsQuery.isLoading || groupOptions.length === 0}
        >
          <SelectTrigger className='w-full sm:w-48'>
            <SelectValue placeholder={t('Group')} />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {groupOptions.map((option) => (
                <SelectItem key={option} value={option}>
                  {option}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Input
          value={keywordInput}
          onChange={(event) => setKeywordInput(event.target.value)}
          placeholder={t('Search models...')}
          className='sm:w-72'
        />
        <Button onClick={applyFilters} size='sm'>
          <Search className='h-4 w-4' />
          {t('Search')}
        </Button>
        <Button
          variant='outline'
          size='icon-sm'
          aria-label={t('Refresh')}
          title={t('Refresh')}
          onClick={() => routesQuery.refetch()}
        >
          <RefreshCw className='h-4 w-4' />
        </Button>
      </div>

      <div className='overflow-x-auto rounded-md border'>
        <Table className='min-w-[960px]'>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Group')}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Current channel')}</TableHead>
              <TableHead>{t('Provider')}</TableHead>
              <TableHead>{t('Upstream model')}</TableHead>
              <TableHead>{t('Candidates')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {routesQuery.isLoading ? (
              <TableRow>
                <TableCell colSpan={8} className='text-muted-foreground'>
                  {t('Loading...')}
                </TableCell>
              </TableRow>
            ) : rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={8} className='text-muted-foreground'>
                  {t('No model routes found')}
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row) => (
                <TableRow key={`${row.group}:${row.model_name}`}>
                  <TableCell>{row.group}</TableCell>
                  <TableCell className='min-w-72 max-w-96'>
                    <div className='space-y-1'>
                      <div className='truncate' title={row.model_name}>
                        {row.model_name}
                      </div>
                      {!row.binding && row.automatic_channel && (
                        <div className='text-muted-foreground text-xs md:hidden'>
                          {t('Automatic')}: {channelLabel(row.automatic_channel)}
                        </div>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>{channelLabel(currentRouteChannel(row))}</TableCell>
                  <TableCell>
                    {channelTypeName(currentRouteChannel(row)?.type)}
                  </TableCell>
                  <TableCell>
                    {currentRouteChannel(row)?.upstream_model || '-'}
                  </TableCell>
                  <TableCell>{row.candidate_count}</TableCell>
                  <TableCell>
                    <RouteStatus row={row} />
                  </TableCell>
                  <TableCell className='text-right'>
                    <div className='flex justify-end gap-2'>
                      <Button size='sm' onClick={() => setSelectedRow(row)}>
                        {t('Switch')}
                      </Button>
                      <Button
                        variant='outline'
                        size='icon-sm'
                        disabled={!row.binding || clearMutation.isPending}
                        aria-label={t('Clear route')}
                        title={t('Clear route')}
                        onClick={() => clearMutation.mutate(row)}
                      >
                        <X className='h-4 w-4' />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <div className='flex items-center justify-end gap-2'>
        <span className='text-muted-foreground text-sm'>
          {t('Total:')} {total}
        </span>
        <Button
          variant='outline'
          size='sm'
          disabled={page <= 1}
          onClick={() => setPage((value) => Math.max(1, value - 1))}
        >
          {t('Previous')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          disabled={!hasNextPage}
          onClick={() => setPage((value) => value + 1)}
        >
          {t('Next')}
        </Button>
      </div>

      <SwitchRouteDialog
        row={selectedRow}
        open={selectedRow != null}
        onOpenChange={(open) => {
          if (!open) setSelectedRow(null)
        }}
      />
    </div>
  )
}
