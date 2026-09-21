import { CopyIcon, LoaderCircleIcon, MagnetIcon, SettingsIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import {
  type MagnetCollectionItem,
  type MagnetShare,
  useCreateMagnetCollection,
  useDeleteMagnetCollection,
  useDisableMagnetShare,
  useEnableMagnetShare,
  useImportMagnetShare,
  useMagnetCollection,
  useMagnetCollections,
  useMagnetShareDetail,
  useRenameMagnetCollection
} from '@/api/magnet'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { copyMagnet } from './clipboard'
import { MagnetDetailDialog } from './detail-dialog'
import { MagnetDownloadButton } from './download-button'

// The server can list, create, rename, delete, share and import collections,
// but it has no tool for adding an item to one, so this panel browses what was
// collected elsewhere rather than offering a "save" button of its own.
export function MagnetCollectionsCard() {
  const collections = useMagnetCollections()
  const [selected, setSelected] = useState('')
  const [manageOpen, setManageOpen] = useState(false)
  const [inspecting, setInspecting] = useState<{ name: string; magnet: string } | null>(null)

  const list = collections.data ?? []
  const activeKey = list.some(collection => collection.key === selected)
    ? selected
    : (list[0]?.key ?? '')
  const detail = useMagnetCollection(activeKey)
  const active = list.find(collection => collection.key === activeKey)

  return (
    <Card>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="flex items-center gap-2 text-sm font-semibold">
            <MagnetIcon className="size-4 text-muted-foreground" />
            合集
          </h2>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={list.length === 0}
            onClick={() => setManageOpen(true)}
          >
            <SettingsIcon />
            管理
          </Button>
        </div>

        {collections.isPending ? <Skeleton className="h-9 w-full rounded-full" /> : null}
        {collections.isError ? <InlineError>{collections.error.message}</InlineError> : null}

        {list.length > 0 ? (
          <ul className="flex flex-wrap items-center gap-1">
            {list.map(collection => (
              <li key={collection.key}>
                <Button
                  type="button"
                  variant={collection.key === activeKey ? 'outline' : 'ghost'}
                  size="sm"
                  onClick={() => setSelected(collection.key)}
                >
                  {collection.label}
                </Button>
              </li>
            ))}
          </ul>
        ) : null}

        {detail.isPending && activeKey !== '' ? (
          <Skeleton className="h-24 w-full rounded-2xl" />
        ) : null}
        {detail.isError ? <InlineError>{detail.error.message}</InlineError> : null}

        {detail.data ? (
          detail.data.items.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              「{active?.label ?? detail.data.collection.label}
              」里还没有条目。合集的内容要在磁力服务自己的站点里添加。
            </p>
          ) : (
            <ul className="space-y-2">
              {detail.data.items.map(item => (
                <CollectionRow
                  key={`${item.collection}-${item.magnet_url}`}
                  item={item}
                  onInspect={() => setInspecting({ name: item.name, magnet: item.magnet_url })}
                />
              ))}
            </ul>
          )
        ) : null}
      </CardContent>

      {manageOpen && active ? (
        <CollectionsDialog
          activeKey={active.key}
          activeLabel={active.label}
          deletable={active.deletable}
          onClose={() => setManageOpen(false)}
        />
      ) : null}

      {inspecting ? (
        <MagnetDetailDialog
          title={inspecting.name}
          magnet={inspecting.magnet}
          onClose={() => setInspecting(null)}
        />
      ) : null}
    </Card>
  )
}

