import { toast } from 'sonner'

// A magnet is copied far more often than it is opened, and a failed clipboard
// write must not look like a failed search.
export async function copyMagnet(magnet: string) {
  try {
    await navigator.clipboard.writeText(magnet)
    toast.success('磁力链接已复制')
  } catch {
    toast.error('无法访问剪贴板', { description: magnet })
  }
}
