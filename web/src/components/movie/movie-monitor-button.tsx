import { BellPlusIcon, BellRingIcon, LoaderCircleIcon } from 'lucide-react'
import type { MouseEvent } from 'react'

import type { DiscoverMovie } from '@/api/discover'
import { useAddMonitor, useMonitor } from '@/api/monitor'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

// Shown on unreleased cards without a magnet. Sits inside a Link, so clicks
// must not navigate.
export function MovieMonitorButton({ movie }: { movie: DiscoverMovie }) {
  const { monitor, isPending } = useMonitor(movie.id)
  const add = useAddMonitor()
  const monitoring = monitor?.status === 'waiting'

  function handleClick(event: MouseEvent<HTMLButtonElement>) {
    event.preventDefault()
    event.stopPropagation()
    if (monitoring || add.isPending) return
    add.mutate(movie.id)
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant={monitoring ? 'default' : 'outline'}
          size="icon-sm"
          aria-label={monitoring ? '监控中' : '加入监控'}
          aria-pressed={monitoring}
          disabled={isPending || add.isPending}
          className={monitoring ? undefined : 'bg-background/85 backdrop-blur'}
          onClick={handleClick}
        >
          {add.isPending ? (
            <LoaderCircleIcon className="animate-spin" />
          ) : monitoring ? (
            <BellRingIcon />
          ) : (
            <BellPlusIcon />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="left">
        {monitoring ? '监控中，出现磁力后自动加入 115' : '加入监控'}
      </TooltipContent>
    </Tooltip>
  )
}
