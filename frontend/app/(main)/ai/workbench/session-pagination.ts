export const SESSION_GROUP_VISIBLE_LIMIT = 5

// Expanding local rows and fetching another server page are separate actions.
export function sessionGroupPagination(loaded: number, visible: number, hasMorePages: boolean, total?: number) {
  const hasMore = hasMorePages && (total === undefined || total > loaded)
  const complete = visible >= loaded && !hasMore
  return {
    total: Math.max(loaded, total ?? loaded),
    complete,
    showControl: loaded > SESSION_GROUP_VISIBLE_LIMIT || hasMore,
    needsFetch: !complete && visible + SESSION_GROUP_VISIBLE_LIMIT > loaded && hasMore,
    nextVisible: complete ? SESSION_GROUP_VISIBLE_LIMIT : Math.min(visible + SESSION_GROUP_VISIBLE_LIMIT, hasMore ? (total ?? Infinity) : loaded),
  }
}
