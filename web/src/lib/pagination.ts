// pageNumbers returns the page buttons to render: a window that keeps the
// current page centred while staying inside the list. The caller draws the
// ellipsis, because only it knows whether a first or last button is shown.
export function pageNumbers(page: number, totalPages: number, size = 5): number[] {
  if (!Number.isInteger(totalPages) || totalPages < 1) return []
  const count = Math.min(size, totalPages)
  const start = Math.min(Math.max(1, page - Math.floor(count / 2)), totalPages - count + 1)
  return Array.from({ length: count }, (_, index) => start + index)
}
