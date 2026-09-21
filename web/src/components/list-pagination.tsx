import { useRef, useState, type FormEvent, type KeyboardEvent } from 'react'

import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationNext,
  PaginationPrevious
} from '@/components/ui/pagination'

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
  const [prevPage, setPrevPage] = useState(page)
  const [jumpPage, setJumpPage] = useState(String(page))
  const inputRef = useRef<HTMLInputElement>(null)

  if (prevPage !== page) {
    setPrevPage(page)
    setJumpPage(String(page))
  }

  function changePage(nextPage: number) {
    if (nextPage === page) return
    onPageChange(nextPage)
    if (scrollToTop) window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  function handleJumpSubmit(e?: FormEvent) {
    if (e) e.preventDefault()
    if (totalPages === undefined || totalPages <= 1 || disabled) return
    const parsed = parseInt(jumpPage.trim(), 10)
    if (!isNaN(parsed)) {
      const clamped = Math.max(1, Math.min(totalPages, parsed))
      setJumpPage(String(clamped))
      if (clamped !== page) {
        changePage(clamped)
      }
    } else {
      setJumpPage(String(page))
    }
    inputRef.current?.blur()
  }

  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      handleJumpSubmit()
    } else if (e.key === 'Escape') {
      setJumpPage(String(page))
      inputRef.current?.blur()
    }
  }

  const canJump = totalPages !== undefined && totalPages > 1

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
        <PaginationItem>
          {canJump ? (
            <form
              onSubmit={handleJumpSubmit}
              className="flex h-9 items-center gap-1 px-1 text-sm tabular-nums"
            >
              <span className="text-muted-foreground">第</span>
              <input
                ref={inputRef}
                type="text"
                inputMode="numeric"
                pattern="[0-9]*"
                aria-label="跳转页码"
                title="输入页码并按回车跳转"
                disabled={disabled}
                value={jumpPage}
                onChange={e => {
                  const val = e.target.value
                  if (/^\d*$/.test(val)) {
                    setJumpPage(val)
                  }
                }}
                onKeyDown={handleKeyDown}
                onBlur={() => handleJumpSubmit()}
                className="h-7 w-12 rounded-md border border-input bg-muted/30 px-1 text-center text-sm font-medium tabular-nums transition-colors hover:bg-muted/60 focus:border-ring focus:bg-background focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              />
              <span className="text-muted-foreground">/ {totalPages} 页</span>
            </form>
          ) : (
            <span className="flex h-9 min-w-20 items-center justify-center px-2 text-sm tabular-nums text-muted-foreground">
              第 <span className="font-medium text-foreground px-1">{page}</span>
              {totalPages === undefined ? '' : ` / ${totalPages}`} 页
            </span>
          )}
        </PaginationItem>
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
