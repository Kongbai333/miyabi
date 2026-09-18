import { Link } from '@tanstack/react-router'
import { BellOffIcon, LoaderCircleIcon, RotateCcwIcon } from 'lucide-react'

import { useRemoveMonitor, useRetryMonitor, type MonitorItem } from '@/api/monitor'
import { MovieCard } from '@/components/movie'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

const dateTimeFormat = new Intl.DateTimeFormat('zh-CN', {
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit'
})

const statusLabels: Record<MonitorItem['status'], string> = {
  waiting: '监控中',
  added: '已加入 115',
  stale: '长期无源'
}

export function MonitorCard({ item }: { item: MonitorItem }) {
  const remove = useRemoveMonitor()
  const retry = useRetryMonitor()
  const busy = remove.isPending || retry.isPending

  return (
    <div className="relative h-full min-w-0">
      <Link
        to="/discover/$movieId"
        params={{ movieId: item.movie_id }}
        className="block h-full rounded-2xl outline-ring"
      >
        <MovieCard
          movie={item}
          description={
            <div className="space-y-1">
              <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
                <span>{item.release_date ? `发行 ${item.release_date}` : '发行日期未知'}</span>
                <span className="tabular-nums">{describeSchedule(item)}</span>
              </div>
              {item.error ? (
                <p className="line-clamp-2 text-destructive" title={item.error}>
                  {item.error}
                </p>
              ) : null}
            </div>
          }
          state={<Badge variant={statusVariant(item.status)}>{statusLabels[item.status]}</Badge>}
        >
          <div className="flex w-full items-center justify-end gap-2">
            {item.status !== 'waiting' ? (
              <Button
                type="button"
                variant="outline"
                size="xs"
                disabled={busy}
                onClick={event => {
                  event.preventDefault()
                  retry.mutate(item.movie_id)
                }}
              >
                {retry.isPending ? (
                  <LoaderCircleIcon className="animate-spin" />
                ) : (
                  <RotateCcwIcon />
                )}
                重新监控
              </Button>
            ) : null}
            <Button
              type="button"
              variant="ghost"
              size="xs"
              disabled={busy}
              onClick={event => {
                event.preventDefault()
                remove.mutate(item.movie_id)
              }}
            >
              {remove.isPending ? <LoaderCircleIcon className="animate-spin" /> : <BellOffIcon />}
              取消监控
            </Button>
          </div>
        </MovieCard>
      </Link>
    </div>
  )
}

function statusVariant(status: MonitorItem['status']) {
  if (status === 'added') return 'success' as const
  if (status === 'stale') return 'secondary' as const
  return 'default' as const
}

function describeSchedule(item: MonitorItem) {
  if (item.status === 'added') return '已自动加入 115'
  if (item.status === 'stale') return '发行 30 天后仍无磁力'
  if (item.next_check_at) {
    const next = new Date(item.next_check_at)
    if (next.getTime() <= Date.now()) return '即将检查'
    return `下次检查 ${dateTimeFormat.format(next)}`
  }
  return '等待检查'
}
