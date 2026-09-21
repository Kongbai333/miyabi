import type { LibraryFilter } from '@/api/library'

export const EMPTY_LIBRARY_FILTER: LibraryFilter = {
  tagIds: [],
  actorIds: [],
  seriesIds: [],
  makerIds: [],
  directorIds: [],
  years: [],
  watched: ''
}

// The bar writes one value per dimension, so a repeated parameter means a
// hand-edited URL: refuse it instead of guessing which value was meant.
function scalar(value: unknown, label: string): string {
  if (value === undefined) return ''
  if (typeof value === 'string' || typeof value === 'number') return String(value).trim()
  throw new Error(`${label}无效`)
}

function integer(value: string, label: string, min: number, max?: number): number | undefined {
  if (value === '') return undefined
  const parsed = Number(value)
  if (!Number.isInteger(parsed) || parsed < min || (max !== undefined && parsed > max)) {
    throw new Error(`${label}无效`)
  }
  return parsed
}

function text(value: string, label: string): string | undefined {
  if (value === '') return undefined
  if (value.length > 64) throw new Error(`${label}无效`)
  return value
}

// The single shape the page reads. An unset dimension stays out of the URL
// instead of being written as an empty string.
export type LibrarySearch = {
  page?: number
  group?: number
  tag?: number
  actor?: string
  series?: string
  maker?: string
  director?: string
  year?: number
  watched?: 'yes' | 'no'
}

// validateSearch: turn a hand-edited or stale URL into the shape the page reads.
export function parseLibrarySearch(search: Record<string, unknown>): LibrarySearch {
  const page = integer(scalar(search.page, '媒体库页码'), '媒体库页码', 1)
  const tag = integer(scalar(search.tag, '媒体库标签'), '媒体库标签', 1)
  const year = integer(scalar(search.year, '媒体库年份'), '媒体库年份', 1900, 2999)
  const group = integer(scalar(search.group, '媒体库分组'), '媒体库分组', 0)
  const watched = scalar(search.watched, '媒体库观看状态')
  if (watched !== '' && watched !== 'yes' && watched !== 'no') throw new Error('媒体库观看状态无效')
  const actor = text(scalar(search.actor, '媒体库演员'), '媒体库演员')
  const series = text(scalar(search.series, '媒体库系列'), '媒体库系列')
  const maker = text(scalar(search.maker, '媒体库片商'), '媒体库片商')
  const director = text(scalar(search.director, '媒体库导演'), '媒体库导演')

  return {
    ...(page !== undefined && page > 1 && { page }),
    ...(group !== undefined && group > 0 && { group }),
    ...(tag !== undefined && { tag }),
    ...(actor !== undefined && { actor }),
    ...(series !== undefined && { series }),
    ...(maker !== undefined && { maker }),
    ...(director !== undefined && { director }),
    ...(year !== undefined && { year }),
    ...(watched !== '' && { watched: watched as 'yes' | 'no' })
  }
}

// The group tabs own their own parameter, so only the bar's dimensions come
// from here; useLibraryMovies merges the selected group into the same request.
export function libraryFilterOf(search: LibrarySearch): LibraryFilter {
  return {
    ...EMPTY_LIBRARY_FILTER,
    ...(search.tag !== undefined && { tagIds: [search.tag] }),
    ...(search.actor !== undefined && { actorIds: [search.actor] }),
    ...(search.series !== undefined && { seriesIds: [search.series] }),
    ...(search.maker !== undefined && { makerIds: [search.maker] }),
    ...(search.director !== undefined && { directorIds: [search.director] }),
    ...(search.year !== undefined && { years: [search.year] }),
    ...(search.watched !== undefined && { watched: search.watched })
  }
}

// Changing a filter or a group returns to the first page: the new list is
// shorter, and a stale page number would show an empty grid.
export function libraryFilterSearch(filter: LibraryFilter, group: number): LibrarySearch {
  const [tag] = filter.tagIds
  const [actor] = filter.actorIds
  const [series] = filter.seriesIds
  const [maker] = filter.makerIds
  const [director] = filter.directorIds
  const [year] = filter.years

  return {
    ...(group > 0 && { group }),
    ...(tag !== undefined && { tag }),
    ...(actor !== undefined && { actor }),
    ...(series !== undefined && { series }),
    ...(maker !== undefined && { maker }),
    ...(director !== undefined && { director }),
    ...(year !== undefined && { year }),
    ...(filter.watched !== '' && { watched: filter.watched })
  }
}

export function hasLibraryFilter(filter: LibraryFilter): boolean {
  return (
    filter.tagIds.length > 0 ||
    filter.actorIds.length > 0 ||
    filter.seriesIds.length > 0 ||
    filter.makerIds.length > 0 ||
    filter.directorIds.length > 0 ||
    filter.years.length > 0 ||
    filter.watched !== ''
  )
}
