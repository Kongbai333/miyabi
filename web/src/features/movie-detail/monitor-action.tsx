import { BellOffIcon, BellPlusIcon, BellRingIcon, LoaderCircleIcon } from 'lucide-react'

import { useAddMonitor, useMonitor, useRemoveMonitor } from '@/api/monitor'
import { Button } from '@/components/ui/button'

// Inline monitor toggle for the detail page when no magnet exists yet.
export function MovieMonitorAction({ movieID }: { movieID: string }) {
  const { monitor, isPending } = useMonitor(movieID)
  const add = useAddMonitor()
  const remove = useRemoveMonitor()
  const monitoring = monitor?.status === 'waiting'
  const busy = isPending || add.isPending || remove.isPending

  return (
    <div className="flex flex-wrap items-center gap-3">
      <p className="text-sm text-muted-foreground">
        {monitoring ? '监控中，出现磁力后会自动加入 115。' : '暂无磁力链'}
      </p>
      <Button
        type="button"
        variant={monitoring ? 'outline' : 'default'}
        size="sm"
        disabled={busy}
        onClick={() => (monitoring ? remove.mutate(movieID) : add.mutate(movieID))}
      >
        {add.isPending || remove.isPending ? (
          <LoaderCircleIcon className="animate-spin" />
        ) : monitoring ? (
          <BellOffIcon />
        ) : monitor ? (
          <BellRingIcon />
        ) : (
          <BellPlusIcon />
        )}
        {monitoring ? '取消监控' : monitor ? '重新监控' : '加入监控'}
      </Button>
    </div>
  )
}
