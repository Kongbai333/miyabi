import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'
import { LibraryPage } from '@/features/library/page'

type LibrarySearch = { page?: number; group?: number }

export const Route = createFileRoute('/')({
  validateSearch: (search: Record<string, unknown>): LibrarySearch => {
    const page = Number(search.page ?? 1)
    if (!Number.isInteger(page) || page < 1) throw new Error('媒体库页码无效')
    const group = Number(search.group ?? 0)
    if (!Number.isInteger(group) || group < 0) throw new Error('媒体库分组无效')
    return { ...(page > 1 && { page }), ...(group > 0 && { group }) }
  },
  component: LibraryRoute,
  errorComponent: () => (
    <AppPage>
      <ErrorState message="媒体库页码无效" />
    </AppPage>
  )
})

function LibraryRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const page = search.page ?? 1
  const group = search.group ?? 0

  return (
    <LibraryPage
      page={page}
      group={group}
      onPageChange={next =>
        void navigate({ search: { ...search, page: next }, resetScroll: false })
      }
      onGroupChange={next =>
        // Switching groups returns to the first page: the new list is shorter.
        void navigate({
          search: next > 0 ? { group: next } : {},
          resetScroll: false
        })
      }
    />
  )
}
