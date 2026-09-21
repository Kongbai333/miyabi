import type { LibraryMovie } from '@/api/library'
import type { MovieMetadataValues } from '@/features/movie-detail/metadata'

export function libraryMovieMetadata(movie: LibraryMovie): MovieMetadataValues {
  return {
    maker: movie.maker,
    series: movie.series,
    director: movie.director,
    actors: movie.actors ?? [],
    // Database tag IDs identify local rows, not JavDB search targets.
    tags: (movie.tags ?? []).map(tag => ({ id: tag.javdb_id, name: tag.name }))
  }
}

const sizeFormat = new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 2 })

const megabyte = 1024 * 1024

// The card's one-line summary. Duration comes from the catalogue, so a movie
// whose scrape failed shows its file size on its own. Sizes are quoted in whole
// megabytes rather than the mixed-unit helper, which would flip a 2 GB movie to
// "2 GB" and make two cards harder to compare.
export function libraryMovieSummary(movie: LibraryMovie): string {
  const parts: string[] = []
  if (movie.duration > 0) parts.push(`${movie.duration} 分钟`)
  if (movie.size > 0) parts.push(`${sizeFormat.format(movie.size / megabyte)} MB`)
  return parts.join(' · ')
}
