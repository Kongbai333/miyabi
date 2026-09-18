import { useMonitors } from '@/api/monitor'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { MovieGridLayout, MovieGridSkeleton } from '@/components/movie'
import { MonitorCard } from './monitor-card'

export function MonitorList() {
  const monitors = useMonitors()
  const items = monitors.data ?? []

  if (monitors.isPending) return <MovieGridSkeleton />
  if (monitors.isError) {
    return (
      <ErrorState
        message="监控列表加载失败"
        onRetry={() => void monitors.refetch()}
        retrying={monitors.isFetching}
      />
    )
  }
  if (items.length === 0) {
    return <EmptyState title="还没有监控的影片，在「即将发行」卡片右上角点击铃铛加入监控" />
  }
  return (
    <MovieGridLayout>
      {items.map(item => (
        <MonitorCard key={item.id} item={item} />
      ))}
    </MovieGridLayout>
  )
}
