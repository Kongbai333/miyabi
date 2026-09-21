import { useState } from 'react'

import type { LibraryMovie } from '@/api/library'
import { MovieCard } from '@/components/movie'
import { Button } from '@/components/ui/button'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { Progress } from '@/components/ui/progress'
import { formatWatchTime, watchProgressPercent } from '@/lib/watch-progress'
import { useUIStore } from '@/stores/ui'
import { FavoriteDialog } from './favorite-dialog'
import { LibraryMovieHoverDetails } from './movie-hover-details'
import { libraryMovieSummary } from './movie-metadata'
import { LibraryMovieStatus } from './movie-status'
import { useDesktopHover } from './use-desktop-hover'

export function LibraryMovieCard({ movie }: { movie: LibraryMovie }) {
  const canHover = useDesktopHover()
  return (
    <LibraryMovieCardContent
      key={canHover ? 'desktop' : 'touch'}
      movie={movie}
      canHover={canHover}
    />
  )
}

function LibraryMovieCardContent({ movie, canHover }: { movie: LibraryMovie; canHover: boolean }) {
  const openPlayer = useUIStore(state => state.openPlayer)
  const [open, setOpen] = useState(false)

  // The bar sits on the cover so a half-watched movie is recognisable without
  // opening it; a movie that was only opened carries no progress at all.
  const progress = movie.progress
  const coverOverlay = progress ? (
    <div className="absolute right-2 bottom-2 left-2">
      <Progress
        value={watchProgressPercent(progress.position, progress.duration)}
        variant="success"
        className="h-1.5 bg-black/40"
      />
    </div>
  ) : undefined
  const progressLabel = progress ? (
    <span className="tabular-nums">
      {formatWatchTime(progress.position)} / {formatWatchTime(progress.duration)}
    </span>
  ) : null
  const summary = libraryMovieSummary(movie)
  const description =
    summary || progressLabel ? (
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
        {summary ? <span>{summary}</span> : null}
        {progressLabel}
      </div>
    ) : undefined

  // The star sits outside the play button: a button cannot nest inside another.
  const star = (
    <div className="absolute top-2 right-2 z-10">
      <FavoriteDialog movie={movie} />
    </div>
  )

  return (
    <div className="relative h-full min-w-0">
      <HoverCard
        open={canHover && open}
        onOpenChange={value => setOpen(canHover && value)}
        openDelay={400}
        closeDelay={180}
      >
        <HoverCardTrigger asChild>
          <Button
            variant="ghost"
            className="block h-auto w-full min-w-0 rounded-2xl p-0 text-left whitespace-normal hover:bg-transparent hover:text-current dark:hover:bg-transparent"
            aria-label={`播放 ${movie.code}${movie.title ? `：${movie.title}` : ''}`}
            onClick={() => {
              setOpen(false)
              openPlayer(movie.id)
            }}
          >
            <MovieCard
              movie={movie}
              titleTooltip={!canHover}
              coverOverlay={coverOverlay}
              description={description}
            >
              <LibraryMovieStatus movie={movie} />
            </MovieCard>
          </Button>
        </HoverCardTrigger>
        {canHover ? (
          <HoverCardContent
            side="right"
            align="start"
            sideOffset={12}
            collisionPadding={16}
            className="max-h-[min(42rem,var(--radix-hover-card-content-available-height))] w-96 max-w-[calc(100vw-2rem)] overflow-y-auto overscroll-contain p-0"
            role="region"
            aria-label={`${movie.code} 影片详情`}
            onClick={event => {
              if (event.target instanceof Element && event.target.closest('a')) setOpen(false)
            }}
          >
            <LibraryMovieHoverDetails movie={movie} />
          </HoverCardContent>
        ) : null}
      </HoverCard>
      {star}
    </div>
  )
}
