import { CopyIcon, LoaderCircleIcon, SearchIcon } from 'lucide-react'
import { type FormEvent, useState } from 'react'

import { type MagnetItem, useMagnetSearch } from '@/api/magnet'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { copyMagnet } from './clipboard'
import { MagnetDetailDialog } from './detail-dialog'
import { MagnetDownloadButton } from './download-button'

const LIMITS = ['5', '7', '10', '20']

export function MagnetSearchCard() {
  const search = useMagnetSearch()
  const [query, setQuery] = useState('')
  const [limit, setLimit] = useState('7')
  const [selected, setSelected] = useState<MagnetItem | null>(null)

  function submit(event: FormEvent) {
    event.preventDefault()
    const trimmed = query.trim()
    if (trimmed === '') return
    search.mutate({ query: trimmed, limit: Number(limit) })
  }

  const result = search.data

  return (
    <Card>
      <CardContent className="space-y-4">
        <form className="flex flex-wrap items-center gap-2" onSubmit={submit}>
          <Input
            value={query}
            onChange={event => setQuery(event.target.value)}
            placeholder="番号、片名或关键词"
            aria-label="搜索关键词"
            className="min-w-0 flex-1"
          />
          <Select value={limit} onValueChange={setLimit}>
            <SelectTrigger className="w-28" aria-label="返回条数">
              <SelectValue />
            </SelectTrigger>
            <SelectContent position="popper" align="start">
              <SelectGroup>
                {LIMITS.map(value => (
                  <SelectItem key={value} value={value}>
                    {value} 条
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Button type="submit" disabled={search.isPending || query.trim() === ''}>
            {search.isPending ? <LoaderCircleIcon className="animate-spin" /> : <SearchIcon />}
            搜索
          </Button>
        </form>

        {search.isError ? <InlineError>{search.error.message}</InlineError> : null}

        {search.isPending ? (
          <div className="space-y-2">
            <Skeleton className="h-16 w-full rounded-2xl" />
            <Skeleton className="h-16 w-full rounded-2xl" />
            <Skeleton className="h-16 w-full rounded-2xl" />
          </div>
        ) : null}

        {!search.isPending && result
          ? result.items.length === 0 && (
              <p className="text-sm text-muted-foreground">没有找到匹配的磁力。</p>
            )
          : null}

        {!search.isPending && result && result.items.length > 0 ? (
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">
              {result.count} 条结果
              {result.source === 'local' ? ' · 来自服务端数据库' : ' · 来自爬取'}
            </p>
            <ul className="space-y-2">
              {result.items.map(item => (
                <MagnetRow key={item.magnet_url} item={item} onInspect={() => setSelected(item)} />
              ))}
            </ul>
          </div>
        ) : null}
      </CardContent>

      {selected ? (
        <MagnetDetailDialog
          title={selected.name}
          magnet={selected.magnet_url}
          onClose={() => setSelected(null)}
        />
      ) : null}
    </Card>
  )
}

function MagnetRow({ item, onInspect }: { item: MagnetItem; onInspect: () => void }) {
  return (
    <li className="flex flex-col gap-2 rounded-2xl border border-border/60 p-3 sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1 space-y-1">
        <p className="text-sm font-medium break-all">{item.name}</p>
        <p className="text-xs text-muted-foreground tabular-nums">
          {item.size} · {item.date} · 匹配度 {item.score}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button type="button" variant="outline" size="sm" onClick={onInspect}>
          详情
        </Button>
        <MagnetDownloadButton magnet={item.magnet_url} />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => void copyMagnet(item.magnet_url)}
        >
          <CopyIcon />
          复制
        </Button>
      </div>
    </li>
  )
}
