import { cn } from '@/lib/utils'
import { getPageNumbers, type PageItem } from '@/lib/pagination'
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious
} from '@/components/ui/pagination'

export { getPageNumbers, type PageItem }

export function ListPagination({
  page,
  totalPages,
  hasMore,
  disabled,
  scrollToTop = true,
  onPageChange
}: {
  page: number
  totalPages?: number
  hasMore: boolean
  disabled: boolean
  scrollToTop?: boolean
  onPageChange: (page: number) => void
}) {
  function changePage(nextPage: number) {
    if (nextPage === page) return
    onPageChange(nextPage)
    if (scrollToTop) window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  return (
    <Pagination className="py-3">
      <PaginationContent>
        <PaginationItem>
          <PaginationPrevious
            text="上一页"
            disabled={page <= 1 || disabled}
            onClick={() => changePage(page - 1)}
          />
        </PaginationItem>

        {totalPages !== undefined ? (
          getPageNumbers(page, totalPages).map(item => {
            if (typeof item === 'string') {
              return (
                <PaginationItem key={item}>
                  <PaginationEllipsis />
                </PaginationItem>
              )
            }
            const isActive = item === page
            return (
              <PaginationItem key={item}>
                <PaginationLink
                  href="#"
                  isActive={isActive}
                  aria-disabled={disabled || isActive}
                  tabIndex={disabled || isActive ? -1 : undefined}
                  className={cn(disabled && 'pointer-events-none opacity-50')}
                  onClick={e => {
                    e.preventDefault()
                    if (!disabled && !isActive) {
                      changePage(item)
                    }
                  }}
                >
                  {item}
                </PaginationLink>
              </PaginationItem>
            )
          })
        ) : (
          <PaginationItem>
            <PaginationLink
              href="#"
              isActive
              aria-disabled
              tabIndex={-1}
              className={cn(disabled && 'pointer-events-none opacity-50')}
              onClick={e => e.preventDefault()}
            >
              {page}
            </PaginationLink>
          </PaginationItem>
        )}

        <PaginationItem>
          <PaginationNext
            text="下一页"
            disabled={disabled || !hasMore}
            onClick={() => changePage(page + 1)}
          />
        </PaginationItem>
      </PaginationContent>
    </Pagination>
  )
}
