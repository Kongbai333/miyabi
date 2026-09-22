import type { LibrarySearch } from './filter-params'

// The nav points at the library rail, not at the page the user was reading, so a
// bare "/" would drop the page and the filters. Remembering the last search lets
// the nav return there; while the library itself is open its URL is still the
// only authority, and this is just the copy the shell reads from elsewhere.
let lastSearch: LibrarySearch | undefined

export function rememberLibrarySearch(search: LibrarySearch) {
  lastSearch = search
}

export function lastLibrarySearch(): LibrarySearch | undefined {
  return lastSearch
}
