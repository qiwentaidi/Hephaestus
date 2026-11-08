export const FINDING_LIST_PATH = '/vulnerabilities'

export const FINDING_LIST_PAGE_SIZE_OPTIONS: number[] = [10, 20, 50, 100]

export const DEFAULT_FINDING_LIST_PAGE_SIZE = 20

export function parseFindingListPage(value: string | null | undefined): number {
  const parsed = Number.parseInt(value ?? '', 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 1
}

export function parseFindingListPageSize(
  value: string | null | undefined,
  options: number[] = FINDING_LIST_PAGE_SIZE_OPTIONS,
  fallback: number = DEFAULT_FINDING_LIST_PAGE_SIZE,
): number {
  const parsed = Number.parseInt(value ?? '', 10)
  return options.includes(parsed) ? parsed : fallback
}

/**
 * Builds the finding list URL for the current pagination.
 *
 * The page size and page number are kept in the URL so opening a finding and
 * coming back (through the detail back link, a content tab, or browser Back)
 * restores the page the user was browsing instead of jumping to page 1.
 * Defaults stay implicit to keep shared links short.
 */
export function findingListHref(page: number, pageSize: number, search = ''): string {
  const params = new URLSearchParams(search)
  if (page > 1) {
    params.set('page', String(page))
  } else {
    params.delete('page')
  }
  if (pageSize !== DEFAULT_FINDING_LIST_PAGE_SIZE) {
    params.set('pageSize', String(pageSize))
  } else {
    params.delete('pageSize')
  }

  const query = params.toString()
  return query ? `${FINDING_LIST_PATH}?${query}` : FINDING_LIST_PATH
}
