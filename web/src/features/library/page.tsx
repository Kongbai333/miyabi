import { Link } from '@tanstack/react-router'
import { LoaderCircleIcon, RefreshCwIcon, ScanLineIcon } from 'lucide-react'

import {
  LIBRARY_PAGE_SIZE,
  type LibraryFilter,
  useLibraryFilterOptions,
  useLibraryMovies,
  useStartLibraryScan
} from '@/api/library'
import { isTaskActive, useTasks } from '@/api/tasks'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState, InlineError } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridLayout } from '@/components/movie/movie-grid'
import { MovieGridSkeleton } from '@/components/movie/movie-grid-skeleton'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { useTaskConnection } from '@/features/tasks/task-events'
import { LibraryFilterBar } from './filter-bar'
import { hasLibraryFilter } from './filter-params'
import { LibraryGroupTabs } from './group-tabs'

export function LibraryPage({
  page,
  group,
  filter,
  onPageChange,
  onGroupChange,
  onFilterChange
}: {
  page: number
  group: number
  filter: LibraryFilter
  onPageChange: (page: number) => void
  onGroupChange: (group: number) => void
  onFilterChange: (filter: LibraryFilter) => void
}) {
  const library = useLibraryMovies(page, group, filter)
  const options = useLibraryFilterOptions()
  const tasks = useTasks()
  const connection = useTaskConnection()
  const startScan = useStartLibraryScan()
  const source = library.data?.source
  const filtered = hasLibraryFilter(filter)
  const latest = tasks.data?.find(
    task =>
      source !== undefined &&
      task.source.account_id === source.account_id &&
      task.source.directory.id === source.directory.id
  )
  const scanning = latest !== undefined && isTaskActive(latest)
  const processing = scanning && connection.status === 'connected'
  const scanLabel = scanning
    ? processing
      ? '正在处理'
      : '等待同步'
    : latest?.status === 'failed'
      ? '重新扫描'
      : '扫描媒体库'

  return (
    <AppPage>
      <PageHeader title="媒体库" description="来自 115 网盘的影片索引" inlineActions>
        {library.isPending ? (
          <Skeleton className="h-9 w-9 rounded-4xl sm:w-30" />
        ) : source ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                className="w-9 px-0 sm:w-auto sm:px-3"
                disabled={scanning || startScan.isPending}
                onClick={() => startScan.mutate(undefined, { onSuccess: () => onPageChange(1) })}
              >
                {processing || startScan.isPending ? (
                  <LoaderCircleIcon className="size-4 animate-spin" />
                ) : scanning ? (
                  <RefreshCwIcon className="size-4" />
                ) : (
                  <ScanLineIcon className="size-4" />
                )}
                <span className="hidden sm:inline">{scanLabel}</span>
              </Button>
            </TooltipTrigger>
            <TooltipContent className="sm:hidden">{scanLabel}</TooltipContent>
          </Tooltip>
        ) : library.isSuccess ? (
          <Button asChild>
            <Link to="/settings">挂载媒体目录</Link>
          </Button>
        ) : null}
      </PageHeader>

      <LibraryGroupTabs group={group} onGroupChange={onGroupChange} />

      {source ? (
        <LibraryFilterBar
          filter={filter}
          options={options.data}
          loading={options.isPending}
          error={options.isError}
          onChange={onFilterChange}
          onRetry={() => void options.refetch()}
        />
      ) : null}

      {tasks.isError ? (
        <InlineError
          onRetry={connection.reconnect}
          retrying={connection.status === 'connecting'}
          retryLabel="重新连接"
        >
          暂时无法获取任务状态，请检查后端服务后重试。
        </InlineError>
      ) : null}

      {library.isPending ? (
        <MovieGridSkeleton count={LIBRARY_PAGE_SIZE} compact />
      ) : library.isError ? (
        <ErrorState
          message="无法读取媒体库，请检查后端服务后重试"
          onRetry={() => void library.refetch()}
          retrying={library.isFetching}
        />
      ) : (
        <>
          {source ? (
            <p className="text-sm">
              {filtered
                ? `筛选出 ${library.data.total} 部影片`
                : group > 0
                  ? `本分组 ${library.data.total} 部影片`
                  : `共 ${library.data.total} 部影片`}{' '}
              · 每页 {LIBRARY_PAGE_SIZE} 部
            </p>
          ) : null}

          {library.data.movies.length > 0 ? (
            <MovieGridLayout>
              {library.data.movies.map(movie => (
                <LibraryMovieCard key={movie.id} movie={movie} />
              ))}
            </MovieGridLayout>
          ) : (
            <EmptyState
              className="min-h-0 flex-1 py-12"
              title={
                !source
                  ? '登录 115 并挂载媒体目录后，将自动扫描入库'
                  : scanning
                    ? '正在扫描，识别到的影片会陆续显示'
                    : filtered
                      ? '没有符合筛选条件的影片'
                      : group > 0
                        ? '这个分组还没有收藏的影片'
                        : '未识别到影片'
              }
            />
          )}
          {/* Whether there is anything to page through is the bar's own
              business, and a background refetch must not grey it out: every
              library change invalidates the list, which would make the numbers
              flicker under the cursor. */}
          <ListPagination
            page={page}
            totalPages={Math.max(1, Math.ceil(library.data.total / LIBRARY_PAGE_SIZE))}
            hasMore={library.data.has_more}
            onPageChange={onPageChange}
          />
        </>
      )}
    </AppPage>
  )
}
