import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'
import {
  libraryFilterOf,
  libraryFilterSearch,
  parseLibrarySearch,
  type LibrarySearch
} from '@/features/library/filter-params'
import { LibraryPage } from '@/features/library/page'

export const Route = createFileRoute('/')({
  validateSearch: (search: Record<string, unknown>): LibrarySearch => parseLibrarySearch(search),
  component: LibraryRoute,
  errorComponent: () => (
    <AppPage>
      <ErrorState message="媒体库链接无效" />
    </AppPage>
  )
})

function LibraryRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const page = search.page ?? 1
  const group = search.group ?? 0
  const filter = libraryFilterOf(search)

  return (
    <LibraryPage
      page={page}
      group={group}
      filter={filter}
      onPageChange={next =>
        void navigate({ search: { ...search, page: next }, resetScroll: false })
      }
      // Switching groups or filters returns to the first page: the new list is shorter.
      onGroupChange={next =>
        void navigate({ search: libraryFilterSearch(filter, next), resetScroll: false })
      }
      onFilterChange={next =>
        void navigate({ search: libraryFilterSearch(next, group), resetScroll: false })
      }
    />
  )
}
