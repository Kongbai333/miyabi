import type { LibraryFilter, LibraryFilterOptions } from '@/api/library'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { EMPTY_LIBRARY_FILTER, hasLibraryFilter } from './filter-params'

type FilterChoice = { id: string; name: string; count?: number }

const WATCHED_OPTIONS: FilterChoice[] = [
  { id: 'yes', name: '已看' },
  { id: 'no', name: '未看' }
]

const toIDs = (value: string) => (value === '' ? [] : [Number(value)])
const toNames = (value: string) => (value === '' ? [] : [value])
const firstValue = (values: readonly (string | number)[]) =>
  values.length > 0 ? String(values[0]) : ''

// One select per dimension, the way the discover page narrows its browse list.
// The options come from the mounted library, so the bar never offers a tag or
// an actor that no movie carries.
export function LibraryFilterBar({
  filter,
  options,
  loading,
  error,
  onChange,
  onRetry
}: {
  filter: LibraryFilter
  options: LibraryFilterOptions | undefined
  loading: boolean
  error: boolean
  onChange: (filter: LibraryFilter) => void
  onRetry: () => void
}) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
      {loading ? (
        <>
          <Skeleton className="h-9 w-full rounded-full sm:w-44" />
          <Skeleton className="h-9 w-full rounded-full sm:w-44" />
          <Skeleton className="h-9 w-full rounded-full sm:w-44" />
        </>
      ) : error ? (
        <InlineError onRetry={onRetry} retryLabel="重试筛选条件">
          筛选条件加载失败
        </InlineError>
      ) : (
        <>
          <FilterSelect
            label="标签"
            value={firstValue(filter.tagIds)}
            options={options?.tags ?? []}
            onChange={value => onChange({ ...filter, tagIds: toIDs(value) })}
          />
          <FilterSelect
            label="演员"
            value={firstValue(filter.actorIds)}
            options={options?.actors ?? []}
            onChange={value => onChange({ ...filter, actorIds: toNames(value) })}
          />
          <FilterSelect
            label="系列"
            value={firstValue(filter.seriesIds)}
            options={options?.series ?? []}
            onChange={value => onChange({ ...filter, seriesIds: toNames(value) })}
          />
          <FilterSelect
            label="片商"
            value={firstValue(filter.makerIds)}
            options={options?.makers ?? []}
            onChange={value => onChange({ ...filter, makerIds: toNames(value) })}
          />
          <FilterSelect
            label="导演"
            value={firstValue(filter.directorIds)}
            options={options?.directors ?? []}
            onChange={value => onChange({ ...filter, directorIds: toNames(value) })}
          />
          <FilterSelect
            label="年份"
            value={firstValue(filter.years)}
            options={options?.years ?? []}
            onChange={value => onChange({ ...filter, years: toIDs(value) })}
          />
          <FilterSelect
            label="观看状态"
            value={filter.watched}
            options={WATCHED_OPTIONS}
            onChange={value => onChange({ ...filter, watched: value as LibraryFilter['watched'] })}
          />
        </>
      )}

      {hasLibraryFilter(filter) ? (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="shrink-0"
          onClick={() => onChange(EMPTY_LIBRARY_FILTER)}
        >
          清除筛选
        </Button>
      ) : null}
    </div>
  )
}

function FilterSelect({
  label,
  value,
  options,
  onChange
}: {
  label: string
  value: string
  options: readonly FilterChoice[]
  onChange: (value: string) => void
}) {
  return (
    <Select value={value || 'all'} onValueChange={next => onChange(next === 'all' ? '' : next)}>
      <SelectTrigger className="w-full sm:w-44" aria-label={label}>
        <SelectValue placeholder={`全部${label}`} />
      </SelectTrigger>
      <SelectContent position="popper" align="start">
        <SelectGroup>
          <SelectItem value="all">全部{label}</SelectItem>
          {options.map(option => (
            <SelectItem key={option.id} value={option.id}>
              {option.name}
              <span className="text-xs text-muted-foreground tabular-nums">{option.count}</span>
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}
