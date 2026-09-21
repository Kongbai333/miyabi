import { CloudDownloadIcon, LoaderCircleIcon } from 'lucide-react'

import { useAddMagnet } from '@/api/offline'
import { Button } from '@/components/ui/button'

// Pushes the magnet to 115 as an offline download in the mounted directory. The
// file lands there and the next scan identifies it, so this needs no catalogue
// entry and reports its own progress through the task toast.
export function MagnetDownloadButton({
  magnet,
  size = 'sm'
}: {
  magnet: string
  size?: 'sm' | 'default'
}) {
  const add = useAddMagnet()
  return (
    <Button
      type="button"
      variant="outline"
      size={size}
      disabled={add.isPending}
      onClick={() => add.mutate(magnet)}
    >
      {add.isPending ? <LoaderCircleIcon className="animate-spin" /> : <CloudDownloadIcon />}
      一键 115
    </Button>
  )
}
