import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'

import type { DiscoverMovie } from '@/api/discover'
import { useMovieState } from '@/api/movie-states'
import { MovieResourceBadges, MovieStateBadge } from '@/components/movie/movie-badges'
import { MovieCover } from '@/components/movie/movie-cover'
import { MovieMonitorButton } from '@/components/movie/movie-monitor-button'
import { OverflowTooltip } from '@/components/overflow-tooltip'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'

export function DiscoverMovieCard({ movie }: { movie: DiscoverMovie }) {
  const viewed = useMovieState(movie)?.viewed ?? false
  const monitorable = movie.release_status === 'upcoming' && movie.magnets_count === 0
  return (
    <Link
      to="/discover/$movieId"
      params={{ movieId: movie.id }}
      className="block rounded-2xl outline-ring"
    >
      <MovieCard
        movie={movie}
        description={movie.release_date}
        state={<MovieStateBadge movie={movie} />}
        viewed={viewed}
        coverOverlay={
          monitorable ? (
            <div className="absolute top-2 right-2">
              <MovieMonitorButton movie={movie} />
            </div>
          ) : undefined
        }
      >
        <MovieResourceBadges movie={movie} />
      </MovieCard>
    </Link>
  )
}

export function MovieCard({
  movie,
  description,
  coverLoading = 'lazy',
  onCoverReady,
  coverOverlay,
  titleTooltip = true,
  viewed = false,
  state,
  children
}: {
  movie: { code: string; title: string; cover?: string }
  description?: ReactNode
  coverLoading?: 'eager' | 'lazy'
  onCoverReady?: () => void
  coverOverlay?: ReactNode
  titleTooltip?: boolean
  viewed?: boolean
  state?: ReactNode
  children?: ReactNode
}) {
  const title = movie.title || movie.code
  // A viewed movie keeps its title readable but recedes, so the grid still scans.
  const heading = (
    <h3
      className={cn('truncate text-sm leading-5 font-semibold', viewed && 'text-muted-foreground')}
    >
      {title}
    </h3>
  )
  return (
    <Card size="sm" className="h-full gap-0 overflow-hidden py-0">
      <div className="relative flex aspect-3/2 items-center justify-center overflow-hidden bg-muted">
        <div className="absolute inset-0">
          <MovieCover source={movie.cover ?? ''} loading={coverLoading} onReady={onCoverReady} />
        </div>
        <div className="absolute top-2 left-2 flex max-w-[calc(100%-3.5rem)] flex-wrap gap-1.5">
          <Badge variant="outline" className="max-w-full truncate bg-background/85 backdrop-blur">
            {movie.code}
          </Badge>
          {state}
        </div>
        {coverOverlay}
      </div>
      <CardContent className="min-w-0 space-y-2 p-3">
        {titleTooltip ? <OverflowTooltip content={title}>{heading}</OverflowTooltip> : heading}
        {description != null ? (
          <div className="text-xs text-muted-foreground">{description}</div>
        ) : null}
        {children ? <div className="flex flex-wrap gap-1.5">{children}</div> : null}
      </CardContent>
    </Card>
  )
}
