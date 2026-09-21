import { FileIcon, FolderIcon } from 'lucide-react'

import type { MagnetFile } from '@/api/magnet'
import { formatSize } from '@/lib/format'

// The server hands back a tree with nested sub_files, so this renders itself.
export function MagnetFileTree({ files }: { files: MagnetFile[] }) {
  if (files.length === 0) {
    return <p className="text-xs text-muted-foreground">这个磁力没有文件信息</p>
  }
  return (
    <ul className="space-y-1">
      {files.map(file => (
        <MagnetFileNode key={file.id} file={file} />
      ))}
    </ul>
  )
}

function MagnetFileNode({ file }: { file: MagnetFile }) {
  return (
    <li>
      <div className="flex items-center gap-2 text-sm">
        {file.is_dir ? (
          <FolderIcon className="size-4 shrink-0 text-muted-foreground" />
        ) : (
          <FileIcon className="size-4 shrink-0 text-muted-foreground" />
        )}
        <span className="min-w-0 flex-1 truncate" title={file.name}>
          {file.name}
        </span>
        <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
          {formatSize(file.file_size_bytes)}
        </span>
      </div>
      {file.sub_files.length > 0 ? (
        <ul className="mt-1 ml-2 space-y-1 border-l border-border/60 pl-3">
          {file.sub_files.map(child => (
            <MagnetFileNode key={child.id} file={child} />
          ))}
        </ul>
      ) : null}
    </li>
  )
}
