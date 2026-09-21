import { useTasks } from '@/api/tasks'
import { AppPage } from '@/components/app-page'
import { InlineError } from '@/components/error-state'
import { PageHeader } from '@/components/page-header'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ScanProgressView } from './scan-progress'
import { TaskQueueCard } from './queue-card'
import { useTaskConnection } from './task-events'

export function TasksPage() {
  const tasks = useTasks()
  const connection = useTaskConnection()

  return (
    <AppPage>
      <PageHeader title="任务中心" description="查看扫描、刮削和封面队列，暂停或清空它们" />
      <TaskQueueCard />
      <Card>
        <CardHeader>
          <CardTitle>媒体库任务</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {tasks.isPending ? <Skeleton className="h-24 w-full rounded-2xl" /> : null}
          {tasks.isError ? (
            <InlineError
              onRetry={connection.reconnect}
              retrying={connection.status === 'connecting'}
            >
              无法读取任务，请启动后端服务后重试。
            </InlineError>
          ) : null}
          {tasks.data?.length === 0 ? (
            <p className="text-sm text-muted-foreground">还没有扫描任务。</p>
          ) : null}
          <div className="divide-y divide-border">
            {tasks.data?.map(task => (
              <div key={task.id} className="space-y-2 py-3 first:pt-0 last:pb-0">
                <ScanProgressView task={task} />
                <p className="text-xs text-muted-foreground">
                  {new Date(task.created_at).toLocaleString('zh-CN')}
                </p>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </AppPage>
  )
}
