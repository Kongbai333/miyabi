import { createFileRoute } from '@tanstack/react-router'

import { MagnetPage } from '@/features/magnet/page'

export const Route = createFileRoute('/magnet')({
  component: MagnetPage
})
