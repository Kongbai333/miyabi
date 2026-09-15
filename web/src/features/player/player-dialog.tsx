import { lazy, Suspense, useRef } from 'react'

import { Dialog, DialogContent } from '@/components/ui/dialog'
import { useUIStore } from '@/stores/ui'
import { PlayerLoading } from './player-status'

const MoviePlayer = lazy(() => import('./movie-player'))

export function PlayerDialog() {
  const movieID = useUIStore(state => state.playbackMovieID)
  const close = useUIStore(state => state.closePlayer)
  const contentRef = useRef<HTMLDivElement>(null)

  // Unmount the portal with the player so the exit animation cannot show a collapsed, empty frame.
  if (movieID === null) return null

  return (
    <Dialog
      open
      onOpenChange={open => {
        if (!open) close()
      }}
    >
      <DialogContent
        ref={contentRef}
        className="dark aspect-video w-[min(72rem,calc(100vw-2rem),calc(88dvh*16/9))] max-w-none gap-0 overflow-hidden rounded-2xl bg-black p-0 text-foreground ring-0 sm:max-w-none"
        showCloseButton={false}
        onOpenAutoFocus={event => {
          event.preventDefault()
          const content = contentRef.current
          const target = content?.querySelector<HTMLElement>('[data-media-player]') ?? content
          target?.focus({ preventScroll: true })
        }}
        onPointerDownOutside={event => {
          // Native mobile controls can replace the in-player close button.
          if (event.detail.originalEvent.pointerType !== 'touch') event.preventDefault()
        }}
      >
        <Suspense fallback={<PlayerLoading />}>
          <MoviePlayer key={movieID} movieID={movieID} />
        </Suspense>
      </DialogContent>
    </Dialog>
  )
}
