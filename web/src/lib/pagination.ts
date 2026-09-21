// pageNumbers returns the page buttons to render: a window that keeps the
// current page centred while staying inside the list. A list short enough that
// the window would leave only a page or two behind an ellipsis is shown whole.
// The caller draws the ellipsis, because only it knows whether a first or last
// button is shown.
export function pageNumbers(page: number, totalPages: number, size = 3): number[] {
  if (!Number.isInteger(totalPages) || totalPages < 1) return []
  const count = Math.min(size, totalPages)
  // The window plus the page on either side of it.
  if (totalPages <= count + 2) {
    return Array.from({ length: totalPages }, (_, index) => index + 1)
  }
  const start = Math.min(Math.max(1, page - Math.floor(count / 2)), totalPages - count + 1)
  return Array.from({ length: count }, (_, index) => start + index)
}
