import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationNext,
  PaginationPrevious
} from '@/components/ui/pagination'
import { Button } from '@/components/ui/button'
import { pageNumbers } from '@/lib/pagination'

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
  disabled?: boolean
  scrollToTop?: boolean
  onPageChange: (page: number) => void
}) {
  function changePage(nextPage: number) {
    if (nextPage === page) return
    onPageChange(nextPage)
    if (scrollToTop) window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  // A library that fits on one page has nothing to page through, so the bar
  // stays out of the way rather than offering a dead "previous" and "next".
  const singlePage = page <= 1 && !hasMore && (totalPages === undefined || totalPages <= 1)
  if (singlePage) return null

  // An endpoint that only reports has_more cannot name its last page, so the
  // numbered window and the last-page button stay hidden there instead of
  // guessing a bound the server never sent.
  const numbers = totalPages === undefined ? [] : pageNumbers(page, totalPages)
  const atEnd = totalPages === undefined ? !hasMore : page >= totalPages
  const atStart = page <= 1

  return (
    <Pagination className="py-3">
      <PaginationContent className="flex-wrap justify-center">
        <PaginationItem>
          <Button
            type="button"
            variant="ghost"
            disabled={atStart || disabled}
            onClick={() => changePage(1)}
          >
            首页
          </Button>
        </PaginationItem>
        <PaginationItem>
          <PaginationPrevious
            text="上一页"
            disabled={atStart || disabled}
            onClick={() => changePage(page - 1)}
          />
        </PaginationItem>

        {numbers.length > 0 && numbers[0] !== 1 ? (
          <PaginationItem>
            <PaginationEllipsis />
          </PaginationItem>
        ) : null}
        {numbers.map(number => (
          <PaginationItem key={number}>
            <Button
              type="button"
              variant={number === page ? 'outline' : 'ghost'}
              size="icon"
              aria-label={`第 ${number} 页`}
              aria-current={number === page ? 'page' : undefined}
              disabled={disabled}
              onClick={() => changePage(number)}
            >
              {number}
            </Button>
          </PaginationItem>
        ))}
        {numbers.length > 0 && numbers[numbers.length - 1] !== totalPages ? (
          <PaginationItem>
            <PaginationEllipsis />
          </PaginationItem>
        ) : null}

        {numbers.length === 0 ? (
          <PaginationItem>
            <span className="flex h-9 min-w-20 items-center justify-center px-2 text-sm tabular-nums">
              第 {page} 页
            </span>
          </PaginationItem>
        ) : null}

        <PaginationItem>
          <PaginationNext
            text="下一页"
            disabled={disabled || atEnd}
            onClick={() => changePage(page + 1)}
          />
        </PaginationItem>
        {totalPages === undefined ? null : (
          <PaginationItem>
            <Button
              type="button"
              variant="ghost"
              disabled={disabled || atEnd}
              onClick={() => changePage(totalPages)}
            >
              尾页
            </Button>
          </PaginationItem>
        )}
      </PaginationContent>
    </Pagination>
  )
}
