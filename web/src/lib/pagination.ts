export type PageItem = number | 'ellipsis-left' | 'ellipsis-right'

export function getPageNumbers(page: number, totalPages: number): PageItem[] {
  const safeTotalPages = Math.max(1, Math.floor(totalPages))
  if (safeTotalPages <= 7) {
    return Array.from({ length: safeTotalPages }, (_, i) => i + 1)
  }

  if (page <= 4) {
    return [1, 2, 3, 4, 5, 'ellipsis-right', safeTotalPages]
  }

  if (page >= safeTotalPages - 3) {
    return [
      1,
      'ellipsis-left',
      safeTotalPages - 4,
      safeTotalPages - 3,
      safeTotalPages - 2,
      safeTotalPages - 1,
      safeTotalPages
    ]
  }

  return [1, 'ellipsis-left', page - 1, page, page + 1, 'ellipsis-right', safeTotalPages]
}
