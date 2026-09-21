import { useState } from 'react'
import { PauseIcon, PlayIcon, Trash2Icon } from 'lucide-react'
import { toast } from 'sonner'

import {
  useCancelQueuedTasks,
  usePauseTasks,
  useResumeTasks,
  useTaskQueues,
  type TaskQueue,
  type TaskType
} from '@/api/tasks'
import { InlineError } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useTaskConnection } from './task-events'

const QUEUE_KINDS: Record<TaskType, { label: string; description: string }> = {
  scan: { label: '扫描', description: '识别媒体目录里新增和改变的文件' },
  scrape: { label: '刮削', description: '从 JavDB 读取影片元数据' },
  cover: { label: '封面', description: '把海报和 NFO 写回 115' },
  frame: { label: '抽帧', description: '刮削失败时从视频里取一张封面' }
}

export function TaskQueueCard() {
  const queues = useTaskQueues()
  const connection = useTaskConnection()

  return (
    <Card>
      <CardHeader>
        <CardTitle>任务队列</CardTitle>
        <CardDescription>
          暂停只是让这一类任务不再被领取，正在跑的那个会照常结束。清空排队会丢弃还没开始的任务。
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {queues.isPending ? <Skeleton className="h-32 w-full rounded-2xl" /> : null}
        {queues.isError ? (
          <InlineError onRetry={connection.reconnect} retrying={connection.status === 'connecting'}>
            无法读取任务队列，请启动后端服务后重试。
          </InlineError>
        ) : null}
        <div className="divide-y divide-border">
          {queues.data?.map(queue => (
            <QueueRow key={queue.type} queue={queue} />
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

function QueueRow({ queue }: { queue: TaskQueue }) {
  const pause = usePauseTasks()
  const resume = useResumeTasks()
  const cancel = useCancelQueuedTasks()
  const [armed, setArmed] = useState<number | null>(null)
  const busy = pause.isPending || resume.isPending || cancel.isPending
  const kind = QUEUE_KINDS[queue.type]
  const failure = pause.error ?? resume.error ?? cancel.error
  const idle = queue.queued === 0 && queue.running === 0
  // Arming records the queue it was armed against, so a queue that drains or
  // refills never stays one click away from being cleared.
  const confirming = armed === queue.queued && queue.queued > 0
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2 py-3 first:pt-0 last:pb-0">
      <div className="min-w-0 flex-1 space-y-1">
        <p className="flex items-center gap-2 text-sm font-medium">
          {kind.label}
          {queue.paused ? <Badge variant="secondary">已暂停</Badge> : null}
        </p>
        <p className="text-xs text-muted-foreground">
          {idle ? '空闲' : `排队 ${queue.queued} · 运行 ${queue.running}`}
          {' · '}
          {kind.description}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={busy}
          onClick={() => (queue.paused ? resume : pause).mutate([queue.type])}
        >
          {queue.paused ? <PlayIcon /> : <PauseIcon />}
          {queue.paused ? '继续' : '暂停'}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={busy || queue.queued === 0}
          onClick={() => {
            if (!confirming) {
              setArmed(queue.queued)
              return
            }
            cancel.mutate([queue.type], {
              onSuccess: result => toast.success(`已清空 ${result.cancelled} 个排队任务`)
            })
          }}
        >
          <Trash2Icon />
          {confirming ? '确认清空' : '清空排队'}
        </Button>
      </div>
      {failure ? <p className="w-full text-xs text-destructive">{failure.message}</p> : null}
    </div>
  )
}
