import { LoaderCircleIcon, PlusIcon, StarIcon } from 'lucide-react'
import { useState } from 'react'

import {
  useCreateFavoriteGroup,
  useFavoriteGroups,
  useSetFavorite,
  type FavoriteGroup
} from '@/api/favorite'
import type { LibraryMovie } from '@/api/library'
import { Button } from '@/components/ui/button'
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

// A movie may sit in several groups, so the dialog edits a set and saves it
// wholesale. The star is filled as soon as at least one group is chosen.
export function FavoriteDialog({ movie }: { movie: LibraryMovie }) {
  const [open, setOpen] = useState(false)
  const groups = useFavoriteGroups()
  const setFavorite = useSetFavorite()
  const starred = movie.favorite_group_ids.length > 0

  return (
    <>
      <Button
        type="button"
        variant="secondary"
        size="icon-xs"
        aria-label={starred ? '修改收藏分组' : '加入收藏'}
        title={starred ? '修改收藏分组' : '加入收藏'}
        className={cn('backdrop-blur', starred && 'text-amber-500')}
        onClick={event => {
          event.preventDefault()
          event.stopPropagation()
          setOpen(true)
        }}
      >
        <StarIcon className={starred ? 'fill-current' : undefined} />
      </Button>
      {open ? (
        <FavoriteDialogContent
          movie={movie}
          groups={groups.data ?? []}
          loading={groups.isPending}
          saving={setFavorite.isPending}
          onClose={() => setOpen(false)}
          onSave={groupIDs =>
            setFavorite.mutate({ movieID: movie.id, groupIDs }, { onSuccess: () => setOpen(false) })
          }
        />
      ) : null}
    </>
  )
}

function FavoriteDialogContent({
  movie,
  groups,
  loading,
  saving,
  onClose,
  onSave
}: {
  movie: LibraryMovie
  groups: FavoriteGroup[]
  loading: boolean
  saving: boolean
  onClose: () => void
  onSave: (groupIDs: number[]) => void
}) {
  const [selected, setSelected] = useState<number[]>(movie.favorite_group_ids)
  const [draft, setDraft] = useState('')
  const create = useCreateFavoriteGroup()

  function toggle(id: number) {
    setSelected(current =>
      current.includes(id) ? current.filter(item => item !== id) : [...current, id]
    )
  }

  function addGroup() {
    const name = draft.trim()
    if (!name) return
    create.mutate(name, {
      onSuccess: group => {
        setDraft('')
        setSelected(current => [...current, group.id])
      }
    })
  }

  return (
    <Dialog open onOpenChange={next => (next ? undefined : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{movie.title || movie.code}</DialogTitle>
          <DialogDescription>选择要放入的分组，可多选。</DialogDescription>
        </DialogHeader>

        {loading ? (
          <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
            <LoaderCircleIcon className="size-4 animate-spin" />
            正在读取分组
          </div>
        ) : (
          <div className="space-y-3">
            {groups.length === 0 ? (
              <p className="text-sm text-muted-foreground">还没有分组，先创建一个。</p>
            ) : (
              <div className="flex flex-wrap gap-2">
                {groups.map(group => {
                  const active = selected.includes(group.id)
                  return (
                    <Button
                      key={group.id}
                      type="button"
                      variant={active ? 'default' : 'outline'}
                      size="sm"
                      aria-pressed={active}
                      onClick={() => toggle(group.id)}
                    >
                      {group.name}
                    </Button>
                  )
                })}
              </div>
            )}

            <div className="flex gap-2">
              <Input
                value={draft}
                placeholder="新建分组"
                maxLength={24}
                disabled={create.isPending}
                onChange={event => setDraft(event.target.value)}
                onKeyDown={event => {
                  if (event.key !== 'Enter') return
                  event.preventDefault()
                  addGroup()
                }}
              />
              <Button
                type="button"
                variant="outline"
                size="icon"
                aria-label="创建分组"
                disabled={!draft.trim() || create.isPending}
                onClick={addGroup}
              >
                {create.isPending ? <LoaderCircleIcon className="animate-spin" /> : <PlusIcon />}
              </Button>
            </div>
            {create.error ? (
              <p className="text-sm text-destructive">{create.error.message}</p>
            ) : null}
          </div>
        )}

        <DialogFooter>
          <Button type="button" variant="outline" disabled={saving} onClick={onClose}>
            取消
          </Button>
          <Button
            type="button"
            variant={selected.length === 0 ? 'destructive' : 'default'}
            disabled={saving}
            onClick={() => onSave(selected)}
          >
            {saving ? <LoaderCircleIcon className="animate-spin" /> : null}
            {selected.length === 0 ? '取消收藏' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