function CollectionRow({ item, onInspect }: { item: MagnetCollectionItem; onInspect: () => void }) {
  return (
    <li className="flex flex-col gap-2 rounded-2xl border border-border/60 p-3 sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1 space-y-1">
        <p className="text-sm font-medium break-all">{item.name}</p>
        <p className="text-xs text-muted-foreground tabular-nums">
          {item.tagsText.length > 0 ? `${item.tagsText.join(' · ')} · ` : ''}
          {new Date(item.added_at).toLocaleDateString('zh-CN')}
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

function CollectionsDialog({
  activeKey,
  activeLabel,
  deletable,
  onClose
}: {
  activeKey: string
  activeLabel: string
  deletable: boolean
  onClose: () => void
}) {
  const create = useCreateMagnetCollection()
  const rename = useRenameMagnetCollection()
  const remove = useDeleteMagnetCollection()
  const enableShare = useEnableMagnetShare()
  const disableShare = useDisableMagnetShare()
  const importShare = useImportMagnetShare()

  const [newLabel, setNewLabel] = useState('')
  const [renameLabel, setRenameLabel] = useState(activeLabel)
  const [shareCode, setShareCode] = useState('')
  const [importCode, setImportCode] = useState('')
  const [importLabel, setImportLabel] = useState('')
  const [share, setShare] = useState<MagnetShare | null>(null)
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  const busy =
    create.isPending ||
    rename.isPending ||
    remove.isPending ||
    enableShare.isPending ||
    disableShare.isPending ||
    importShare.isPending

  function run(action: () => Promise<unknown>, success: string) {
    void action()
      .then(() => toast.success(success))
      .catch((error: Error) => toast.error('操作失败', { description: error.message }))
  }

  return (
    <Dialog open onOpenChange={open => (open ? undefined : onClose())}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>管理合集</DialogTitle>
          <DialogDescription>
            当前合集：{activeLabel}。合集内容需要在磁力服务自己的站点里添加。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-5">
          <section className="space-y-2">
            <h3 className="text-sm font-medium">新建合集</h3>
            <div className="flex items-center gap-2">
              <Input
                value={newLabel}
                onChange={event => setNewLabel(event.target.value)}
                placeholder="合集名称"
                className="min-w-0 flex-1"
              />
              <Button
                type="button"
                size="sm"
                disabled={busy || newLabel.trim() === ''}
                onClick={() => {
                  const label = newLabel.trim()
                  run(() => create.mutateAsync(label), `已新建合集「${label}」`)
                  setNewLabel('')
                }}
              >
                新建
              </Button>
            </div>
          </section>

          <section className="space-y-2">
            <h3 className="text-sm font-medium">重命名当前合集</h3>
            <div className="flex items-center gap-2">
              <Input
                value={renameLabel}
                onChange={event => setRenameLabel(event.target.value)}
                placeholder="新名称"
                className="min-w-0 flex-1"
              />
              <Button
                type="button"
                size="sm"
                disabled={busy || renameLabel.trim() === '' || renameLabel.trim() === activeLabel}
                onClick={() =>
                  run(
                    () => rename.mutateAsync({ key: activeKey, label: renameLabel.trim() }),
                    '已重命名'
                  )
                }
              >
                重命名
              </Button>
            </div>
          </section>

          <section className="space-y-2">
            <h3 className="text-sm font-medium">分享</h3>
            {share ? (
              <p className="text-xs break-all text-muted-foreground">
                分享码 <span className="font-mono text-foreground">{share.code}</span>
                {share.share_url ? ` · ${share.share_url}` : ''}
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">
                开启分享后，任何拿到分享码的人都能看到这个合集的内容。
              </p>
            )}
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={busy}
                onClick={() =>
                  run(async () => {
                    const result = await enableShare.mutateAsync(activeKey)
                    setShare(result)
                  }, '已开启分享')
                }
              >
                {enableShare.isPending ? <LoaderCircleIcon className="animate-spin" /> : null}
                开启分享
              </Button>
              {share ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => void copyMagnet(share.share_url)}
                >
                  <CopyIcon />
                  复制链接
                </Button>
              ) : null}
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={busy}
                onClick={() =>
                  run(async () => {
                    await disableShare.mutateAsync(activeKey)
                    setShare(null)
                  }, '已关闭分享')
                }
              >
                关闭分享
              </Button>
            </div>
            <Input
              value={shareCode}
              onChange={event => setShareCode(event.target.value)}
              placeholder="查看某个分享码的内容（可选）"
              className="font-mono text-xs"
            />
            <SharePreview code={shareCode.trim()} />
          </section>

          <section className="space-y-2">
            <h3 className="text-sm font-medium">导入分享</h3>
            <div className="flex flex-wrap items-center gap-2">
              <Input
                value={importCode}
                onChange={event => setImportCode(event.target.value)}
                placeholder="分享码"
                className="min-w-0 flex-1 font-mono text-xs"
              />
              <Input
                value={importLabel}
                onChange={event => setImportLabel(event.target.value)}
                placeholder="新合集名称（可选）"
                className="min-w-0 flex-1"
              />
              <Button
                type="button"
                size="sm"
                disabled={busy || importCode.trim() === ''}
                onClick={() =>
                  run(async () => {
                    await importShare.mutateAsync({
                      code: importCode.trim(),
                      label: importLabel.trim() || undefined
                    })
                    setImportCode('')
                    setImportLabel('')
                  }, '已导入分享')
                }
              >
                导入
              </Button>
            </div>
          </section>
        </div>

        <DialogFooter className={cn('sm:justify-between')}>
          <Button
            type="button"
            variant="destructive"
            size="sm"
            disabled={busy || !deletable}
            title={deletable ? undefined : '默认合集不能删除'}
            onClick={() => {
              if (!confirmingDelete) {
                setConfirmingDelete(true)
                return
              }
              run(async () => {
                await remove.mutateAsync(activeKey)
                onClose()
              }, '已删除合集')
            }}
          >
            {confirmingDelete ? '确认删除' : '删除合集'}
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={onClose}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// favorite_share_detail is public data for a code, so this reads it live.
function SharePreview({ code }: { code: string }) {
  const detail = useMagnetShareDetail(code)
  if (code === '') return null
  if (detail.isPending) return <Skeleton className="h-4 w-40" />
  if (detail.isError) return <p className="text-xs text-muted-foreground">{detail.error.message}</p>
  if (!detail.data) return null
  return (
    <p className="text-xs text-muted-foreground">
      「{detail.data.label}」共 {detail.data.count} 条
      {detail.data.updated_at
        ? ` · 更新于 ${new Date(detail.data.updated_at).toLocaleString('zh-CN')}`
        : ''}
    </p>
  )
}
