import { LoaderCircleIcon, PlusIcon, SettingsIcon } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import {
  useCreateFavoriteGroup,
  useDeleteFavoriteGroup,
  useFavoriteGroups,
  useRenameFavoriteGroup
} from '@/api/favorite'

// The tab row mirrors the saved groups: the first tab shows everything, and
// every group becomes its own tab. Group management lives behind one dialog
// so the row stays a plain switcher.
export function LibraryGroupTabs({
  group,
  onGroupChange
}: {
  group: number
  onGroupChange: (group: number) => void
}) {
  const [manageOpen, setManageOpen] = useState(false)
  const groups = useFavoriteGroups()

  return (
    <div className="flex flex-wrap items-center gap-2">
      {groups.isPending ? (
        <Skeleton className="h-9 w-48 rounded-4xl" />
      ) : (
        <Tabs value={String(group)} onValueChange={value => onGroupChange(Number(value))}>
          <TabsList className="h-auto flex-wrap justify-start">
            <TabsTrigger value="0">全部</TabsTrigger>
            {(groups.data ?? []).map(item => (
              <TabsTrigger key={item.id} value={String(item.id)}>
                {item.name}
                <span className="text-xs text-muted-foreground tabular-nums">{item.count}</span>
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      )}
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className={cn('shrink-0')}
        onClick={() => setManageOpen(true)}
      >
        <SettingsIcon />
        管理分组
      </Button>
      {manageOpen ? <ManageGroupsDialog onClose={() => setManageOpen(false)} /> : null}
    </div>
  )
}

function ManageGroupsDialog({ onClose }: { onClose: () => void }) {
  const groups = useFavoriteGroups()
  const create = useCreateFavoriteGroup()
  const rename = useRenameFavoriteGroup()
  const remove = useDeleteFavoriteGroup()
  const [draft, setDraft] = useState('')
  const [editing, setEditing] = useState<{ id: number; name: string } | null>(null)

  const busy = create.isPending || rename.isPending || remove.isPending

  function add() {
    const name = draft.trim()
    if (!name) return
    create.mutate(name, { onSuccess: () => setDraft('') })
  }

  function saveRename() {
    if (!editing) return
    const name = editing.name.trim()
    if (!name) return
    rename.mutate({ id: editing.id, name }, { onSuccess: () => setEditing(null) })
  }

  return (
    <Dialog open onOpenChange={next => (next ? undefined : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>管理收藏分组</DialogTitle>
          <DialogDescription>
            删除分组只会取消其中影片的收藏，不会删除媒体库里的影片。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          {(groups.data ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">还没有分组，先创建一个。</p>
          ) : (
            <ul className="space-y-2">
              {(groups.data ?? []).map(item => (
                <li key={item.id} className="flex items-center gap-2">
                  {editing?.id === item.id ? (
                    <>
                      <Input
                        value={editing.name}
                        maxLength={24}
                        autoFocus
                        disabled={busy}
                        onChange={event => setEditing({ id: item.id, name: event.target.value })}
                        onKeyDown={event => {
                          if (event.key === 'Enter') {
                            event.preventDefault()
                            saveRename()
                          }
                        }}
                      />
                      <Button type="button" size="sm" disabled={busy} onClick={saveRename}>
                        保存
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={busy}
                        onClick={() => setEditing(null)}
                      >
                        取消
                      </Button>
                    </>
                  ) : (
                    <>
                      <span className="min-w-0 flex-1 truncate text-sm">
                        {item.name}
                        <span className="ml-2 text-xs text-muted-foreground tabular-nums">
                          {item.count} 部
                        </span>
                      </span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={busy}
                        onClick={() => setEditing({ id: item.id, name: item.name })}
                      >
                        重命名
                      </Button>
                      <Button
                        type="button"
                        variant="destructive"
                        size="sm"
                        disabled={busy}
                        onClick={() => remove.mutate(item.id)}
                      >
                        删除
                      </Button>
                    </>
                  )}
                </li>
              ))}
            </ul>
          )}

          <div className="flex gap-2">
            <Input
              value={draft}
              placeholder="新建分组"
              maxLength={24}
              disabled={busy}
              onChange={event => setDraft(event.target.value)}
              onKeyDown={event => {
                if (event.key !== 'Enter') return
                event.preventDefault()
                add()
              }}
            />
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label="创建分组"
              disabled={!draft.trim() || busy}
              onClick={add}
            >
              {create.isPending ? <LoaderCircleIcon className="animate-spin" /> : <PlusIcon />}
            </Button>
          </div>
          {[create.error, rename.error, remove.error].map(
            (error, index) =>
              error && (
                <p key={index} className="text-sm text-destructive">
                  {error.message}
                </p>
              )
          )}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
