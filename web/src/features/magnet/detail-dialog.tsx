import { CopyIcon } from 'lucide-react'

import { useMagnetFiles, useMagnetPreview } from '@/api/magnet'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { formatSize } from '@/lib/format'
import { copyMagnet } from './clipboard'
import { MagnetFileTree } from './file-tree'
import { MagnetDownloadButton } from './download-button'

// The preview and the file tree are two separate tool calls, so each half of
// the dialog reports its own loading and failure.
export function MagnetDetailDialog({
  title,
  magnet,
  onClose
}: {
  title: string
  magnet: string
  onClose: () => void
}) {
  const preview = useMagnetPreview(magnet)
  const files = useMagnetFiles(magnet)

  return (
    <Dialog open onOpenChange={open => (open ? undefined : onClose())}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="break-all">{title}</DialogTitle>
          <DialogDescription className="break-all">{magnet}</DialogDescription>
        </DialogHeader>

        <div className="space-y-6">
          <section className="space-y-2">
            <h3 className="text-sm font-semibold">预览</h3>
            {preview.isPending ? <Skeleton className="h-24 w-full rounded-2xl" /> : null}
            {preview.isError ? <InlineError>{preview.error.message}</InlineError> : null}
            {preview.data ? (
              <div className="space-y-3">
                <p className="text-xs text-muted-foreground tabular-nums">
                  {preview.data.data.name || title} · {formatSize(preview.data.size)} ·{' '}
                  {preview.data.data.file_type || preview.data.data.type || '未知类型'} ·{' '}
                  {preview.data.data.count} 个文件
                </p>
                {preview.data.data.error ? (
                  <p className="text-xs text-muted-foreground">{preview.data.data.error}</p>
                ) : null}
                {preview.data.data.screenshots.length > 0 ? (
                  <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                    {preview.data.data.screenshots.map(screenshot => (
                      <li key={screenshot.screenshot}>
                        <a
                          href={screenshot.screenshot}
                          target="_blank"
                          rel="noreferrer noopener"
                          className="block overflow-hidden rounded-xl border border-border/60"
                        >
                          <img
                            src={screenshot.screenshot}
                            alt="预览截图"
                            loading="lazy"
                            className="aspect-video w-full object-cover"
                          />
                        </a>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-xs text-muted-foreground">这个磁力没有可用的预览图。</p>
                )}
              </div>
            ) : null}
          </section>

          <section className="space-y-2">
            <h3 className="text-sm font-semibold">
              文件
              {files.data ? (
                <span className="ml-2 text-xs font-normal text-muted-foreground tabular-nums">
                  {formatSize(files.data.total_size)} · {files.data.file_count} 个顶层条目
                </span>
              ) : null}
            </h3>
            {files.isPending ? <Skeleton className="h-24 w-full rounded-2xl" /> : null}
            {files.isError ? <InlineError>{files.error.message}</InlineError> : null}
            {files.data ? <MagnetFileTree files={files.data.files} /> : null}
          </section>

          <div className="flex flex-wrap items-center gap-2">
            <MagnetDownloadButton magnet={magnet} size="default" />
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void copyMagnet(magnet)}
            >
              <CopyIcon />
              复制磁力链接
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
